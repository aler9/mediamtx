package rtmp

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
)

var errNoSupportedCodecsFrom = errors.New(
	"the stream doesn't contain any supported codec, which are currently H264, MPEG-4 Audio, MPEG-1/2 Audio")

func multiplyAndDivide2(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func timestampToDuration(t int64, clockRate int) time.Duration {
	return multiplyAndDivide2(time.Duration(t), time.Second, time.Duration(clockRate))
}

func setupVideo(
	strea *stream.Stream,
	reader stream.Reader,
	w **Writer,
	nconn net.Conn,
	writeTimeout time.Duration,
) format.Format {
	var videoFormatH264 *format.H264
	videoMedia := strea.Desc().FindFormat(&videoFormatH264)

	if videoFormatH264 != nil {
		var videoDTSExtractor *h264.DTSExtractor

		strea.AddReader(
			reader,
			videoMedia,
			videoFormatH264,
			func(u unit.Unit) error {
				tunit := u.(*unit.H264)

				if tunit.AU == nil {
					return nil
				}

				idrPresent := false
				nonIDRPresent := false

				for _, nalu := range tunit.AU {
					typ := h264.NALUType(nalu[0] & 0x1F)
					switch typ {
					case h264.NALUTypeIDR:
						idrPresent = true

					case h264.NALUTypeNonIDR:
						nonIDRPresent = true
					}
				}

				// wait until we receive an IDR
				if videoDTSExtractor == nil {
					if !idrPresent {
						return nil
					}

					videoDTSExtractor = h264.NewDTSExtractor()
				} else if !idrPresent && !nonIDRPresent {
					return nil
				}

				dts, err := videoDTSExtractor.Extract(tunit.AU, tunit.PTS)
				if err != nil {
					return err
				}

				nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
				return (*w).WriteH264(
					timestampToDuration(tunit.PTS, videoFormatH264.ClockRate()),
					timestampToDuration(dts, videoFormatH264.ClockRate()),
					tunit.AU)
			})

		return videoFormatH264
	}

	return nil
}

func setupAudio(
	strea *stream.Stream,
	reader stream.Reader,
	w **Writer,
	nconn net.Conn,
	writeTimeout time.Duration,
) format.Format {
	var audioFormatMPEG4Audio *format.MPEG4Audio
	audioMedia := strea.Desc().FindFormat(&audioFormatMPEG4Audio)

	if audioMedia != nil {
		strea.AddReader(
			reader,
			audioMedia,
			audioFormatMPEG4Audio,
			func(u unit.Unit) error {
				tunit := u.(*unit.MPEG4Audio)

				if tunit.AUs == nil {
					return nil
				}

				for i, au := range tunit.AUs {
					pts := tunit.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

					nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err := (*w).WriteMPEG4Audio(
						timestampToDuration(pts, audioFormatMPEG4Audio.ClockRate()),
						au,
					)
					if err != nil {
						return err
					}
				}

				return nil
			})

		return audioFormatMPEG4Audio
	}

	var audioFormatMPEG1 *format.MPEG1Audio
	audioMedia = strea.Desc().FindFormat(&audioFormatMPEG1)

	if audioMedia != nil {
		strea.AddReader(
			reader,
			audioMedia,
			audioFormatMPEG1,
			func(u unit.Unit) error {
				tunit := u.(*unit.MPEG1Audio)

				pts := tunit.PTS

				for _, frame := range tunit.Frames {
					var h mpeg1audio.FrameHeader
					err := h.Unmarshal(frame)
					if err != nil {
						return err
					}

					if !(!h.MPEG2 && h.Layer == 3) {
						return fmt.Errorf("RTMP only supports MPEG-1 layer 3 audio")
					}

					nconn.SetWriteDeadline(time.Now().Add(writeTimeout))
					err = (*w).WriteMPEG1Audio(
						timestampToDuration(pts, audioFormatMPEG1.ClockRate()),
						&h,
						frame)
					if err != nil {
						return err
					}

					pts += int64(h.SampleCount()) *
						int64(audioFormatMPEG1.ClockRate()) / int64(h.SampleRate)
				}

				return nil
			})

		return audioFormatMPEG1
	}

	return nil
}

// FromStream maps a MediaMTX stream to a RTMP stream.
func FromStream(
	stream *stream.Stream,
	reader stream.Reader,
	conn *Conn,
	nconn net.Conn,
	writeTimeout time.Duration,
) error {
	var w *Writer

	videoFormat := setupVideo(
		stream,
		reader,
		&w,
		nconn,
		writeTimeout,
	)

	audioFormat := setupAudio(
		stream,
		reader,
		&w,
		nconn,
		writeTimeout,
	)

	if videoFormat == nil && audioFormat == nil {
		return errNoSupportedCodecsFrom
	}

	var err error
	w, err = NewWriter(conn, videoFormat, audioFormat)
	if err != nil {
		return err
	}

	n := 1
	for _, media := range stream.Desc().Medias {
		for _, forma := range media.Formats {
			if forma != videoFormat && forma != audioFormat {
				reader.Log(logger.Warn, "skipping track %d (%s)", n, forma.Codec())
			}
			n++
		}
	}

	return nil
}
