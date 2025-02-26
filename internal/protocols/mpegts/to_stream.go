// Package mpegts contains MPEG-ts utilities.
package mpegts

import (
	"errors"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecs = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"H265, H264, MPEG-4 Video, MPEG-1/2 Video, Opus, MPEG-4 Audio, MPEG-1 Audio, AC-3")

// ToStream maps a MPEG-TS stream to a MediaMTX stream.
func ToStream(
	r *mpegts.Reader,
	stream **stream.Stream,
	l logger.Writer,
) ([]*description.Media, error) {
	var medias []*description.Media //nolint:prealloc
	var unsupportedTracks []int

	td := mpegts.NewTimeDecoder()

	for i, track := range r.Tracks() { //nolint:dupl
		var medi *description.Media

		switch codec := track.Codec.(type) {
		case *mpegts.CodecH265:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H265{
					PayloadTyp: 96,
				}},
			}

			r.OnDataH265(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H265{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecH264:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.H264{
					PayloadTyp:        96,
					PacketizationMode: 1,
				}},
			}

			r.OnDataH264(track, func(pts int64, _ int64, au [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					AU: au,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Video:
			medi = &description.Media{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG4Video{
					PayloadTyp: 96,
				}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Video:
			medi = &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{&format.MPEG1Video{}},
			}

			r.OnDataMPEGxVideo(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Video{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frame: frame,
				})
				return nil
			})

		case *mpegts.CodecOpus:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataOpus(track, func(pts int64, packets [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.Opus{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					Packets: packets,
				})
				return nil
			})

		case *mpegts.CodecMPEG4Audio:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG4Audio{
					PayloadTyp:       96,
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
					Config:           &codec.Config,
				}},
			}

			r.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG4Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					AUs: aus,
				})
				return nil
			})

		case *mpegts.CodecMPEG1Audio:
			medi = &description.Media{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.MPEG1Audio{}},
			}

			r.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.MPEG1Audio{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: pts, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
					},
					Frames: frames,
				})
				return nil
			})

		case *mpegts.CodecAC3:
			medi = &description.Media{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.AC3{
					PayloadTyp:   96,
					SampleRate:   codec.SampleRate,
					ChannelCount: codec.ChannelCount,
				}},
			}

			r.OnDataAC3(track, func(pts int64, frame []byte) error {
				pts = td.Decode(pts)

				(*stream).WriteUnit(medi, medi.Formats[0], &unit.AC3{
					Base: unit.Base{
						NTP: time.Now(),
						PTS: multiplyAndDivide(pts, int64(medi.Formats[0].ClockRate()), 90000),
					},
					Frames: [][]byte{frame},
				})
				return nil
			})

		default:
			unsupportedTracks = append(unsupportedTracks, i+1)
			continue
		}

		medias = append(medias, medi)
	}

	if len(medias) == 0 {
		return nil, errNoSupportedCodecs
	}

	for _, id := range unsupportedTracks {
		l.Log(logger.Warn, "skipping track %d (unsupported codec)", id)
	}

	return medias, nil
}
