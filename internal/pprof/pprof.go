// Package pprof contains a pprof exporter.
package pprof

import (
	"net"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

type pprofAuthManager interface {
	Authenticate(req *auth.Request) error
}

type pprofParent interface {
	logger.Writer
}

// PPROF is a pprof exporter.
type PPROF struct {
	Address        string
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigin    string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	AuthManager    pprofAuthManager
	Parent         pprofParent

	httpServer *httpp.Server
}

// Initialize initializes PPROF.
func (pp *PPROF) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(pp.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(pp.middlewareOrigin)
	router.Use(pp.middlewareAuth)

	pprof.Register(router)

	network, address := restrictnetwork.Restrict("tcp", pp.Address)

	pp.httpServer = &httpp.Server{
		Network:     network,
		Address:     address,
		ReadTimeout: time.Duration(pp.ReadTimeout),
		Encryption:  pp.Encryption,
		ServerCert:  pp.ServerCert,
		ServerKey:   pp.ServerKey,
		Handler:     router,
		Parent:      pp,
	}
	err := pp.httpServer.Initialize()
	if err != nil {
		return err
	}

	pp.Log(logger.Info, "listener opened on "+address)

	return nil
}

// Close closes PPROF.
func (pp *PPROF) Close() {
	pp.Log(logger.Info, "listener is closing")
	pp.httpServer.Close()
}

// Log implements logger.Writer.
func (pp *PPROF) Log(level logger.Level, format string, args ...interface{}) {
	pp.Parent.Log(level, "[pprof] "+format, args...)
}

func (pp *PPROF) middlewareOrigin(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", pp.AllowOrigin)
	ctx.Header("Access-Control-Allow-Credentials", "true")

	// preflight requests
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (pp *PPROF) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		IP:     net.ParseIP(ctx.ClientIP()),
		Action: conf.AuthActionPprof,
	}
	req.FillFromHTTPRequest(ctx.Request)

	err := pp.AuthManager.Authenticate(req)
	if err != nil {
		if err.(auth.Error).AskCredentials { //nolint:errorlint
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
}
