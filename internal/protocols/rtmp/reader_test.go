package rtmp

import (
	"bytes"
	"testing"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/amf0"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/bytecounter"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
	"github.com/bluenviron/mediamtx/internal/test"
)

func TestReadTracks(t *testing.T) {
	var spsp h265.SPS
	err := spsp.Unmarshal(test.FormatH265.SPS)
	require.NoError(t, err)

	hvcc := &mp4.HvcC{
		ConfigurationVersion:        1,
		GeneralProfileIdc:           spsp.ProfileTierLevel.GeneralProfileIdc,
		GeneralProfileCompatibility: spsp.ProfileTierLevel.GeneralProfileCompatibilityFlag,
		GeneralConstraintIndicator: [6]uint8{
			test.FormatH265.SPS[7], test.FormatH265.SPS[8], test.FormatH265.SPS[9],
			test.FormatH265.SPS[10], test.FormatH265.SPS[11], test.FormatH265.SPS[12],
		},
		GeneralLevelIdc: spsp.ProfileTierLevel.GeneralLevelIdc,
		// MinSpatialSegmentationIdc
		// ParallelismType
		ChromaFormatIdc:      uint8(spsp.ChromaFormatIdc),
		BitDepthLumaMinus8:   uint8(spsp.BitDepthLumaMinus8),
		BitDepthChromaMinus8: uint8(spsp.BitDepthChromaMinus8),
		// AvgFrameRate
		// ConstantFrameRate
		NumTemporalLayers: 1,
		// TemporalIdNested
		LengthSizeMinusOne: 3,
		NumOfNaluArrays:    3,
		NaluArrays: []mp4.HEVCNaluArray{
			{
				NaluType: byte(h265.NALUType_VPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.VPS)),
					NALUnit: test.FormatH265.VPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_SPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.SPS)),
					NALUnit: test.FormatH265.SPS,
				}},
			},
			{
				NaluType: byte(h265.NALUType_PPS_NUT),
				NumNalus: 1,
				Nalus: []mp4.HEVCNalu{{
					Length:  uint16(len(test.FormatH265.PPS)),
					NALUnit: test.FormatH265.PPS,
				}},
			},
		},
	}

	for _, ca := range []struct {
		name     string
		tracks   []format.Format
		messages []message.Message
	}{
		{
			"h264 + aac",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(message.CodecMPEG4Audio),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"h264",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
				},
			},
		},
		{
			"issue mediamtx/386 (missing metadata)",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"issue mediamtx/3301 (metadata without tracks)",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "metadatacreator",
								Value: "Agora.io SDK",
							},
							{
								Key:   "encoder",
								Value: "Agora.io Encoder",
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
			},
		},
		{
			"issue mediamtx/386 (missing metadata)",
			[]format.Format{
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeAU,
				},
			},
		},
		{
			"issue mediamtx/3414 (empty audio payload)",
			[]format.Format{
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   44100,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(0),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(message.CodecMPEG4Audio),
							},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload:         nil,
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload: func() []byte {
						enc, err2 := mpeg4audio.Config{
							Type:         2,
							SampleRate:   44100,
							ChannelCount: 2,
						}.Marshal()
						require.NoError(t, err2)
						return enc
					}(),
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG4Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					AACType:         message.AudioAACTypeAU,
				},
			},
		},
		{
			"issue mediamtx/2232 (xsplit broadcaster)",
			[]format.Format{
				&format.H265{
					PayloadTyp: 96,
					VPS:        test.FormatH265.VPS,
					SPS:        test.FormatH265.SPS,
					PPS:        test.FormatH265.PPS,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: "hvc1",
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.VideoExSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					HEVCHeader:      hvcc,
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					DTS:             2 * time.Second,
				},
			},
		},
		{
			"h265, obs 30.0",
			[]format.Format{
				&format.H265{
					PayloadTyp: 96,
					VPS:        test.FormatH265.VPS,
					SPS:        test.FormatH265.SPS,
					PPS:        test.FormatH265.PPS,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.FourCCHEVC),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.VideoExSequenceStart{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					HEVCHeader:      hvcc,
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCHEVC,
					DTS:             2 * time.Second,
				},
			},
		},
		{
			"av1, ffmpeg",
			[]format.Format{
				&format.AV1{
					PayloadTyp: 96,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "duration",
								Value: float64(0),
							},
							{
								Key:   "width",
								Value: float64(1920),
							},
							{
								Key:   "height",
								Value: float64(1080),
							},
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.FourCCAV1),
							},
							{
								Key:   "encoder",
								Value: "Lavf60.10.101",
							},
							{
								Key:   "filesize",
								Value: float64(0),
							},
						},
					},
				},
				&message.VideoExSequenceStart{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCAV1,
					AV1Header: &mp4.Av1C{
						Marker:             0x1,
						Version:            0x1,
						SeqLevelIdx0:       0x8,
						ChromaSubsamplingX: 0x1,
						ChromaSubsamplingY: 0x1,
						ConfigOBUs:         []uint8{0xa, 0xb, 0x0, 0x0, 0x0, 0x42, 0xab, 0xbf, 0xc3, 0x70, 0xb, 0xe0, 0x1},
					},
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCAV1,
				},
				&message.VideoExCodedFrames{
					ChunkStreamID:   6,
					MessageStreamID: 0x1000000,
					FourCC:          message.FourCCAV1,
					DTS:             2 * time.Second,
				},
			},
		},
		{
			"issue mediamtx/2289 (missing videocodecid)",
			[]format.Format{
				&format.H264{
					PayloadTyp: 96,
					SPS: []byte{
						0x67, 0x64, 0x00, 0x1f, 0xac, 0x2c, 0x6a, 0x81,
						0x40, 0x16, 0xe9, 0xb8, 0x28, 0x08, 0x2a, 0x00,
						0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00,
						0xc9, 0x08,
					},
					PPS:               []byte{0x68, 0xee, 0x31, 0xb2, 0x1b},
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.Config{
						Type:         2,
						SampleRate:   48000,
						ChannelCount: 1,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "width",
								Value: float64(1280),
							},
							{
								Key:   "height",
								Value: float64(720),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "audiocodecid",
								Value: float64(10),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   0x15,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Payload: []uint8{
						0x01, 0x64, 0x00, 0x1f, 0xff, 0xe1, 0x00, 0x1a,
						0x67, 0x64, 0x00, 0x1f, 0xac, 0x2c, 0x6a, 0x81,
						0x40, 0x16, 0xe9, 0xb8, 0x28, 0x08, 0x2a, 0x00,
						0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00,
						0xc9, 0x08, 0x01, 0x00, 0x05, 0x68, 0xee, 0x31,
						0xb2, 0x1b,
					},
				},
				&message.Audio{
					ChunkStreamID:   0x14,
					MessageStreamID: 0x1000000,
					Codec:           0xa,
					Rate:            0x3,
					Depth:           0x1,
					IsStereo:        true,
					Payload:         []uint8{0x11, 0x88},
				},
				&message.Audio{
					ChunkStreamID:   0x14,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           0xa,
					Rate:            0x3,
					Depth:           0x1,
					IsStereo:        true,
					AACType:         message.AudioAACTypeAU,
					Payload:         []uint8{0x11, 0x88},
				},
			},
		},
		{
			"issue mediamtx/2352 (streamlabs)",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   8,
					MessageStreamID: 0x1000000,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "audiodatarate",
								Value: float64(128),
							},
							{
								Key:   "framerate",
								Value: float64(30),
							},
							{
								Key:   "videocodecid",
								Value: float64(7),
							},
							{
								Key:   "videodatarate",
								Value: float64(2500),
							},
							{
								Key:   "audiocodecid",
								Value: float64(10),
							},
							{
								Key:   "height",
								Value: float64(720),
							},
							{
								Key:   "width",
								Value: float64(1280),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload:         []uint8{5},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
					Payload:         []uint8{5},
				},
			},
		},
		{
			"mpeg-1 audio, ffmpeg",
			[]format.Format{
				&format.MPEG1Audio{},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(2)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG1Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecMPEG1Audio,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{ //nolint:dupl
			"pcma, ffmpeg",
			[]format.Format{
				&format.G711{
					PayloadTyp:   8,
					MULaw:        false,
					SampleRate:   8000,
					ChannelCount: 1,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(7)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMA,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMA,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{ //nolint:dupl
			"pcmu, ffmpeg",
			[]format.Format{
				&format.G711{
					PayloadTyp:   0,
					MULaw:        true,
					SampleRate:   8000,
					ChannelCount: 1,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(8)},
							{Key: "encoder", Value: "Lavf58.45.100"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMU,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecPCMU,
					Rate:            message.Rate5512,
					Depth:           message.Depth16,
					IsStereo:        false,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{
			"lpcm, gstreamer",
			[]format.Format{
				&format.LPCM{
					PayloadTyp:   96,
					BitDepth:     16,
					SampleRate:   44100,
					ChannelCount: 2,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{Key: "duration", Value: float64(0)},
							{Key: "audiocodecid", Value: float64(3)},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecLPCM,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					Payload:         []byte{1, 2, 3, 4},
				},
				&message.Audio{
					ChunkStreamID:   message.AudioChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecLPCM,
					Rate:            message.Rate44100,
					Depth:           message.Depth16,
					IsStereo:        true,
					Payload:         []byte{1, 2, 3, 4},
				},
			},
		},
		{
			"h264+aac+aac, obs 31 vod track",
			[]format.Format{
				&format.H264{
					PayloadTyp: 96,
					SPS: []byte{
						0x67, 0x64, 0x00, 0x2a, 0xac, 0x2b, 0x20, 0x0f,
						0x00, 0x44, 0xfc, 0xb8, 0x0b, 0x50, 0x10, 0x10,
						0x14, 0x00, 0x00, 0x0f, 0xa0, 0x00, 0x07, 0x53,
						0x03, 0x80, 0x00, 0x00, 0x5b, 0x8d, 0x80, 0x00,
						0x0b, 0x71, 0xb1, 0xbb, 0xcb, 0x82, 0x80,
					},
					PPS: []byte{
						0x68, 0xeb, 0x8f, 0x2c,
					},
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:         2,
						SampleRate:   48000,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:         2,
						SampleRate:   48000,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{ //nolint:dupl
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.ECMAArray{
							{Key: "duration", Value: float64(0)},
							{Key: "fileSize", Value: float64(0)},
							{Key: "width", Value: float64(1920)},
							{Key: "height", Value: float64(1080)},
							{Key: "videocodecid", Value: float64(7)},
							{Key: "videodatarate", Value: float64(6000)},
							{Key: "framerate", Value: float64(60)},
							{Key: "audiocodecid", Value: float64(10)},
							{Key: "audiodatarate", Value: float64(160)},
							{Key: "audiosamplerate", Value: float64(48000)},
							{Key: "audiosamplesize", Value: float64(16)},
							{Key: "audiochannels", Value: float64(2)},
							{Key: "stereo", Value: true},
							{Key: "2.1", Value: false},
							{Key: "3.1", Value: false},
							{Key: "4.0", Value: false},
							{Key: "4.1", Value: false},
							{Key: "5.1", Value: false},
							{Key: "7.1", Value: false},
							{Key: "encoder", Value: "obs-output module (libobs version 31.0.0)"},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					Codec:           0xa,
					Rate:            0x3,
					Depth:           0x1,
					IsStereo:        true,
					AACType:         message.AudioAACTypeConfig,
					Payload:         []uint8{0x11, 0x90, 0x56, 0xe5, 0x0},
				},
				&message.Video{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: []uint8{
						0x01, 0x64, 0x00, 0x2a, 0xff, 0xe1, 0x00, 0x27,
						0x67, 0x64, 0x00, 0x2a, 0xac, 0x2b, 0x20, 0x0f,
						0x00, 0x44, 0xfc, 0xb8, 0x0b, 0x50, 0x10, 0x10,
						0x14, 0x00, 0x00, 0x0f, 0xa0, 0x00, 0x07, 0x53,
						0x03, 0x80, 0x00, 0x00, 0x5b, 0x8d, 0x80, 0x00,
						0x0b, 0x71, 0xb1, 0xbb, 0xcb, 0x82, 0x80, 0x01,
						0x00, 0x04, 0x68, 0xeb, 0x8f, 0x2c, 0xfd, 0xf8,
						0xf8, 0x00,
					},
				},
				&message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        0x1,
					Wrapped: &message.AudioExSequenceStart{
						ChunkStreamID:   0x4,
						MessageStreamID: 0x1000000,
						FourCC:          0x6d703461,
						AACHeader: &mpeg4audio.AudioSpecificConfig{
							Type:         mpeg4audio.ObjectTypeAACLC,
							SampleRate:   48000,
							ChannelCount: 2,
						},
					},
				},
				&message.AudioExMultitrack{
					MultitrackType: 0x0,
					TrackID:        0x1,
					Wrapped: &message.AudioExCodedFrames{
						ChunkStreamID:   0x4,
						MessageStreamID: 0x1000000,
						FourCC:          0x6d703461,
						DTS:             2 * time.Second,
					},
				},
			},
		},
		{
			"h264+h264+aac, obs 31 multitrack video",
			[]format.Format{
				&format.H264{
					PayloadTyp: 96,
					SPS: []byte{
						0x67, 0x64, 0x00, 0x2a, 0xac, 0x2c, 0xac, 0x07,
						0x80, 0x22, 0x7e, 0x5c, 0x05, 0xa8, 0x08, 0x08,
						0x0a, 0x00, 0x00, 0x07, 0xd0, 0x00, 0x03, 0xa9,
						0x81, 0xc0, 0x00, 0x00, 0x2d, 0xc6, 0xc0, 0x00,
						0x05, 0xb8, 0xd8, 0xdd, 0xe5, 0xc1, 0x40,
					},
					PPS: []byte{
						0x68, 0xee, 0x3c, 0xb0,
					},
					PacketizationMode: 1,
				},
				&format.H264{
					PayloadTyp: 96,
					SPS: []byte{
						0x67, 0x4d, 0x40, 0x1e, 0x96, 0x56, 0x05, 0x01,
						0x7f, 0xcb, 0x80, 0xb5, 0x01, 0x01, 0x01, 0x40,
						0x00, 0x00, 0xfa, 0x00, 0x00, 0x3a, 0x98, 0x38,
						0x00, 0x00, 0x7a, 0x10, 0x00, 0x0f, 0x42, 0x5b,
						0xbc, 0xb8, 0x28,
					},
					PPS: []byte{
						0x68, 0xee, 0x3c, 0x80,
					},
					PacketizationMode: 1,
				},
				&format.MPEG4Audio{
					PayloadTyp: 96,
					Config: &mpeg4audio.AudioSpecificConfig{
						Type:         2,
						SampleRate:   48000,
						ChannelCount: 2,
					},
					SizeLength:       13,
					IndexLength:      3,
					IndexDeltaLength: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{ //nolint:dupl
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.ECMAArray{
							{Key: "duration", Value: float64(0)},
							{Key: "fileSize", Value: float64(0)},
							{Key: "width", Value: float64(1920)},
							{Key: "height", Value: float64(1080)},
							{Key: "videocodecid", Value: float64(7)},
							{Key: "videodatarate", Value: float64(6000)},
							{Key: "framerate", Value: float64(60)},
							{Key: "audiocodecid", Value: float64(10)},
							{Key: "audiodatarate", Value: float64(160)},
							{Key: "audiosamplerate", Value: float64(48000)},
							{Key: "audiosamplesize", Value: float64(16)},
							{Key: "audiochannels", Value: float64(2)},
							{Key: "stereo", Value: true},
							{Key: "2.1", Value: false},
							{Key: "3.1", Value: false},
							{Key: "4.0", Value: false},
							{Key: "4.1", Value: false},
							{Key: "5.1", Value: false},
							{Key: "7.1", Value: false},
							{Key: "encoder", Value: "obs-output module (libobs version 31.0.0)"},
						},
					},
				},
				&message.Audio{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					Codec:           0xa,
					Rate:            0x3,
					Depth:           0x1,
					IsStereo:        true,
					AACType:         0x0,
					Payload:         []uint8{0x11, 0x90, 0x56, 0xe5, 0x0},
				},
				&message.Video{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					Codec:           0x7,
					IsKeyFrame:      true,
					Type:            0x0,
					Payload: []uint8{
						0x01, 0x64, 0x00, 0x2a, 0xff, 0xe1, 0x00, 0x27,
						0x67, 0x64, 0x00, 0x2a, 0xac, 0x2c, 0xac, 0x07,
						0x80, 0x22, 0x7e, 0x5c, 0x05, 0xa8, 0x08, 0x08,
						0x0a, 0x00, 0x00, 0x07, 0xd0, 0x00, 0x03, 0xa9,
						0x81, 0xc0, 0x00, 0x00, 0x2d, 0xc6, 0xc0, 0x00,
						0x05, 0xb8, 0xd8, 0xdd, 0xe5, 0xc1, 0x40, 0x01,
						0x00, 0x04, 0x68, 0xee, 0x3c, 0xb0, 0xfd, 0xf8,
						0xf8, 0x00,
					},
				},
				&message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        0x1,
					Wrapped: &message.VideoExSequenceStart{
						ChunkStreamID:   0x4,
						MessageStreamID: 0x1000000,
						FourCC:          0x61766331,
						AVCHeader: &mp4.AVCDecoderConfiguration{
							AnyTypeBox:                 mp4.AnyTypeBox{Type: mp4.BoxType{0x61, 0x76, 0x63, 0x43}},
							ConfigurationVersion:       0x1,
							Profile:                    0x4d,
							ProfileCompatibility:       0x40,
							Level:                      0x1e,
							Reserved:                   0x3f,
							LengthSizeMinusOne:         0x3,
							Reserved2:                  0x7,
							NumOfSequenceParameterSets: 0x1,
							SequenceParameterSets: []mp4.AVCParameterSet{
								{
									Length: 0x23,
									NALUnit: []uint8{
										0x67, 0x4d, 0x40, 0x1e, 0x96, 0x56, 0x05, 0x01,
										0x7f, 0xcb, 0x80, 0xb5, 0x01, 0x01, 0x01, 0x40,
										0x00, 0x00, 0xfa, 0x00, 0x00, 0x3a, 0x98, 0x38,
										0x00, 0x00, 0x7a, 0x10, 0x00, 0x0f, 0x42, 0x5b,
										0xbc, 0xb8, 0x28,
									},
								},
							},
							NumOfPictureParameterSets: 0x1,
							PictureParameterSets: []mp4.AVCParameterSet{
								{
									Length:  0x4,
									NALUnit: []uint8{0x68, 0xee, 0x3c, 0x80},
								},
							},
						},
					},
				},
				&message.VideoExMultitrack{
					MultitrackType: 0x0,
					TrackID:        0x1,
					Wrapped: &message.VideoExCodedFrames{
						ChunkStreamID:   0x4,
						DTS:             2 * time.Second,
						MessageStreamID: 0x1000000,
						FourCC:          0x61766331,
						PTSDelta:        100000000,
					},
				},
			},
		},
		{
			"ac-3, ffmpeg",
			[]format.Format{
				&format.AC3{
					PayloadTyp:   96,
					SampleRate:   48000,
					ChannelCount: 3,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.ECMAArray{
							{Key: "duration", Value: float64(0)},
							{Key: "audiodatarate", Value: float64(62.5)},
							{Key: "audiosamplerate", Value: float64(48000)},
							{Key: "audiosamplesize", Value: float64(16)},
							{Key: "stereo", Value: false},
							{Key: "audiocodecid", Value: float64(1.633889587e+09)},
							{Key: "encoder", Value: "Lavf61.9.102"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.AudioExSequenceStart{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					FourCC:          0x61632d33,
				},
				&message.AudioExMultichannelConfig{
					ChunkStreamID:     0x4,
					MessageStreamID:   0x1000000,
					FourCC:            0x61632d33,
					AudioChannelOrder: 0x1,
					ChannelCount:      0x3,
					AudioChannelFlags: 0x103,
				},
				&message.AudioExCodedFrames{ //nolint:dupl
					ChunkStreamID:   0x4,
					DTS:             0,
					MessageStreamID: 0x1000000,
					FourCC:          0x61632d33,
					Payload: []uint8{ //nolint:dupl
						0x0b, 0x77, 0x93, 0x98, 0x08, 0x40, 0x8b, 0xe1,
						0x03, 0xbe, 0x00, 0x43, 0x03, 0x03, 0x08, 0x60,
						0x60, 0x61, 0xfc, 0x3c, 0x3c, 0x3f, 0x5a, 0x1b,
						0xea, 0xee, 0x8b, 0xdc, 0x31, 0xd1, 0x7b, 0x86,
						0x7f, 0xce, 0xaf, 0x9f, 0x3e, 0x7c, 0xf9, 0xf3,
						0xe7, 0xcf, 0x9f, 0x19, 0x7f, 0x48, 0x20, 0x40,
						0x81, 0x20, 0x09, 0xe7, 0x57, 0xee, 0xa2, 0x1f,
						0xfb, 0xe8, 0x84, 0xf7, 0xd9, 0x27, 0x5d, 0x61,
						0x56, 0x70, 0xb6, 0xde, 0xa7, 0x4e, 0xb1, 0xd8,
						0xc6, 0xf1, 0x0a, 0xad, 0x2f, 0xd4, 0xc2, 0xf5,
						0x07, 0x31, 0x2b, 0x67, 0x2e, 0x19, 0x99, 0x09,
						0x83, 0xe6, 0x26, 0x9e, 0x75, 0x7e, 0xea, 0x21,
						0xff, 0xbe, 0x88, 0x4f, 0x7d, 0x92, 0x75, 0xd2,
						0xb3, 0x85, 0xb6, 0xf5, 0x3a, 0x75, 0x8d, 0xa6,
						0x37, 0x88, 0x49, 0x1c, 0x1a, 0x94, 0xe9, 0xf2,
						0x69, 0x0b, 0x27, 0xbf, 0xca, 0xef, 0xab, 0xbb,
						0x58, 0x31, 0x52, 0xa2, 0x42, 0xc9, 0xef, 0xf2,
						0xbb, 0xea, 0xee, 0xd6, 0x0c, 0x54, 0xa8, 0x01,
						0xa8, 0x10, 0x26, 0x03, 0xb3, 0x3f, 0x30, 0xf3,
						0xc1, 0xa8, 0x10, 0x26, 0x03, 0xb3, 0x3f, 0x30,
						0x91, 0xc1, 0x81, 0x31, 0x1f, 0x26, 0x01, 0x61,
						0x8c, 0x18, 0x6a, 0x21, 0x29, 0x20, 0x92, 0xd1,
						0x61, 0x8c, 0x18, 0x6a, 0x21, 0x29, 0x20, 0x91,
						0xc1, 0x81, 0x4f, 0x6b, 0x3f, 0x00, 0x32, 0x07,
						0xfc, 0x8a, 0x79, 0x62, 0x7a, 0x91, 0xe0, 0x32,
						0x07, 0xfc, 0x8a, 0x79, 0x62, 0x7a, 0x91, 0xc1,
						0x81, 0x36, 0xed, 0xa6, 0x06, 0xe1, 0x2b, 0xe2,
						0x4f, 0x56, 0x2f, 0x58, 0xc0, 0xf6, 0xe1, 0x2b,
						0xe2, 0x4f, 0x56, 0x2f, 0x58, 0x91, 0xc1, 0x81,
						0x35, 0xe9, 0x57, 0x06, 0x57, 0x87, 0xdd, 0xdf,
						0xce, 0x87, 0x51, 0x20, 0x06, 0x57, 0x87, 0xdd,
						0xdf, 0xce, 0x87, 0x50, 0x90, 0x00, 0x07, 0x6a,
					},
				},
				&message.AudioExCodedFrames{
					ChunkStreamID:   0x4,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					FourCC:          0x61632d33,
					Payload: []uint8{ //nolint:dupl
						0x0b, 0x77, 0x93, 0x98, 0x08, 0x40, 0x8b, 0xe1,
						0x03, 0xbe, 0x00, 0x43, 0x03, 0x03, 0x08, 0x60,
						0x60, 0x61, 0xfc, 0x3c, 0x3c, 0x3f, 0x5a, 0x1b,
						0xea, 0xee, 0x8b, 0xdc, 0x31, 0xd1, 0x7b, 0x86,
						0x7f, 0xce, 0xaf, 0x9f, 0x3e, 0x7c, 0xf9, 0xf3,
						0xe7, 0xcf, 0x9f, 0x19, 0x7f, 0x48, 0x20, 0x40,
						0x81, 0x20, 0x09, 0xe7, 0x57, 0xee, 0xa2, 0x1f,
						0xfb, 0xe8, 0x84, 0xf7, 0xd9, 0x27, 0x5d, 0x61,
						0x56, 0x70, 0xb6, 0xde, 0xa7, 0x4e, 0xb1, 0xd8,
						0xc6, 0xf1, 0x0a, 0xad, 0x2f, 0xd4, 0xc2, 0xf5,
						0x07, 0x31, 0x2b, 0x67, 0x2e, 0x19, 0x99, 0x09,
						0x83, 0xe6, 0x26, 0x9e, 0x75, 0x7e, 0xea, 0x21,
						0xff, 0xbe, 0x88, 0x4f, 0x7d, 0x92, 0x75, 0xd2,
						0xb3, 0x85, 0xb6, 0xf5, 0x3a, 0x75, 0x8d, 0xa6,
						0x37, 0x88, 0x49, 0x1c, 0x1a, 0x94, 0xe9, 0xf2,
						0x69, 0x0b, 0x27, 0xbf, 0xca, 0xef, 0xab, 0xbb,
						0x58, 0x31, 0x52, 0xa2, 0x42, 0xc9, 0xef, 0xf2,
						0xbb, 0xea, 0xee, 0xd6, 0x0c, 0x54, 0xa8, 0x01,
						0xa8, 0x10, 0x26, 0x03, 0xb3, 0x3f, 0x30, 0xf3,
						0xc1, 0xa8, 0x10, 0x26, 0x03, 0xb3, 0x3f, 0x30,
						0x91, 0xc1, 0x81, 0x31, 0x1f, 0x26, 0x01, 0x61,
						0x8c, 0x18, 0x6a, 0x21, 0x29, 0x20, 0x92, 0xd1,
						0x61, 0x8c, 0x18, 0x6a, 0x21, 0x29, 0x20, 0x91,
						0xc1, 0x81, 0x4f, 0x6b, 0x3f, 0x00, 0x32, 0x07,
						0xfc, 0x8a, 0x79, 0x62, 0x7a, 0x91, 0xe0, 0x32,
						0x07, 0xfc, 0x8a, 0x79, 0x62, 0x7a, 0x91, 0xc1,
						0x81, 0x36, 0xed, 0xa6, 0x06, 0xe1, 0x2b, 0xe2,
						0x4f, 0x56, 0x2f, 0x58, 0xc0, 0xf6, 0xe1, 0x2b,
						0xe2, 0x4f, 0x56, 0x2f, 0x58, 0x91, 0xc1, 0x81,
						0x35, 0xe9, 0x57, 0x06, 0x57, 0x87, 0xdd, 0xdf,
						0xce, 0x87, 0x51, 0x20, 0x06, 0x57, 0x87, 0xdd,
						0xdf, 0xce, 0x87, 0x50, 0x90, 0x00, 0x07, 0x6a,
					},
				},
			},
		},
		{
			"opus, ffmpeg",
			[]format.Format{
				&format.Opus{
					PayloadTyp:   96,
					ChannelCount: 2,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 0x1000000,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.ECMAArray{
							{Key: "duration", Value: float64(0)},
							{Key: "audiodatarate", Value: float64(62.5)},
							{Key: "audiosamplerate", Value: float64(48000)},
							{Key: "audiosamplesize", Value: float64(16)},
							{Key: "stereo", Value: false},
							{Key: "audiocodecid", Value: float64(1.332770163e+09)},
							{Key: "encoder", Value: "Lavf61.9.102"},
							{Key: "filesize", Value: float64(0)},
						},
					},
				},
				&message.AudioExSequenceStart{
					ChunkStreamID:   0x4,
					MessageStreamID: 0x1000000,
					FourCC:          0x4f707573,
					OpusHeader: &message.OpusIDHeader{
						Version:              0x1,
						ChannelCount:         0x2,
						PreSkip:              0x3801,
						InputSampleRate:      0xc05d0000,
						OutputGain:           0x0,
						ChannelMappingFamily: 0x0,
						ChannelMappingTable:  []uint8{},
					},
				},
				&message.AudioExMultichannelConfig{
					ChunkStreamID:     0x4,
					MessageStreamID:   0x1000000,
					FourCC:            0x4f707573,
					AudioChannelOrder: 0x1,
					ChannelCount:      0x2,
					AudioChannelFlags: 0x03,
				},
				&message.AudioExCodedFrames{ //nolint:dupl
					ChunkStreamID:   0x4,
					DTS:             0,
					MessageStreamID: 0x1000000,
					FourCC:          0x4f707573,
					Payload: []uint8{ //nolint:dupl
						0xdc, 0xb4, 0x06, 0xa0, 0x80, 0x22, 0x38, 0x7d,
						0x05, 0x32, 0x20, 0xdc, 0xd5, 0xea, 0x34, 0xa6,
						0xd3, 0xd9, 0x21, 0xee, 0x94, 0x5f, 0x16, 0xce,
						0xb7, 0x43, 0x24, 0xa2, 0x5a, 0x9d, 0x6b, 0x01,
						0x00, 0x38, 0x03, 0xcb, 0x7f, 0x71, 0x27, 0x9b,
						0x78, 0x49, 0xa3, 0x03, 0xcb, 0xdf, 0x6c, 0x8f,
						0xc9, 0x6a, 0x50, 0x58, 0x41, 0xa7, 0xe7, 0xa9,
						0xa1, 0xcd, 0x7f, 0x18, 0x25, 0x3a, 0x56, 0x9a,
						0x50, 0x9e, 0x05, 0x2b, 0x0b, 0xb4, 0xc2, 0xca,
						0xb2, 0x68, 0x06, 0x26, 0x0e, 0xab, 0x2a, 0x3e,
						0x4e, 0x63, 0xbc, 0xa3, 0x50, 0xce, 0x18, 0xc1,
						0x63, 0xf6, 0xac, 0x56, 0x35, 0x68, 0x41, 0xaa,
						0x07, 0x15, 0x81, 0xd2, 0x67, 0xd8, 0x74, 0xe1,
						0x2d, 0x55, 0x29, 0x8d, 0x49, 0xb5, 0xb9, 0xa1,
						0xec, 0x88, 0x7d, 0x12, 0xff, 0x08, 0xb7, 0xc2,
						0x4a, 0x32, 0x6b, 0xe1, 0xb9, 0xa4, 0x59, 0x04,
						0xdd, 0xf6, 0x17, 0x67, 0x22, 0x36, 0xc3, 0x3d,
						0xc8, 0x37, 0xa3, 0x43, 0xdd, 0xec, 0x1c, 0x1f,
						0x54, 0x4f, 0xce, 0x08, 0x92, 0x1d, 0xee, 0x84,
						0xaf, 0x7c, 0xd7, 0x8a, 0x68, 0x83, 0x36, 0x78,
						0x5a, 0x61, 0x32, 0x38, 0x50, 0x78, 0x4f, 0xcf,
						0x26, 0x97, 0x0b, 0x90, 0x0c, 0xce, 0x13, 0x1b,
						0x74, 0xfb, 0xbb, 0x4a, 0x42, 0xab, 0xe2, 0x3c,
						0xf7, 0xd4, 0x8a, 0x02, 0x53, 0x22, 0x5c, 0xf6,
						0x06, 0x97, 0xe0, 0x3d, 0xdd, 0x65, 0x8b, 0x38,
						0x23, 0x56, 0x5c, 0x46, 0x3d, 0xd6, 0x88, 0x22,
						0x96, 0x93, 0x10, 0x35, 0x17, 0xd2, 0xf6, 0x3e,
						0x7f, 0x01, 0xf0, 0xf0, 0xfd, 0xf6, 0x00, 0xac,
						0x63, 0x1d, 0xba, 0xa9, 0x02, 0xc2, 0x53, 0xd4,
						0x64, 0x9c, 0xf0, 0x91, 0x8c, 0x45, 0x8a, 0xe1,
						0x42, 0x2d, 0x20, 0x1b, 0x37, 0x68, 0x1b, 0x37,
						0x4b, 0xf2, 0x02, 0x92, 0x95, 0x2a, 0x5f, 0x79,
					},
				},
				&message.AudioExCodedFrames{
					ChunkStreamID:   0x4,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					FourCC:          0x4f707573,
					Payload: []uint8{
						0xdc, 0xb1, 0xa4, 0x8c, 0xcc, 0x5c, 0x50, 0xe7,
						0x15, 0x1e, 0x33, 0x13, 0x87, 0x83, 0x39, 0xfc,
						0x7b, 0x8f, 0x58, 0x16, 0xa5, 0x34, 0xca, 0xcf,
						0x2e, 0x39, 0xa3, 0x5a, 0xff, 0xf6, 0x87, 0x00,
						0x51, 0x04, 0xd0, 0xa0, 0x95, 0x84, 0x0a, 0x56,
						0x62, 0xc4, 0xd2, 0xaf, 0x43, 0xaa, 0x09, 0x40,
						0x41, 0xfa, 0x57, 0xbf, 0x3d, 0x57, 0xcf, 0x77,
						0x41, 0xd5, 0x6f, 0x43, 0x87, 0xe8, 0x76, 0x9f,
						0xb8, 0x05, 0x1b, 0x39, 0xd9, 0xc2, 0x86, 0xe0,
						0xa0, 0xde, 0x33, 0xca, 0x47, 0x66, 0x35, 0xcf,
						0xd5, 0xc2, 0x04, 0xfa, 0x56, 0xc5, 0x63, 0x9a,
						0xd0, 0xbb, 0x07, 0x93, 0x87, 0x70, 0x68, 0x1c,
						0xa0, 0x6a, 0xe0, 0x09, 0xd0, 0x2f, 0x06, 0xa6,
						0x81, 0xb6, 0x12, 0x5f, 0x67, 0x8d, 0x64, 0xea,
						0xd8, 0x50, 0x54, 0x05, 0x49, 0x3f, 0x6d, 0x56,
						0x11, 0xab, 0x29, 0x6c, 0x77, 0x1a, 0xb8, 0x1c,
						0x0d, 0xd4, 0x3a, 0x10, 0x40, 0xf1, 0xd5, 0x9c,
						0x52, 0xba, 0xc8, 0x88, 0x67, 0xad, 0x96, 0xd9,
						0xd9, 0xf8, 0xb2, 0xaf, 0xce, 0x5f, 0xea, 0x96,
						0xcd, 0xcf, 0x96, 0x78, 0x3c, 0x5c, 0x9b, 0x3e,
						0x27, 0xd0, 0xec, 0xb7, 0x7d, 0x82, 0x4b, 0x70,
						0x48, 0xd2, 0xbc, 0x88, 0x53, 0xca, 0x0d, 0x4b,
						0x17, 0x46, 0x66, 0x73, 0x03, 0x23, 0x00, 0xe9,
						0x4f, 0xa6, 0x96, 0x8a, 0x0a, 0x64, 0x36, 0xfc,
						0xa9, 0x5f, 0x5c, 0xd3, 0x25, 0x2f, 0x22, 0x8c,
						0x71, 0xe4, 0x8b, 0xe5, 0x2e, 0xf9, 0x7f, 0xf7,
						0xfb, 0x7b, 0x49, 0x68, 0x00, 0x2a, 0xd7, 0x94,
						0x01, 0x99, 0xce, 0x5e, 0xec, 0x64, 0x63, 0xb9,
					},
				},
			},
		},
		{
			"issue mediamtx/3802 (double video config)",
			[]format.Format{
				&format.H264{
					PayloadTyp:        96,
					SPS:               test.FormatH264.SPS,
					PPS:               test.FormatH264.PPS,
					PacketizationMode: 1,
				},
			},
			[]message.Message{
				&message.DataAMF0{
					ChunkStreamID:   4,
					MessageStreamID: 1,
					Payload: []interface{}{
						"@setDataFrame",
						"onMetaData",
						amf0.Object{
							{
								Key:   "videodatarate",
								Value: float64(0),
							},
							{
								Key:   "videocodecid",
								Value: float64(message.CodecH264),
							},
							{
								Key:   "audiodatarate",
								Value: float64(0),
							},
							{
								Key:   "audiocodecid",
								Value: float64(0),
							},
						},
					},
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeConfig,
					Payload: func() []byte {
						buf, _ := h264conf.Conf{
							SPS: test.FormatH264.SPS,
							PPS: test.FormatH264.PPS,
						}.Marshal()
						return buf
					}(),
				},
				&message.Video{
					ChunkStreamID:   message.VideoChunkStreamID,
					DTS:             2 * time.Second,
					MessageStreamID: 0x1000000,
					Codec:           message.CodecH264,
					IsKeyFrame:      true,
					Type:            message.VideoTypeAU,
				},
			},
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			var buf bytes.Buffer
			bc := bytecounter.NewReadWriter(&buf)
			mrw := message.NewReadWriter(bc, bc, true)

			for _, msg := range ca.messages {
				err := mrw.Write(msg)
				require.NoError(t, err)
			}

			c := newNoHandshakeConn(&buf)

			r, err := NewReader(c)
			require.NoError(t, err)
			tracks := r.Tracks()
			require.Equal(t, ca.tracks, tracks)
		})
	}
}
