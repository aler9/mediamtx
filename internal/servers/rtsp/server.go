// Package rtsp contains a RTSP server.
package rtsp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/liberrors"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/certloader"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// ErrConnNotFound is returned when a connection is not found.
var ErrConnNotFound = errors.New("connection not found")

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

func printAddresses(srv *gortsplib.Server) string {
	var ret []string

	ret = append(ret, fmt.Sprintf("%s (TCP)", srv.RTSPAddress))

	if srv.UDPRTPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTP)", srv.UDPRTPAddress))
	}

	if srv.UDPRTCPAddress != "" {
		ret = append(ret, fmt.Sprintf("%s (UDP/RTCP)", srv.UDPRTCPAddress))
	}

	return strings.Join(ret, ", ")
}

type serverPathManager interface {
	Describe(req defs.PathDescribeReq) defs.PathDescribeRes
	AddPublisher(_ defs.PathAddPublisherReq) (defs.Path, error)
	AddReader(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error)
}

type serverParent interface {
	logger.Writer
}

// Server is a RTSP server.
type Server struct {
	Address             string
	AuthMethods         []auth.VerifyMethod
	ReadTimeout         conf.Duration
	WriteTimeout        conf.Duration
	WriteQueueSize      int
	UseUDP              bool
	UseMulticast        bool
	RTPAddress          string
	RTCPAddress         string
	MulticastIPRange    string
	MulticastRTPPort    int
	MulticastRTCPPort   int
	IsTLS               bool
	ServerCert          string
	ServerKey           string
	RTSPAddress         string
	Transports          conf.RTSPTransports
	RunOnConnect        string
	RunOnConnectRestart bool
	RunOnDisconnect     string
	ExternalCmdPool     *externalcmd.Pool
	PathManager         serverPathManager
	Parent              serverParent

	ctx       context.Context
	ctxCancel func()
	wg        sync.WaitGroup
	srv       *gortsplib.Server
	mutex     sync.RWMutex
	conns     map[*gortsplib.ServerConn]*conn
	sessions  map[*gortsplib.ServerSession]*session
	loader    *certloader.CertLoader
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.conns = make(map[*gortsplib.ServerConn]*conn)
	s.sessions = make(map[*gortsplib.ServerSession]*session)

	s.srv = &gortsplib.Server{
		Handler:        s,
		ReadTimeout:    time.Duration(s.ReadTimeout),
		WriteTimeout:   time.Duration(s.WriteTimeout),
		WriteQueueSize: s.WriteQueueSize,
		RTSPAddress:    s.Address,
		AuthMethods:    s.AuthMethods,
	}

	if s.UseUDP {
		s.srv.UDPRTPAddress = s.RTPAddress
		s.srv.UDPRTCPAddress = s.RTCPAddress
	}

	if s.UseMulticast {
		s.srv.MulticastIPRange = s.MulticastIPRange
		s.srv.MulticastRTPPort = s.MulticastRTPPort
		s.srv.MulticastRTCPPort = s.MulticastRTCPPort
	}

	if s.IsTLS {
		var err error
		s.loader, err = certloader.New(s.ServerCert, s.ServerKey, s.Parent)
		if err != nil {
			return err
		}

		s.srv.TLSConfig = &tls.Config{GetCertificate: s.loader.GetCertificate()}
	}

	err := s.srv.Start()
	if err != nil {
		return err
	}

	s.Log(logger.Info, "listener opened on %s", printAddresses(s.srv))

	s.wg.Add(1)
	go s.run()

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...interface{}) {
	label := func() string {
		if s.IsTLS {
			return "RTSPS"
		}
		return "RTSP"
	}()
	s.Parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "listener is closing")
	s.ctxCancel()
	s.wg.Wait()
	if s.loader != nil {
		s.loader.Close()
	}
}

func (s *Server) run() {
	defer s.wg.Done()

	serverErr := make(chan error)
	go func() {
		serverErr <- s.srv.Wait()
	}()

outer:
	select {
	case err := <-serverErr:
		s.Log(logger.Error, "%s", err)
		break outer

	case <-s.ctx.Done():
		s.srv.Close()
		<-serverErr
		break outer
	}

	s.ctxCancel()
}

// OnConnOpen implements gortsplib.ServerHandlerOnConnOpen.
func (s *Server) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	c := &conn{
		isTLS:               s.IsTLS,
		rtspAddress:         s.RTSPAddress,
		authMethods:         s.AuthMethods,
		readTimeout:         s.ReadTimeout,
		runOnConnect:        s.RunOnConnect,
		runOnConnectRestart: s.RunOnConnectRestart,
		runOnDisconnect:     s.RunOnDisconnect,
		externalCmdPool:     s.ExternalCmdPool,
		pathManager:         s.PathManager,
		rconn:               ctx.Conn,
		rserver:             s.srv,
		parent:              s,
	}
	c.initialize()
	s.mutex.Lock()
	s.conns[ctx.Conn] = c
	s.mutex.Unlock()

	ctx.Conn.SetUserData(c)
}

// OnConnClose implements gortsplib.ServerHandlerOnConnClose.
func (s *Server) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	s.mutex.Lock()
	c := s.conns[ctx.Conn]
	delete(s.conns, ctx.Conn)
	s.mutex.Unlock()
	c.onClose(ctx.Error)
}

// OnRequest implements gortsplib.ServerHandlerOnRequest.
func (s *Server) OnRequest(sc *gortsplib.ServerConn, req *base.Request) {
	c := sc.UserData().(*conn)
	c.onRequest(req)
}

// OnResponse implements gortsplib.ServerHandlerOnResponse.
func (s *Server) OnResponse(sc *gortsplib.ServerConn, res *base.Response) {
	c := sc.UserData().(*conn)
	c.OnResponse(res)
}

// OnSessionOpen implements gortsplib.ServerHandlerOnSessionOpen.
func (s *Server) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	se := &session{
		isTLS:           s.IsTLS,
		transports:      s.Transports,
		rsession:        ctx.Session,
		rconn:           ctx.Conn,
		rserver:         s.srv,
		externalCmdPool: s.ExternalCmdPool,
		pathManager:     s.PathManager,
		parent:          s,
	}
	se.initialize()
	s.mutex.Lock()
	s.sessions[ctx.Session] = se
	s.mutex.Unlock()
	ctx.Session.SetUserData(se)
}

// OnSessionClose implements gortsplib.ServerHandlerOnSessionClose.
func (s *Server) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	s.mutex.Lock()
	se := s.sessions[ctx.Session]
	delete(s.sessions, ctx.Session)
	s.mutex.Unlock()

	if se != nil {
		se.onClose(ctx.Error)
	}
}

// OnDescribe implements gortsplib.ServerHandlerOnDescribe.
func (s *Server) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*conn)
	return c.onDescribe(ctx)
}

// OnAnnounce implements gortsplib.ServerHandlerOnAnnounce.
func (s *Server) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	c := ctx.Conn.UserData().(*conn)
	se := ctx.Session.UserData().(*session)
	return se.onAnnounce(c, ctx)
}

// OnSetup implements gortsplib.ServerHandlerOnSetup.
func (s *Server) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	c := ctx.Conn.UserData().(*conn)
	se := ctx.Session.UserData().(*session)
	return se.onSetup(c, ctx)
}

// OnPlay implements gortsplib.ServerHandlerOnPlay.
func (s *Server) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*session)
	return se.onPlay(ctx)
}

// OnRecord implements gortsplib.ServerHandlerOnRecord.
func (s *Server) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*session)
	return se.onRecord(ctx)
}

// OnPause implements gortsplib.ServerHandlerOnPause.
func (s *Server) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	se := ctx.Session.UserData().(*session)
	return se.onPause(ctx)
}

// OnPacketLost implements gortsplib.ServerHandlerOnDecodeError.
func (s *Server) OnPacketLost(ctx *gortsplib.ServerHandlerOnPacketLostCtx) {
	se := ctx.Session.UserData().(*session)
	se.onPacketLost(ctx)
}

// OnDecodeError implements gortsplib.ServerHandlerOnDecodeError.
func (s *Server) OnDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	se := ctx.Session.UserData().(*session)
	se.onDecodeError(ctx)
}

// OnStreamWriteError implements gortsplib.ServerHandlerOnStreamWriteError.
func (s *Server) OnStreamWriteError(ctx *gortsplib.ServerHandlerOnStreamWriteErrorCtx) {
	se := ctx.Session.UserData().(*session)
	se.onStreamWriteError(ctx)
}

func (s *Server) findConnByUUID(uuid uuid.UUID) *conn {
	for _, c := range s.conns {
		if c.uuid == uuid {
			return c
		}
	}
	return nil
}

func (s *Server) findSessionByUUID(uuid uuid.UUID) (*gortsplib.ServerSession, *session) {
	for key, sx := range s.sessions {
		if sx.uuid == uuid {
			return key, sx
		}
	}
	return nil, nil
}

func (s *Server) findSessionByRSessionUnsafe(rsession *gortsplib.ServerSession) *session {
	return s.sessions[rsession]
}

// APIConnsList is called by api and metrics.
func (s *Server) APIConnsList() (*defs.APIRTSPConnsList, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &defs.APIRTSPConnsList{
		Items: []*defs.APIRTSPConn{},
	}

	for _, c := range s.conns {
		data.Items = append(data.Items, c.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// APIConnsGet is called by api.
func (s *Server) APIConnsGet(uuid uuid.UUID) (*defs.APIRTSPConn, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	conn := s.findConnByUUID(uuid)
	if conn == nil {
		return nil, ErrConnNotFound
	}

	return conn.apiItem(), nil
}

// APISessionsList is called by api and metrics.
func (s *Server) APISessionsList() (*defs.APIRTSPSessionList, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	data := &defs.APIRTSPSessionList{
		Items: []*defs.APIRTSPSession{},
	}

	for _, s := range s.sessions {
		data.Items = append(data.Items, s.apiItem())
	}

	sort.Slice(data.Items, func(i, j int) bool {
		return data.Items[i].Created.Before(data.Items[j].Created)
	})

	return data, nil
}

// APISessionsGet is called by api.
func (s *Server) APISessionsGet(uuid uuid.UUID) (*defs.APIRTSPSession, error) {
	select {
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	_, sx := s.findSessionByUUID(uuid)
	if sx == nil {
		return nil, ErrSessionNotFound
	}

	return sx.apiItem(), nil
}

// APISessionsKick is called by api.
func (s *Server) APISessionsKick(uuid uuid.UUID) error {
	select {
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	default:
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key, sx := s.findSessionByUUID(uuid)
	if sx == nil {
		return ErrSessionNotFound
	}

	sx.Close()
	delete(s.sessions, key)
	sx.onClose(liberrors.ErrServerTerminated{})
	return nil
}
