package webrtc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func uint16Ptr(v uint16) *uint16 {
	return &v
}

func checkClose(t *testing.T, closeFunc func() error) {
	require.NoError(t, closeFunc())
}

type dummyPath struct {
	stream        *stream.Stream
	streamCreated chan struct{}
}

func (p *dummyPath) Name() string {
	return "teststream"
}

func (p *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (p *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return externalcmd.Environment{}
}

func (p *dummyPath) StartPublisher(req defs.PathStartPublisherReq) (*stream.Stream, error) {
	var err error
	p.stream, err = stream.New(
		512,
		1460,
		req.Desc,
		true,
		test.NilLogger,
		false,
	)
	if err != nil {
		return nil, err
	}
	close(p.streamCreated)
	return p.stream, nil
}

func (p *dummyPath) StopPublisher(_ defs.PathStopPublisherReq) {
}

func (p *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (p *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

func initializeTestServer(t *testing.T) *Server {
	pm := &test.PathManager{
		FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*conf.Path, error) {
			return &conf.Path{}, nil
		},
	}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "*",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.Duration(10 * time.Second),
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		HandshakeTimeout:      conf.Duration(10 * time.Second),
		TrackGatherTimeout:    conf.Duration(2 * time.Second),
		STUNGatherTimeout:     conf.Duration(5 * time.Second),
		ExternalCmdPool:       nil,
		PathManager:           pm,
		Parent:                test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)

	return s
}

func TestServerStaticPages(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	for _, path := range []string{"/stream", "/stream/publish", "/publish"} {
		func() {
			req, err := http.NewRequest(http.MethodGet, "http://myuser:mypass@localhost:8886"+path, nil)
			require.NoError(t, err)

			res, err := hc.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)
		}()
	}
}

func TestPreflightRequest(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:8886", nil)
	require.NoError(t, err)

	req.Header.Add("Access-Control-Request-Method", "GET")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", res.Header.Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "OPTIONS, GET, POST, PATCH, DELETE", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization, Content-Type, If-Match", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestServerOptionsICEServer(t *testing.T) {
	pathManager := &test.PathManager{
		FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*conf.Path, error) {
			return &conf.Path{}, nil
		},
	}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.Duration(10 * time.Second),
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers: []conf.WebRTCICEServer{{
			URL:      "example.com",
			Username: "myuser",
			Password: "mypass",
		}},
		HandshakeTimeout:   conf.Duration(10 * time.Second),
		TrackGatherTimeout: conf.Duration(2 * time.Second),
		STUNGatherTimeout:  conf.Duration(5 * time.Second),
		ExternalCmdPool:    nil,
		PathManager:        pathManager,
		Parent:             test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions,
		"http://myuser:mypass@localhost:8886/nonexisting/whep", nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)

	iceServers, err := whip.LinkHeaderUnmarshal(res.Header["Link"])
	require.NoError(t, err)

	require.Equal(t, []pwebrtc.ICEServer{{
		URLs:       []string{"example.com"},
		Username:   "myuser",
		Credential: "mypass",
	}}, iceServers)
}

func TestServerPublish(t *testing.T) {
	path := &dummyPath{
		streamCreated: make(chan struct{}),
	}

	pathManager := &test.PathManager{
		FindPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.User)
			require.Equal(t, "mypass", req.AccessRequest.Pass)
			return &conf.Path{}, nil
		},
		AddPublisherImpl: func(req defs.PathAddPublisherReq) (defs.Path, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.User)
			require.Equal(t, "mypass", req.AccessRequest.Pass)
			return path, nil
		},
	}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.Duration(10 * time.Second),
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		HandshakeTimeout:      conf.Duration(10 * time.Second),
		TrackGatherTimeout:    conf.Duration(2 * time.Second),
		STUNGatherTimeout:     conf.Duration(5 * time.Second),
		ExternalCmdPool:       nil,
		PathManager:           pathManager,
		Parent:                test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	su, err := url.Parse("http://myuser:mypass@localhost:8886/teststream/whip?param=value")
	require.NoError(t, err)

	wc := &whip.Client{
		HTTPClient: hc,
		URL:        su,
		Log:        test.NilLogger,
	}

	track := &webrtc.OutgoingTrack{
		Caps: pwebrtc.RTPCodecCapability{
			MimeType:    pwebrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
	}

	err = wc.Publish(context.Background(), []*webrtc.OutgoingTrack{track})
	require.NoError(t, err)
	defer checkClose(t, wc.Close)

	err = track.WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{1},
	})
	require.NoError(t, err)

	<-path.streamCreated

	reader := test.NilLogger

	recv := make(chan struct{})

	path.stream.AddReader(
		reader,
		path.stream.Desc().Medias[0],
		path.stream.Desc().Medias[0].Formats[0],
		func(u unit.Unit) error {
			select {
			case <-recv:
				return nil
			default:
			}

			require.Equal(t, [][]byte{
				{1},
			}, u.(*unit.H264).AU)
			close(recv)

			return nil
		})

	path.stream.StartReader(reader)
	defer path.stream.RemoveReader(reader)

	err = track.WriteRTP(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 124,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{1},
	})
	require.NoError(t, err)

	<-recv
}

func TestServerRead(t *testing.T) {
	for _, ca := range []struct {
		name          string
		medias        []*description.Media
		unit          []unit.Unit
		outRTPPayload []byte
		gopCache      bool
	}{
		{
			"av1",
			[]*description.Media{{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.AV1{
					PayloadTyp: 96,
				}},
			}},
			[]unit.Unit{
				&unit.AV1{
					TU: [][]byte{{1, 2}},
				},
			},
			[]byte{0, 2, 1, 2},
			false,
		},
		{
			"vp9",
			[]*description.Media{{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP9{
					PayloadTyp: 96,
				}},
			}},
			[]unit.Unit{
				&unit.VP9{
					Frame: []byte{0x82, 0x49, 0x83, 0x42, 0x0, 0x77, 0xf0, 0x32, 0x34},
				},
			},
			[]byte{
				0x8f, 0xa0, 0xfd, 0x18, 0x07, 0x80, 0x03, 0x24,
				0x01, 0x14, 0x01, 0x82, 0x49, 0x83, 0x42, 0x00,
				0x77, 0xf0, 0x32, 0x34,
			},
			false,
		},
		{
			"vp8",
			[]*description.Media{{
				Type: description.MediaTypeVideo,
				Formats: []format.Format{&format.VP8{
					PayloadTyp: 96,
				}},
			}},
			[]unit.Unit{
				&unit.VP8{
					Frame: []byte{1, 2},
				},
			},
			[]byte{0x10, 1, 2},
			false,
		},
		{
			"h264",
			[]*description.Media{test.MediaH264},
			[]unit.Unit{
				&unit.H264{
					AU: [][]byte{
						{5, 1},
					},
				},
			},
			[]byte{
				0x18, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9,
				0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00,
				0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0,
				0x3c, 0x60, 0xc9, 0x20, 0x00, 0x04, 0x08, 0x06,
				0x07, 0x08, 0x00, 0x02, 0x05, 0x01,
			},
			false,
		},
		{
			"h264 with gop cache",
			[]*description.Media{test.MediaH264},
			[]unit.Unit{
				// ffmpeg -f lavfi -i color=blue:s=2x2 -vframes 10 -c:v libx264 out.264
				&unit.H264{
					AU: [][]byte{
						{
							0x65, 0x88, 0x84, 0x00, 0x37, 0xff, 0xfe, 0xe1,
							0x03, 0xf8, 0x14, 0xd7, 0x4d, 0xfe, 0x63, 0x8f,
							0x43, 0xd9, 0x01, 0x68, 0xc1,
						},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x41, 0x9a, 0x24, 0x6c, 0x43, 0x7f, 0xfe, 0xe0},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x41, 0x9e, 0x42, 0x78, 0x85, 0xff, 0xc1, 0x81},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x01, 0x9e, 0x61, 0x74, 0x42, 0xbf, 0xc4, 0x80},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x01, 0x9e, 0x63, 0x6a, 0x42, 0xbf, 0xc4, 0x81},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x41, 0x9a, 0x68, 0x49, 0xa8, 0x41, 0x68, 0x99, 0x4c, 0x08, 0x5f, 0xff, 0xfe, 0xe1},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x41, 0x9e, 0x86, 0x45, 0x11, 0x2c, 0x2f, 0xff, 0xc1, 0x81},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x01, 0x9e, 0xa5, 0x74, 0x42, 0xbf, 0xc4, 0x81},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x01, 0x9e, 0xa7, 0x6a, 0x42, 0xbf, 0xc4, 0x80},
					},
				},
				&unit.H264{
					AU: [][]byte{
						{0x41, 0x9a, 0xa9, 0x49, 0xa8, 0x41, 0x6c, 0x99, 0x4c, 0x08, 0x57, 0xff, 0xfe, 0xc0},
					},
				},
			},
			[]byte{
				0x18, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00,
				0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20, 0x00, 0x04, 0x08, 0x06,
				0x07, 0x08, 0x00, 0x15, 0x65, 0x88, 0x84, 0x00, 0x37, 0xff, 0xfe, 0xe1, 0x03, 0xf8, 0x14, 0xd7,
				0x4d, 0xfe, 0x63, 0x8f, 0x43, 0xd9, 0x01, 0x68, 0xc1,
			},
			true,
		},
		{
			"h265 with gop cache",
			[]*description.Media{test.MediaH265},
			[]unit.Unit{
				// ffmpeg -f lavfi -i color=blue:s=16x16 -vframes 10 -c:v libx265 out.265
				&unit.H265{
					AU: [][]byte{
						{
							0x28, 0x01, 0xaf, 0x1d, 0x80, 0xf0, 0x0e, 0x9e, 0x0f, 0xfd, 0x7d, 0x3a, 0x39, 0xb1,
							0xc7, 0x6f, 0x98,
						},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x02, 0x01, 0xd0, 0x29, 0x4b, 0xe1, 0x0c, 0x63, 0x90, 0xfa, 0x84},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x02, 0x01, 0xe0, 0x64, 0x9d, 0x78, 0x61, 0x24, 0xc5, 0x60},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x00, 0x01, 0xe0, 0x24, 0xf5, 0x5f, 0xa2, 0xc2, 0x98, 0xc8, 0x20},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x00, 0x01, 0xe0, 0x44, 0xd7, 0x5f, 0xa2, 0xc2, 0x88, 0xc8, 0x20},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x00, 0x01, 0xe0, 0x86, 0xb7, 0xfd, 0x46, 0x14, 0xc0, 0xc8, 0x20},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x02, 0x01, 0xd0, 0x48, 0x92, 0x55, 0xfd, 0xc4, 0x30, 0x18, 0xec, 0xfa, 0x84},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x02, 0x01, 0xe0, 0xe2, 0x25, 0x57, 0x5f, 0x71, 0x84, 0x90, 0xc5, 0x60},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x00, 0x01, 0xe0, 0xc6, 0xf5, 0xd7, 0xd2, 0x2c, 0x29, 0x80, 0xc8, 0x20},
					},
				},
				&unit.H265{
					AU: [][]byte{
						{0x00, 0x01, 0xe1, 0x02, 0x2d, 0x57, 0xf7, 0x18, 0x51, 0xc8, 0x20},
					},
				},
			},
			[]byte{
				0x60, 0x0, 0x0, 0x18, 0x40, 0x1, 0xc, 0x1, 0xff, 0xff, 0x2, 0x20, 0x0, 0x0, 0x3, 0x0, 0xb0, 0x0,
				0x0, 0x3, 0x0, 0x0, 0x3, 0x0, 0x7b, 0x18, 0xb0, 0x24, 0x0, 0x3c, 0x42, 0x1, 0x1, 0x2, 0x20, 0x0,
				0x0, 0x3, 0x0, 0xb0, 0x0, 0x0, 0x3, 0x0, 0x0, 0x3, 0x0, 0x7b, 0xa0, 0x7, 0x82, 0x0, 0x88, 0x7d,
				0xb6, 0x71, 0x8b, 0x92, 0x44, 0x80, 0x53, 0x88, 0x88, 0x92, 0xcf, 0x24, 0xa6, 0x92, 0x72, 0xc9,
				0x12, 0x49, 0x22, 0xdc, 0x91, 0xaa, 0x48, 0xfc, 0xa2, 0x23, 0xff, 0x0, 0x1, 0x0, 0x1, 0x6a, 0x2,
				0x2, 0x2, 0x1, 0x0, 0x8, 0x44, 0x1, 0xc0, 0x25, 0x2f, 0x5, 0x32, 0x40, 0x0, 0x11, 0x28, 0x1, 0xaf,
				0x1d, 0x80, 0xf0, 0xe, 0x9e, 0xf, 0xfd, 0x7d, 0x3a, 0x39, 0xb1, 0xc7, 0x6f, 0x98,
			},
			true,
		},
		{
			"opus",
			[]*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.Opus{
					PayloadTyp:   96,
					ChannelCount: 2,
				}},
			}},
			[]unit.Unit{
				&unit.Opus{
					Packets: [][]byte{{1, 2}},
				},
			},
			[]byte{1, 2},
			false,
		},
		{
			"g722",
			[]*description.Media{{
				Type:    description.MediaTypeAudio,
				Formats: []format.Format{&format.G722{}},
			}},
			[]unit.Unit{
				&unit.Generic{
					Base: unit.Base{
						RTPPackets: []*rtp.Packet{{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    9,
								SequenceNumber: 1123,
								Timestamp:      45343,
								SSRC:           563423,
							},
							Payload: []byte{1, 2},
						}},
					},
				},
			},
			[]byte{1, 2},
			false,
		},
		{
			"g711 8khz mono",
			[]*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.G711{
					MULaw:        true,
					SampleRate:   8000,
					ChannelCount: 1,
				}},
			}},
			[]unit.Unit{
				&unit.G711{
					Samples: []byte{1, 2, 3},
				},
			},
			[]byte{1, 2, 3},
			false,
		},
		{
			"g711 16khz stereo",
			[]*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.G711{
					MULaw:        true,
					SampleRate:   16000,
					ChannelCount: 2,
				}},
			}},
			[]unit.Unit{
				&unit.G711{
					Samples: []byte{1, 2, 3, 4},
				},
			},
			[]byte{0x86, 0x84, 0x8a, 0x84, 0x8e, 0x84, 0x92, 0x84},
			false,
		},
		{
			"lpcm",
			[]*description.Media{{
				Type: description.MediaTypeAudio,
				Formats: []format.Format{&format.LPCM{
					PayloadTyp:   96,
					BitDepth:     16,
					SampleRate:   48000,
					ChannelCount: 2,
				}},
			}},
			[]unit.Unit{
				&unit.LPCM{
					Samples: []byte{1, 2, 3, 4},
				},
			},
			[]byte{1, 2, 3, 4},
			false,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			desc := &description.Session{Medias: ca.medias}

			str, err := stream.New(
				512,
				1460,
				desc,
				reflect.TypeOf(ca.unit[0]) != reflect.TypeOf(&unit.Generic{}),
				test.NilLogger,
				ca.gopCache,
			)
			require.NoError(t, err)

			path := &dummyPath{stream: str}

			pathManager := &test.PathManager{
				FindPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.User)
					require.Equal(t, "mypass", req.AccessRequest.Pass)
					return &conf.Path{}, nil
				},
				AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.User)
					require.Equal(t, "mypass", req.AccessRequest.Pass)
					return path, str, nil
				},
			}

			s := &Server{
				Address:               "127.0.0.1:8886",
				Encryption:            false,
				ServerKey:             "",
				ServerCert:            "",
				AllowOrigin:           "",
				TrustedProxies:        conf.IPNetworks{},
				ReadTimeout:           conf.Duration(10 * time.Second),
				LocalUDPAddress:       "127.0.0.1:8887",
				LocalTCPAddress:       "127.0.0.1:8887",
				IPsFromInterfaces:     true,
				IPsFromInterfacesList: []string{},
				AdditionalHosts:       []string{},
				ICEServers:            []conf.WebRTCICEServer{},
				HandshakeTimeout:      conf.Duration(10 * time.Second),
				TrackGatherTimeout:    conf.Duration(2 * time.Second),
				STUNGatherTimeout:     conf.Duration(5 * time.Second),
				ExternalCmdPool:       nil,
				PathManager:           pathManager,
				Parent:                test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:8886/teststream/whep?param=value")
			require.NoError(t, err)

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			wc := &whip.Client{
				HTTPClient: hc,
				URL:        u,
				Log:        test.NilLogger,
			}

			writerDone := make(chan struct{})

			go func() {
				defer close(writerDone)

				// When testing for gopCache, start pushing packets before the client connects
				if !ca.gopCache {
					str.WaitRunningReader()
				}

				for i, u := range ca.unit {
					r := reflect.New(reflect.TypeOf(u).Elem())
					r.Elem().Set(reflect.ValueOf(u).Elem())

					// When testing for gopCache, wait until half-way before pushing the rest of segments.
					if i == len(ca.unit)/2 && ca.gopCache {
						str.WaitRunningReader()
					}

					if g, ok := r.Interface().(*unit.Generic); ok {
						clone := *g.RTPPackets[0]
						str.WriteRTPPacket(desc.Medias[0], desc.Medias[0].Formats[0], &clone, time.Time{}, 0)
					} else {
						str.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], r.Interface().(unit.Unit))
					}
				}
			}()

			tracks, err := wc.Read(context.Background())
			require.NoError(t, err)
			defer checkClose(t, wc.Close)

			done := make(chan struct{})

			tracks[0].OnPacketRTP = func(pkt *rtp.Packet) {
				select {
				case <-done:
				default:
					require.Equal(t, ca.outRTPPayload, pkt.Payload)
					close(done)
				}
			}

			wc.StartReading()

			<-writerDone
			<-done
		})
	}
}

func TestServerReadNotFound(t *testing.T) {
	pm := &test.PathManager{
		FindPathConfImpl: func(_ defs.PathFindPathConfReq) (*conf.Path, error) {
			return &conf.Path{}, nil
		},
		AddReaderImpl: func(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			return nil, nil, defs.PathNoOnePublishingError{}
		},
	}

	s := &Server{
		Address:               "127.0.0.1:8886",
		Encryption:            false,
		ServerKey:             "",
		ServerCert:            "",
		AllowOrigin:           "",
		TrustedProxies:        conf.IPNetworks{},
		ReadTimeout:           conf.Duration(10 * time.Second),
		LocalUDPAddress:       "127.0.0.1:8887",
		LocalTCPAddress:       "127.0.0.1:8887",
		IPsFromInterfaces:     true,
		IPsFromInterfacesList: []string{},
		AdditionalHosts:       []string{},
		ICEServers:            []conf.WebRTCICEServer{},
		HandshakeTimeout:      conf.Duration(10 * time.Second),
		TrackGatherTimeout:    conf.Duration(2 * time.Second),
		STUNGatherTimeout:     conf.Duration(5 * time.Second),
		ExternalCmdPool:       nil,
		PathManager:           pm,
		Parent:                test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	pc, err := pwebrtc.NewPeerConnection(pwebrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(pwebrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost,
		"http://myuser:mypass@localhost:8886/nonexisting/whep", bytes.NewReader([]byte(offer.SDP)))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/sdp")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestServerPatchNotFound(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	pc, err := pwebrtc.NewPeerConnection(pwebrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close() //nolint:errcheck

	_, err = pc.AddTransceiverFromKind(pwebrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	frag, err := whip.ICEFragmentMarshal(offer.SDP, []*pwebrtc.ICECandidateInit{{
		Candidate:     "mycandidate",
		SDPMLineIndex: uint16Ptr(0),
	}})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch,
		"http://localhost:8886/nonexisting/whep/"+uuid.UUID{}.String(), bytes.NewReader(frag))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestServerDeleteNotFound(t *testing.T) {
	s := initializeTestServer(t)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodDelete,
		"http://localhost:8886/nonexisting/whep/"+uuid.UUID{}.String(), nil)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestICEServerNoClientOnly(t *testing.T) {
	s := &Server{
		ICEServers: []conf.WebRTCICEServer{
			{
				URL:      "turn:turn.example.com:1234",
				Username: "user",
				Password: "passwrd",
			},
		},
	}
	clientICEServers, err := s.generateICEServers(true)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(clientICEServers))
	serverICEServers, err := s.generateICEServers(false)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(serverICEServers))
}

func TestICEServerClientOnly(t *testing.T) {
	s := &Server{
		ICEServers: []conf.WebRTCICEServer{
			{
				URL:        "turn:turn.example.com:1234",
				Username:   "user",
				Password:   "passwrd",
				ClientOnly: true,
			},
		},
	}
	clientICEServers, err := s.generateICEServers(true)
	require.NoError(t, err)
	require.Equal(t, len(s.ICEServers), len(clientICEServers))
	serverICEServers, err := s.generateICEServers(false)
	require.NoError(t, err)
	require.Empty(t, serverICEServers)
}
