package recorder

import (
	"bytes"
	"fmt"
	"time"

	rtspformat "github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/jpeg"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg1audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4video"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/opus"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/vp9"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"

	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/formatprocessor"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/unit"
)

func mpeg1audioChannelCount(cm mpeg1audio.ChannelMode) int {
	switch cm {
	case mpeg1audio.ChannelModeStereo,
		mpeg1audio.ChannelModeJointStereo,
		mpeg1audio.ChannelModeDualChannel:
		return 2

	default:
		return 1
	}
}

func jpegExtractSize(image []byte) (int, int, error) {
	l := len(image)
	if l < 2 || image[0] != 0xFF || image[1] != jpeg.MarkerStartOfImage {
		return 0, 0, fmt.Errorf("invalid header")
	}

	image = image[2:]

	for {
		if len(image) < 2 {
			return 0, 0, fmt.Errorf("not enough bits")
		}

		h0, h1 := image[0], image[1]
		image = image[2:]

		if h0 != 0xFF {
			return 0, 0, fmt.Errorf("invalid image")
		}

		switch h1 {
		case 0xE0, 0xE1, 0xE2, // JFIF
			jpeg.MarkerDefineHuffmanTable,
			jpeg.MarkerComment,
			jpeg.MarkerDefineQuantizationTable,
			jpeg.MarkerDefineRestartInterval:
			mlen := int(image[0])<<8 | int(image[1])
			if len(image) < mlen {
				return 0, 0, fmt.Errorf("not enough bits")
			}
			image = image[mlen:]

		case jpeg.MarkerStartOfFrame1:
			mlen := int(image[0])<<8 | int(image[1])
			if len(image) < mlen {
				return 0, 0, fmt.Errorf("not enough bits")
			}

			var sof jpeg.StartOfFrame1
			err := sof.Unmarshal(image[2:mlen])
			if err != nil {
				return 0, 0, err
			}

			return sof.Width, sof.Height, nil

		case jpeg.MarkerStartOfScan:
			return 0, 0, fmt.Errorf("SOF not found")

		default:
			return 0, 0, fmt.Errorf("unknown marker: 0x%.2x", h1)
		}
	}
}

type formatFMP4 struct {
	ri *recorderInstance

	tracks             []*formatFMP4Track
	hasVideo           bool
	currentSegment     *formatFMP4Segment
	nextSequenceNumber uint32
}

func (f *formatFMP4) initialize() bool {
	nextID := 1
	var setuppedFormats []rtspformat.Format
	setuppedFormatsMap := make(map[rtspformat.Format]struct{})

	addTrack := func(format rtspformat.Format, codec fmp4.Codec) *formatFMP4Track {
		initTrack := &fmp4.InitTrack{
			TimeScale: uint32(format.ClockRate()),
			Codec:     codec,
		}
		initTrack.ID = nextID
		nextID++

		track := &formatFMP4Track{
			f:         f,
			initTrack: initTrack,
		}

		f.tracks = append(f.tracks, track)
		setuppedFormats = append(setuppedFormats, format)
		setuppedFormatsMap[format] = struct{}{}
		return track
	}

	updateCodecs := func() {
		// if codec parameters have been updated,
		// and current segment has already written codec parameters on disk,
		// close current segment.
		if f.currentSegment != nil && f.currentSegment.fi != nil {
			f.currentSegment.close() //nolint:errcheck
			f.currentSegment = nil
		}
	}

	for _, media := range f.ri.rec.Stream.Desc().Medias {
		for _, forma := range media.Formats {
			clockRate := forma.ClockRate()

			switch forma := forma.(type) {
			case *rtspformat.AV1:
				codec := &fmp4.CodecAV1{
					SequenceHeader: formatprocessor.AV1DefaultSequenceHeader,
				}
				track := addTrack(forma, codec)

				firstReceived := false

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.AV1)
						if tunit.TU == nil {
							return nil
						}

						randomAccess := false

						for _, obu := range tunit.TU {
							var h av1.OBUHeader
							err := h.Unmarshal(obu)
							if err != nil {
								return err
							}

							if h.Type == av1.OBUTypeSequenceHeader {
								if !bytes.Equal(codec.SequenceHeader, obu) {
									codec.SequenceHeader = obu
									updateCodecs()
								}
								randomAccess = true
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						}

						sampl, err := fmp4.NewPartSampleAV12(tunit.TU)
						if err != nil {
							return err
						}

						return track.write(&sample{
							PartSample: sampl,
							dts:        tunit.PTS,
							ntp:        tunit.NTP,
						})
					})

			case *rtspformat.VP9:
				codec := &fmp4.CodecVP9{
					Width:             1280,
					Height:            720,
					Profile:           1,
					BitDepth:          8,
					ChromaSubsampling: 1,
					ColorRange:        false,
				}
				track := addTrack(forma, codec)

				firstReceived := false

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.VP9)
						if tunit.Frame == nil {
							return nil
						}

						var h vp9.Header
						err := h.Unmarshal(tunit.Frame)
						if err != nil {
							return err
						}

						randomAccess := false

						if !h.NonKeyFrame {
							randomAccess = true

							if w := h.Width(); codec.Width != w {
								codec.Width = w
								updateCodecs()
							}
							if h := h.Width(); codec.Height != h {
								codec.Height = h
								updateCodecs()
							}
							if codec.Profile != h.Profile {
								codec.Profile = h.Profile
								updateCodecs()
							}
							if codec.BitDepth != h.ColorConfig.BitDepth {
								codec.BitDepth = h.ColorConfig.BitDepth
								updateCodecs()
							}
							if c := h.ChromaSubsampling(); codec.ChromaSubsampling != c {
								codec.ChromaSubsampling = c
								updateCodecs()
							}
							if codec.ColorRange != h.ColorConfig.ColorRange {
								codec.ColorRange = h.ColorConfig.ColorRange
								updateCodecs()
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						}

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								IsNonSyncSample: !randomAccess,
								Payload:         tunit.Frame,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
					})

			case *rtspformat.VP8:
				// TODO

			case *rtspformat.H265:
				vps, sps, pps := forma.SafeParams()

				if vps == nil || sps == nil || pps == nil {
					vps = formatprocessor.H265DefaultVPS
					sps = formatprocessor.H265DefaultSPS
					pps = formatprocessor.H265DefaultPPS
				}

				codec := &fmp4.CodecH265{
					VPS: vps,
					SPS: sps,
					PPS: pps,
				}
				track := addTrack(forma, codec)

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

						randomAccess := false

						for _, nalu := range tunit.AU {
							typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

							switch typ {
							case h265.NALUType_VPS_NUT:
								if !bytes.Equal(codec.VPS, nalu) {
									codec.VPS = nalu
									updateCodecs()
								}

							case h265.NALUType_SPS_NUT:
								if !bytes.Equal(codec.SPS, nalu) {
									codec.SPS = nalu
									updateCodecs()
								}

							case h265.NALUType_PPS_NUT:
								if !bytes.Equal(codec.PPS, nalu) {
									codec.PPS = nalu
									updateCodecs()
								}

							case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
								randomAccess = true
							}
						}

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

						sampl, err := fmp4.NewPartSampleH265(
							int32(tunit.PTS-dts),
							tunit.AU)
						if err != nil {
							return err
						}

						return track.write(&sample{
							PartSample: sampl,
							dts:        dts,
							ntp:        tunit.NTP,
						})
					})

			case *rtspformat.H264:
				sps, pps := forma.SafeParams()

				if sps == nil || pps == nil {
					sps = formatprocessor.H264DefaultSPS
					pps = formatprocessor.H264DefaultPPS
				}

				codec := &fmp4.CodecH264{
					SPS: sps,
					PPS: pps,
				}
				track := addTrack(forma, codec)

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

						randomAccess := false

						for _, nalu := range tunit.AU {
							typ := h264.NALUType(nalu[0] & 0x1F)
							switch typ {
							case h264.NALUTypeSPS:
								if !bytes.Equal(codec.SPS, nalu) {
									codec.SPS = nalu
									updateCodecs()
								}

							case h264.NALUTypePPS:
								if !bytes.Equal(codec.PPS, nalu) {
									codec.PPS = nalu
									updateCodecs()
								}

							case h264.NALUTypeIDR:
								randomAccess = true
							}
						}

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

						sampl, err := fmp4.NewPartSampleH264(
							int32(tunit.PTS-dts),
							tunit.AU)
						if err != nil {
							return err
						}

						return track.write(&sample{
							PartSample: sampl,
							dts:        dts,
							ntp:        tunit.NTP,
						})
					})

			case *rtspformat.MPEG4Video:
				config := forma.SafeParams()

				if config == nil {
					config = formatprocessor.MPEG4VideoDefaultConfig
				}

				codec := &fmp4.CodecMPEG4Video{
					Config: config,
				}
				track := addTrack(forma, codec)

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

						randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})

						if bytes.HasPrefix(tunit.Frame, []byte{0, 0, 1, byte(mpeg4video.VisualObjectSequenceStartCode)}) {
							end := bytes.Index(tunit.Frame[4:], []byte{0, 0, 1, byte(mpeg4video.GroupOfVOPStartCode)})
							if end >= 0 {
								config := tunit.Frame[:end+4]

								if !bytes.Equal(codec.Config, config) {
									codec.Config = config
									updateCodecs()
								}
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						} else if tunit.PTS < lastPTS {
							return fmt.Errorf("MPEG-4 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = tunit.PTS

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								Payload:         tunit.Frame,
								IsNonSyncSample: !randomAccess,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
					})

			case *rtspformat.MPEG1Video:
				codec := &fmp4.CodecMPEG1Video{
					Config: formatprocessor.MPEG1VideoDefaultConfig,
				}
				track := addTrack(forma, codec)

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

						randomAccess := bytes.Contains(tunit.Frame, []byte{0, 0, 1, 0xB8})

						if bytes.HasPrefix(tunit.Frame, []byte{0, 0, 1, 0xB3}) {
							end := bytes.Index(tunit.Frame[4:], []byte{0, 0, 1, 0xB8})
							if end >= 0 {
								config := tunit.Frame[:end+4]

								if !bytes.Equal(codec.Config, config) {
									codec.Config = config
									updateCodecs()
								}
							}
						}

						if !firstReceived {
							if !randomAccess {
								return nil
							}
							firstReceived = true
						} else if tunit.PTS < lastPTS {
							return fmt.Errorf("MPEG-1 Video streams with B-frames are not supported (yet)")
						}
						lastPTS = tunit.PTS

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								Payload:         tunit.Frame,
								IsNonSyncSample: !randomAccess,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
					})

			case *rtspformat.MJPEG:
				codec := &fmp4.CodecMJPEG{
					Width:  800,
					Height: 600,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MJPEG)
						if tunit.Frame == nil {
							return nil
						}

						if !parsed {
							parsed = true
							width, height, err := jpegExtractSize(tunit.Frame)
							if err != nil {
								return err
							}
							codec.Width = width
							codec.Height = height
							updateCodecs()
						}

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								Payload: tunit.Frame,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
					})

			case *rtspformat.Opus:
				codec := &fmp4.CodecOpus{
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.Opus)
						if tunit.Packets == nil {
							return nil
						}

						pts := tunit.PTS

						for _, packet := range tunit.Packets {
							err := track.write(&sample{
								PartSample: &fmp4.PartSample{
									Payload: packet,
								},
								dts: pts,
								ntp: tunit.NTP.Add(timestampToDuration(pts-tunit.PTS, clockRate)),
							})
							if err != nil {
								return err
							}

							pts += int64(opus.PacketDuration(packet)) * int64(clockRate) / int64(time.Second)
						}

						return nil
					})

			case *rtspformat.MPEG4Audio:
				co := forma.GetConfig()
				if co != nil {
					codec := &fmp4.CodecMPEG4Audio{
						Config: *co,
					}
					track := addTrack(forma, codec)

					f.ri.rec.Stream.AddReader(
						f.ri,
						media,
						forma,
						func(u unit.Unit) error {
							tunit := u.(*unit.MPEG4Audio)
							if tunit.AUs == nil {
								return nil
							}

							for i, au := range tunit.AUs {
								pts := tunit.PTS + int64(i)*mpeg4audio.SamplesPerAccessUnit

								err := track.write(&sample{
									PartSample: &fmp4.PartSample{
										Payload: au,
									},
									dts: pts,
									ntp: tunit.NTP.Add(timestampToDuration(pts-tunit.PTS, clockRate)),
								})
								if err != nil {
									return err
								}
							}

							return nil
						})
				}

			case *rtspformat.MPEG1Audio:
				codec := &fmp4.CodecMPEG1Audio{
					SampleRate:   32000,
					ChannelCount: 2,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.MPEG1Audio)
						if tunit.Frames == nil {
							return nil
						}

						var dt time.Duration

						for _, frame := range tunit.Frames {
							var h mpeg1audio.FrameHeader
							err := h.Unmarshal(frame)
							if err != nil {
								return err
							}

							if !parsed {
								parsed = true
								codec.SampleRate = h.SampleRate
								codec.ChannelCount = mpeg1audioChannelCount(h.ChannelMode)
								updateCodecs()
							}

							err = track.write(&sample{
								PartSample: &fmp4.PartSample{
									Payload: frame,
								},
								dts: tunit.PTS + tunit.PTS,
								ntp: tunit.NTP,
							})
							if err != nil {
								return err
							}

							dt += time.Duration(h.SampleCount()) *
								time.Second / time.Duration(h.SampleRate)
						}

						return nil
					})

			case *rtspformat.AC3:
				codec := &fmp4.CodecAC3{
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
					Fscod:        0,
					Bsid:         8,
					Bsmod:        0,
					Acmod:        7,
					LfeOn:        true,
					BitRateCode:  7,
				}
				track := addTrack(forma, codec)

				parsed := false

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.AC3)
						if tunit.Frames == nil {
							return nil
						}

						for i, frame := range tunit.Frames {
							var syncInfo ac3.SyncInfo
							err := syncInfo.Unmarshal(frame)
							if err != nil {
								return fmt.Errorf("invalid AC-3 frame: %w", err)
							}

							var bsi ac3.BSI
							err = bsi.Unmarshal(frame[5:])
							if err != nil {
								return fmt.Errorf("invalid AC-3 frame: %w", err)
							}

							if !parsed {
								parsed = true
								codec.SampleRate = syncInfo.SampleRate()
								codec.ChannelCount = bsi.ChannelCount()
								codec.Fscod = syncInfo.Fscod
								codec.Bsid = bsi.Bsid
								codec.Bsmod = bsi.Bsmod
								codec.Acmod = bsi.Acmod
								codec.LfeOn = bsi.LfeOn
								codec.BitRateCode = syncInfo.Frmsizecod >> 1
								updateCodecs()
							}

							pts := tunit.PTS + int64(i)*ac3.SamplesPerFrame

							err = track.write(&sample{
								PartSample: &fmp4.PartSample{
									Payload: frame,
								},
								dts: pts,
								ntp: tunit.NTP.Add(timestampToDuration(pts-tunit.PTS, clockRate)),
							})
							if err != nil {
								return err
							}
						}

						return nil
					})

			case *rtspformat.G722:
				// TODO

			case *rtspformat.G711:
				codec := &fmp4.CodecLPCM{
					LittleEndian: false,
					BitDepth:     16,
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.G711)
						if tunit.Samples == nil {
							return nil
						}

						var out []byte
						if forma.MULaw {
							out = g711.DecodeMulaw(tunit.Samples)
						} else {
							out = g711.DecodeAlaw(tunit.Samples)
						}

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								Payload: out,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
					})

			case *rtspformat.LPCM:
				codec := &fmp4.CodecLPCM{
					LittleEndian: false,
					BitDepth:     forma.BitDepth,
					SampleRate:   forma.SampleRate,
					ChannelCount: forma.ChannelCount,
				}
				track := addTrack(forma, codec)

				f.ri.rec.Stream.AddReader(
					f.ri,
					media,
					forma,
					func(u unit.Unit) error {
						tunit := u.(*unit.LPCM)
						if tunit.Samples == nil {
							return nil
						}

						return track.write(&sample{
							PartSample: &fmp4.PartSample{
								Payload: tunit.Samples,
							},
							dts: tunit.PTS,
							ntp: tunit.NTP,
						})
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

	f.ri.Log(logger.Info, "recording %s",
		defs.FormatsInfo(setuppedFormats))

	return true
}

func (f *formatFMP4) close() {
	if f.currentSegment != nil {
		for _, track := range f.tracks {
			if track.nextSample != nil &&
				timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale)) > f.currentSegment.lastDTS {
				f.currentSegment.lastDTS = timestampToDuration(track.nextSample.dts, int(track.initTrack.TimeScale))
			}
		}

		f.currentSegment.close() //nolint:errcheck
	}
}
