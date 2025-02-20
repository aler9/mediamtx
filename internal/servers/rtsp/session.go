package rtsp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/google/uuid"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

type session struct {
	isTLS           bool
	transports      conf.RTSPTransports
	rsession        *gortsplib.ServerSession
	rconn           *gortsplib.ServerConn
	rserver         *gortsplib.Server
	externalCmdPool *externalcmd.Pool
	pathManager     serverPathManager
	parent          logger.Writer

	uuid            uuid.UUID
	created         time.Time
	path            defs.Path
	stream          *stream.Stream
	onUnreadHook    func()
	mutex           sync.Mutex
	state           gortsplib.ServerSessionState
	transport       *gortsplib.Transport
	pathName        string
	query           string
	decodeErrLogger logger.Writer
	writeErrLogger  logger.Writer
}

func (s *session) initialize() {
	s.uuid = uuid.New()
	s.created = time.Now()

	s.decodeErrLogger = logger.NewLimitedLogger(s)
	s.writeErrLogger = logger.NewLimitedLogger(s)

	s.Log(logger.Info, "created by %v", s.rconn.NetConn().RemoteAddr())
}

// Close closes a Session.
func (s *session) Close() {
	s.rsession.Close()
}

func (s *session) remoteAddr() net.Addr {
	return s.rconn.NetConn().RemoteAddr()
}

// Log implements logger.Writer.
func (s *session) Log(level logger.Level, format string, args ...interface{}) {
	id := hex.EncodeToString(s.uuid[:4])
	s.parent.Log(level, "[session %s] "+format, append([]interface{}{id}, args...)...)
}

// onClose is called by rtspServer.
func (s *session) onClose(err error) {
	if s.rsession.State() == gortsplib.ServerSessionStatePlay {
		s.onUnreadHook()
	}

	switch s.rsession.State() {
	case gortsplib.ServerSessionStatePrePlay, gortsplib.ServerSessionStatePlay:
		s.path.RemoveReader(defs.PathRemoveReaderReq{Author: s})

	case gortsplib.ServerSessionStatePreRecord, gortsplib.ServerSessionStateRecord:
		s.path.RemovePublisher(defs.PathRemovePublisherReq{Author: s})
	}

	s.path = nil
	s.stream = nil

	s.Log(logger.Info, "destroyed: %v", err)
}

// onAnnounce is called by rtspServer.
func (s *session) onAnnounce(c *conn, ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	req := defs.PathAccessRequest{
		Name:    ctx.Path,
		Query:   ctx.Query,
		Publish: true,
		IP:      c.ip(),
		Proto:   auth.ProtocolRTSP,
		ID:      &c.uuid,
		CustomVerifyFunc: func(expectedUser, expectedPass string) bool {
			return c.rconn.VerifyCredentials(ctx.Request, expectedUser, expectedPass)
		},
	}
	req.FillFromRTSPRequest(ctx.Request)

	path, err := s.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author:        s,
		AccessRequest: req,
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			return c.handleAuthError(ctx.Request)
		}

		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.path = path

	s.mutex.Lock()
	s.state = gortsplib.ServerSessionStatePreRecord
	s.pathName = ctx.Path
	s.query = ctx.Query
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onSetup is called by rtspServer.
func (s *session) onSetup(c *conn, ctx *gortsplib.ServerHandlerOnSetupCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	if len(ctx.Path) == 0 || ctx.Path[0] != '/' {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, nil, fmt.Errorf("invalid path")
	}
	ctx.Path = ctx.Path[1:]

	// in case the client is setupping a stream with UDP or UDP-multicast, and these
	// transport protocols are disabled, gortsplib already blocks the request.
	// we have only to handle the case in which the transport protocol is TCP
	// and it is disabled.
	if ctx.Transport == gortsplib.TransportTCP {
		if _, ok := s.transports[gortsplib.TransportTCP]; !ok {
			return &base.Response{
				StatusCode: base.StatusUnsupportedTransport,
			}, nil, nil
		}
	}

	switch s.rsession.State() {
	case gortsplib.ServerSessionStateInitial, gortsplib.ServerSessionStatePrePlay: // play
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

		path, stream, err := s.pathManager.AddReader(defs.PathAddReaderReq{
			Author:        s,
			AccessRequest: req,
		})
		if err != nil {
			var terr auth.Error
			if errors.As(err, &terr) {
				res, err2 := c.handleAuthError(ctx.Request)
				return res, nil, err2
			}

			var terr2 defs.PathNoOnePublishingError
			if errors.As(err, &terr2) {
				return &base.Response{
					StatusCode: base.StatusNotFound,
				}, nil, err
			}

			return &base.Response{
				StatusCode: base.StatusBadRequest,
			}, nil, err
		}

		s.path = path
		s.stream = stream

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.pathName = ctx.Path
		s.query = ctx.Query
		s.mutex.Unlock()

		var rstream *gortsplib.ServerStream
		if !s.isTLS {
			rstream = stream.RTSPStream(s.rserver)
		} else {
			rstream = stream.RTSPSStream(s.rserver)
		}

		return &base.Response{
			StatusCode: base.StatusOK,
		}, rstream, nil

	default: // record
		return &base.Response{
			StatusCode: base.StatusOK,
		}, nil, nil
	}
}

// onPlay is called by rtspServer.
func (s *session) onPlay(_ *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	h := make(base.Header)

	if s.rsession.State() == gortsplib.ServerSessionStatePrePlay {
		s.Log(logger.Info, "is reading from path '%s', with %s, %s",
			s.path.Name(),
			s.rsession.SetuppedTransport(),
			defs.MediasInfo(s.rsession.SetuppedMedias()))

		s.onUnreadHook = hooks.OnRead(hooks.OnReadParams{
			Logger:          s,
			ExternalCmdPool: s.externalCmdPool,
			Conf:            s.path.SafeConf(),
			ExternalCmdEnv:  s.path.ExternalCmdEnv(),
			Reader:          s.APIReaderDescribe(),
			Query:           s.rsession.SetuppedQuery(),
		})

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePlay
		s.transport = s.rsession.SetuppedTransport()
		s.mutex.Unlock()

		if len(s.stream.CachedUnits) > 0 {
			lastTimestamp := s.stream.CachedUnits[len(s.stream.CachedUnits)-1].GetRTPPackets()[0].Timestamp
			for _, medi := range s.stream.Desc().Medias {
				if medi.Type == description.MediaTypeVideo {
					for _, u := range s.stream.CachedUnits {
						for _, pkt := range u.GetRTPPackets() {
							pkt.Timestamp = lastTimestamp
							err := s.rsession.WritePacketRTP(medi, pkt)
							if err != nil {
								break
							}
						}
					}
				}
			}
		}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     h,
	}, nil
}

// onRecord is called by rtspServer.
func (s *session) onRecord(_ *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	stream, err := s.path.StartPublisher(defs.PathStartPublisherReq{
		Author:             s,
		Desc:               s.rsession.AnnouncedDescription(),
		GenerateRTPPackets: false,
	})
	if err != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, err
	}

	s.stream = stream

	for _, medi := range s.rsession.AnnouncedDescription().Medias {
		for _, forma := range medi.Formats {
			cmedi := medi
			cforma := forma

			s.rsession.OnPacketRTP(cmedi, cforma, func(pkt *rtp.Packet) {
				pts, ok := s.rsession.PacketPTS2(cmedi, pkt)
				if !ok {
					return
				}

				stream.WriteRTPPacket(cmedi, cforma, pkt, time.Now(), pts)
			})
		}
	}

	s.mutex.Lock()
	s.state = gortsplib.ServerSessionStateRecord
	s.transport = s.rsession.SetuppedTransport()
	s.mutex.Unlock()

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// onPause is called by rtspServer.
func (s *session) onPause(_ *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	switch s.rsession.State() {
	case gortsplib.ServerSessionStatePlay:
		s.onUnreadHook()

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePrePlay
		s.mutex.Unlock()

	case gortsplib.ServerSessionStateRecord:
		s.path.StopPublisher(defs.PathStopPublisherReq{Author: s})

		s.mutex.Lock()
		s.state = gortsplib.ServerSessionStatePreRecord
		s.mutex.Unlock()
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// APIReaderDescribe implements reader.
func (s *session) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: func() string {
			if s.isTLS {
				return "rtspsSession"
			}
			return "rtspSession"
		}(),
		ID: s.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (s *session) APISourceDescribe() defs.APIPathSourceOrReader {
	return s.APIReaderDescribe()
}

// onPacketLost is called by rtspServer.
func (s *session) onPacketLost(ctx *gortsplib.ServerHandlerOnPacketLostCtx) {
	s.decodeErrLogger.Log(logger.Warn, ctx.Error.Error())
}

// onDecodeError is called by rtspServer.
func (s *session) onDecodeError(ctx *gortsplib.ServerHandlerOnDecodeErrorCtx) {
	s.decodeErrLogger.Log(logger.Warn, ctx.Error.Error())
}

// onStreamWriteError is called by rtspServer.
func (s *session) onStreamWriteError(ctx *gortsplib.ServerHandlerOnStreamWriteErrorCtx) {
	s.writeErrLogger.Log(logger.Warn, ctx.Error.Error())
}

func (s *session) apiItem() *defs.APIRTSPSession {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	stats := s.rsession.Stats()

	return &defs.APIRTSPSession{
		ID:         s.uuid,
		Created:    s.created,
		RemoteAddr: s.remoteAddr().String(),
		State: func() defs.APIRTSPSessionState {
			switch s.state {
			case gortsplib.ServerSessionStatePrePlay,
				gortsplib.ServerSessionStatePlay:
				return defs.APIRTSPSessionStateRead

			case gortsplib.ServerSessionStatePreRecord,
				gortsplib.ServerSessionStateRecord:
				return defs.APIRTSPSessionStatePublish
			}
			return defs.APIRTSPSessionStateIdle
		}(),
		Path:  s.pathName,
		Query: s.query,
		Transport: func() *string {
			if s.transport == nil {
				return nil
			}
			v := s.transport.String()
			return &v
		}(),
		BytesReceived:       stats.BytesReceived,
		BytesSent:           stats.BytesSent,
		RTPPacketsReceived:  stats.RTPPacketsReceived,
		RTPPacketsSent:      stats.RTPPacketsSent,
		RTPPacketsLost:      stats.RTPPacketsLost,
		RTPPacketsInError:   stats.RTPPacketsInError,
		RTPPacketsJitter:    stats.RTPPacketsJitter,
		RTCPPacketsReceived: stats.RTCPPacketsReceived,
		RTCPPacketsSent:     stats.RTCPPacketsSent,
		RTCPPacketsInError:  stats.RTCPPacketsInError,
	}
}
