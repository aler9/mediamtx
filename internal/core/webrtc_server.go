package core

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/aler9/mediamtx/internal/conf"
	"github.com/aler9/mediamtx/internal/logger"
	"github.com/aler9/mediamtx/internal/websocket"
)

//go:embed webrtc_publish_index.html
var webrtcPublishIndex []byte

//go:embed webrtc_read_index.html
var webrtcReadIndex []byte

type webRTCServerAPIConnsListItem struct {
	Created                   time.Time `json:"created"`
	RemoteAddr                string    `json:"remoteAddr"`
	PeerConnectionEstablished bool      `json:"peerConnectionEstablished"`
	LocalCandidate            string    `json:"localCandidate"`
	RemoteCandidate           string    `json:"remoteCandidate"`
	BytesReceived             uint64    `json:"bytesReceived"`
	BytesSent                 uint64    `json:"bytesSent"`
}

type webRTCServerAPIConnsListData struct {
	Items map[string]webRTCServerAPIConnsListItem `json:"items"`
}

type webRTCServerAPIConnsListRes struct {
	data *webRTCServerAPIConnsListData
	err  error
}

type webRTCServerAPIConnsListReq struct {
	res chan webRTCServerAPIConnsListRes
}

type webRTCServerAPIConnsKickRes struct {
	err error
}

type webRTCServerAPIConnsKickReq struct {
	id  string
	res chan webRTCServerAPIConnsKickRes
}

type webRTCConnNewReq struct {
	pathName     string
	publish      bool
	wsconn       *websocket.ServerConn
	res          chan *webRTCConn
	videoCodec   string
	audioCodec   string
	videoBitrate string
}

type webRTCServerParent interface {
	logger.Writer
}

type webRTCServer struct {
	allowOrigin     string
	trustedProxies  conf.IPsOrCIDRs
	iceServers      []string
	readBufferCount int
	pathManager     *pathManager
	metrics         *metrics
	parent          webRTCServerParent

	ctx               context.Context
	ctxCancel         func()
	ln                net.Listener
	requestPool       *httpRequestPool
	httpServer        *http.Server
	udpMuxLn          net.PacketConn
	tcpMuxLn          net.Listener
	conns             map[*webRTCConn]struct{}
	iceHostNAT1To1IPs []string
	iceUDPMux         ice.UDPMux
	iceTCPMux         ice.TCPMux

	// in
	connNew        chan webRTCConnNewReq
	chConnClose    chan *webRTCConn
	chAPIConnsList chan webRTCServerAPIConnsListReq
	chAPIConnsKick chan webRTCServerAPIConnsKickReq

	// out
	done chan struct{}
}

func newWebRTCServer(
	parentCtx context.Context,
	address string,
	encryption bool,
	serverKey string,
	serverCert string,
	allowOrigin string,
	trustedProxies conf.IPsOrCIDRs,
	iceServers []string,
	readTimeout conf.StringDuration,
	readBufferCount int,
	pathManager *pathManager,
	metrics *metrics,
	parent webRTCServerParent,
	iceHostNAT1To1IPs []string,
	iceUDPMuxAddress string,
	iceTCPMuxAddress string,
) (*webRTCServer, error) {
	ln, err := net.Listen(restrictNetwork("tcp", address))
	if err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if encryption {
		crt, err := tls.LoadX509KeyPair(serverCert, serverKey)
		if err != nil {
			ln.Close()
			return nil, err
		}

		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
	}

	var iceUDPMux ice.UDPMux
	var udpMuxLn net.PacketConn
	if iceUDPMuxAddress != "" {
		udpMuxLn, err = net.ListenPacket(restrictNetwork("udp", iceUDPMuxAddress))
		if err != nil {
			return nil, err
		}
		iceUDPMux = webrtc.NewICEUDPMux(nil, udpMuxLn)
	}

	var iceTCPMux ice.TCPMux
	var tcpMuxLn net.Listener
	if iceTCPMuxAddress != "" {
		tcpMuxLn, err = net.Listen(restrictNetwork("tcp", iceTCPMuxAddress))
		if err != nil {
			return nil, err
		}
		iceTCPMux = webrtc.NewICETCPMux(nil, tcpMuxLn, 8)
	}

	ctx, ctxCancel := context.WithCancel(parentCtx)

	s := &webRTCServer{
		allowOrigin:       allowOrigin,
		trustedProxies:    trustedProxies,
		iceServers:        iceServers,
		readBufferCount:   readBufferCount,
		pathManager:       pathManager,
		metrics:           metrics,
		parent:            parent,
		ctx:               ctx,
		ctxCancel:         ctxCancel,
		ln:                ln,
		udpMuxLn:          udpMuxLn,
		tcpMuxLn:          tcpMuxLn,
		iceUDPMux:         iceUDPMux,
		iceTCPMux:         iceTCPMux,
		iceHostNAT1To1IPs: iceHostNAT1To1IPs,
		conns:             make(map[*webRTCConn]struct{}),
		connNew:           make(chan webRTCConnNewReq),
		chConnClose:       make(chan *webRTCConn),
		chAPIConnsList:    make(chan webRTCServerAPIConnsListReq),
		chAPIConnsKick:    make(chan webRTCServerAPIConnsKickReq),
		done:              make(chan struct{}),
	}

	s.requestPool = newHTTPRequestPool()

	router := gin.New()
	httpSetTrustedProxies(router, trustedProxies)

	router.NoRoute(s.requestPool.mw, httpLoggerMiddleware(s), httpServerHeaderMiddleware, s.onRequest)

	s.httpServer = &http.Server{
		Handler:           router,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: time.Duration(readTimeout),
		ErrorLog:          log.New(&nilWriter{}, "", 0),
	}

	str := "listener opened on " + address + " (HTTP)"
	if udpMuxLn != nil {
		str += ", " + iceUDPMuxAddress + " (ICE/UDP)"
	}
	if tcpMuxLn != nil {
		str += ", " + iceTCPMuxAddress + " (ICE/TCP)"
	}
	s.Log(logger.Info, str)

	if s.metrics != nil {
		s.metrics.webRTCServerSet(s)
	}

	go s.run()

	return s, nil
}

// Log is the main logging function.
func (s *webRTCServer) Log(level logger.Level, format string, args ...interface{}) {
	s.parent.Log(level, "[WebRTC] "+format, append([]interface{}{}, args...)...)
}

func (s *webRTCServer) close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	<-s.done
}

func (s *webRTCServer) run() {
	defer close(s.done)

	if s.httpServer.TLSConfig != nil {
		go s.httpServer.ServeTLS(s.ln, "", "")
	} else {
		go s.httpServer.Serve(s.ln)
	}

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-s.connNew:
			c := newWebRTCConn(
				s.ctx,
				s.readBufferCount,
				req.pathName,
				req.publish,
				req.wsconn,
				req.videoCodec,
				req.audioCodec,
				req.videoBitrate,
				s.iceServers,
				&wg,
				s.pathManager,
				s,
				s.iceHostNAT1To1IPs,
				s.iceUDPMux,
				s.iceTCPMux,
			)
			s.conns[c] = struct{}{}
			req.res <- c

		case conn := <-s.chConnClose:
			delete(s.conns, conn)

		case req := <-s.chAPIConnsList:
			data := &webRTCServerAPIConnsListData{
				Items: make(map[string]webRTCServerAPIConnsListItem),
			}

			for c := range s.conns {
				peerConnectionEstablished := false
				localCandidate := ""
				remoteCandidate := ""
				bytesReceived := uint64(0)
				bytesSent := uint64(0)

				pc := c.safePC()
				if pc != nil {
					peerConnectionEstablished = true
					localCandidate = pc.localCandidate()
					remoteCandidate = pc.remoteCandidate()
					bytesReceived = pc.bytesReceived()
					bytesSent = pc.bytesSent()
				}

				data.Items[c.uuid.String()] = webRTCServerAPIConnsListItem{
					Created:                   c.created,
					RemoteAddr:                c.remoteAddr().String(),
					PeerConnectionEstablished: peerConnectionEstablished,
					LocalCandidate:            localCandidate,
					RemoteCandidate:           remoteCandidate,
					BytesReceived:             bytesReceived,
					BytesSent:                 bytesSent,
				}
			}

			req.res <- webRTCServerAPIConnsListRes{data: data}

		case req := <-s.chAPIConnsKick:
			res := func() bool {
				for c := range s.conns {
					if c.uuid.String() == req.id {
						delete(s.conns, c)
						c.close()
						return true
					}
				}
				return false
			}()
			if res {
				req.res <- webRTCServerAPIConnsKickRes{}
			} else {
				req.res <- webRTCServerAPIConnsKickRes{fmt.Errorf("not found")}
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	s.httpServer.Shutdown(context.Background())
	s.ln.Close() // in case Shutdown() is called before Serve()

	s.requestPool.close()
	wg.Wait()

	if s.udpMuxLn != nil {
		s.udpMuxLn.Close()
	}

	if s.tcpMuxLn != nil {
		s.tcpMuxLn.Close()
	}
}

func (s *webRTCServer) onRequest(ctx *gin.Context) {
	ctx.Writer.Header().Set("Access-Control-Allow-Origin", s.allowOrigin)
	ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

	switch ctx.Request.Method {
	case http.MethodGet:

	case http.MethodOptions:
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", ctx.Request.Header.Get("Access-Control-Request-Headers"))
		ctx.Writer.WriteHeader(http.StatusOK)
		return

	default:
		return
	}

	// remove leading prefix
	pa := ctx.Request.URL.Path[1:]

	var dir string
	var fname string
	var publish bool

	switch {
	case strings.HasSuffix(pa, "/publish/ws"):
		dir = pa[:len(pa)-len("/publish/ws")]
		fname = "publish/ws"
		publish = true

	case strings.HasSuffix(pa, "/publish"):
		dir = pa[:len(pa)-len("/publish")]
		fname = "publish"
		publish = true

	case strings.HasSuffix(pa, "/ws"):
		dir = pa[:len(pa)-len("/ws")]
		fname = "ws"
		publish = false

	case pa == "favicon.ico":
		return

	default:
		dir = pa
		fname = ""
		publish = false

		if !strings.HasSuffix(dir, "/") {
			ctx.Writer.Header().Set("Location", "/"+dir+"/")
			ctx.Writer.WriteHeader(http.StatusMovedPermanently)
			return
		}
	}

	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return
	}

	user, pass, hasCredentials := ctx.Request.BasicAuth()

	res := s.pathManager.getPathConf(pathGetPathConfReq{
		name:    dir,
		publish: publish,
		credentials: authCredentials{
			query: ctx.Request.URL.RawQuery,
			ip:    net.ParseIP(ctx.ClientIP()),
			user:  user,
			pass:  pass,
			proto: authProtocolWebRTC,
		},
	})
	if res.err != nil {
		if terr, ok := res.err.(pathErrAuth); ok {
			if !hasCredentials {
				ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
				ctx.Writer.WriteHeader(http.StatusUnauthorized)
				return
			}

			s.Log(logger.Info, "authentication error: %v", terr.wrapped)
			ctx.Writer.WriteHeader(http.StatusUnauthorized)
			return
		}

		ctx.Writer.WriteHeader(http.StatusNotFound)
		return
	}

	switch fname {
	case "":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcReadIndex)

	case "publish":
		ctx.Writer.Header().Set("Content-Type", "text/html")
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.Write(webrtcPublishIndex)

	case "ws", "publish/ws":
		wsconn, err := websocket.NewServerConn(ctx.Writer, ctx.Request)
		if err != nil {
			return
		}
		defer wsconn.Close()

		c := s.newConn(webRTCConnNewReq{
			pathName:     dir,
			publish:      (fname == "publish/ws"),
			wsconn:       wsconn,
			videoCodec:   ctx.Query("video_codec"),
			audioCodec:   ctx.Query("audio_codec"),
			videoBitrate: ctx.Query("video_bitrate"),
		})
		if c == nil {
			return
		}

		c.wait()
	}
}

func (s *webRTCServer) newConn(req webRTCConnNewReq) *webRTCConn {
	req.res = make(chan *webRTCConn)

	select {
	case s.connNew <- req:
		return <-req.res
	case <-s.ctx.Done():
		return nil
	}
}

// connClose is called by webRTCConn.
func (s *webRTCServer) connClose(c *webRTCConn) {
	select {
	case s.chConnClose <- c:
	case <-s.ctx.Done():
	}
}

// apiConnsList is called by api.
func (s *webRTCServer) apiConnsList() webRTCServerAPIConnsListRes {
	req := webRTCServerAPIConnsListReq{
		res: make(chan webRTCServerAPIConnsListRes),
	}

	select {
	case s.chAPIConnsList <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCServerAPIConnsListRes{err: fmt.Errorf("terminated")}
	}
}

// apiConnsKick is called by api.
func (s *webRTCServer) apiConnsKick(id string) webRTCServerAPIConnsKickRes {
	req := webRTCServerAPIConnsKickReq{
		id:  id,
		res: make(chan webRTCServerAPIConnsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCServerAPIConnsKickRes{err: fmt.Errorf("terminated")}
	}
}
