package webrtc

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

//go:embed publish_index.html
var publishIndex []byte

//go:embed publisher.js
var publisherJS []byte

//go:embed read_index.html
var readIndex []byte

//go:embed reader.js
var readerJS []byte

var (
	reWHIPWHEPNoID   = regexp.MustCompile("^/(.+?)/(whip|whep)$")
	reWHIPWHEPWithID = regexp.MustCompile("^/(.+?)/(whip|whep)/(.+?)$")
)

func mergePathAndQuery(path string, rawQuery string) string {
	res := path
	if rawQuery != "" {
		res += "?" + rawQuery
	}
	return res
}

func writeError(ctx *gin.Context, statusCode int, err error) {
	ctx.JSON(statusCode, &defs.APIError{
		Error: err.Error(),
	})
}

func sessionLocation(publish bool, path string, rawQuery string, secret uuid.UUID) string {
	ret := "/" + path + "/"

	if publish {
		ret += "whip"
	} else {
		ret += "whep"
	}

	ret += "/" + secret.String()

	if rawQuery != "" {
		ret += "?" + rawQuery
	}

	return ret
}

type httpServer struct {
	address        string
	encryption     bool
	serverKey      string
	serverCert     string
	allowOrigin    string
	trustedProxies conf.IPNetworks
	readTimeout    conf.Duration
	pathManager    serverPathManager
	parent         *Server

	inner *httpp.Server
}

func (s *httpServer) initialize() error {
	router := gin.New()
	router.SetTrustedProxies(s.trustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(s.middlewareOrigin)

	router.Use(s.onRequest)

	network, address := restrictnetwork.Restrict("tcp", s.address)

	s.inner = &httpp.Server{
		Network:     network,
		Address:     address,
		ReadTimeout: time.Duration(s.readTimeout),
		Encryption:  s.encryption,
		ServerCert:  s.serverCert,
		ServerKey:   s.serverKey,
		Handler:     router,
		Parent:      s,
	}
	err := s.inner.Initialize()
	if err != nil {
		return err
	}

	return nil
}

// Log implements logger.Writer.
func (s *httpServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, format, args...)
}

func (s *httpServer) close() {
	s.inner.Close()
}

func (s *httpServer) checkAuthOutsideSession(ctx *gin.Context, pathName string, publish bool) bool {
	req := defs.PathAccessRequest{
		Name:    pathName,
		Publish: publish,
		IP:      net.ParseIP(ctx.ClientIP()),
		Proto:   auth.ProtocolWebRTC,
	}
	req.FillFromHTTPRequest(ctx.Request)

	_, err := s.pathManager.FindPathConf(defs.PathFindPathConfReq{
		AccessRequest: req,
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			if terr.AskCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return false
			}

			s.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Message)

			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)

			writeError(ctx, http.StatusUnauthorized, terr)
			return false
		}

		writeError(ctx, http.StatusInternalServerError, err)
		return false
	}

	return true
}

func (s *httpServer) onWHIPOptions(ctx *gin.Context, pathName string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, pathName, publish) {
		return
	}

	servers, err := s.parent.generateICEServers(true)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
	ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
	ctx.Header("Access-Control-Expose-Headers", "Link")
	ctx.Writer.Header()["Link"] = whip.LinkHeaderMarshal(servers)
	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *httpServer) onWHIPPost(ctx *gin.Context, pathName string, publish bool) {
	contentType := httpp.ParseContentType(ctx.Request.Header.Get("Content-Type"))
	if contentType != "application/sdp" {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	offer, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	res := s.parent.newSession(webRTCNewSessionReq{
		pathName:    pathName,
		remoteAddr:  httpp.RemoteAddr(ctx),
		offer:       offer,
		publish:     publish,
		httpRequest: ctx.Request,
	})
	if res.err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			if terr.AskCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			s.Log(logger.Info, "connection %v failed to authenticate: %v", httpp.RemoteAddr(ctx), terr.Message)

			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)

			writeError(ctx, http.StatusUnauthorized, terr)
			return
		}

		writeError(ctx, res.errStatusCode, res.err)
		return
	}

	servers, err := s.parent.generateICEServers(true)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.Header("Content-Type", "application/sdp")
	ctx.Header("Access-Control-Expose-Headers", "ETag, ID, Accept-Patch, Link, Location")
	ctx.Header("ETag", "*")
	ctx.Header("ID", res.sx.uuid.String())
	ctx.Header("Accept-Patch", "application/trickle-ice-sdpfrag")
	ctx.Writer.Header()["Link"] = whip.LinkHeaderMarshal(servers)
	ctx.Header("Location", sessionLocation(publish, pathName, ctx.Request.URL.RawQuery, res.sx.secret))
	ctx.Writer.WriteHeader(http.StatusCreated)
	ctx.Writer.Write(res.answer)
}

func (s *httpServer) onWHIPPatch(ctx *gin.Context, pathName string, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	contentType := httpp.ParseContentType(ctx.Request.Header.Get("Content-Type"))
	if contentType != "application/trickle-ice-sdpfrag" {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid Content-Type"))
		return
	}

	byts, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return
	}

	candidates, err := whip.ICEFragmentUnmarshal(byts)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err)
		return
	}

	res := s.parent.addSessionCandidates(webRTCAddSessionCandidatesReq{
		pathName:   pathName,
		secret:     secret,
		candidates: candidates,
	})
	if res.err != nil {
		if errors.Is(res.err, ErrSessionNotFound) {
			writeError(ctx, http.StatusNotFound, res.err)
		} else {
			writeError(ctx, http.StatusInternalServerError, res.err)
		}
		return
	}

	ctx.Writer.WriteHeader(http.StatusNoContent)
}

func (s *httpServer) onWHIPDelete(ctx *gin.Context, pathName string, rawSecret string) {
	secret, err := uuid.Parse(rawSecret)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid secret"))
		return
	}

	err = s.parent.deleteSession(webRTCDeleteSessionReq{
		pathName: pathName,
		secret:   secret,
	})
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			writeError(ctx, http.StatusNotFound, err)
		} else {
			writeError(ctx, http.StatusInternalServerError, err)
		}
		return
	}

	ctx.Writer.WriteHeader(http.StatusOK)
}

func (s *httpServer) onPage(ctx *gin.Context, pathName string, publish bool) {
	if !s.checkAuthOutsideSession(ctx, pathName, publish) {
		return
	}

	ctx.Header("Cache-Control", "max-age=3600")
	ctx.Header("Content-Type", "text/html")
	ctx.Writer.WriteHeader(http.StatusOK)

	if publish {
		ctx.Writer.Write(publishIndex)
	} else {
		ctx.Writer.Write(readIndex)
	}
}

func (s *httpServer) middlewareOrigin(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Header("Access-Control-Allow-Credentials", "true")

	// preflight requests
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PATCH, DELETE")
		ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (s *httpServer) onRequest(ctx *gin.Context) {
	if strings.HasSuffix(ctx.Request.URL.Path, "/publisher.js") {
		ctx.Header("Cache-Control", "max-age=3600")
		ctx.Header("Content-Type", "application/javascript")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(publisherJS)
		return
	}

	if strings.HasSuffix(ctx.Request.URL.Path, "/reader.js") {
		ctx.Header("Cache-Control", "max-age=3600")
		ctx.Header("Content-Type", "application/javascript")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(readerJS)
		return
	}

	// WHIP/WHEP, outside session
	if m := reWHIPWHEPNoID.FindStringSubmatch(ctx.Request.URL.Path); m != nil {
		switch ctx.Request.Method {
		case http.MethodOptions:
			s.onWHIPOptions(ctx, m[1], m[2] == "whip")

		case http.MethodPost:
			s.onWHIPPost(ctx, m[1], m[2] == "whip")

		case http.MethodGet, http.MethodHead, http.MethodPut:
			// RFC draft-ietf-whip-09
			// The WHIP endpoints MUST return an "405 Method Not Allowed" response
			// for any HTTP GET, HEAD or PUT requests
			writeError(ctx, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		}
		return
	}

	// WHIP/WHEP, inside session
	if m := reWHIPWHEPWithID.FindStringSubmatch(ctx.Request.URL.Path); m != nil {
		switch ctx.Request.Method {
		case http.MethodPatch:
			s.onWHIPPatch(ctx, m[1], m[3])

		case http.MethodDelete:
			s.onWHIPDelete(ctx, m[1], m[3])
		}
		return
	}

	// static resources
	if ctx.Request.Method == http.MethodGet {
		switch {
		case ctx.Request.URL.Path == "/favicon.ico":

		case len(ctx.Request.URL.Path) >= 2:
			switch {
			case len(ctx.Request.URL.Path) > len("/publish") && strings.HasSuffix(ctx.Request.URL.Path, "/publish"):
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-len("/publish")], true)

			case ctx.Request.URL.Path[len(ctx.Request.URL.Path)-1] != '/':
				ctx.Header("Location", mergePathAndQuery(ctx.Request.URL.Path+"/", ctx.Request.URL.RawQuery))
				ctx.Writer.WriteHeader(http.StatusMovedPermanently)

			default:
				s.onPage(ctx, ctx.Request.URL.Path[1:len(ctx.Request.URL.Path)-1], false)
			}
		}
		return
	}
}
