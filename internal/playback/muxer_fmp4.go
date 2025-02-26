package playback

import (
	"io"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

const (
	partDuration = 1 * time.Second
)

type muxerFMP4Track struct {
	id        int
	timeScale uint32
	firstDTS  int64
	lastDTS   int64
	samples   []*fmp4.PartSample
}

func findTrack(tracks []*muxerFMP4Track, id int) *muxerFMP4Track {
	for _, track := range tracks {
		if track.id == id {
			return track
		}
	}
	return nil
}

type muxerFMP4 struct {
	w io.Writer

	init               *fmp4.Init
	nextSequenceNumber uint32
	tracks             []*muxerFMP4Track
	curTrack           *muxerFMP4Track
	outBuf             seekablebuffer.Buffer
}

func (w *muxerFMP4) writeInit(init *fmp4.Init) {
	w.init = init

	w.tracks = make([]*muxerFMP4Track, len(init.Tracks))

	for i, track := range init.Tracks {
		w.tracks[i] = &muxerFMP4Track{
			id:        track.ID,
			timeScale: track.TimeScale,
			firstDTS:  -1,
		}
	}
}

func (w *muxerFMP4) setTrack(trackID int) {
	w.curTrack = findTrack(w.tracks, trackID)
}

func (w *muxerFMP4) writeSample(
	dts int64,
	ptsOffset int32,
	isNonSyncSample bool,
	_ uint32,
	getPayload func() ([]byte, error),
) error {
	pl, err := getPayload()
	if err != nil {
		return err
	}

	if dts >= 0 {
		if w.curTrack.firstDTS < 0 {
			w.curTrack.firstDTS = dts

			// if frame is a IDR, remove previous GOP
			if !isNonSyncSample {
				w.curTrack.samples = nil
			}
		} else {
			diff := dts - w.curTrack.lastDTS
			if diff < 0 {
				diff = 0
			}
			w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
		}

		w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
			PTSOffset:       ptsOffset,
			IsNonSyncSample: isNonSyncSample,
			Payload:         pl,
		})
		w.curTrack.lastDTS = dts

		partDurationMP4 := durationGoToMp4(partDuration, w.curTrack.timeScale)

		if (w.curTrack.lastDTS - w.curTrack.firstDTS) >= partDurationMP4 {
			err := w.innerFlush(false)
			if err != nil {
				return err
			}
		}
	} else {
		// store GOP of the first frame, and set PTSOffset = 0 and Duration = 0 in each sample
		if !isNonSyncSample { // if frame is a IDR, reset GOP
			w.curTrack.samples = []*fmp4.PartSample{{
				IsNonSyncSample: isNonSyncSample,
				Payload:         pl,
			}}
		} else {
			// append frame to current GOP
			w.curTrack.samples = append(w.curTrack.samples, &fmp4.PartSample{
				IsNonSyncSample: isNonSyncSample,
				Payload:         pl,
			})
		}
	}

	return nil
}

func (w *muxerFMP4) writeFinalDTS(dts int64) {
	if w.curTrack.firstDTS >= 0 {
		diff := dts - w.curTrack.lastDTS
		if diff < 0 {
			diff = 0
		}
		w.curTrack.samples[len(w.curTrack.samples)-1].Duration = uint32(diff)
	}
}

func (w *muxerFMP4) innerFlush(final bool) error {
	var part fmp4.Part

	for _, track := range w.tracks {
		if track.firstDTS >= 0 && (len(track.samples) > 1 || (final && len(track.samples) != 0)) {
			// do not write the final sample
			// in order to allow changing its duration to compensate NTP-DTS differences
			var samples []*fmp4.PartSample
			if !final {
				samples = track.samples[:len(track.samples)-1]
			} else {
				samples = track.samples
			}

			part.Tracks = append(part.Tracks, &fmp4.PartTrack{
				ID:       track.id,
				BaseTime: uint64(track.firstDTS),
				Samples:  samples,
			})

			if !final {
				track.samples = track.samples[len(track.samples)-1:]
				track.firstDTS = track.lastDTS
			}
		}
	}

	if part.Tracks != nil {
		part.SequenceNumber = w.nextSequenceNumber
		w.nextSequenceNumber++

		if w.init != nil {
			err := w.init.Marshal(&w.outBuf)
			if err != nil {
				return err
			}

			_, err = w.w.Write(w.outBuf.Bytes())
			if err != nil {
				return err
			}

			w.init = nil
			w.outBuf.Reset()
		}

		err := part.Marshal(&w.outBuf)
		if err != nil {
			return err
		}

		_, err = w.w.Write(w.outBuf.Bytes())
		if err != nil {
			return err
		}

		w.outBuf.Reset()
	}

	return nil
}

func (w *muxerFMP4) flush() error {
	return w.innerFlush(true)
}
