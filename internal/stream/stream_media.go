package stream

import (
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"

	"github.com/bluenviron/mediamtx/internal/logger"
)

type streamMedia struct {
	formats map[format.Format]*streamFormat
}

func newStreamMedia(udpMaxPayloadSize int,
	medi *description.Media,
	generateRTPPackets bool,
	decodeErrLogger logger.Writer,
	gopCache bool,
) (*streamMedia, error) {
	sm := &streamMedia{
		formats: make(map[format.Format]*streamFormat),
	}

	for _, forma := range medi.Formats {
		sf := &streamFormat{
			udpMaxPayloadSize:  udpMaxPayloadSize,
			format:             forma,
			generateRTPPackets: generateRTPPackets,
			decodeErrLogger:    decodeErrLogger,
			gopCache:           gopCache,
		}
		err := sf.initialize()
		if err != nil {
			return nil, err
		}
		sm.formats[forma] = sf
	}

	return sm, nil
}
