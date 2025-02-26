package recorder

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

const (
	mpegtsMaxBufferSize = 64 * 1024
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(t), time.Second, time.Duration(clockRate))
}

type dynamicWriter struct {
	w io.Writer
}

func (d *dynamicWriter) Write(p []byte) (int, error) {
	return d.w.Write(p)
}

func (d *dynamicWriter) setTarget(w io.Writer) {
	d.w = w
}

type formatMPEGTS struct {
	ri *recorderInstance

	dw             *dynamicWriter
	bw             *bufio.Writer
	mw             *mpegts.Writer
	hasVideo       bool
	currentSegment *formatMPEGTSSegment
}

func (f *formatMPEGTS) initialize() bool {
	var tracks []*mpegts.Track
	var setuppedFormats []rtspformat.Format
	setuppedFormatsMap := make(map[rtspformat.Format]struct{})

	addTrack := func(format rtspformat.Format, codec mpegts.Codec) *mpegts.Track {
		track := &mpegts.Track{
			Codec: codec,
		}

		tracks = append(tracks, track)
		setuppedFormats = append(setuppedFormats, format)
		setuppedFormatsMap[format] = struct{}{}
		return track
	}

	for _, media := range f.ri.rec.Stream.Desc().Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *rtspformat.H265: //nolint:dupl
				track := addTrack(forma, &mpegts.CodecH265{})

				var dtsExtractor *h265.DTSExtractor

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.H265)
						if tunit.AU == nil {
							return nil
						}

						randomAccess := h265.IsRandomAccess(tunit.AU)

						if dtsExtractor == nil {
							if !randomAccess {
								return nil
							}
							dtsExtractor = h265.NewDTSExtractor()
						}

						dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
						if err != nil {
							return err
						}

						return f.write(
							timestampToDuration(dts, clockRate),
							tunit.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteH265(
									track,
									tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									dts,
									tunit.AU)
							},
						)
					})

			case *rtspformat.H264: //nolint:dupl
				track := addTrack(forma, &mpegts.CodecH264{})

				var dtsExtractor *h264.DTSExtractor

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.H264)
						if tunit.AU == nil {
							return nil
						}

						randomAccess := h264.IsRandomAccess(tunit.AU)

						if dtsExtractor == nil {
							if !randomAccess {
								return nil
							}
							dtsExtractor = h264.NewDTSExtractor()
						}

						dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
						if err != nil {
							return err
						}

						return f.write(
							timestampToDuration(dts, clockRate),
							tunit.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteH264(
									track,
									tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									dts,
									tunit.AU)
							},
						)
					})

			case *rtspformat.MPEG4Video:
				track := addTrack(forma, &mpegts.CodecMPEG4Video{})

				firstReceived := false
				var lastPTS int64

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG4Video)
						if tunit.Frame == nil {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if tunit.PTS < lastPTS {
							return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = tunit.PTS

						randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

						return f.write(
							timestampToDuration(tunit.PTS, clockRate),
							tunit.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteMPEG4Video(
									track,
									tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									tunit.Frame)
							},
						)
					})

			case *rtspformat.MPEG1Video:
				track := addTrack(forma, &mpegts.CodecMPEG1Video{})

				firstReceived := false
				var lastPTS int64

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG1Video)
						if tunit.Frame == nil {
							return nil
						}

						if !firstReceived {
							firstReceived = true
						} else if tunit.PTS < lastPTS {
							return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = tunit.PTS

						randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, 0xB8})

						return f.write(
							timestampToDuration(tunit.PTS, clockRate),
							tunit.NTP,
							true,
							randomAccess,
							func() error {
								return f.mw.WriteMPEG1Video(
									track,
									tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									tunit.Frame)
							},
						)
					})

			case *rtspformat.Opus:
				track := addTrack(forma, &mpegts.CodecOpus{
					ChannelCount: forma.ChannelCount,
				})

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.Opus)
						if tunit.Packets == nil {
							return nil
						}

						return f.write(
							timestampToDuration(tunit.PTS, clockRate),
							tunit.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteOpus(
									track,
									multiplyAndDivide(tunit.PTS, 90000, int64(clockRate)),
									tunit.Packets)
							},
						)
					})

			case *rtspformat.MPEG4Audio:
				co := forma.GetConfig()
				if co == nil {
					f.ri.Log(logger.Warn, "skipping MPEG-4 audio track: tracks without explicit configuration are not supported")
				} else {
					track := addTrack(forma, &mpegts.CodecMPEG4Audio{
						Config: *co,
					})

					f.ri.rec.Stream.AddReader(
						f.ri,
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.MPEG4Audio)
							if tunit.AUs == nil {
								return nil
							}

							return f.write(
								timestampToDuration(tunit.PTS, clockRate),
								tunit.NTP,
								false,
								true,
								func() error {
									return f.mw.WriteMPEG4Audio(
										track,
										multiplyAndDivide(tunit.PTS, 90000, int64(clockRate)),
										tunit.AUs)
								},
							)
						})
				}

			case *rtspformat.MPEG1Audio:
				track := addTrack(forma, &mpegts.CodecMPEG1Audio{})

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG1Audio)
						if tunit.Frames == nil {
							return nil
						}

						return f.write(
							timestampToDuration(tunit.PTS, clockRate),
							tunit.NTP,
							false,
							true,
							func() error {
								return f.mw.WriteMPEG1Audio(
									track,
									tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
									tunit.Frames)
							},
						)
					})

			case *rtspformat.AC3:
				track := addTrack(forma, &mpegts.CodecAC3{})

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.AC3)
						if tunit.Frames == nil {
							return nil
						}

						return f.write(
							timestampToDuration(tunit.PTS, clockRate),
							tunit.NTP,
							false,
							true,
							func() error {
								for i, frame := range tunit.Frames {
									framePTS := tunit.PTS + int64(i)*ac3.SamplesPerFrame

									err := f.mw.WriteAC3(
										track,
										multiplyAndDivide(framePTS, 90000, int64(clockRate)),
										frame)
									if err != nil {
										return err
									}
								}

								return nil
							},
						)
					})
			}
		}
	}

	if len(setuppedFormats) == 0 {
		f.ri.Log(logger.Warn, "no supported tracks found, skipping recording")
		return false
	}

	n := 1
	for _, medi := range f.ri.rec.Stream.Desc().Medias {
		for _, forma := range medi.Formats {
			if _, ok := setuppedFormatsMap[forma]; !ok {
				f.ri.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	f.dw = &dynamicWriter{}
	f.bw = bufio.NewWriterSize(f.dw, mpegtsMaxBufferSize)
	f.mw = mpegts.NewWriter(f.bw, tracks)

	f.ri.Log(logger.Info, "recording %s",
		defs.FormatsInfo(setuppedFormats))

	return true
}

func (f *formatMPEGTS) close() {
	if f.currentSegment != nil {
		f.currentSegment.close() //nolint:errcheck
	}
}

func (f *formatMPEGTS) write(
	dtsDuration time.Duration,
	ntp time.Time,
	isVideo bool,
	randomAccess bool,
	writeCB func() error,
) error {
	if isVideo {
		f.hasVideo = true
	}

	switch {
	case f.currentSegment == nil:
		f.currentSegment = &formatMPEGTSSegment{
			f:        f,
			startDTS: dtsDuration,
			startNTP: ntp,
		}
		f.currentSegment.initialize()
	case (!f.hasVideo || isVideo) &&
		randomAccess &&
		(dtsDuration-f.currentSegment.startDTS) >= f.ri.rec.SegmentDuration:
		f.currentSegment.lastDTS = dtsDuration
		err := f.currentSegment.close()
		if err != nil {
			return err
		}

		f.currentSegment = &formatMPEGTSSegment{
			f:        f,
			startDTS: dtsDuration,
			startNTP: ntp,
		}
		f.currentSegment.initialize()

	case (dtsDuration - f.currentSegment.lastFlush) >= f.ri.rec.PartDuration:
		err := f.bw.Flush()
		if err != nil {
			return err
		}

		f.currentSegment.lastFlush = dtsDuration
	}

	f.currentSegment.lastDTS = dtsDuration

	return writeCB()
}
