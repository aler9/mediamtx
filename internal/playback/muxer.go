package playback

import "github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

type muxer interface {
	writeInit(init *fmp4.Init)
	setTrack(trackID int)
	writeSample(
		dts int64,
		ptsOffset int32,
		isNonSyncSample bool,
		payloadSize uint32,
		getPayload func() ([]byte, error),
	) error
	writeFinalDTS(dts int64)
	flush() error
}
