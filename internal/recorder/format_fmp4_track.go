package recorder

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

type formatFMP4Track struct {
	f         *formatFMP4
	initTrack *fmp4.InitTrack

	nextSample *sample
}

func (t *formatFMP4Track) write(sample *sample) error {
	// wait the first video sample before setting hasVideo
	if t.initTrack.Codec.IsVideo() {
		t.f.hasVideo = true
	}

	sample, t.nextSample = t.nextSample, sample
	if sample == nil {
		return nil
	}
	sample.Duration = uint32(t.nextSample.dts - sample.dts)

	dtsDuration := timestampToDuration(sample.dts, int(t.initTrack.TimeScale))

	if t.f.currentSegment == nil {
		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: dtsDuration,
			startNTP: sample.ntp,
		}
		t.f.currentSegment.initialize()
		// BaseTime is negative, this is not supported by fMP4. Reject the sample silently.
	} else if (dtsDuration - t.f.currentSegment.startDTS) < 0 {
		return nil
	}

	err := t.f.currentSegment.write(t, sample, dtsDuration)
	if err != nil {
		return err
	}

	nextDTSDuration := timestampToDuration(t.nextSample.dts, int(t.initTrack.TimeScale))

	if (!t.f.hasVideo || t.initTrack.Codec.IsVideo()) &&
		!t.nextSample.IsNonSyncSample &&
		(nextDTSDuration-t.f.currentSegment.startDTS) >= t.f.ri.rec.SegmentDuration {
		t.f.currentSegment.lastDTS = nextDTSDuration
		err := t.f.currentSegment.close()
		if err != nil {
			return err
		}

		t.f.currentSegment = &formatFMP4Segment{
			f:        t.f,
			startDTS: nextDTSDuration,
			startNTP: t.nextSample.ntp,
		}
		t.f.currentSegment.initialize()
	}

	return nil
}
