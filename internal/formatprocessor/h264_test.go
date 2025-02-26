package formatprocessor

import (
	"bytes"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/unit"
)

func TestH264DynamicParams(t *testing.T) {
	for _, ca := range []string{"standard", "aggregated"} {
		t.Run(ca, func(t *testing.T) {
			forma := &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
			}

			p, err := New(1472, forma, false)
			require.NoError(t, err)

			enc, err := forma.CreateEncoder()
			require.NoError(t, err)

			pkts, err := enc.Encode([][]byte{{byte(h264.NALUTypeIDR)}})
			require.NoError(t, err)

			data, err := p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{byte(h264.NALUTypeIDR)},
			}, data.(*unit.H264).AU)

			if ca == "standard" {
				pkts, err = enc.Encode([][]byte{{7, 4, 5, 6}}) // SPS
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)

				pkts, err = enc.Encode([][]byte{{8, 1}}) // PPS
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			} else {
				pkts, err = enc.Encode([][]byte{
					{7, 4, 5, 6}, // SPS
					{8, 1},       // PPS
				})
				require.NoError(t, err)

				_, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, false)
				require.NoError(t, err)
			}

			require.Equal(t, []byte{7, 4, 5, 6}, forma.SPS)
			require.Equal(t, []byte{8, 1}, forma.PPS)

			pkts, err = enc.Encode([][]byte{{byte(h264.NALUTypeIDR)}})
			require.NoError(t, err)

			data, err = p.ProcessRTPPacket(pkts[0], time.Time{}, 0, true)
			require.NoError(t, err)

			require.Equal(t, [][]byte{
				{0x07, 4, 5, 6},
				{0x08, 1},
				{byte(h264.NALUTypeIDR)},
			}, data.(*unit.H264).AU)
		})
	}
}

func TestH264OversizedPackets(t *testing.T) {
	forma := &format.H264{
		PayloadTyp:        96,
		SPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PPS:               []byte{0x01, 0x02, 0x03, 0x04},
		PacketizationMode: 1,
	}

	p, err := New(1472, forma, false)
	require.NoError(t, err)

	var out []*rtp.Packet

	for _, pkt := range []*rtp.Packet{
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         false,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: append([]byte{0x1c, 0b10000000}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 2000/4)...),
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 125,
				Timestamp:      45343,
				SSRC:           563423,
				Padding:        true,
			},
			Payload: []byte{0x1c, 0b01000000, 0x01, 0x02, 0x03, 0x04},
		},
	} {
		data, err := p.ProcessRTPPacket(pkt, time.Time{}, 0, false)
		require.NoError(t, err)

		out = append(out, data.GetRTPPackets()...)
	}

	require.Equal(t, []*rtp.Packet{
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 123,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: []byte{0x01, 0x02, 0x03, 0x04},
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         false,
				PayloadType:    96,
				SequenceNumber: 124,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: append(
				append([]byte{0x1c, 0x80}, bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 364)...),
				[]byte{0x01, 0x02}...,
			),
		},
		{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: 125,
				Timestamp:      45343,
				SSRC:           563423,
			},
			Payload: append(
				[]byte{0x1c, 0x40, 0x03, 0x04},
				bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 136)...,
			),
		},
	}, out)
}

func TestH264EmptyPacket(t *testing.T) {
	forma := &format.H264{
		PayloadTyp:        96,
		PacketizationMode: 1,
	}

	p, err := New(1472, forma, true)
	require.NoError(t, err)

	unit := &unit.H264{
		AU: [][]byte{
			{0x07, 0x01, 0x02, 0x03}, // SPS
			{0x08, 0x01, 0x02},       // PPS
		},
	}

	err = p.ProcessUnit(unit)
	require.NoError(t, err)

	// if all NALUs have been removed, no RTP packets must be generated.
	require.Equal(t, []*rtp.Packet(nil), unit.RTPPackets)
}

func FuzzRTPH264ExtractParams(f *testing.F) {
	f.Fuzz(func(_ *testing.T, b []byte) {
		rtpH264ExtractParams(b)
	})
}
