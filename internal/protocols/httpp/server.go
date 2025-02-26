// Package httpp contains HTTP utilities.
package httpp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// Server is a wrapper around http.Server that provides:
// - net.Listener allocation and closure
// - TLS allocation
// - exit on panic
// - logging
// - server header
// - filtering of invalid requests
type Server struct {
	Network     string
	Address     string
	ReadTimeout time.Duration
	Encryption  bool
	ServerCert  string
	ServerKey   string
	Handler     http.Handler
	Parent      logger.Writer

	ln     net.Listener
	inner  *http.Server
	loader *certloader.CertLoader
}

// Initialize initializes a Server.
func (s *Server) Initialize() error {
	var tlsConfig *tls.Config
	if s.Encryption {
		if s.ServerCert == "" {
			return fmt.Errorf("server cert is missing")
		}

		var err error
		s.loader, err = certloader.New(s.ServerCert, s.ServerKey, s.Parent)
		if err != nil {
			return err
		}

		tlsConfig = &tls.Config{
			GetCertificate: s.loader.GetCertificate(),
		}
	}

	if strings.HasPrefix(s.Network, "tcp") {
		var isunix bool
		s.Address, isunix = strings.CutPrefix(s.Address, "unix://")
		if isunix {
			s.Network = "unix"
			os.Remove(s.Address)
		}
	}

	var err error
	s.ln, err = net.Listen(s.Network, s.Address)
	if err != nil {
		return err
	}

	if s.Network == "unix" {
		os.Chmod(s.Address, 0775)
	}

	h := s.Handler
	h = &handlerFilterRequests{h}
	h = &handlerFilterRequests{h}
	h = &handlerServerHeader{h}
	h = &handlerLogger{h, s.Parent}
	h = &handlerExitOnPanic{h}

	s.inner = &http.Server{
		Handler:           h,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: s.ReadTimeout,
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	if tlsConfig != nil {
		go s.inner.ServeTLS(s.ln, "", "")
	} else {
		go s.inner.Serve(s.ln)
	}

	return nil
}

// Close closes all resources and waits for all routines to return.
func (s *Server) Close() {
	ctx, ctxCancel := context.WithCancel(context.Background())
	ctxCancel()
	s.inner.Shutdown(ctx)
	s.ln.Close() // in case Shutdown() is called before Serve()
	if s.loader != nil {
		s.loader.Close()
	}
}
