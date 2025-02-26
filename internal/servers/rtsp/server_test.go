package rtsp

import (
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	rtspauth "github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

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

func TestServerPublish(t *testing.T) {
	path := &dummyPath{
		streamCreated: make(chan struct{}),
	}

	pathManager := &test.PathManager{
		AddPublisherImpl: func(req defs.PathAddPublisherReq) (defs.Path, error) {
			if req.AccessRequest.User == "" && req.AccessRequest.Pass == "" {
				return nil, auth.Error{Message: "", AskCredentials: true}
			}
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.User)
			require.Equal(t, "mypass", req.AccessRequest.Pass)
			return path, nil
		},
	}

	s := &Server{
		Address:        "127.0.0.1:8557",
		AuthMethods:    []rtspauth.VerifyMethod{rtspauth.VerifyMethodBasic},
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		WriteQueueSize: 512,
		Transports:     conf.RTSPTransports{gortsplib.TransportTCP: {}},
		PathManager:    pathManager,
		Parent:         test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	source := gortsplib.Client{}

	media0 := test.UniqueMediaH264()

	err = source.StartRecording(
		"rtsp://myuser:mypass@127.0.0.1:8557/teststream?param=value",
		&description.Session{Medias: []*description.Media{media0}})
	require.NoError(t, err)
	defer source.Close()

	<-path.streamCreated

	reader := test.NilLogger

	recv := make(chan struct{})

	path.stream.AddReader(
		reader,
		path.stream.Desc().Medias[0],
		path.stream.Desc().Medias[0].Formats[0],
		func(u unit.Unit) error {
			require.Equal(t, [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5, 2, 3, 4},
			}, u.(*unit.H264).AU)
			close(recv)
			return nil
		})

	path.stream.StartReader(reader)
	defer path.stream.RemoveReader(reader)

	err = source.WritePacketRTP(media0, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Marker:         true,
			PayloadType:    96,
			SequenceNumber: 123,
			Timestamp:      45343,
			SSRC:           563423,
		},
		Payload: []byte{5, 2, 3, 4},
	})
	require.NoError(t, err)

	<-recv
}

func TestServerRead(t *testing.T) {
	desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

	str, err := stream.New(
		512,
		1460,
		desc,
		true,
		test.NilLogger,
		false,
	)
	require.NoError(t, err)

	path := &dummyPath{stream: str}

	pathManager := &test.PathManager{
		DescribeImpl: func(req defs.PathDescribeReq) defs.PathDescribeRes {
			if req.AccessRequest.User == "" && req.AccessRequest.Pass == "" {
				return defs.PathDescribeRes{Err: auth.Error{Message: "", AskCredentials: true}}
			}
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.User)
			require.Equal(t, "mypass", req.AccessRequest.Pass)

			return defs.PathDescribeRes{
				Path:   path,
				Stream: path.stream,
				Err:    nil,
			}
		},
		AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			require.Equal(t, "teststream", req.AccessRequest.Name)
			require.Equal(t, "param=value", req.AccessRequest.Query)
			require.Equal(t, "myuser", req.AccessRequest.User)
			require.Equal(t, "mypass", req.AccessRequest.Pass)
			return path, path.stream, nil
		},
	}

	s := &Server{
		Address:        "127.0.0.1:8557",
		AuthMethods:    []rtspauth.VerifyMethod{rtspauth.VerifyMethodBasic},
		ReadTimeout:    conf.Duration(10 * time.Second),
		WriteTimeout:   conf.Duration(10 * time.Second),
		WriteQueueSize: 512,
		Transports:     conf.RTSPTransports{gortsplib.TransportTCP: {}},
		PathManager:    pathManager,
		Parent:         test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	reader := gortsplib.Client{}

	u, err := base.ParseURL("rtsp://myuser:mypass@127.0.0.1:8557/teststream?param=value")
	require.NoError(t, err)

	err = reader.Start(u.Scheme, u.Host)
	require.NoError(t, err)
	defer reader.Close()

	desc2, _, err := reader.Describe(u)
	require.NoError(t, err)

	err = reader.SetupAll(desc2.BaseURL, desc2.Medias)
	require.NoError(t, err)

	recv := make(chan struct{})

	reader.OnPacketRTPAny(func(_ *description.Media, _ format.Format, p *rtp.Packet) {
		require.Equal(t, &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				PayloadType:    96,
				SequenceNumber: p.SequenceNumber,
				Timestamp:      p.Timestamp,
				SSRC:           p.SSRC,
				CSRC:           []uint32{},
			},
			Payload: []byte{
				0x18, 0x00, 0x19, 0x67, 0x42, 0xc0, 0x28, 0xd9,
				0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00,
				0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0,
				0x3c, 0x60, 0xc9, 0x20, 0x00, 0x04, 0x08, 0x06,
				0x07, 0x08, 0x00, 0x04, 0x05, 0x02, 0x03, 0x04,
			},
		}, p)
		close(recv)
	})

	_, err = reader.Play(nil)
	require.NoError(t, err)

	str.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
		Base: unit.Base{
			NTP: time.Time{},
		},
		AU: [][]byte{
			{5, 2, 3, 4}, // IDR
		},
	})

	<-recv
}

func TestServerRedirect(t *testing.T) {
	for _, ca := range []string{"relative", "absolute"} {
		t.Run(ca, func(t *testing.T) {
			desc := &description.Session{Medias: []*description.Media{test.MediaH264}}

			str, err := stream.New(
				512,
				1460,
				desc,
				true,
				test.NilLogger,
			)
			require.NoError(t, err)

			path := &dummyPath{stream: str}

			pathManager := &test.PathManager{
				DescribeImpl: func(req defs.PathDescribeReq) defs.PathDescribeRes {
					if req.AccessRequest.Name == "path1" {
						if ca == "relative" {
							return defs.PathDescribeRes{
								Redirect: "/path2",
							}
						}
						return defs.PathDescribeRes{
							Redirect: "rtsp://localhost:8557/path2",
						}
					}

					if req.AccessRequest.User == "" && req.AccessRequest.Pass == "" {
						return defs.PathDescribeRes{Err: auth.Error{Message: "", AskCredentials: true}}
					}

					require.Equal(t, "path2", req.AccessRequest.Name)
					require.Equal(t, "", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.User)
					require.Equal(t, "mypass", req.AccessRequest.Pass)

					return defs.PathDescribeRes{
						Path:   path,
						Stream: path.stream,
						Err:    nil,
					}
				},
			}

			s := &Server{
				Address:        "127.0.0.1:8557",
				AuthMethods:    []rtspauth.VerifyMethod{rtspauth.VerifyMethodBasic},
				ReadTimeout:    conf.Duration(10 * time.Second),
				WriteTimeout:   conf.Duration(10 * time.Second),
				WriteQueueSize: 512,
				Transports:     conf.RTSPTransports{gortsplib.TransportTCP: {}},
				PathManager:    pathManager,
				Parent:         test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			reader := gortsplib.Client{}

			u, err := base.ParseURL("rtsp://myuser:mypass@127.0.0.1:8557/path1?param=value")
			require.NoError(t, err)

			err = reader.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer reader.Close()

			desc2, _, err := reader.Describe(u)
			require.NoError(t, err)

			require.Equal(t, desc.Medias[0].Formats, desc2.Medias[0].Formats)
		})
	}
}
