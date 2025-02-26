package mpegts

import (
	"bufio"
	"fmt"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	mcmpegts "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"

	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// FromStream maps a MediaMTX stream to a MPEG-TS writer.
func FromStream(
	strea *stream.Stream,
	reader stream.Reader,
	bw *bufio.Writer,
	sconn srt.Conn,
	writeTimeout time.Duration,
) error {
	var w *mcmpegts.Writer
	var tracks []*mcmpegts.Track
	setuppedFormats := make(map[format.Format]struct{})

	addTrack := func(
		media *description.Media,
		forma format.Format,
		track *mcmpegts.Track,
		readFunc stream.ReadFunc,
	) {
		tracks = append(tracks, track)
		setuppedFormats[forma] = struct{}{}
		strea.AddReader(reader, media, forma, readFunc)
	}

	for _, media := range strea.Desc().Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *format.H265: //nolint:dupl
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecH265{}}

				var dtsExtractor *h265.DTSExtractor

				addTrack(
					media,
					forma,
					track,
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

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err = (*w).WriteH265(
							track,
							tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							dts,
							tunit.AU)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.H264: //nolint:dupl
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecH264{}}

				var dtsExtractor *h264.DTSExtractor

				addTrack(
					media,
					forma,
					track,
					func(u unit.Unit) error {
						tunit := u.(*unit.H264)
						if tunit.AU == nil {
							return nil
						}

						idrPresent := h264.IsRandomAccess(tunit.AU)

						if dtsExtractor == nil {
							if !idrPresent {
								return nil
							}
							dtsExtractor = h264.NewDTSExtractor()
						}

						dts, err := dtsExtractor.Extract(tunit.AU, tunit.PTS)
						if err != nil {
							return err
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err = (*w).WriteH264(
							track,
							tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							dts,
							tunit.AU)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4Video:
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecMPEG4Video{}}

				firstReceived := false
				var lastPTS int64

				addTrack(
					media,
					forma,
					track,
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

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG4Video(
							track,
							tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							tunit.Frame)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG1Video:
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecMPEG1Video{}}

				firstReceived := false
				var lastPTS int64

				addTrack(
					media,
					forma,
					track,
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

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG1Video(
							track,
							tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							tunit.Frame)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.Opus:
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecOpus{
					ChannelCount: forma.ChannelCount,
				}}

				addTrack(
					media,
					forma,
					track,
					func(u unit.Unit) error {
						tunit := u.(*unit.Opus)
						if tunit.Packets == nil {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteOpus(
							track,
							multiplyAndDivide(tunit.PTS, 90000, int64(clockRate)),
							tunit.Packets)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.MPEG4Audio:
				co := forma.GetConfig()
				if co != nil {
					track := &mcmpegts.Track{Codec: &mcmpegts.CodecMPEG4Audio{
						Config: *co,
					}}

					addTrack(
						media,
						forma,
						track,
						func(u unit.Unit) error {
							tunit := u.(*unit.MPEG4Audio)
							if tunit.AUs == nil {
								return nil
							}

							sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteMPEG4Audio(
								track,
								multiplyAndDivide(tunit.PTS, 90000, int64(clockRate)),
								tunit.AUs)
							if err != nil {
								return err
							}
							return bw.Flush()
						})
				}

			case *format.MPEG1Audio:
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecMPEG1Audio{}}

				addTrack(
					media,
					forma,
					track,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG1Audio)
						if tunit.Frames == nil {
							return nil
						}

						sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
						err := (*w).WriteMPEG1Audio(
							track,
							tunit.PTS, // no conversion is needed since clock rate is 90khz in both MPEG-TS and RTSP
							tunit.Frames)
						if err != nil {
							return err
						}
						return bw.Flush()
					})

			case *format.AC3:
				track := &mcmpegts.Track{Codec: &mcmpegts.CodecAC3{}}

				addTrack(
					media,
					forma,
					track,
					func(u unit.Unit) error {
						tunit := u.(*unit.AC3)
						if tunit.Frames == nil {
							return nil
						}

						for i, frame := range tunit.Frames {
							framePTS := tunit.PTS + int64(i)*ac3.SamplesPerFrame

							sconn.SetWriteDeadline(time.Now().Add(writeTimeout))
							err := (*w).WriteAC3(
								track,
								multiplyAndDivide(framePTS, 90000, int64(clockRate)),
								frame)
							if err != nil {
								return err
							}
						}
						return bw.Flush()
					})
			}
		}
	}

	if len(tracks) == 0 {
		return errNoSupportedCodecs
	}

	n := 1
	for _, medi := range strea.Desc().Medias {
		for _, forma := range medi.Formats {
			if _, ok := setuppedFormats[forma]; !ok {
				reader.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	w = mcmpegts.NewWriter(bw, tracks)

	return nil
}
