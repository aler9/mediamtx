package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestPeerConnectionCloseImmediately(t *testing.T) {
	pc := &PeerConnection{
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		STUNGatherTimeout:  conf.Duration(5 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            false,
		Log:                test.NilLogger,
	}
	err := pc.Start()
	require.NoError(t, err)
	defer pc.Close()

	_, err = pc.CreatePartialOffer()
	require.NoError(t, err)

	// wait for ICE candidates to be generated
	time.Sleep(500 * time.Millisecond)

	pc.Close()
}

// test that an audio codec is present regardless of the fact that an audio track is.
func TestPeerConnectionFallbackCodecs(t *testing.T) {
	pc1 := &PeerConnection{
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            false,
		Log:                test.NilLogger,
	}
	err := pc1.Start()
	require.NoError(t, err)
	defer pc1.Close()

	pc2 := &PeerConnection{
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		LocalRandomUDP:     true,
		IPsFromInterfaces:  true,
		Publish:            true,
		OutgoingTracks: []*OutgoingTrack{{
			Caps: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeAV1,
				ClockRate: 90000,
			},
		}},
		Log: test.NilLogger,
	}
	err = pc2.Start()
	require.NoError(t, err)
	defer pc2.Close()

	offer, err := pc1.CreatePartialOffer()
	require.NoError(t, err)

	answer, err := pc2.CreateFullAnswer(context.Background(), offer)
	require.NoError(t, err)

	var s sdp.SessionDescription
	err = s.Unmarshal([]byte(answer.SDP))
	require.NoError(t, err)

	require.Equal(t, []*sdp.MediaDescription{
		{
			MediaName: sdp.MediaName{
				Media:   "video",
				Port:    sdp.RangedPort{Value: 9},
				Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
				Formats: []string{"97"},
			},
			ConnectionInformation: s.MediaDescriptions[0].ConnectionInformation,
			Attributes:            s.MediaDescriptions[0].Attributes,
		},
		{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Port:    sdp.RangedPort{Value: 9},
				Protos:  []string{"UDP", "TLS", "RTP", "SAVPF"},
				Formats: []string{"0"},
			},
			ConnectionInformation: s.MediaDescriptions[1].ConnectionInformation,
			Attributes:            s.MediaDescriptions[1].Attributes,
		},
	}, s.MediaDescriptions)
}
