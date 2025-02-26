package hls

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

type dummyPath struct{}

func (pa *dummyPath) Name() string {
	return "teststream"
}

func (pa *dummyPath) SafeConf() *conf.Path {
	return &conf.Path{}
}

func (pa *dummyPath) ExternalCmdEnv() externalcmd.Environment {
	return nil
}

func (pa *dummyPath) StartPublisher(_ defs.PathStartPublisherReq) (*stream.Stream, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (pa *dummyPath) StopPublisher(_ defs.PathStopPublisherReq) {
}

func (pa *dummyPath) RemovePublisher(_ defs.PathRemovePublisherReq) {
}

func (pa *dummyPath) RemoveReader(_ defs.PathRemoveReaderReq) {
}

func TestPreflightRequest(t *testing.T) {
	s := &Server{
		Address:     "127.0.0.1:8888",
		AllowOrigin: "*",
		ReadTimeout: conf.Duration(10 * time.Second),
		Parent:      test.NilLogger,
	}
	err := s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	req, err := http.NewRequest(http.MethodOptions, "http://localhost:8888", nil)
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
	require.Equal(t, "OPTIONS, GET", res.Header.Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Authorization, Range", res.Header.Get("Access-Control-Allow-Headers"))
	require.Equal(t, byts, []byte{})
}

func TestServerNotFound(t *testing.T) {
	for _, ca := range []string{
		"always remux off",
		"always remux on",
	} {
		t.Run(ca, func(t *testing.T) {
			pm := &test.PathManager{
				FindPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return &conf.Path{}, nil
				},
				AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "nonexisting", req.AccessRequest.Name)
					return nil, nil, fmt.Errorf("not found")
				},
			}

			s := &Server{
				Address:         "127.0.0.1:8888",
				Encryption:      false,
				ServerKey:       "",
				ServerCert:      "",
				AlwaysRemux:     ca == "always remux on",
				Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
				SegmentCount:    7,
				SegmentDuration: conf.Duration(1 * time.Second),
				PartDuration:    conf.Duration(200 * time.Millisecond),
				SegmentMaxSize:  50 * 1024 * 1024,
				AllowOrigin:     "",
				TrustedProxies:  conf.IPNetworks{},
				Directory:       "",
				ReadTimeout:     conf.Duration(10 * time.Second),
				PathManager:     pm,
				Parent:          test.NilLogger,
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://myuser:mypass@127.0.0.1:8888/nonexisting/", nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusOK, res.StatusCode)
			}()

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://myuser:mypass@127.0.0.1:8888/nonexisting/index.m3u8", nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()
				require.Equal(t, http.StatusNotFound, res.StatusCode)
			}()
		})
	}
}

func TestServerRead(t *testing.T) {
	t.Run("always remux off", func(t *testing.T) {
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

		pm := &test.PathManager{
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
				return &dummyPath{}, str, nil
			},
		}

		s := &Server{
			Address:         "127.0.0.1:8888",
			Encryption:      false,
			ServerKey:       "",
			ServerCert:      "",
			AlwaysRemux:     false,
			Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
			SegmentCount:    7,
			SegmentDuration: conf.Duration(1 * time.Second),
			PartDuration:    conf.Duration(200 * time.Millisecond),
			SegmentMaxSize:  50 * 1024 * 1024,
			AllowOrigin:     "",
			TrustedProxies:  conf.IPNetworks{},
			Directory:       "",
			ReadTimeout:     conf.Duration(10 * time.Second),
			PathManager:     pm,
			Parent:          test.NilLogger,
		}
		err = s.Initialize()
		require.NoError(t, err)
		defer s.Close()

		c := &gohlslib.Client{
			URI: "http://myuser:mypass@127.0.0.1:8888/teststream/index.m3u8?param=value",
		}

		recv := make(chan struct{})

		c.OnTracks = func(tracks []*gohlslib.Track) error {
			require.Equal(t, []*gohlslib.Track{{
				Codec:     &codecs.H264{},
				ClockRate: 90000,
			}}, tracks)

			c.OnDataH26x(tracks[0], func(pts, dts int64, au [][]byte) {
				require.Equal(t, int64(0), pts)
				require.Equal(t, int64(0), dts)
				require.Equal(t, [][]byte{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 1},
				}, au)
				close(recv)
			})

			return nil
		}

		err = c.Start()
		require.NoError(t, err)

		defer func() { <-c.Wait() }()
		defer c.Close()

		go func() {
			time.Sleep(100 * time.Millisecond)
			for i := 0; i < 4; i++ {
				str.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: int64(i) * 90000,
					},
					AU: [][]byte{
						{5, 1}, // IDR
					},
				})
			}
		}()

		<-recv
	})

	t.Run("always remux on", func(t *testing.T) {
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

		pm := &test.PathManager{
			FindPathConfImpl: func(req defs.PathFindPathConfReq) (*conf.Path, error) {
				require.Equal(t, "teststream", req.AccessRequest.Name)
				require.Equal(t, "param=value", req.AccessRequest.Query)
				require.Equal(t, "myuser", req.AccessRequest.User)
				require.Equal(t, "mypass", req.AccessRequest.Pass)
				return &conf.Path{}, nil
			},
			AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
				require.Equal(t, "teststream", req.AccessRequest.Name)
				require.Equal(t, "", req.AccessRequest.Query)
				return &dummyPath{}, str, nil
			},
		}

		s := &Server{
			Address:         "127.0.0.1:8888",
			Encryption:      false,
			ServerKey:       "",
			ServerCert:      "",
			AlwaysRemux:     true,
			Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
			SegmentCount:    7,
			SegmentDuration: conf.Duration(1 * time.Second),
			PartDuration:    conf.Duration(200 * time.Millisecond),
			SegmentMaxSize:  50 * 1024 * 1024,
			AllowOrigin:     "",
			TrustedProxies:  conf.IPNetworks{},
			Directory:       "",
			ReadTimeout:     conf.Duration(10 * time.Second),
			PathManager:     pm,
			Parent:          test.NilLogger,
		}
		err = s.Initialize()
		require.NoError(t, err)
		defer s.Close()

		s.PathReady(&dummyPath{})

		str.WaitRunningReader()

		for i := 0; i < 4; i++ {
			str.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
				Base: unit.Base{
					NTP: time.Time{},
					PTS: int64(i) * 90000,
				},
				AU: [][]byte{
					{5, 1}, // IDR
				},
			})
		}

		c := &gohlslib.Client{
			URI: "http://myuser:mypass@127.0.0.1:8888/teststream/index.m3u8?param=value",
		}

		recv := make(chan struct{})

		c.OnTracks = func(tracks []*gohlslib.Track) error {
			require.Equal(t, []*gohlslib.Track{{
				Codec:     &codecs.H264{},
				ClockRate: 90000,
			}}, tracks)

			c.OnDataH26x(tracks[0], func(pts, dts int64, au [][]byte) {
				require.Equal(t, int64(0), pts)
				require.Equal(t, int64(0), dts)
				require.Equal(t, [][]byte{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 1},
				}, au)
				close(recv)
			})

			return nil
		}

		err = c.Start()
		require.NoError(t, err)
		defer func() { <-c.Wait() }()
		defer c.Close()

		<-recv
	})
}

func TestDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-playback")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

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

	pm := &test.PathManager{
		AddReaderImpl: func(_ defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
			return &dummyPath{}, str, nil
		},
	}

	s := &Server{
		Address:         "127.0.0.1:8888",
		Encryption:      false,
		ServerKey:       "",
		ServerCert:      "",
		AlwaysRemux:     true,
		Variant:         conf.HLSVariant(gohlslib.MuxerVariantMPEGTS),
		SegmentCount:    7,
		SegmentDuration: conf.Duration(1 * time.Second),
		PartDuration:    conf.Duration(200 * time.Millisecond),
		SegmentMaxSize:  50 * 1024 * 1024,
		AllowOrigin:     "",
		TrustedProxies:  conf.IPNetworks{},
		Directory:       filepath.Join(dir, "mydir"),
		ReadTimeout:     conf.Duration(10 * time.Second),
		PathManager:     pm,
		Parent:          test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	s.PathReady(&dummyPath{})

	time.Sleep(100 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "mydir", "teststream"))
	require.NoError(t, err)
}
