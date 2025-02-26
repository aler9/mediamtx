package srt

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/hooks"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/stream"
)

func srtCheckPassphrase(connReq srt.ConnRequest, passphrase string) error {
	if passphrase == "" {
		return nil
	}

	if !connReq.IsEncrypted() {
		return fmt.Errorf("connection is encrypted, but not passphrase is defined in configuration")
	}

	err := connReq.SetPassphrase(passphrase)
	if err != nil {
		return fmt.Errorf("invalid passphrase")
	}

	return nil
}

type connState int

const (
	connStateRead connState = iota + 1
	connStatePublish
)

type conn struct {
	parentCtx           context.Context
	rtspAddress         string
	readTimeout         conf.Duration
	writeTimeout        conf.Duration
	udpMaxPayloadSize   int
	connReq             srt.ConnRequest
	runOnConnect        string
	runOnConnectRestart bool
	runOnDisconnect     string
	wg                  *sync.WaitGroup
	externalCmdPool     *externalcmd.Pool
	pathManager         serverPathManager
	parent              *Server

	ctx       context.Context
	ctxCancel func()
	created   time.Time
	uuid      uuid.UUID
	mutex     sync.RWMutex
	state     connState
	pathName  string
	query     string
	sconn     srt.Conn
}

func (c *conn) initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(c.parentCtx)

	c.created = time.Now()
	c.uuid = uuid.New()

	c.Log(logger.Info, "opened")

	c.wg.Add(1)
	go c.run()
}

func (c *conn) Close() {
	c.ctxCancel()
}

// Log implements logger.Writer.
func (c *conn) Log(level logger.Level, format string, args ...interface{}) {
	c.parent.Log(level, "[conn %v] "+format, append([]interface{}{c.connReq.RemoteAddr()}, args...)...)
}

func (c *conn) ip() net.IP {
	return c.connReq.RemoteAddr().(*net.UDPAddr).IP
}

func (c *conn) run() { //nolint:dupl
	defer c.wg.Done()

	onDisconnectHook := hooks.OnConnect(hooks.OnConnectParams{
		Logger:              c,
		ExternalCmdPool:     c.externalCmdPool,
		RunOnConnect:        c.runOnConnect,
		RunOnConnectRestart: c.runOnConnectRestart,
		RunOnDisconnect:     c.runOnDisconnect,
		RTSPAddress:         c.rtspAddress,
		Desc:                c.APIReaderDescribe(),
	})
	defer onDisconnectHook()

	err := c.runInner()

	c.ctxCancel()

	c.parent.closeConn(c)

	c.Log(logger.Info, "closed: %v", err)
}

func (c *conn) runInner() error {
	var streamID streamID
	err := streamID.unmarshal(c.connReq.StreamId())
	if err != nil {
		c.connReq.Reject(srt.REJ_PEER)
		return fmt.Errorf("invalid stream ID '%s': %w", c.connReq.StreamId(), err)
	}

	if streamID.mode == streamIDModePublish {
		return c.runPublish(&streamID)
	}
	return c.runRead(&streamID)
}

func (c *conn) runPublish(streamID *streamID) error {
	path, err := c.pathManager.AddPublisher(defs.PathAddPublisherReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:    streamID.path,
			Query:   streamID.query,
			IP:      c.ip(),
			Publish: true,
			User:    streamID.user,
			Pass:    streamID.pass,
			Proto:   auth.ProtocolSRT,
			ID:      &c.uuid,
		},
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)
			c.connReq.Reject(srt.REJ_PEER)
			return terr
		}
		c.connReq.Reject(srt.REJ_PEER)
		return err
	}

	defer path.RemovePublisher(defs.PathRemovePublisherReq{Author: c})

	err = srtCheckPassphrase(c.connReq, path.SafeConf().SRTPublishPassphrase)
	if err != nil {
		c.connReq.Reject(srt.REJ_PEER)
		return err
	}

	sconn, err := c.connReq.Accept()
	if err != nil {
		return err
	}

	c.mutex.Lock()
	c.state = connStatePublish
	c.pathName = streamID.path
	c.query = streamID.query
	c.sconn = sconn
	c.mutex.Unlock()

	readerErr := make(chan error)
	go func() {
		readerErr <- c.runPublishReader(sconn, path)
	}()

	select {
	case err := <-readerErr:
		sconn.Close()
		return err

	case <-c.ctx.Done():
		sconn.Close()
		<-readerErr
		return errors.New("terminated")
	}
}

func (c *conn) runPublishReader(sconn srt.Conn, path defs.Path) error {
	sconn.SetReadDeadline(time.Now().Add(time.Duration(c.readTimeout)))
	r, err := mcmpegts.NewReader(mcmpegts.NewBufferedReader(sconn))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(c)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegts.ToStream(r, &stream, c)
	if err != nil {
		return err
	}

	stream, err = path.StartPublisher(defs.PathStartPublisherReq{
		Author:             c,
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if err != nil {
		return err
	}

	for {
		err = r.Read()
		if err != nil {
			return err
		}
	}
}

func (c *conn) runRead(streamID *streamID) error {
	path, stream, err := c.pathManager.AddReader(defs.PathAddReaderReq{
		Author: c,
		AccessRequest: defs.PathAccessRequest{
			Name:  streamID.path,
			Query: streamID.query,
			IP:    c.ip(),
			User:  streamID.user,
			Pass:  streamID.pass,
			Proto: auth.ProtocolSRT,
			ID:    &c.uuid,
		},
	})
	if err != nil {
		var terr auth.Error
		if errors.As(err, &terr) {
			// wait some seconds to mitigate brute force attacks
			<-time.After(auth.PauseAfterError)
			c.connReq.Reject(srt.REJ_PEER)
			return terr
		}
		c.connReq.Reject(srt.REJ_PEER)
		return err
	}

	defer path.RemoveReader(defs.PathRemoveReaderReq{Author: c})

	err = srtCheckPassphrase(c.connReq, path.SafeConf().SRTReadPassphrase)
	if err != nil {
		c.connReq.Reject(srt.REJ_PEER)
		return err
	}

	sconn, err := c.connReq.Accept()
	if err != nil {
		return err
	}
	defer sconn.Close()

	c.mutex.Lock()
	c.state = connStateRead
	c.pathName = streamID.path
	c.query = streamID.query
	c.sconn = sconn
	c.mutex.Unlock()

	bw := bufio.NewWriterSize(sconn, srtMaxPayloadSize(c.udpMaxPayloadSize))

	err = mpegts.FromStream(stream, c, bw, sconn, time.Duration(c.writeTimeout))
	if err != nil {
		return err
	}

	c.Log(logger.Info, "is reading from path '%s', %s",
		path.Name(), defs.FormatsInfo(stream.ReaderFormats(c)))

	onUnreadHook := hooks.OnRead(hooks.OnReadParams{
		Logger:          c,
		ExternalCmdPool: c.externalCmdPool,
		Conf:            path.SafeConf(),
		ExternalCmdEnv:  path.ExternalCmdEnv(),
		Reader:          c.APIReaderDescribe(),
		Query:           streamID.query,
	})
	defer onUnreadHook()

	// disable read deadline
	sconn.SetReadDeadline(time.Time{})

	stream.StartReader(c)
	defer stream.RemoveReader(c)

	select {
	case <-c.ctx.Done():
		return fmt.Errorf("terminated")

	case err = <-stream.ReaderError(c):
		return err
	}
}

// APIReaderDescribe implements reader.
func (c *conn) APIReaderDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "srtConn",
		ID:   c.uuid.String(),
	}
}

// APISourceDescribe implements source.
func (c *conn) APISourceDescribe() defs.APIPathSourceOrReader {
	return c.APIReaderDescribe()
}

func (c *conn) apiItem() *defs.APISRTConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item := &defs.APISRTConn{
		ID:         c.uuid,
		Created:    c.created,
		RemoteAddr: c.connReq.RemoteAddr().String(),
		State: func() defs.APISRTConnState {
			switch c.state {
			case connStateRead:
				return defs.APISRTConnStateRead

			case connStatePublish:
				return defs.APISRTConnStatePublish

			default:
				return defs.APISRTConnStateIdle
			}
		}(),
		Path:  c.pathName,
		Query: c.query,
	}

	if c.sconn != nil {
		var s srt.Statistics
		c.sconn.Stats(&s)

		item.PacketsSent = s.Accumulated.PktSent
		item.PacketsReceived = s.Accumulated.PktRecv
		item.PacketsSentUnique = s.Accumulated.PktSentUnique
		item.PacketsReceivedUnique = s.Accumulated.PktRecvUnique
		item.PacketsSendLoss = s.Accumulated.PktSendLoss
		item.PacketsReceivedLoss = s.Accumulated.PktRecvLoss
		item.PacketsRetrans = s.Accumulated.PktRetrans
		item.PacketsReceivedRetrans = s.Accumulated.PktRecvRetrans
		item.PacketsSentACK = s.Accumulated.PktSentACK
		item.PacketsReceivedACK = s.Accumulated.PktRecvACK
		item.PacketsSentNAK = s.Accumulated.PktSentNAK
		item.PacketsReceivedNAK = s.Accumulated.PktRecvNAK
		item.PacketsSentKM = s.Accumulated.PktSentKM
		item.PacketsReceivedKM = s.Accumulated.PktRecvKM
		item.UsSndDuration = s.Accumulated.UsSndDuration
		item.PacketsReceivedBelated = s.Accumulated.PktRecvBelated
		item.PacketsSendDrop = s.Accumulated.PktSendDrop
		item.PacketsReceivedDrop = s.Accumulated.PktRecvDrop
		item.PacketsReceivedUndecrypt = s.Accumulated.PktRecvUndecrypt
		item.BytesSent = s.Accumulated.ByteSent
		item.BytesReceived = s.Accumulated.ByteRecv
		item.BytesSentUnique = s.Accumulated.ByteSentUnique
		item.BytesReceivedUnique = s.Accumulated.ByteRecvUnique
		item.BytesReceivedLoss = s.Accumulated.ByteRecvLoss
		item.BytesRetrans = s.Accumulated.ByteRetrans
		item.BytesReceivedRetrans = s.Accumulated.ByteRecvRetrans
		item.BytesReceivedBelated = s.Accumulated.ByteRecvBelated
		item.BytesSendDrop = s.Accumulated.ByteSendDrop
		item.BytesReceivedDrop = s.Accumulated.ByteRecvDrop
		item.BytesReceivedUndecrypt = s.Accumulated.ByteRecvUndecrypt
		item.UsPacketsSendPeriod = s.Instantaneous.UsPktSendPeriod
		item.PacketsFlowWindow = s.Instantaneous.PktFlowWindow
		item.PacketsFlightSize = s.Instantaneous.PktFlightSize
		item.MsRTT = s.Instantaneous.MsRTT
		item.MbpsSendRate = s.Instantaneous.MbpsSentRate
		item.MbpsReceiveRate = s.Instantaneous.MbpsRecvRate
		item.MbpsLinkCapacity = s.Instantaneous.MbpsLinkCapacity
		item.BytesAvailSendBuf = s.Instantaneous.ByteAvailSendBuf
		item.BytesAvailReceiveBuf = s.Instantaneous.ByteAvailRecvBuf
		item.MbpsMaxBW = s.Instantaneous.MbpsMaxBW
		item.ByteMSS = s.Instantaneous.ByteMSS
		item.PacketsSendBuf = s.Instantaneous.PktSendBuf
		item.BytesSendBuf = s.Instantaneous.ByteSendBuf
		item.MsSendBuf = s.Instantaneous.MsSendBuf
		item.MsSendTsbPdDelay = s.Instantaneous.MsSendTsbPdDelay
		item.PacketsReceiveBuf = s.Instantaneous.PktRecvBuf
		item.BytesReceiveBuf = s.Instantaneous.ByteRecvBuf
		item.MsReceiveBuf = s.Instantaneous.MsRecvBuf
		item.MsReceiveTsbPdDelay = s.Instantaneous.MsRecvTsbPdDelay
		item.PacketsReorderTolerance = s.Instantaneous.PktReorderTolerance
		item.PacketsReceivedAvgBelatedTime = s.Instantaneous.PktRecvAvgBelatedTime
		item.PacketsSendLossRate = s.Instantaneous.PktSendLossRate
		item.PacketsReceivedLossRate = s.Instantaneous.PktRecvLossRate
	}

	return item
}
