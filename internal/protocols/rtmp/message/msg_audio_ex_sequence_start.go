package message

import (
	"fmt"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp/rawmessage"
)

// AudioExSequenceStart is a sequence start extended message.
type AudioExSequenceStart struct {
	ChunkStreamID   byte
	MessageStreamID uint32
	FourCC          FourCC
	OpusHeader      *OpusIDHeader
	AACHeader       *mpeg4audio.AudioSpecificConfig
}

func (m *AudioExSequenceStart) unmarshal(raw *rawmessage.Message) error {
	if len(raw.Body) < 5 {
		return fmt.Errorf("not enough bytes")
	}

	m.ChunkStreamID = raw.ChunkStreamID
	m.MessageStreamID = raw.MessageStreamID

	m.FourCC = FourCC(raw.Body[1])<<24 | FourCC(raw.Body[2])<<16 | FourCC(raw.Body[3])<<8 | FourCC(raw.Body[4])
	switch m.FourCC {
	case FourCCOpus:
		m.OpusHeader = &OpusIDHeader{}
		err := m.OpusHeader.unmarshal(raw.Body[5:])
		if err != nil {
			return fmt.Errorf("invalid Opus ID header: %w", err)
		}

	case FourCCAC3, FourCCMP3:
		if len(raw.Body) != 5 {
			return fmt.Errorf("unexpected size")
		}

	case FourCCMP4A:
		m.AACHeader = &mpeg4audio.AudioSpecificConfig{}
		err := m.AACHeader.Unmarshal(raw.Body[5:])
		if err != nil {
			return fmt.Errorf("invalid MPEG-4 audio config: %w", err)
		}

	default:
		return fmt.Errorf("unsupported fourCC: %v", m.FourCC)
	}

	return nil
}

func (m AudioExSequenceStart) marshal() (*rawmessage.Message, error) {
	var addBody []byte

	switch m.FourCC {
	case FourCCOpus:
		buf, err := m.OpusHeader.marshal()
		if err != nil {
			return nil, err
		}
		addBody = buf

	case FourCCMP4A:
		buf, err := m.AACHeader.Marshal()
		if err != nil {
			return nil, err
		}
		addBody = buf
	}

	body := make([]byte, 5+len(addBody))

	body[0] = (9 << 4) | byte(AudioExTypeSequenceStart)
	body[1] = uint8(m.FourCC >> 24)
	body[2] = uint8(m.FourCC >> 16)
	body[3] = uint8(m.FourCC >> 8)
	body[4] = uint8(m.FourCC)
	copy(body[5:], addBody)

	return &rawmessage.Message{
		ChunkStreamID:   m.ChunkStreamID,
		Type:            uint8(TypeAudio),
		MessageStreamID: m.MessageStreamID,
		Body:            body,
	}, nil
}
