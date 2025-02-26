package rtmp

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/ac3"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/h264conf"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/message"
)

const (
	analyzePeriod = 2 * time.Second
)

// OnDataAV1Func is the prototype of the callback passed to OnDataAV1().
type OnDataAV1Func func(pts time.Duration, tu [][]byte)

// OnDataVP9Func is the prototype of the callback passed to OnDataVP9().
type OnDataVP9Func func(pts time.Duration, frame []byte)

// OnDataH26xFunc is the prototype of the callback passed to OnDataH26x().
type OnDataH26xFunc func(pts time.Duration, au [][]byte)

// OnDataOpusFunc is the prototype of the callback passed to OnDataOpus().
type OnDataOpusFunc func(pts time.Duration, packet []byte)

// OnDataMPEG4AudioFunc is the prototype of the callback passed to OnDataMPEG4Audio().
type OnDataMPEG4AudioFunc func(pts time.Duration, au []byte)

// OnDataMPEG1AudioFunc is the prototype of the callback passed to OnDataMPEG1Audio().
type OnDataMPEG1AudioFunc func(pts time.Duration, frame []byte)

// OnDataAC3Func is the prototype of the callback passed to OnDataAC3().
type OnDataAC3Func func(pts time.Duration, frame []byte)

// OnDataG711Func is the prototype of the callback passed to OnDataG711().
type OnDataG711Func func(pts time.Duration, samples []byte)

// OnDataLPCMFunc is the prototype of the callback passed to OnDataLPCM().
type OnDataLPCMFunc func(pts time.Duration, samples []byte)

func h265FindNALU(array []mp4.HEVCNaluArray, typ h265.NALUType) []byte {
	for _, entry := range array {
		if entry.NaluType == byte(typ) && entry.NumNalus == 1 &&
			h265.NALUType((entry.Nalus[0].NALUnit[0]>>1)&0b111111) == typ {
			return entry.Nalus[0].NALUnit
		}
	}
	return nil
}

func h264TrackFromConfig(data []byte) (*format.H264, error) {
	var conf h264conf.Conf
	err := conf.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unable to parse H264 config: %w", err)
	}

	return &format.H264{
		PayloadTyp:        96,
		SPS:               conf.SPS,
		PPS:               conf.PPS,
		PacketizationMode: 1,
	}, nil
}

func mpeg4AudioTrackFromConfig(data []byte) (*format.MPEG4Audio, error) {
	var mpegConf mpeg4audio.Config
	err := mpegConf.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return &format.MPEG4Audio{
		PayloadTyp:       96,
		Config:           &mpegConf,
		SizeLength:       13,
		IndexLength:      3,
		IndexDeltaLength: 3,
	}, nil
}

func audioTrackFromData(msg *message.Audio) (format.Format, error) {
	switch {
	case msg.Codec == message.CodecMPEG1Audio:
		return &format.MPEG1Audio{}, nil

	case msg.Codec == message.CodecPCMA:
		return &format.G711{
			PayloadTyp: 8,
			MULaw:      false,
			SampleRate: 8000,
			ChannelCount: func() int {
				if msg.IsStereo {
					return 2
				}
				return 1
			}(),
		}, nil

	case msg.Codec == message.CodecPCMU:
		return &format.G711{
			PayloadTyp: 0,
			MULaw:      true,
			SampleRate: 8000,
			ChannelCount: func() int {
				if msg.IsStereo {
					return 2
				}
				return 1
			}(),
		}, nil

	case msg.Codec == message.CodecLPCM:
		return &format.LPCM{
			PayloadTyp: 96,
			BitDepth: func() int {
				if msg.Depth == message.Depth16 {
					return 16
				}
				return 8
			}(),
			SampleRate: audioRateRTMPToInt(msg.Rate),
			ChannelCount: func() int {
				if msg.IsStereo {
					return 2
				}
				return 1
			}(),
		}, nil

	default:
		panic("should not happen")
	}
}

func videoTrackFromSequenceStart(msg *message.VideoExSequenceStart) (format.Format, error) {
	switch msg.FourCC {
	case message.FourCCAV1:
		// parse sequence header and metadata contained in ConfigOBUs, but do not use them
		var tu av1.Bitstream
		err := tu.Unmarshal(msg.AV1Header.ConfigOBUs)
		if err != nil {
			return nil, fmt.Errorf("invalid AV1 configuration: %w", err)
		}

		return &format.AV1{
			PayloadTyp: 96,
		}, nil

	case message.FourCCVP9:
		return &format.VP9{
			PayloadTyp: 96,
		}, nil

	case message.FourCCHEVC:
		vps := h265FindNALU(msg.HEVCHeader.NaluArrays, h265.NALUType_VPS_NUT)
		sps := h265FindNALU(msg.HEVCHeader.NaluArrays, h265.NALUType_SPS_NUT)
		pps := h265FindNALU(msg.HEVCHeader.NaluArrays, h265.NALUType_PPS_NUT)
		if vps == nil || sps == nil || pps == nil {
			return nil, fmt.Errorf("H265 parameters are missing")
		}

		return &format.H265{
			PayloadTyp: 96,
			VPS:        vps,
			SPS:        sps,
			PPS:        pps,
		}, nil

	case message.FourCCAVC:
		if len(msg.AVCHeader.SequenceParameterSets) != 1 || len(msg.AVCHeader.PictureParameterSets) != 1 {
			return nil, fmt.Errorf("H264 parameters are missing")
		}

		return &format.H264{
			PayloadTyp:        96,
			SPS:               msg.AVCHeader.SequenceParameterSets[0].NALUnit,
			PPS:               msg.AVCHeader.PictureParameterSets[0].NALUnit,
			PacketizationMode: 1,
		}, nil

	default:
		panic("should not happen")
	}
}

func audioTrackFromExtendedMessages(
	sequenceStart *message.AudioExSequenceStart,
	frames *message.AudioExCodedFrames,
) (format.Format, error) {
	if frames.FourCC != message.FourCCMP3 {
		if sequenceStart == nil {
			return nil, fmt.Errorf("sequence start not received")
		}
		if sequenceStart.FourCC != frames.FourCC {
			return nil, fmt.Errorf("AudioExSequenceStart FourCC and AudioExCodedFrames are different")
		}
	}

	switch frames.FourCC {
	case message.FourCCOpus:
		if len(frames.Payload) < 1 {
			return nil, fmt.Errorf("invalid Opus frame")
		}

		return &format.Opus{
			PayloadTyp:   96,
			ChannelCount: int(sequenceStart.OpusHeader.ChannelCount),
		}, nil

	case message.FourCCAC3:
		if len(frames.Payload) < 6 {
			return nil, fmt.Errorf("invalid AC-3 frame")
		}

		var syncInfo ac3.SyncInfo
		err := syncInfo.Unmarshal(frames.Payload)
		if err != nil {
			return nil, fmt.Errorf("invalid AC-3 frame: %w", err)
		}

		var bsi ac3.BSI
		err = bsi.Unmarshal(frames.Payload[5:])
		if err != nil {
			return nil, fmt.Errorf("invalid AC-3 frame: %w", err)
		}

		return &format.AC3{
			PayloadTyp:   96,
			SampleRate:   syncInfo.SampleRate(),
			ChannelCount: bsi.ChannelCount(),
		}, nil

	case message.FourCCMP4A:
		return &format.MPEG4Audio{
			PayloadTyp:       96,
			Config:           sequenceStart.AACHeader,
			SizeLength:       13,
			IndexLength:      3,
			IndexDeltaLength: 3,
		}, nil

	case message.FourCCMP3:
		return &format.MPEG1Audio{}, nil

	default:
		panic("should not happen")
	}
}

func sortedKeys(m map[uint8]format.Format) []int {
	ret := make([]int, len(m))
	i := 0
	for k := range m {
		ret[i] = int(k)
		i++
	}
	sort.Ints(ret)
	return ret
}

// Reader is a wrapper around Conn that provides utilities to demux incoming data.
type Reader struct {
	conn        *Conn
	videoTracks map[uint8]format.Format
	audioTracks map[uint8]format.Format
	onVideoData map[uint8]func(message.Message) error
	onAudioData map[uint8]func(message.Message) error
}

// NewReader allocates a Reader.
func NewReader(conn *Conn) (*Reader, error) {
	r := &Reader{
		conn: conn,
	}

	var err error
	r.videoTracks, r.audioTracks, err = r.readTracks()
	if err != nil {
		return nil, err
	}

	r.onVideoData = make(map[uint8]func(message.Message) error)
	r.onAudioData = make(map[uint8]func(message.Message) error)

	return r, nil
}

func (r *Reader) readTracks() (map[uint8]format.Format, map[uint8]format.Format, error) {
	firstReceived := false
	var startTime time.Duration
	var curTime time.Duration

	videoTracks := make(map[uint8]format.Format)
	audioTracks := make(map[uint8]format.Format)

	handleVideoSequenceStart := func(trackID uint8, msg *message.VideoExSequenceStart) error {
		if videoTracks[trackID] != nil {
			return fmt.Errorf("video track %d already setupped", trackID)
		}

		var err error
		videoTracks[trackID], err = videoTrackFromSequenceStart(msg)
		if err != nil {
			return err
		}

		return nil
	}

	handleVideoExCodedFrames := func(_ uint8, msg *message.VideoExCodedFrames) error {
		if !firstReceived {
			firstReceived = true
			startTime = msg.DTS
		}
		curTime = msg.DTS
		return nil
	}

	handleVideoExFramesX := func(_ uint8, msg *message.VideoExFramesX) error {
		if !firstReceived {
			firstReceived = true
			startTime = msg.DTS
		}
		curTime = msg.DTS
		return nil
	}

	audioSequenceStarts := make(map[uint8]*message.AudioExSequenceStart)

	handleAudioSequenceStart := func(trackID uint8, msg *message.AudioExSequenceStart) error {
		if audioSequenceStarts[trackID] != nil {
			return fmt.Errorf("audio track %d already setupped", trackID)
		}

		audioSequenceStarts[trackID] = msg
		return nil
	}

	handleAudioCodedFrames := func(trackID uint8, msg *message.AudioExCodedFrames) error {
		if !firstReceived {
			firstReceived = true
			startTime = msg.DTS
		}
		curTime = msg.DTS

		if audioTracks[trackID] != nil {
			return nil
		}

		var err error
		audioTracks[trackID], err = audioTrackFromExtendedMessages(audioSequenceStarts[trackID], msg)
		if err != nil {
			return err
		}

		return nil
	}

	for {
		msg, err := r.conn.Read()
		if err != nil {
			return nil, nil, err
		}

		switch msg := msg.(type) {
		case *message.Video:
			if !firstReceived {
				firstReceived = true
				startTime = msg.DTS
			}
			curTime = msg.DTS

			if msg.Type == message.VideoTypeConfig && videoTracks[0] == nil {
				videoTracks[0], err = h264TrackFromConfig(msg.Payload)
				if err != nil {
					return nil, nil, err
				}
			}

		case *message.VideoExSequenceStart:
			err = handleVideoSequenceStart(0, msg)
			if err != nil {
				return nil, nil, err
			}

		case *message.VideoExCodedFrames:
			err = handleVideoExCodedFrames(0, msg)
			if err != nil {
				return nil, nil, err
			}

		case *message.VideoExFramesX:
			err = handleVideoExFramesX(0, msg)
			if err != nil {
				return nil, nil, err
			}

		case *message.VideoExMultitrack:
			if _, ok := videoTracks[msg.TrackID]; !ok {
				videoTracks[msg.TrackID] = nil
			}

			switch wmsg := msg.Wrapped.(type) {
			case *message.VideoExSequenceStart:
				err = handleVideoSequenceStart(msg.TrackID, wmsg)
				if err != nil {
					return nil, nil, err
				}

			case *message.VideoExCodedFrames:
				err = handleVideoExCodedFrames(msg.TrackID, wmsg)
				if err != nil {
					return nil, nil, err
				}

			case *message.VideoExFramesX:
				err = handleVideoExFramesX(msg.TrackID, wmsg)
				if err != nil {
					return nil, nil, err
				}
			}

		case *message.Audio:
			if !firstReceived {
				firstReceived = true
				startTime = msg.DTS
			}
			curTime = msg.DTS

			if audioTracks[0] == nil && len(msg.Payload) != 0 {
				if msg.Codec == message.CodecMPEG4Audio {
					if msg.AACType == message.AudioAACTypeConfig {
						audioTracks[0], err = mpeg4AudioTrackFromConfig(msg.Payload)
						if err != nil {
							return nil, nil, err
						}
					}
				} else {
					audioTracks[0], err = audioTrackFromData(msg)
					if err != nil {
						return nil, nil, err
					}
				}
			}

		case *message.AudioExSequenceStart:
			err := handleAudioSequenceStart(0, msg)
			if err != nil {
				return nil, nil, err
			}

		case *message.AudioExCodedFrames:
			err := handleAudioCodedFrames(0, msg)
			if err != nil {
				return nil, nil, err
			}

		case *message.AudioExMultitrack:
			if _, ok := audioTracks[msg.TrackID]; !ok {
				audioTracks[msg.TrackID] = nil
			}

			switch wmsg := msg.Wrapped.(type) {
			case *message.AudioExSequenceStart:
				err := handleAudioSequenceStart(msg.TrackID, wmsg)
				if err != nil {
					return nil, nil, err
				}

			case *message.AudioExCodedFrames:
				err := handleAudioCodedFrames(msg.TrackID, wmsg)
				if err != nil {
					return nil, nil, err
				}
			}
		}

		if (curTime - startTime) >= analyzePeriod {
			break
		}
	}

	if len(videoTracks) == 0 && len(audioTracks) == 0 {
		return nil, nil, fmt.Errorf("no tracks found")
	}

	return videoTracks, audioTracks, nil
}

// Tracks returns detected tracks
func (r *Reader) Tracks() []format.Format {
	ret := make([]format.Format, len(r.videoTracks)+len(r.audioTracks))
	i := 0

	for _, k := range sortedKeys(r.videoTracks) {
		ret[i] = r.videoTracks[uint8(k)]
		i++
	}
	for _, k := range sortedKeys(r.audioTracks) {
		ret[i] = r.audioTracks[uint8(k)]
		i++
	}

	return ret
}

func (r *Reader) videoTrackID(t format.Format) uint8 {
	for id, track := range r.videoTracks {
		if track == t {
			return id
		}
	}
	return 255
}

func (r *Reader) audioTrackID(t format.Format) uint8 {
	for id, track := range r.audioTracks {
		if track == t {
			return id
		}
	}
	return 255
}

// OnDataAV1 sets a callback that is called when AV1 data is received.
func (r *Reader) OnDataAV1(track *format.AV1, cb OnDataAV1Func) {
	r.onVideoData[r.videoTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.VideoExFramesX:
			var tu av1.Bitstream
			err := tu.Unmarshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("unable to decode bitstream: %w", err)
			}

			cb(msg.DTS, tu)

		case *message.VideoExCodedFrames:
			var tu av1.Bitstream
			err := tu.Unmarshal(msg.Payload)
			if err != nil {
				return fmt.Errorf("unable to decode bitstream: %w", err)
			}

			cb(msg.DTS+msg.PTSDelta, tu)
		}
		return nil
	}
}

// OnDataVP9 sets a callback that is called when VP9 data is received.
func (r *Reader) OnDataVP9(track *format.VP9, cb OnDataVP9Func) {
	r.onVideoData[r.videoTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.VideoExFramesX:
			cb(msg.DTS, msg.Payload)

		case *message.VideoExCodedFrames:
			cb(msg.DTS+msg.PTSDelta, msg.Payload)
		}
		return nil
	}
}

// OnDataH265 sets a callback that is called when H265 data is received.
func (r *Reader) OnDataH265(track *format.H265, cb OnDataH26xFunc) {
	r.onVideoData[r.videoTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.VideoExFramesX:
			var au h264.AVCC
			err := au.Unmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS, au)

		case *message.VideoExCodedFrames:
			var au h264.AVCC
			err := au.Unmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS+msg.PTSDelta, au)
		}
		return nil
	}
}

// OnDataH264 sets a callback that is called when H264 data is received.
func (r *Reader) OnDataH264(track *format.H264, cb OnDataH26xFunc) {
	r.onVideoData[r.videoTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.Video:
			switch msg.Type {
			case message.VideoTypeConfig:
				var conf h264conf.Conf
				err := conf.Unmarshal(msg.Payload)
				if err != nil {
					return fmt.Errorf("unable to parse H264 config: %w", err)
				}

				au := [][]byte{
					conf.SPS,
					conf.PPS,
				}

				cb(msg.DTS+msg.PTSDelta, au)

			case message.VideoTypeAU:
				var au h264.AVCC
				err := au.Unmarshal(msg.Payload)
				if err != nil {
					if errors.Is(err, h264.ErrAVCCNoNALUs) {
						return nil
					}
					return fmt.Errorf("unable to decode AVCC: %w", err)
				}

				cb(msg.DTS+msg.PTSDelta, au)
			}

			return nil

		case *message.VideoExFramesX:
			var au h264.AVCC
			err := au.Unmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS, au)

		case *message.VideoExCodedFrames:
			var au h264.AVCC
			err := au.Unmarshal(msg.Payload)
			if err != nil {
				if errors.Is(err, h264.ErrAVCCNoNALUs) {
					return nil
				}
				return fmt.Errorf("unable to decode AVCC: %w", err)
			}

			cb(msg.DTS+msg.PTSDelta, au)
		}
		return nil
	}
}

// OnDataOpus sets a callback that is called when Opus data is received.
func (r *Reader) OnDataOpus(track *format.Opus, cb OnDataOpusFunc) {
	r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
		if msg, ok := msg.(*message.AudioExCodedFrames); ok {
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataMPEG4Audio sets a callback that is called when MPEG-4 Audio data is received.
func (r *Reader) OnDataMPEG4Audio(track *format.MPEG4Audio, cb OnDataMPEG4AudioFunc) {
	r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.Audio:
			if msg.AACType == message.AudioAACTypeAU {
				cb(msg.DTS, msg.Payload)
			}

		case *message.AudioExCodedFrames:
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataMPEG1Audio sets a callback that is called when MPEG-1 Audio data is received.
func (r *Reader) OnDataMPEG1Audio(track *format.MPEG1Audio, cb OnDataMPEG1AudioFunc) {
	r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
		switch msg := msg.(type) {
		case *message.Audio:
			cb(msg.DTS, msg.Payload)

		case *message.AudioExCodedFrames:
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataAC3 sets a callback that is called when AC-3 data is received.
func (r *Reader) OnDataAC3(track *format.AC3, cb OnDataAC3Func) {
	r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
		if msg, ok := msg.(*message.AudioExCodedFrames); ok {
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataG711 sets a callback that is called when G711 data is received.
func (r *Reader) OnDataG711(track *format.G711, cb OnDataG711Func) {
	r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
		if msg, ok := msg.(*message.Audio); ok {
			cb(msg.DTS, msg.Payload)
		}
		return nil
	}
}

// OnDataLPCM sets a callback that is called when LPCM data is received.
func (r *Reader) OnDataLPCM(track *format.LPCM, cb OnDataLPCMFunc) {
	bitDepth := track.BitDepth

	if bitDepth == 16 {
		r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
			if msg, ok := msg.(*message.Audio); ok {
				le := len(msg.Payload)
				if le%2 != 0 {
					return fmt.Errorf("invalid payload length: %d", le)
				}

				// convert from little endian to big endian
				for i := 0; i < le; i += 2 {
					msg.Payload[i], msg.Payload[i+1] = msg.Payload[i+1], msg.Payload[i]
				}

				cb(msg.DTS, msg.Payload)
			}
			return nil
		}
	} else {
		r.onAudioData[r.audioTrackID(track)] = func(msg message.Message) error {
			if msg, ok := msg.(*message.Audio); ok {
				cb(msg.DTS, msg.Payload)
			}
			return nil
		}
	}
}

// Read reads data.
func (r *Reader) Read() error {
	msg, err := r.conn.Read()
	if err != nil {
		return err
	}

	switch msg := msg.(type) {
	case *message.Video, *message.VideoExCodedFrames, *message.VideoExFramesX:
		if r.videoTracks[0] == nil {
			return fmt.Errorf("received a packet for video track 0, but track is not set up")
		}

		return r.onVideoData[0](msg)

	case *message.Audio, *message.AudioExCodedFrames:
		if r.audioTracks[0] == nil {
			return fmt.Errorf("received a packet for audio track 0, but track is not set up")
		}

		return r.onAudioData[0](msg)

	case *message.VideoExMultitrack:
		switch wmsg := msg.Wrapped.(type) {
		case *message.VideoExCodedFrames, *message.VideoExFramesX:
			if r.videoTracks[msg.TrackID] == nil {
				return fmt.Errorf("received a packet for video track %d, but track is not set up", msg.TrackID)
			}

			return r.onVideoData[msg.TrackID](wmsg)
		}

	case *message.AudioExMultitrack:
		if wmsg, ok := msg.Wrapped.(*message.AudioExCodedFrames); ok {
			if r.audioTracks[msg.TrackID] == nil {
				return fmt.Errorf("received a packet for audio track %d, but track is not set up", msg.TrackID)
			}

			return r.onAudioData[msg.TrackID](wmsg)
		}
	}

	return nil
}
