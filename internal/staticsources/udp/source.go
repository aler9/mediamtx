// Package udp contains the UDP static source.
package udp

import (
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/multicast"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/mpegts"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
	"github.com/bluenviron/mediamtx/internal/stream"
)

const (
	// same size as GStreamer's rtspsrc
	udpKernelReadBufferSize = 0x80000
)

type packetConnReader struct {
	net.PacketConn
}

func newPacketConnReader(pc net.PacketConn) *packetConnReader {
	return &packetConnReader{
		PacketConn: pc,
	}
}

func (r *packetConnReader) Read(p []byte) (int, error) {
	n, _, err := r.PacketConn.ReadFrom(p)
	return n, err
}

type packetConn interface {
	net.PacketConn
	SetReadBuffer(int) error
}

// Source is a UDP static source.
type Source struct {
	ReadTimeout conf.Duration
	Parent      defs.StaticSourceParent
}

// Log implements logger.Writer.
func (s *Source) Log(level logger.Level, format string, args ...interface{}) {
	s.Parent.Log(level, "[UDP source] "+format, args...)
}

// Run implements StaticSource.
func (s *Source) Run(params defs.StaticSourceRunParams) error {
	s.Log(logger.Debug, "connecting")

	hostPort := params.ResolvedSource[len("udp://"):]

	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return err
	}

	var pc packetConn

	if ip4 := addr.IP.To4(); ip4 != nil && addr.IP.IsMulticast() {
		pc, err = multicast.NewMultiConn(hostPort, true, net.ListenPacket)
		if err != nil {
			return err
		}
	} else {
		var tmp net.PacketConn
		tmp, err = net.ListenPacket(restrictnetwork.Restrict("udp", addr.String()))
		if err != nil {
			return err
		}
		pc = tmp.(*net.UDPConn)
	}

	defer pc.Close()

	err = pc.SetReadBuffer(udpKernelReadBufferSize)
	if err != nil {
		return err
	}

	readerErr := make(chan error)
	go func() {
		readerErr <- s.runReader(pc)
	}()

	select {
	case err := <-readerErr:
		return err

	case <-params.Context.Done():
		pc.Close()
		<-readerErr
		return fmt.Errorf("terminated")
	}
}

func (s *Source) runReader(pc net.PacketConn) error {
	pc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
	r, err := mcmpegts.NewReader(mcmpegts.NewBufferedReader(newPacketConnReader(pc)))
	if err != nil {
		return err
	}

	decodeErrLogger := logger.NewLimitedLogger(s)

	r.OnDecodeError(func(err error) {
		decodeErrLogger.Log(logger.Warn, err.Error())
	})

	var stream *stream.Stream

	medias, err := mpegts.ToStream(r, &stream, s)
	if err != nil {
		return err
	}

	res := s.Parent.SetReady(defs.PathSourceStaticSetReadyReq{
		Desc:               &description.Session{Medias: medias},
		GenerateRTPPackets: true,
	})
	if res.Err != nil {
		return res.Err
	}

	defer s.Parent.SetNotReady(defs.PathSourceStaticSetNotReadyReq{})

	stream = res.Stream

	for {
		pc.SetReadDeadline(time.Now().Add(time.Duration(s.ReadTimeout)))
		err := r.Read()
		if err != nil {
			return err
		}
	}
}

// APISourceDescribe implements StaticSource.
func (*Source) APISourceDescribe() defs.APIPathSourceOrReader {
	return defs.APIPathSourceOrReader{
		Type: "udpSource",
		ID:   "",
	}
}
