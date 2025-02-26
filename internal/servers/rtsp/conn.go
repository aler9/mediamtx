package rtsp

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	rtspauth "github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/gortsplib/v4/pkg/liberrors"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
)

func absoluteURL(req *base.Request, v string) string {
	if strings.HasPrefix(v, "/") {
		ur := base.URL{
			Scheme: req.URL.Scheme,
			Host:   req.URL.Host,
			Path:   v,
		}
		return ur.String()
	}

	return v
}

func credentialsProvided(req *base.Request) bool {
	var auth headers.Authorization
	err := auth.Unmarshal(req.Header["Authorization"])
	return err == nil && auth.Username != ""
}

type connParent interface {
	logger.Writer
	findSessionByRSessionUnsafe(rsession *gortsplib.ServerSession) *session
}

type conn struct {
	isTLS               bool
	rtspAddress         string
	authMethods         []rtspauth.VerifyMethod
	readTimeout         conf.Duration
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	externalCmdPool     *externalcmd.Pool
	pathManager         serverPathManager
	rconn               *gortsplib.ServerConn
	rserver             *gortsplib.Server
	parent              connParent

	uuid             uuid.UUID
	created          time.Time
	onDisconnectHook func()
}

func (c *conn) initialize() {
	c.uuid = uuid.New()
	c.created = time.Now()

	c.Log(logger.Info, "opened")

	desc := defs.APIPathSourceOrReader{
		Type: func() string {
			if c.isTLS {
				return "rtspsConn"
			}
			return "rtspConn"
		}(),
		ID: c.uuid.String(),
	}

	c.onDisconnectHook = hooks.OnConnect(hooks.OnConnectParams{
		Logger:              c,
		ExternalCmdPool:     c.externalCmdPool,
		RunOnConnect:        c.runOnConnect,
		RunOnConnectRestart: c.runOnConnectRestart,
		RunOnDisconnect:     c.runOnDisconnect,
		RTSPAddress:         c.rtspAddress,
		Desc:                desc,
	})
}

// Log implements logger.Writer.
func (c *conn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.rconn.NetConn().RemoteAddr()}, args...)...)
}

// Conn returns the RTSP connection.
func (c *conn) Conn() *gortsplib.ServerConn {
	return c.rconn
}

func (c *conn) remoteAddr() net.Addr {
	return c.rconn.NetConn().RemoteAddr()
}

func (c *conn) ip() net.IP {
	return c.rconn.NetConn().RemoteAddr().(*net.TCPAddr).IP
}

// onClose is called by rtspServer.
func (c *conn) onClose(err error) {
	c.Log(logger.Info, "closed: %v", err)

	c.onDisconnectHook()
}

// onRequest is called by rtspServer.
func (c *conn) onRequest(req *base.Request) {
	c.Log(logger.Debug, "[c->s] %v", req)
}

// OnResponse is called by rtspServer.
func (c *conn) OnResponse(res *base.Response) {
	c.Log(logger.Debug, "[s->c] %v", res)
}

// onDescribe is called by rtspServer.
func (c *conn) onDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	req := defs.PathAccessRequest{
		Name:  ctx.Path,
		Query: ctx.Query,
		IP:    c.ip(),
		Proto: auth.ProtocolRTSP,
		ID:    &c.uuid,
		CustomVerifyFunc: func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		},
	}
	req.FillFromRTSPRequest(ctx.Request)

	res := c.pathManager.Describe(defs.PathDescribeReq{
		AccessRequest: req,
	})

	if res.Err != nil {
		var terr auth.Error
		if errors.As(res.Err, &terr) {
			res, err2 := c.handleAuthError(ctx.Request)
			return res, nil, err2
		}

		var terr2 defs.PathNoOnePublishingError
		if errors.As(res.Err, &terr2) {
			return &base.Response{
				StatusCode: base.StatusNotFound,
			}, nil, res.Err
		}

		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, res.Err
	}

	if res.Redirect != "" {
		return &base.Response{
			StatusCode: base.StatusMovedPermanently,
			Header: base.Header{
				"Location": base.HeaderValue{absoluteURL(ctx.Request, res.Redirect)},
			},
		}, nil, nil
	}

	var stream *gortsplib.ServerStream
	if !c.isTLS {
		stream = res.Stream.RTSPStream(c.rserver)
	} else {
		stream = res.Stream.RTSPSStream(c.rserver)
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, stream, nil
}

func (c *conn) handleAuthError(req *base.Request) (*base.Response, error) {
	if credentialsProvided(req) {
		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)
	}

	// let gortsplib decide whether connection should be terminated,
	// depending on whether credentials have been provided or not.
	return &base.Response{
		StatusCode: base.StatusUnauthorized,
	}, liberrors.ErrServerAuth{}
}

func (c *conn) apiItem() *defs.APIRTSPConn {
	stats := c.rconn.Stats()

	return &defs.APIRTSPConn{
		ID:            c.uuid,
		Created:       c.created,
		RemoteAddr:    c.remoteAddr().String(),
		BytesReceived: stats.BytesReceived,
		BytesSent:     stats.BytesSent,
		Session: func() *uuid.UUID {
			sx := c.parent.findSessionByRSessionUnsafe(c.rconn.Session())
			if sx != nil {
				return &sx.uuid
			}
			return nil
		}(),
	}
}
