// Package webrtc contains a WebRTC server.
package webrtc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/ice/v4"
	"github.com/pion/logging"
	pwebrtc "github.com/pion/webrtc/v4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	webrtcTurnSecretExpiration = 24 * 3600 * time.Second
)

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

var webrtcNilLogger = logging.NewDefaultLeveledLoggerForScope("", 0, &nilWriter{})

func randInt63() (int64, error) {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}

	return int64(uint64(b[0]&0b01111111)<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])), nil
}

// https://cs.opensource.google/go/go/+/refs/tags/go1.20.4:src/math/rand/rand.go;l=119
func randInt63n(n int64) (int64, error) {
	if n&(n-1) == 0 { // n is power of two, can mask
		r, err := randInt63()
		if err != nil {
			return 0, err
		}
		return r & (n - 1), nil
	}

	maxVal := int64((1 << 63) - 1 - (1<<63)%uint64(n))

	v, err := randInt63()
	if err != nil {
		return 0, err
	}

	for v > maxVal {
		v, err = randInt63()
		if err != nil {
			return 0, err
		}
	}

	return v % n, nil
}

func randomTurnUser() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz1234567890"
	b := make([]byte, 20)
	for i := range b {
		j, err := randInt63n(int64(len(charset)))
		if err != nil {
			return "", err
		}

		b[i] = charset[int(j)]
	}

	return string(b), nil
}

type serverAPISessionsListRes struct {
	data *defs.APIWebRTCSessionList
	err  error
}

type serverAPISessionsListReq struct {
	res chan serverAPISessionsListRes
}

type serverAPISessionsGetRes struct {
	data *defs.APIWebRTCSession
	err  error
}

type serverAPISessionsGetReq struct {
	uuid uuid.UUID
	res  chan serverAPISessionsGetRes
}

type serverAPISessionsKickRes struct {
	err error
}

type serverAPISessionsKickReq struct {
	uuid uuid.UUID
	res  chan serverAPISessionsKickRes
}

type webRTCNewSessionRes struct {
	sx            *session
	answer        []byte
	errStatusCode int
	err           error
}

type webRTCNewSessionReq struct {
	pathName    string
	remoteAddr  string
	offer       []byte
	publish     bool
	httpRequest *http.Request
	res         chan webRTCNewSessionRes
}

type webRTCAddSessionCandidatesRes struct {
	sx  *session
	err error
}

type webRTCAddSessionCandidatesReq struct {
	pathName   string
	secret     uuid.UUID
	candidates []*pwebrtc.ICECandidateInit
	res        chan webRTCAddSessionCandidatesRes
}

type webRTCDeleteSessionRes struct {
	err error
}

type webRTCDeleteSessionReq struct {
	pathName string
	secret   uuid.UUID
	res      chan webRTCDeleteSessionRes
}

type serverPathManager interface {
	FindPathConf(req defs.PathFindPathConfReq) (*conf.Path, error)
	AddPublisher(req defs.PathAddPublisherReq) (defs.Path, error)
	AddReader(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a WebRTC server.
type Server struct {
	Address               string
	Encryption            bool
	ServerKey             string
	ServerCert            string
	AllowOrigin           string
	TrustedProxies        conf.IPNetworks
	ReadTimeout           conf.Duration
	LocalUDPAddress       string
	LocalTCPAddress       string
	IPsFromInterfaces     bool
	IPsFromInterfacesList []string
	AdditionalHosts       []string
	ICEServers            []conf.WebRTCICEServer
	HandshakeTimeout      conf.Duration
	TrackGatherTimeout    conf.Duration
	STUNGatherTimeout     conf.Duration
	ExternalCmdPool       *externalcmd.Pool
	PathManager           serverPathManager
	Parent                serverParent

	ctx              context.Context
	ctxCancel        func()
	httpServer       *httpServer
	udpMuxLn         net.PacketConn
	tcpMuxLn         net.Listener
	iceUDPMux        ice.UDPMux
	iceTCPMux        ice.TCPMux
	sessions         map[*session]struct{}
	sessionsBySecret map[uuid.UUID]*session

	// in
	chNewSession           chan webRTCNewSessionReq
	chCloseSession         chan *session
	chAddSessionCandidates chan webRTCAddSessionCandidatesReq
	chDeleteSession        chan webRTCDeleteSessionReq
	chAPISessionsList      chan serverAPISessionsListReq
	chAPISessionsGet       chan serverAPISessionsGetReq
	chAPIConnsKick         chan serverAPISessionsKickReq

	// out
	done chan struct{}
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	ctx, ctxCancel := context.WithCancel(context.Background())

	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.sessions = make(map[*session]struct{})
	s.sessionsBySecret = make(map[uuid.UUID]*session)
	s.chNewSession = make(chan webRTCNewSessionReq)
	s.chCloseSession = make(chan *session)
	s.chAddSessionCandidates = make(chan webRTCAddSessionCandidatesReq)
	s.chDeleteSession = make(chan webRTCDeleteSessionReq)
	s.chAPISessionsList = make(chan serverAPISessionsListReq)
	s.chAPISessionsGet = make(chan serverAPISessionsGetReq)
	s.chAPIConnsKick = make(chan serverAPISessionsKickReq)
	s.done = make(chan struct{})

	s.httpServer = &httpServer{
		address:        s.Address,
		encryption:     s.Encryption,
		serverKey:      s.ServerKey,
		serverCert:     s.ServerCert,
		allowOrigin:    s.AllowOrigin,
		trustedProxies: s.TrustedProxies,
		readTimeout:    s.ReadTimeout,
		pathManager:    s.PathManager,
		parent:         s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		ctxCancel()
		return err
	}

	if s.LocalUDPAddress != "" {
		s.udpMuxLn, err = net.ListenPacket(restrictnetwork.Restrict("udp", s.LocalUDPAddress))
		if err != nil {
			s.httpServer.close()
			ctxCancel()
			return err
		}
		s.iceUDPMux = pwebrtc.NewICEUDPMux(webrtcNilLogger, s.udpMuxLn)
	}

	if s.LocalTCPAddress != "" {
		s.tcpMuxLn, err = net.Listen(restrictnetwork.Restrict("tcp", s.LocalTCPAddress))
		if err != nil {
			s.udpMuxLn.Close()
			s.httpServer.close()
			ctxCancel()
			return err
		}
		s.iceTCPMux = pwebrtc.NewICETCPMux(webrtcNilLogger, s.tcpMuxLn, 8)
	}

	str := "listener opened on " + s.Address + " (HTTP)"
	if s.udpMuxLn != nil {
		str += ", " + s.LocalUDPAddress + " (ICE/UDP)"
	}
	if s.tcpMuxLn != nil {
		str += ", " + s.LocalTCPAddress + " (ICE/TCP)"
	}
	s.Log(logger.Info, str)

	go s.run()

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[WebRTC] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-s.chNewSession:
			sx := &session{
				parentCtx:             s.ctx,
				ipsFromInterfaces:     s.IPsFromInterfaces,
				ipsFromInterfacesList: s.IPsFromInterfacesList,
				additionalHosts:       s.AdditionalHosts,
				iceUDPMux:             s.iceUDPMux,
				iceTCPMux:             s.iceTCPMux,
				req:                   req,
				wg:                    &wg,
				externalCmdPool:       s.ExternalCmdPool,
				pathManager:           s.PathManager,
				parent:                s,
			}
			sx.initialize()
			s.sessions[sx] = struct{}{}
			s.sessionsBySecret[sx.secret] = sx
			req.res <- webRTCNewSessionRes{sx: sx}

		case sx := <-s.chCloseSession:
			delete(s.sessions, sx)
			delete(s.sessionsBySecret, sx.secret)

		case req := <-s.chAddSessionCandidates:
			sx, ok := s.sessionsBySecret[req.secret]
			if !ok || sx.req.pathName != req.pathName {
				req.res <- webRTCAddSessionCandidatesRes{err: ErrSessionNotFound}
				continue
			}

			req.res <- webRTCAddSessionCandidatesRes{sx: sx}

		case req := <-s.chDeleteSession:
			sx, ok := s.sessionsBySecret[req.secret]
			if !ok || sx.req.pathName != req.pathName {
				req.res <- webRTCDeleteSessionRes{err: ErrSessionNotFound}
				continue
			}

			delete(s.sessions, sx)
			delete(s.sessionsBySecret, sx.secret)
			sx.Close()

			req.res <- webRTCDeleteSessionRes{}

		case req := <-s.chAPISessionsList:
			data := &defs.APIWebRTCSessionList{
				Items: []*defs.APIWebRTCSession{},
			}

			for sx := range s.sessions {
				data.Items = append(data.Items, sx.apiItem())
			}

			sort.Slice(data.Items, func(i, j int) bool {
				return data.Items[i].Created.Before(data.Items[j].Created)
			})

			req.res <- serverAPISessionsListRes{data: data}

		case req := <-s.chAPISessionsGet:
			sx := s.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- serverAPISessionsGetRes{err: ErrSessionNotFound}
				continue
			}

			req.res <- serverAPISessionsGetRes{data: sx.apiItem()}

		case req := <-s.chAPIConnsKick:
			sx := s.findSessionByUUID(req.uuid)
			if sx == nil {
				req.res <- serverAPISessionsKickRes{err: ErrSessionNotFound}
				continue
			}

			delete(s.sessions, sx)
			delete(s.sessionsBySecret, sx.secret)
			sx.Close()

			req.res <- serverAPISessionsKickRes{}

		case <-s.ctx.Done():
			break outer
		}
	}

	s.ctxCancel()

	wg.Wait()

	s.httpServer.close()

	if s.udpMuxLn != nil {
		s.udpMuxLn.Close()
	}

	if s.tcpMuxLn != nil {
		s.tcpMuxLn.Close()
	}
}

func (s *Server) findSessionByUUID(uuid uuid.UUID) *session {
	for sx := range s.sessions {
		if sx.uuid == uuid {
			return sx
		}
	}
	return nil
}

func (s *Server) generateICEServers(clientConfig bool) ([]pwebrtc.ICEServer, error) {
	ret := make([]pwebrtc.ICEServer, 0, len(s.ICEServers))

	for _, server := range s.ICEServers {
		if !server.ClientOnly || clientConfig {
			if server.Username == "AUTH_SECRET" {
				expireDate := time.Now().Add(webrtcTurnSecretExpiration).Unix()

				user, err := randomTurnUser()
				if err != nil {
					return nil, err
				}

				server.Username = strconv.FormatInt(expireDate, 10) + ":" + user

				h := hmac.New(sha1.New, []byte(server.Password))
				h.Write([]byte(server.Username))

				server.Password = base64.StdEncoding.EncodeToString(h.Sum(nil))
			}

			ret = append(ret, pwebrtc.ICEServer{
				URLs:       []string{server.URL},
				Username:   server.Username,
				Credential: server.Password,
			})
		}
	}

	return ret, nil
}

// newSession is called by webRTCHTTPServer.
func (s *Server) newSession(req webRTCNewSessionReq) webRTCNewSessionRes {
	req.res = make(chan webRTCNewSessionRes)

	select {
	case s.chNewSession <- req:
		res := <-req.res

		return res.sx.new(req)

	case <-s.ctx.Done():
		return webRTCNewSessionRes{
			errStatusCode: http.StatusInternalServerError,
			err:           fmt.Errorf("terminated"),
		}
	}
}

// closeSession is called by session.
func (s *Server) closeSession(sx *session) {
	select {
	case s.chCloseSession <- sx:
	case <-s.ctx.Done():
	}
}

// addSessionCandidates is called by webRTCHTTPServer.
func (s *Server) addSessionCandidates(
	req webRTCAddSessionCandidatesReq,
) webRTCAddSessionCandidatesRes {
	req.res = make(chan webRTCAddSessionCandidatesRes)
	select {
	case s.chAddSessionCandidates <- req:
		res1 := <-req.res
		if res1.err != nil {
			return res1
		}

		return res1.sx.addCandidates(req)

	case <-s.ctx.Done():
		return webRTCAddSessionCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// deleteSession is called by webRTCHTTPServer.
func (s *Server) deleteSession(req webRTCDeleteSessionReq) error {
	req.res = make(chan webRTCDeleteSessionRes)
	select {
	case s.chDeleteSession <- req:
		res := <-req.res
		return res.err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}

// APISessionsList is called by api.
func (s *Server) APISessionsList() (*defs.APIWebRTCSessionList, error) {
	req := serverAPISessionsListReq{
		res: make(chan serverAPISessionsListRes),
	}

	select {
	case s.chAPISessionsList <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsGet is called by api.
func (s *Server) APISessionsGet(uuid uuid.UUID) (*defs.APIWebRTCSession, error) {
	req := serverAPISessionsGetReq{
		uuid: uuid,
		res:  make(chan serverAPISessionsGetRes),
	}

	select {
	case s.chAPISessionsGet <- req:
		res := <-req.res
		return res.data, res.err

	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsKick is called by api.
func (s *Server) APISessionsKick(uuid uuid.UUID) error {
	req := serverAPISessionsKickReq{
		uuid: uuid,
		res:  make(chan serverAPISessionsKickRes),
	}

	select {
	case s.chAPIConnsKick <- req:
		res := <-req.res
		return res.err

	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}
}
