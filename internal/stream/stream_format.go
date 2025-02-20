package stream

import (
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	maxCachedGOPSize int = 512
)

func unitSize(u unit.Unit) uint64 {
	n := uint64(0)
	for _, pkt := range u.GetRTPPackets() {
		n += uint64(pkt.MarshalSize())
	}
	return n
}

func isKeyFrame(u unit.Unit) bool {
	switch tunit := u.(type) {
	case *unit.H264:
		return h264.IDRPresent(tunit.AU)
	case *unit.H265:
		return h265.IsRandomAccess(tunit.AU)
	}
	return false
}

func isEmptyAU(u unit.Unit) bool {
	switch tunit := u.(type) {
	case *unit.H264:
		return len(tunit.AU) == 0
	case *unit.H265:
		return len(tunit.AU) == 0
	}
	return true
}

type streamFormat struct {
	udpMaxPayloadSize  int
	format             format.Format
	generateRTPPackets bool
	decodeErrLogger    logger.Writer

	proc           formatprocessor.Processor
	pausedReaders  map[*streamReader]ReadFunc
	runningReaders map[*streamReader]ReadFunc
	gopCache       bool
}

func (sf *streamFormat) initialize() error {
	sf.pausedReaders = make(map[*streamReader]ReadFunc)
	sf.runningReaders = make(map[*streamReader]ReadFunc)

	var err error
	sf.proc, err = formatprocessor.New(sf.udpMaxPayloadSize, sf.format, sf.generateRTPPackets)
	if err != nil {
		return err
	}

	return nil
}

func (sf *streamFormat) addReader(sr *streamReader, cb ReadFunc) {
	sf.pausedReaders[sr] = cb
}

func (sf *streamFormat) removeReader(sr *streamReader) {
	delete(sf.pausedReaders, sr)
	delete(sf.runningReaders, sr)
}

func (sf *streamFormat) startReader(sr *streamReader) {
	if cb, ok := sf.pausedReaders[sr]; ok {
		delete(sf.pausedReaders, sr)
		sf.runningReaders[sr] = cb
	}
}

func (sf *streamFormat) writeUnit(s *Stream, medi *description.Media, u unit.Unit) {
	err := sf.proc.ProcessUnit(u)
	if err != nil {
		sf.decodeErrLogger.Log(logger.Warn, err.Error())
		return
	}

	sf.writeUnitInner(s, medi, u)
}

func (sf *streamFormat) writeRTPPacket(
	s *Stream,
	medi *description.Media,
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
) {
	hasNonRTSPReaders := len(sf.pausedReaders) > 0 || len(sf.runningReaders) > 0 || sf.gopCache

	u, err := sf.proc.ProcessRTPPacket(pkt, ntp, pts, hasNonRTSPReaders)
	if err != nil {
		sf.decodeErrLogger.Log(logger.Warn, err.Error())
		return
	}

	sf.writeUnitInner(s, medi, u)
}

func (sf *streamFormat) writeUnitInner(s *Stream, medi *description.Media, u unit.Unit) {
	size := unitSize(u)

	atomic.AddUint64(s.bytesReceived, size)

	if sf.gopCache && medi.Type == description.MediaTypeVideo {
		if isKeyFrame(u) {
			if s.CachedUnits == nil {
				// Initialize the cache and enable caching
				s.CachedUnits = make([]unit.Unit, 0, maxCachedGOPSize)
			} else {
				// Keep the last packets that were used to generate the key frame.
				// This is to send a full key frame in the RTSP stream.
				i := len(s.CachedUnits)
				for ; i > 0; i-- {
					if !isEmptyAU(s.CachedUnits[i-1]) {
						break
					}
				}
				s.CachedUnits = s.CachedUnits[i:]
			}
		}
		if s.CachedUnits != nil {
			s.CachedUnits = append(s.CachedUnits, u)
		}
		l := len(s.CachedUnits)
		if l > maxCachedGOPSize {
			s.CachedUnits = s.CachedUnits[l-maxCachedGOPSize:]
			sf.decodeErrLogger.Log(logger.Warn, "GOP cache is full, dropping packets")
		}
	}

	if s.rtspStream != nil {
		for _, pkt := range u.GetRTPPackets() {
			s.rtspStream.WritePacketRTPWithNTP(medi, pkt, u.GetNTP()) //nolint:errcheck
		}
	}

	if s.rtspsStream != nil {
		for _, pkt := range u.GetRTPPackets() {
			s.rtspsStream.WritePacketRTPWithNTP(medi, pkt, u.GetNTP()) //nolint:errcheck
		}
	}

	for sr, cb := range sf.runningReaders {
		ccb := cb
		sr.push(func() error {
			atomic.AddUint64(s.bytesSent, size)
			return ccb(u)
		})
	}
}
