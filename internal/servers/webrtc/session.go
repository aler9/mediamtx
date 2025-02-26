package webrtc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/google/uuid"
	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	pwebrtc "github.com/pion/webrtc/v4"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func whipOffer(body []byte) *pwebrtc.SessionDescription {
	return &pwebrtc.SessionDescription{
		Type: pwebrtc.SDPTypeOffer,
		SDP:  string(body),
	}
}

type session struct {
	parentCtx             context.Context
	ipsFromInterfaces     bool
	ipsFromInterfacesList []string
	additionalHosts       []string
	iceUDPMux             ice.UDPMux
	iceTCPMux             ice.TCPMux
	req                   webRTCNewSessionReq
	wg                    *sync.WaitGroup
	externalCmdPool       *externalcmd.Pool
	pathManager           serverPathManager
	parent                *Server

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	secret    uuid.UUID
	mutex     sync.RWMutex
	pc        *webrtc.PeerConnection

	chNew           chan webRTCNewSessionReq
	chAddCandidates chan webRTCAddSessionCandidatesReq
}

func (s *session) initialize() {
	ctx, ctxCancel := context.WithCancel(s.parentCtx)

	s.ctx = ctx
	s.ctxCancel = ctxCancel
	s.created = time.Now()
	s.uuid = uuid.New()
	s.secret = uuid.New()
	s.chNew = make(chan webRTCNewSessionReq)
	s.chAddCandidates = make(chan webRTCAddSessionCandidatesReq)

	s.Log(logger.Info, "created by %s", s.req.remoteAddr)

	s.wg.Add(1)

	go s.run()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %v] "+format, append([]interface{}{id}, args...)...)
}

func (s *session) Close() {
	s.ctxCancel()
}

func (s *session) run() {
	defer s.wg.Done()

	err := s.runInner()

	s.ctxCancel()

	s.parent.closeSession(s)

	s.Log(logger.Info, "closed: %v", err)
}

func (s *session) runInner() error {
	select {
	case <-s.chNew:
	case <-s.ctx.Done():
		return fmt.Errorf("terminated")
	}

	errStatusCode, err := s.runInner2()

	if errStatusCode != 0 {
		s.req.res <- webRTCNewSessionRes{
			errStatusCode: errStatusCode,
			err:           err,
		}
	}

	return err
}

func (s *session) runInner2() (int, error) {
	if s.req.publish {
		return s.runPublish()
	}
	return s.runRead()
}

func (s *session) runPublish() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	req := defs.PathAccessRequest{
		Name:    s.req.pathName,
		Publish: true,
		IP:      net.ParseIP(ip),
		Proto:   auth.ProtocolWebRTC,
		ID:      &s.uuid,
	}
	req.FillFromHTTPRequest(s.req.httpRequest)

	path, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        s,
		AccessRequest: req,
	})
	if err != nil {
		return http.StatusBadRequest, err
	}

	defer path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})

	iceServers, err := s.parent.generateICEServers(false)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		ICEServers:            iceServers,
		HandshakeTimeout:      s.parent.HandshakeTimeout,
		TrackGatherTimeout:    s.parent.TrackGatherTimeout,
		STUNGatherTimeout:     s.parent.STUNGatherTimeout,
		IPsFromInterfaces:     s.ipsFromInterfaces,
		IPsFromInterfacesList: s.ipsFromInterfacesList,
		AdditionalHosts:       s.additionalHosts,
		ICEUDPMux:             s.iceUDPMux,
		ICETCPMux:             s.iceTCPMux,
		Publish:               false,
		Log:                   s,
	}
	err = pc.Start()
	if err != nil {
		return http.StatusBadRequest, err
	}
	defer pc.Close()

	offer := whipOffer(s.req.offer)

	var sdp sdp.SessionDescription
	err = sdp.Unmarshal([]byte(offer.SDP))
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = webrtc.TracksAreValid(sdp.MediaDescriptions)
	if err != nil {
		// RFC draft-ietf-wish-whip
		// if the number of audio and or video
		// tracks or number streams is not supported by the WHIP Endpoint, it
		// MUST reject the HTTP POST request with a "406 Not Acceptable" error
		// response.
		return http.StatusNotAcceptable, err
	}

	answer, err := pc.CreateFullAnswer(s.ctx, offer)
	if err != nil {
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilReady(s.ctx)
	if err != nil {
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	_, err = pc.GatherIncomingTracks(s.ctx)
	if err != nil {
		return 0, err
	}

	var stream *stream.Stream

	medias, err := webrtc.ToStream(pc, &stream)
	if err != nil {
		return 0, err
	}

	stream, err = path.StartPublisher(defs.PathStartPublisherReq{
		Author:             s,
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: false,
	})
	if err != nil {
		return 0, err
	}

	pc.StartReading()

	select {
	case <-pc.Failed():
		return 0, fmt.Errorf("peer connection closed")

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *session) runRead() (int, error) {
	ip, _, _ := net.SplitHostPort(s.req.remoteAddr)

	req := defs.PathAccessRequest{
		Name:  s.req.pathName,
		IP:    net.ParseIP(ip),
		Proto: auth.ProtocolWebRTC,
		ID:    &s.uuid,
	}
	req.FillFromHTTPRequest(s.req.httpRequest)

	path, stream, err := s.pathManager.AddReader(defs.PathAddReaderReq{
		Author:        s,
		AccessRequest: req,
	})
	if err != nil {
		var terr2 defs.PathNoOnePublishingError
		if errors.As(err, &terr2) {
			return http.StatusNotFound, err
		}

		return http.StatusBadRequest, err
	}

	defer path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	iceServers, err := s.parent.generateICEServers(false)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	pc := &webrtc.PeerConnection{
		ICEServers:            iceServers,
		HandshakeTimeout:      s.parent.HandshakeTimeout,
		TrackGatherTimeout:    s.parent.TrackGatherTimeout,
		STUNGatherTimeout:     s.parent.STUNGatherTimeout,
		IPsFromInterfaces:     s.ipsFromInterfaces,
		IPsFromInterfacesList: s.ipsFromInterfacesList,
		AdditionalHosts:       s.additionalHosts,
		ICEUDPMux:             s.iceUDPMux,
		ICETCPMux:             s.iceTCPMux,
		Publish:               true,
		Log:                   s,
	}

	err = webrtc.FromStream(stream, s, pc)
	if err != nil {
		return http.StatusBadRequest, err
	}

	err = pc.Start()
	if err != nil {
		stream.RemoveReader(s)
		return http.StatusBadRequest, err
	}
	defer pc.Close()

	offer := whipOffer(s.req.offer)

	answer, err := pc.CreateFullAnswer(s.ctx, offer)
	if err != nil {
		stream.RemoveReader(s)
		return http.StatusBadRequest, err
	}

	s.writeAnswer(answer)

	go s.readRemoteCandidates(pc)

	err = pc.WaitUntilReady(s.ctx)
	if err != nil {
		stream.RemoveReader(s)
		return 0, err
	}

	s.mutex.Lock()
	s.pc = pc
	s.mutex.Unlock()

	s.Log(logger.Info, "is reading from path '%s', %s",
		path.Name(), defs.FormatsInfo(stream.ReaderFormats(s)))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          s,
		ExternalCmdPool: s.externalCmdPool,
		Conf:            path.SafeConf(),
		ExternalCmdEnv:  path.ExternalCmdEnv(),
		Reader:          s.APIReaderDescribe(),
		Query:           s.req.httpRequest.URL.RawQuery,
	})
	defer onUnreadHook()

	stream.StartReader(s)
	defer stream.RemoveReader(s)

	select {
	case <-pc.Failed():
		return 0, fmt.Errorf("peer connection closed")

	case err := <-stream.ReaderError(s):
		return 0, err

	case <-s.ctx.Done():
		return 0, fmt.Errorf("terminated")
	}
}

func (s *session) writeAnswer(answer *pwebrtc.SessionDescription) {
	s.req.res <- webRTCNewSessionRes{
		sx:     s,
		answer: []byte(answer.SDP),
	}
}

func (s *session) readRemoteCandidates(pc *webrtc.PeerConnection) {
	for {
		select {
		case req := <-s.chAddCandidates:
			for _, candidate := range req.candidates {
				err := pc.AddRemoteCandidate(candidate)
				if err != nil {
					req.res <- webRTCAddSessionCandidatesRes{err: err}
				}
			}
			req.res <- webRTCAddSessionCandidatesRes{}

		case <-s.ctx.Done():
			return
		}
	}
}

// new is called by webRTCHTTPServer through Server.
func (s *session) new(req webRTCNewSessionReq) webRTCNewSessionRes {
	select {
	case s.chNew <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCNewSessionRes{err: fmt.Errorf("terminated"), errStatusCode: http.StatusInternalServerError}
	}
}

// addCandidates is called by webRTCHTTPServer through Server.
func (s *session) addCandidates(
	req webRTCAddSessionCandidatesReq,
) webRTCAddSessionCandidatesRes {
	select {
	case s.chAddCandidates <- req:
		return <-req.res

	case <-s.ctx.Done():
		return webRTCAddSessionCandidatesRes{err: fmt.Errorf("terminated")}
	}
}

// APIReaderDescribe implements reader.
func (s *session) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "webRTCSession",
		ID:   s.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (s *session) APISourceDescribe() defs.APIPathSourceOrReader {
	return s.APIReaderDescribe()
}

func (s *session) apiItem() *defs.APIWebRTCSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	peerConnectionEstablished := false
	localCandidate := ""
	remoteCandidate := ""
	bytesReceived := uint64(0)
	bytesSent := uint64(0)

	if s.pc != nil {
		peerConnectionEstablished = true
		localCandidate = s.pc.LocalCandidate()
		remoteCandidate = s.pc.RemoteCandidate()
		bytesReceived = s.pc.BytesReceived()
		bytesSent = s.pc.BytesSent()
	}

	return &defs.APIWebRTCSession{
		ID:                        s.uuid,
		Created:                   s.created,
		RemoteAddr:                s.req.remoteAddr,
		PeerConnectionEstablished: peerConnectionEstablished,
		LocalCandidate:            localCandidate,
		RemoteCandidate:           remoteCandidate,
		State: func() defs.APIWebRTCSessionState {
			if s.req.publish {
				return defs.APIWebRTCSessionStatePublish
			}
			return defs.APIWebRTCSessionStateRead
		}(),
		Path:          s.req.pathName,
		Query:         s.req.httpRequest.URL.RawQuery,
		BytesReceived: bytesReceived,
		BytesSent:     bytesSent,
	}
}
