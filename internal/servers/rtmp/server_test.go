package rtmp

import (
	"crypto/tls"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
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
	for _, encrypt := range []string{
		"plain",
		"tls",
	} {
		t.Run(encrypt, func(t *testing.T) {
			var serverCertFpath string
			var serverKeyFpath string

			if encrypt == "tls" {
				var err error
				serverCertFpath, err = test.CreateTempFile(test.TLSCertPub)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err = test.CreateTempFile(test.TLSCertKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)
			}

			path := &dummyPath{
				streamCreated: make(chan struct{}),
			}

			pathManager := &test.PathManager{
				AddPublisherImpl: func(req defs.PathAddPublisherReq) (defs.Path, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.User)
					require.Equal(t, "mypass", req.AccessRequest.Pass)
					return path, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1935",
				ReadTimeout:         conf.Duration(10 * time.Second),
				WriteTimeout:        conf.Duration(10 * time.Second),
				IsTLS:               encrypt == "tls",
				ServerCert:          serverCertFpath,
				ServerKey:           serverKeyFpath,
				RTSPAddress:         "",
				RunOnConnect:        "",
				RunOnConnectRestart: false,
				RunOnDisconnect:     "",
				ExternalCmdPool:     nil,
				PathManager:         pathManager,
				Parent:              test.NilLogger,
			}
			err := s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=myuser&pass=mypass&param=value")
			require.NoError(t, err)

			nconn, err := func() (net.Conn, error) {
				if encrypt == "plain" {
					return net.Dial("tcp", u.Host)
				}
				return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
			}()
			require.NoError(t, err)
			defer nconn.Close()

			conn, err := rtmp.NewClientConn(nconn, u, true)
			require.NoError(t, err)

			w, err := rtmp.NewWriter(conn, test.FormatH264, test.FormatMPEG4Audio)
			require.NoError(t, err)

			err = w.WriteH264(
				2*time.Second, 2*time.Second, [][]byte{
					{5, 2, 3, 4},
				})
			require.NoError(t, err)

			<-path.streamCreated

			recv := make(chan struct{})

			reader := test.NilLogger

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

			err = w.WriteH264(
				3*time.Second, 3*time.Second, [][]byte{
					{5, 2, 3, 4},
				})
			require.NoError(t, err)

			<-recv
		})
	}
}

func TestServerRead(t *testing.T) {
	for _, encrypt := range []string{
		"plain",
		"tls",
	} {
		t.Run(encrypt, func(t *testing.T) {
			var serverCertFpath string
			var serverKeyFpath string

			if encrypt == "tls" {
				var err error
				serverCertFpath, err = test.CreateTempFile(test.TLSCertPub)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err = test.CreateTempFile(test.TLSCertKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)
			}
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
				AddReaderImpl: func(req defs.PathAddReaderReq) (defs.Path, *stream.Stream, error) {
					require.Equal(t, "teststream", req.AccessRequest.Name)
					require.Equal(t, "user=myuser&pass=mypass&param=value", req.AccessRequest.Query)
					require.Equal(t, "myuser", req.AccessRequest.User)
					require.Equal(t, "mypass", req.AccessRequest.Pass)
					return path, path.stream, nil
				},
			}

			s := &Server{
				Address:             "127.0.0.1:1935",
				ReadTimeout:         conf.Duration(10 * time.Second),
				WriteTimeout:        conf.Duration(10 * time.Second),
				IsTLS:               encrypt == "tls",
				ServerCert:          serverCertFpath,
				ServerKey:           serverKeyFpath,
				RTSPAddress:         "",
				RunOnConnect:        "",
				RunOnConnectRestart: false,
				RunOnDisconnect:     "",
				ExternalCmdPool:     nil,
				PathManager:         pathManager,
				Parent:              test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("rtmp://127.0.0.1:1935/teststream?user=myuser&pass=mypass&param=value")
			require.NoError(t, err)

			nconn, err := func() (net.Conn, error) {
				if encrypt == "plain" {
					return net.Dial("tcp", u.Host)
				}
				return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
			}()
			require.NoError(t, err)
			defer nconn.Close()

			go func() {
				str.WaitRunningReader()

				str.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})

				str.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: 2 * 90000,
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})

				str.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.H264{
					Base: unit.Base{
						NTP: time.Time{},
						PTS: 3 * 90000,
					},
					AU: [][]byte{
						{5, 2, 3, 4}, // IDR
					},
				})
			}()

			conn, err := rtmp.NewClientConn(nconn, u, false)
			require.NoError(t, err)

			r, err := rtmp.NewReader(conn)
			require.NoError(t, err)

			tracks := r.Tracks()
			require.Equal(t, []format.Format{test.FormatH264}, tracks)

			r.OnDataH264(tracks[0].(*format.H264), func(_ time.Duration, au [][]byte) {
				require.Equal(t, [][]byte{
					test.FormatH264.SPS,
					test.FormatH264.PPS,
					{5, 2, 3, 4},
				}, au)
			})

			err = r.Read()
			require.NoError(t, err)
		})
	}
}
