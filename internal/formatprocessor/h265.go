package formatprocessor

import (
	"bytes"
	"errors"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/pion/rtp"

	"github.com/bluenviron/mediamtx/internal/unit"
)

// H265-related parameters
var (
	H265DefaultVPS = []byte{
		0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x02, 0x20,
		0x00, 0x00, 0x03, 0x00, 0xb0, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x03, 0x00, 0x7b, 0x18, 0xb0, 0x24,
	}

	H265DefaultSPS = []byte{
		0x42, 0x01, 0x01, 0x02, 0x20, 0x00, 0x00, 0x03,
		0x00, 0xb0, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
		0x00, 0x7b, 0xa0, 0x07, 0x82, 0x00, 0x88, 0x7d,
		0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88,
		0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
		0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc,
		0xa2, 0x23, 0xff, 0x00, 0x01, 0x00, 0x01, 0x6a,
		0x02, 0x02, 0x02, 0x01,
	}

	H265DefaultPPS = []byte{
		0x44, 0x01, 0xc0, 0x25, 0x2f, 0x05, 0x32, 0x40,
	}
)

// extract VPS, SPS and PPS without decoding RTP packets
func rtpH265ExtractParams(payload []byte) ([]byte, []byte, []byte) {
	if len(payload) < 2 {
		return nil, nil, nil
	}

	typ := h265.NALUType((payload[0] >> 1) & 0b111111)

	switch typ {
	case h265.NALUType_VPS_NUT:
		return payload, nil, nil

	case h265.NALUType_SPS_NUT:
		return nil, payload, nil

	case h265.NALUType_PPS_NUT:
		return nil, nil, payload

	case h265.NALUType_AggregationUnit:
		payload = payload[2:]
		var vps []byte
		var sps []byte
		var pps []byte

		for len(payload) > 0 {
			if len(payload) < 2 {
				break
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			if size == 0 {
				break
			}

			if int(size) > len(payload) {
				return nil, nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h265.NALUType((nalu[0] >> 1) & 0b111111)

			switch typ {
			case h265.NALUType_VPS_NUT:
				vps = nalu

			case h265.NALUType_SPS_NUT:
				sps = nalu

			case h265.NALUType_PPS_NUT:
				pps = nalu
			}
		}

		return vps, sps, pps

	default:
		return nil, nil, nil
	}
}

type formatProcessorH265 struct {
	udpMaxPayloadSize int
	format            *format.H265
	encoder           *rtph265.Encoder
	decoder           *rtph265.Decoder
	randomStart       uint32
}

func newH265(
	udpMaxPayloadSize int,
	forma *format.H265,
	generateRTPPackets bool,
) (*formatProcessorH265, error) {
	t := &formatProcessorH265{
		udpMaxPayloadSize: udpMaxPayloadSize,
		format:            forma,
	}

	if generateRTPPackets {
		err := t.createEncoder(nil, nil)
		if err != nil {
			return nil, err
		}

		t.randomStart, err = randUint32()
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *formatProcessorH265) createEncoder(
	ssrc *uint32,
	initialSequenceNumber *uint16,
) error {
	t.encoder = &rtph265.Encoder{
		PayloadMaxSize:        t.udpMaxPayloadSize - 12,
		PayloadType:           t.format.PayloadTyp,
		SSRC:                  ssrc,
		InitialSequenceNumber: initialSequenceNumber,
		MaxDONDiff:            t.format.MaxDONDiff,
	}
	return t.encoder.Init()
}

func (t *formatProcessorH265) updateTrackParametersFromRTPPacket(payload []byte) {
	vps, sps, pps := rtpH265ExtractParams(payload)

	if (vps != nil && !bytes.Equal(vps, t.format.VPS)) ||
		(sps != nil && !bytes.Equal(sps, t.format.SPS)) ||
		(pps != nil && !bytes.Equal(pps, t.format.PPS)) {
		if vps == nil {
			vps = t.format.VPS
		}
		if sps == nil {
			sps = t.format.SPS
		}
		if pps == nil {
			pps = t.format.PPS
		}
		t.format.SafeSetParams(vps, sps, pps)
	}
}

func (t *formatProcessorH265) updateTrackParametersFromAU(au [][]byte) {
	vps := t.format.VPS
	sps := t.format.SPS
	pps := t.format.PPS
	update := false

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT:
			if !bytes.Equal(nalu, t.format.VPS) {
				vps = nalu
				update = true
			}

		case h265.NALUType_SPS_NUT:
			if !bytes.Equal(nalu, t.format.SPS) {
				sps = nalu
				update = true
			}

		case h265.NALUType_PPS_NUT:
			if !bytes.Equal(nalu, t.format.PPS) {
				pps = nalu
				update = true
			}
		}
	}

	if update {
		t.format.SafeSetParams(vps, sps, pps)
	}
}

func (t *formatProcessorH265) remuxAccessUnit(au [][]byte) [][]byte {
	isKeyFrame := false
	n := 0

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT: // parameters: remove
			continue

		case h265.NALUType_AUD_NUT: // AUD: remove
			continue

		case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT: // key frame
			if !isKeyFrame {
				isKeyFrame = true

				// prepend parameters
				if t.format.VPS != nil && t.format.SPS != nil && t.format.PPS != nil {
					n += 3
				}
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	filteredNALUs := make([][]byte, n)
	i := 0

	if isKeyFrame && t.format.VPS != nil && t.format.SPS != nil && t.format.PPS != nil {
		filteredNALUs[0] = t.format.VPS
		filteredNALUs[1] = t.format.SPS
		filteredNALUs[2] = t.format.PPS
		i = 3
	}

	for _, nalu := range au {
		typ := h265.NALUType((nalu[0] >> 1) & 0b111111)

		switch typ {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			continue

		case h265.NALUType_AUD_NUT:
			continue
		}

		filteredNALUs[i] = nalu
		i++
	}

	return filteredNALUs
}

func (t *formatProcessorH265) ProcessUnit(uu unit.Unit) error { //nolint:dupl
	u := uu.(*unit.H265)

	t.updateTrackParametersFromAU(u.AU)
	u.AU = t.remuxAccessUnit(u.AU)

	if u.AU != nil {
		pkts, err := t.encoder.Encode(u.AU)
		if err != nil {
			return err
		}
		u.RTPPackets = pkts

		for _, pkt := range u.RTPPackets {
			pkt.Timestamp += t.randomStart + uint32(u.PTS)
		}
	}

	return nil
}

func (t *formatProcessorH265) ProcessRTPPacket( //nolint:dupl
	pkt *rtp.Packet,
	ntp time.Time,
	pts int64,
	hasNonRTSPReaders bool,
) (unit.Unit, error) {
	u := &unit.H265{
		Base: unit.Base{
			RTPPackets: []*rtp.Packet{pkt},
			NTP:        ntp,
			PTS:        pts,
		},
	}

	t.updateTrackParametersFromRTPPacket(pkt.Payload)

	if t.encoder == nil {
		// remove padding
		pkt.Header.Padding = false
		pkt.PaddingSize = 0

		// RTP packets exceed maximum size: start re-encoding them
		if pkt.MarshalSize() > t.udpMaxPayloadSize {
			v1 := pkt.SSRC
			v2 := pkt.SequenceNumber
			err := t.createEncoder(&v1, &v2)
			if err != nil {
				return nil, err
			}
		}
	}

	// decode from RTP
	if hasNonRTSPReaders || t.decoder != nil || t.encoder != nil {
		if t.decoder == nil {
			var err error
			t.decoder, err = t.format.CreateDecoder()
			if err != nil {
				return nil, err
			}
		}

		au, err := t.decoder.Decode(pkt)

		if t.encoder != nil {
			u.RTPPackets = nil
		}

		if err != nil {
			if errors.Is(err, rtph265.ErrNonStartingPacketAndNoPrevious) ||
				errors.Is(err, rtph265.ErrMorePacketsNeeded) {
				return u, nil
			}
			return nil, err
		}

		u.AU = t.remuxAccessUnit(au)
	}

	// route packet as is
	if t.encoder == nil {
		return u, nil
	}

	// encode into RTP
	if len(u.AU) != 0 {
		pkts, err := t.encoder.Encode(u.AU)
		if err != nil {
			return nil, err
		}
		u.RTPPackets = pkts

		for _, newPKT := range u.RTPPackets {
			newPKT.Timestamp = pkt.Timestamp
		}
	}

	return u, nil
}
