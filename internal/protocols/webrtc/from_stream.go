package webrtc

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpav1"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph265"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtplpcm"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp8"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpvp9"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/webrtc/v4"
)

const (
	webrtcPayloadMaxSize = 1188 // 1200 - 12 (RTP header)
)

var errNoSupportedCodecsFrom = errors.New(
	"the stream doesn't contain any supported codec, which are currently " +
		"AV1, VP9, VP8, H265, H264, Opus, G722, G711, LPCM")

func uint16Ptr(v uint16) *uint16 {
	return &v
}

func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

func setupVideoTrack(
	stream *stream.Stream,
	reader stream.Reader,
	pc *PeerConnection,
) (format.Format, error) {
	var av1Format *format.AV1
	media := stream.Desc().FindFormat(&av1Format)

	if av1Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpav1.Encoder{
			PayloadType:    105,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		stream.AddReader(
			reader,
			media,
			av1Format,
			func(u unit.Unit) error {
				tunit := u.(*unit.AV1)

				if tunit.TU == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.TU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return av1Format, nil
	}

	var vp9Format *format.VP9
	media = stream.Desc().FindFormat(&vp9Format)

	if vp9Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeVP9,
				ClockRate:   90000,
				SDPFmtpLine: "profile-id=0",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpvp9.Encoder{
			PayloadType:      96,
			PayloadMaxSize:   webrtcPayloadMaxSize,
			InitialPictureID: uint16Ptr(8445),
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		stream.AddReader(
			reader,
			media,
			vp9Format,
			func(u unit.Unit) error {
				tunit := u.(*unit.VP9)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return vp9Format, nil
	}

	var vp8Format *format.VP8
	media = stream.Desc().FindFormat(&vp8Format)

	if vp8Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtpvp8.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		stream.AddReader(
			reader,
			media,
			vp8Format,
			func(u unit.Unit) error {
				tunit := u.(*unit.VP8)

				if tunit.Frame == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Frame)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return vp8Format, nil
	}

	var h265Format *format.H265
	media = stream.Desc().FindFormat(&h265Format)

	if h265Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH265,
				ClockRate:   90000,
				SDPFmtpLine: "level-id=93;profile-id=1;tier-flag=0;tx-mode=SRST",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtph265.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		firstReceived := false
		var lastPTS int64

		stream.AddReader(
			reader,
			media,
			h265Format,
			func(u unit.Unit) error {
				tunit := u.(*unit.H265)

				if tunit.AU == nil {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if tunit.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H265 streams with B-frames")
				}
				lastPTS = tunit.PTS

				packets, err := encoder.Encode(tunit.AU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return h265Format, nil
	}

	var h264Format *format.H264
	media = stream.Desc().FindFormat(&h264Format)

	if h264Format != nil { //nolint:dupl
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtph264.Encoder{
			PayloadType:    96,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		firstReceived := false
		var lastPTS int64

		stream.AddReader(
			reader,
			media,
			h264Format,
			func(u unit.Unit) error {
				tunit := u.(*unit.H264)

				if tunit.AU == nil {
					return nil
				}

				if !firstReceived {
					firstReceived = true
				} else if tunit.PTS < lastPTS {
					return fmt.Errorf("WebRTC doesn't support H264 streams with B-frames")
				}
				lastPTS = tunit.PTS

				packets, err := encoder.Encode(tunit.AU)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					pkt.Timestamp += tunit.RTPPackets[0].Timestamp
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return h264Format, nil
	}

	return nil, nil
}

func setupAudioTrack(
	stream *stream.Stream,
	reader stream.Reader,
	pc *PeerConnection,
) (format.Format, error) {
	var opusFormat *format.Opus
	media := stream.Desc().FindFormat(&opusFormat)

	if opusFormat != nil {
		var caps webrtc.RTPCodecCapability

		switch opusFormat.ChannelCount {
		case 1, 2:
			caps = webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
				SDPFmtpLine: func() string {
					s := "minptime=10;useinbandfec=1"
					if opusFormat.ChannelCount == 2 {
						s += ";stereo=1;sprop-stereo=1"
					}
					return s
				}(),
			}

		case 3, 4, 5, 6, 7, 8:
			caps = webrtc.RTPCodecCapability{
				MimeType:    mimeTypeMultiopus,
				ClockRate:   48000,
				Channels:    uint16(opusFormat.ChannelCount),
				SDPFmtpLine: multichannelOpusSDP[opusFormat.ChannelCount],
			}

		default:
			return nil, fmt.Errorf("unsupported channel count: %d", opusFormat.ChannelCount)
		}

		track := &OutgoingTrack{
			Caps: caps,
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		stream.AddReader(
			reader,
			media,
			opusFormat,
			func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return opusFormat, nil
	}

	var g722Format *format.G722
	media = stream.Desc().FindFormat(&g722Format)

	if g722Format != nil {
		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeG722,
				ClockRate: 8000,
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		stream.AddReader(
			reader,
			media,
			g722Format,
			func(u unit.Unit) error {
				for _, pkt := range u.GetRTPPackets() {
					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return g722Format, nil
	}

	var g711Format *format.G711
	media = stream.Desc().FindFormat(&g711Format)

	if g711Format != nil {
		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if g711Format.ClockRate() != 8000 && g711Format.ClockRate() != 16000 &&
			g711Format.ClockRate() != 32000 && g711Format.ClockRate() != 48000 {
			return nil, fmt.Errorf("unsupported clock rate: %d", g711Format.ClockRate())
		}
		if g711Format.ChannelCount != 1 && g711Format.ChannelCount != 2 {
			return nil, fmt.Errorf("unsupported channel count: %d", g711Format.ChannelCount)
		}

		var caps webrtc.RTPCodecCapability

		if g711Format.SampleRate == 8000 {
			if g711Format.MULaw {
				if g711Format.ChannelCount != 1 {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMU,
						ClockRate: uint32(g711Format.SampleRate),
						Channels:  uint16(g711Format.ChannelCount),
					}
				} else {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMU,
						ClockRate: 8000,
					}
				}
			} else {
				if g711Format.ChannelCount != 1 {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMA,
						ClockRate: uint32(g711Format.SampleRate),
						Channels:  uint16(g711Format.ChannelCount),
					}
				} else {
					caps = webrtc.RTPCodecCapability{
						MimeType:  webrtc.MimeTypePCMA,
						ClockRate: 8000,
					}
				}
			}
		} else {
			caps = webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(g711Format.ClockRate()),
				Channels:  uint16(g711Format.ChannelCount),
			}
		}

		track := &OutgoingTrack{
			Caps: caps,
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		if g711Format.SampleRate == 8000 {
			curTimestamp, err := randUint32()
			if err != nil {
				return nil, err
			}

			stream.AddReader(
				reader,
				media,
				g711Format,
				func(u unit.Unit) error {
					for _, pkt := range u.GetRTPPackets() {
						// recompute timestamp from scratch.
						// Chrome requires a precise timestamp that FFmpeg doesn't provide.
						pkt.Timestamp = curTimestamp
						curTimestamp += uint32(len(pkt.Payload)) / uint32(g711Format.ChannelCount)

						track.WriteRTP(pkt) //nolint:errcheck
					}

					return nil
				})
		} else {
			encoder := &rtplpcm.Encoder{
				PayloadType:    96,
				PayloadMaxSize: webrtcPayloadMaxSize,
				BitDepth:       16,
				ChannelCount:   g711Format.ChannelCount,
			}
			err := encoder.Init()
			if err != nil {
				return nil, err
			}

			curTimestamp, err := randUint32()
			if err != nil {
				return nil, err
			}

			stream.AddReader(
				reader,
				media,
				g711Format,
				func(u unit.Unit) error {
					tunit := u.(*unit.G711)

					if tunit.Samples == nil {
						return nil
					}

					var lpcmSamples []byte
					if g711Format.MULaw {
						lpcmSamples = g711.DecodeMulaw(tunit.Samples)
					} else {
						lpcmSamples = g711.DecodeAlaw(tunit.Samples)
					}

					packets, err := encoder.Encode(lpcmSamples)
					if err != nil {
						return nil //nolint:nilerr
					}

					for _, pkt := range packets {
						// recompute timestamp from scratch.
						// Chrome requires a precise timestamp that FFmpeg doesn't provide.
						pkt.Timestamp = curTimestamp
						curTimestamp += uint32(len(pkt.Payload)) / 2 / uint32(g711Format.ChannelCount)

						track.WriteRTP(pkt) //nolint:errcheck
					}

					return nil
				})
		}

		return g711Format, nil
	}

	var lpcmFormat *format.LPCM
	media = stream.Desc().FindFormat(&lpcmFormat)

	if lpcmFormat != nil {
		if lpcmFormat.BitDepth != 16 {
			return nil, fmt.Errorf("unsupported LPCM bit depth: %d", lpcmFormat.BitDepth)
		}

		// These are the sample rates and channels supported by Chrome.
		// Different sample rates and channels can be streamed too but we don't want compatibility issues.
		// https://webrtc.googlesource.com/src/+/refs/heads/main/modules/audio_coding/codecs/pcm16b/audio_decoder_pcm16b.cc#23
		if lpcmFormat.ClockRate() != 8000 && lpcmFormat.ClockRate() != 16000 &&
			lpcmFormat.ClockRate() != 32000 && lpcmFormat.ClockRate() != 48000 {
			return nil, fmt.Errorf("unsupported clock rate: %d", lpcmFormat.ClockRate())
		}
		if lpcmFormat.ChannelCount != 1 && lpcmFormat.ChannelCount != 2 {
			return nil, fmt.Errorf("unsupported channel count: %d", lpcmFormat.ChannelCount)
		}

		track := &OutgoingTrack{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  mimeTypeL16,
				ClockRate: uint32(lpcmFormat.ClockRate()),
				Channels:  uint16(lpcmFormat.ChannelCount),
			},
		}
		pc.OutgoingTracks = append(pc.OutgoingTracks, track)

		encoder := &rtplpcm.Encoder{
			PayloadType:    96,
			BitDepth:       16,
			ChannelCount:   lpcmFormat.ChannelCount,
			PayloadMaxSize: webrtcPayloadMaxSize,
		}
		err := encoder.Init()
		if err != nil {
			return nil, err
		}

		curTimestamp, err := randUint32()
		if err != nil {
			return nil, err
		}

		stream.AddReader(
			reader,
			media,
			lpcmFormat,
			func(u unit.Unit) error {
				tunit := u.(*unit.LPCM)

				if tunit.Samples == nil {
					return nil
				}

				packets, err := encoder.Encode(tunit.Samples)
				if err != nil {
					return nil //nolint:nilerr
				}

				for _, pkt := range packets {
					// recompute timestamp from scratch.
					// Chrome requires a precise timestamp that FFmpeg doesn't provide.
					pkt.Timestamp = curTimestamp
					curTimestamp += uint32(len(pkt.Payload)) / 2 / uint32(lpcmFormat.ChannelCount)

					track.WriteRTP(pkt) //nolint:errcheck
				}

				return nil
			})

		return lpcmFormat, nil
	}

	return nil, nil
}

// FromStream maps a MediaMTX stream to a WebRTC connection
func FromStream(
	stream *stream.Stream,
	reader stream.Reader,
	pc *PeerConnection,
) error {
	videoFormat, err := setupVideoTrack(stream, reader, pc)
	if err != nil {
		return err
	}

	audioFormat, err := setupAudioTrack(stream, reader, pc)
	if err != nil {
		return err
	}

	if videoFormat == nil && audioFormat == nil {
		return errNoSupportedCodecsFrom
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
