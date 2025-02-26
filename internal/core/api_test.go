//nolint:dupl,lll
package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/test"
)

func checkClose(t *testing.T, closeFunc func() error) {
	require.NoError(t, closeFunc())
}

func httpRequest(t *testing.T, hc *http.Client, method string, ur string, in interface{}, out interface{}) {
	buf := func() io.Reader {
		if in == nil {
			return nil
		}

		byts, err := json.Marshal(in)
		require.NoError(t, err)

		return bytes.NewBuffer(byts)
	}()

	req, err := http.NewRequest(method, ur, buf)
	require.NoError(t, err)

	res, err := hc.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("bad status code: %d", res.StatusCode)
	}

	if out == nil {
		return
	}

	err = json.NewDecoder(res.Body).Decode(out)
	require.NoError(t, err)
}

func checkError(t *testing.T, msg string, body io.Reader) {
	var resErr map[string]interface{}
	err := json.NewDecoder(body).Decode(&resErr)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"error": msg}, resErr)
}

func TestAPIPathsList(t *testing.T) {
	type pathSource struct {
		Type string `json:"type"`
	}

	type path struct {
		Name          string     `json:"name"`
		Source        pathSource `json:"source"`
		Ready         bool       `json:"ready"`
		Tracks        []string   `json:"tracks"`
		BytesReceived uint64     `json:"bytesReceived"`
		BytesSent     uint64     `json:"bytesSent"`
	}

	type pathList struct {
		ItemCount int    `json:"itemCount"`
		PageCount int    `json:"pageCount"`
		Items     []path `json:"items"`
	}

	t.Run("rtsp session", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		media0 := test.UniqueMediaH264()

		source := gortsplib.Client{}
		err := source.StartRecording(
			"rtsp://localhost:8554/mypath",
			&description.Session{Medias: []*description.Media{
				media0,
				test.MediaMPEG4Audio,
			}})
		require.NoError(t, err)
		defer source.Close()

		err = source.WritePacketRTP(media0, &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: 96,
			},
			Payload: []byte{5, 1, 2, 3, 4},
		})
		require.NoError(t, err)

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspSession",
				},
				Ready:         true,
				Tracks:        []string{"H264", "MPEG-4 Audio"},
				BytesReceived: 17,
			}},
		}, out)
	})

	t.Run("rtsps session", func(t *testing.T) {
		serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
		require.NoError(t, err)
		defer os.Remove(serverCertFpath)

		serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
		require.NoError(t, err)
		defer os.Remove(serverKeyFpath)

		p, ok := newInstance("api: yes\n" +
			"rtspEncryption: optional\n" +
			"rtspServerCert: " + serverCertFpath + "\n" +
			"rtspServerKey: " + serverKeyFpath + "\n" +
			"paths:\n" +
			"  mypath:\n")
		require.Equal(t, true, ok)
		defer p.Close()

		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		source := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
		err = source.StartRecording("rtsps://localhost:8322/mypath",
			&description.Session{Medias: []*description.Media{
				test.UniqueMediaH264(),
				test.UniqueMediaMPEG4Audio(),
			}})
		require.NoError(t, err)
		defer source.Close()

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspsSession",
				},
				Ready:  true,
				Tracks: []string{"H264", "MPEG-4 Audio"},
			}},
		}, out)
	})

	t.Run("rtsp source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: rtsp://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtspSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})

	t.Run("rtmp source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: rtmp://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "rtmpSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})

	t.Run("hls source", func(t *testing.T) {
		p, ok := newInstance("api: yes\n" +
			"paths:\n" +
			"  mypath:\n" +
			"    source: http://127.0.0.1:1234/mypath\n" +
			"    sourceOnDemand: yes\n")
		require.Equal(t, true, ok)
		defer p.Close()

		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		hc := &http.Client{Transport: tr}

		var out pathList
		httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/list", nil, &out)
		require.Equal(t, pathList{
			ItemCount: 1,
			PageCount: 1,
			Items: []path{{
				Name: "mypath",
				Source: pathSource{
					Type: "hlsSource",
				},
				Ready:  false,
				Tracks: []string{},
			}},
		}, out)
	})
}

func TestAPIPathsGet(t *testing.T) {
	p, ok := newInstance("api: yes\n" +
		"paths:\n" +
		"  all_others:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	for _, ca := range []string{"ok", "ok-nested", "not found"} {
		t.Run(ca, func(t *testing.T) {
			type pathSource struct {
				Type string `json:"type"`
			}

			type path struct {
				Name          string     `json:"name"`
				Source        pathSource `json:"source"`
				Ready         bool       `json:"Ready"`
				Tracks        []string   `json:"tracks"`
				BytesReceived uint64     `json:"bytesReceived"`
				BytesSent     uint64     `json:"bytesSent"`
			}

			var pathName string

			switch ca {
			case "ok":
				pathName = "mypath"
			case "ok-nested":
				pathName = "my/nested/path"
			case "not found":
				pathName = "nonexisting"
			}

			if ca == "ok" || ca == "ok-nested" {
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/"+pathName,
					&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
				require.NoError(t, err)
				defer source.Close()

				var out path
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/paths/get/"+pathName, nil, &out)
				require.Equal(t, path{
					Name: pathName,
					Source: pathSource{
						Type: "rtspSession",
					},
					Ready:  true,
					Tracks: []string{"H264"},
				}, out)
			} else {
				res, err := hc.Get("http://localhost:9997/v3/paths/get/" + pathName)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)
				checkError(t, "path not found", res.Body)
			}
		})
	}
}

func TestAPIProtocolListGet(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp conns",
		"rtsp sessions",
		"rtsps conns",
		"rtsps sessions",
		"rtmp",
		"rtmps",
		"hls",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			switch ca {
			case "rtsps conns", "rtsps sessions":
				conf += "rtspTransports: [tcp]\n" +
					"rtspEncryption: strict\n" +
					"rtspServerCert: " + serverCertFpath + "\n" +
					"rtspServerKey: " + serverKeyFpath + "\n"

			case "rtmps":
				conf += "rtmpEncryption: strict\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			medi := test.UniqueMediaH264()

			switch ca { //nolint:dupl
			case "rtsp conns", "rtsp sessions":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath?key=val",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps conns", "rtsps sessions":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath?key=val",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp", "rtmps":
				var port string
				if ca == "rtmp" {
					port = "1935"
				} else {
					port = "1936"
				}

				u, err := url.Parse("rtmp://127.0.0.1:" + port + "/mypath?key=val")
				require.NoError(t, err)

				nconn, err := func() (net.Conn, error) {
					if ca == "rtmp" {
						return net.Dial("tcp", u.Host)
					}
					return tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
				}()
				require.NoError(t, err)
				defer nconn.Close()

				conn, err := rtmp.NewClientConn(nconn, u, true)
				require.NoError(t, err)

				w, err := rtmp.NewWriter(conn, test.FormatH264, nil)
				require.NoError(t, err)

				err = w.WriteH264(2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)

			case "hls":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				go func() {
					time.Sleep(500 * time.Millisecond)

					for i := 0; i < 3; i++ {
						/*source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								testSPS,
								0x05,
							},
						})

						[]byte{ // 1920x1080 baseline
							0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02,
							0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04,
							0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20,
						},*/

						err := source.WritePacketRTP(medi, &rtp.Packet{
							Header: rtp.Header{
								Version:        2,
								Marker:         true,
								PayloadType:    96,
								SequenceNumber: 123 + uint16(i),
								Timestamp:      45343 + uint32(i)*90000,
								SSRC:           563423,
							},
							Payload: []byte{
								// testSPS,
								0x05,
							},
						})
						require.NoError(t, err)
					}
				}()

				func() {
					res, err := hc.Get("http://localhost:8888/mypath/index.m3u8")
					require.NoError(t, err)
					defer res.Body.Close()
					require.Equal(t, 200, res.StatusCode)
				}()

			case "webrtc":
				source := gortsplib.Client{}
				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

				u, err := url.Parse("http://localhost:8889/mypath/whep?key=val")
				require.NoError(t, err)

				go func() {
					time.Sleep(500 * time.Millisecond)

					err2 := source.WritePacketRTP(medi, &rtp.Packet{
						Header: rtp.Header{
							Version:        2,
							Marker:         true,
							PayloadType:    96,
							SequenceNumber: 123,
							Timestamp:      45343,
							SSRC:           563423,
						},
						Payload: []byte{5, 1, 2, 3, 4},
					})
					require.NoError(t, err2)
				}()

				c := &whip.Client{
					HTTPClient: hc,
					URL:        u,
					Log:        test.NilLogger,
				}

				_, err = c.Read(context.Background())
				require.NoError(t, err)
				defer checkClose(t, c.Close)

			case "srt":
				conf := srt.DefaultConfig()
				conf.StreamId = "publish:mypath:::key=val"

				conn, err := srt.Dial("srt", "localhost:8890", conf)
				require.NoError(t, err)
				defer conn.Close()

				track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}

				bw := bufio.NewWriter(conn)
				w := mpegts.NewWriter(bw, []*mpegts.Track{track})
				require.NoError(t, err)

				err = w.WriteH264(track, 0, 0, [][]byte{{1}})
				require.NoError(t, err)

				err = bw.Flush()
				require.NoError(t, err)

				time.Sleep(500 * time.Millisecond)
			}

			var pa string
			switch ca {
			case "rtsp conns":
				pa = "rtspconns"

			case "rtsp sessions":
				pa = "rtspsessions"

			case "rtsps conns":
				pa = "rtspsconns"

			case "rtsps sessions":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "rtmps":
				pa = "rtmpsconns"

			case "hls":
				pa = "hlsmuxers"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			var out1 interface{}
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out1)

			switch ca {
			case "rtsp conns":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived": out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesReceived"],
							"bytesSent":     out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"remoteAddr":    out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"session":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["session"],
						},
					},
				}, out1)

			case "rtsp sessions":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived":       float64(0),
							"bytesSent":           out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":             out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":                  out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"path":                "mypath",
							"query":               "key=val",
							"remoteAddr":          out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"state":               "publish",
							"transport":           "UDP",
							"rtpPacketsReceived":  float64(0),
							"rtpPacketsSent":      float64(0),
							"rtpPacketsLost":      float64(0),
							"rtpPacketsInError":   float64(0),
							"rtpPacketsJitter":    float64(0),
							"rtcpPacketsReceived": float64(0),
							"rtcpPacketsSent":     float64(0),
							"rtcpPacketsInError":  float64(0),
						},
					},
				}, out1)

			case "rtsps conns":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived": out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesReceived"],
							"bytesSent":     out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"remoteAddr":    out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"session":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["session"],
						},
					},
				}, out1)

			case "rtsps sessions":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived":       float64(0),
							"bytesSent":           out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":             out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":                  out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"path":                "mypath",
							"query":               "key=val",
							"remoteAddr":          out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"state":               "publish",
							"transport":           "TCP",
							"rtpPacketsReceived":  float64(0),
							"rtpPacketsSent":      float64(0),
							"rtpPacketsLost":      float64(0),
							"rtpPacketsInError":   float64(0),
							"rtpPacketsJitter":    float64(0),
							"rtcpPacketsReceived": float64(0),
							"rtcpPacketsSent":     float64(0),
							"rtcpPacketsInError":  float64(0),
						},
					},
				}, out1)

			case "rtmp":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived": out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesReceived"],
							"bytesSent":     out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"path":          "mypath",
							"query":         "key=val",
							"remoteAddr":    out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"state":         "publish",
						},
					},
				}, out1)

			case "rtmps":
				require.Equal(t, map[string]interface{}{
					"pageCount": float64(1),
					"itemCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived": out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesReceived"],
							"bytesSent":     out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"path":          "mypath",
							"query":         "key=val",
							"remoteAddr":    out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"state":         "publish",
						},
					},
				}, out1)

			case "hls":
				require.Equal(t, map[string]interface{}{
					"itemCount": float64(1),
					"pageCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesSent":   out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":     out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"lastRequest": out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["lastRequest"],
							"path":        "mypath",
						},
					},
				}, out1)

			case "webrtc":
				require.Equal(t, map[string]interface{}{
					"itemCount": float64(1),
					"pageCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"bytesReceived":             out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesReceived"],
							"bytesSent":                 out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["bytesSent"],
							"created":                   out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":                        out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"localCandidate":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["localCandidate"],
							"path":                      "mypath",
							"peerConnectionEstablished": true,
							"query":                     "key=val",
							"remoteAddr":                out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"remoteCandidate":           out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteCandidate"],
							"state":                     "read",
						},
					},
				}, out1)

			case "srt":
				require.Equal(t, map[string]interface{}{
					"itemCount": float64(1),
					"pageCount": float64(1),
					"items": []interface{}{
						map[string]interface{}{
							"byteMSS":                       float64(1500),
							"bytesAvailReceiveBuf":          float64(0),
							"bytesAvailSendBuf":             float64(0),
							"bytesReceiveBuf":               float64(0),
							"bytesReceived":                 float64(628),
							"bytesReceivedBelated":          float64(0),
							"bytesReceivedDrop":             float64(0),
							"bytesReceivedLoss":             float64(0),
							"bytesReceivedRetrans":          float64(0),
							"bytesReceivedUndecrypt":        float64(0),
							"bytesReceivedUnique":           float64(628),
							"bytesRetrans":                  float64(0),
							"bytesSendBuf":                  float64(0),
							"bytesSendDrop":                 float64(0),
							"bytesSent":                     float64(0),
							"bytesSentUnique":               float64(0),
							"created":                       out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["created"],
							"id":                            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"],
							"mbpsLinkCapacity":              float64(0),
							"mbpsMaxBW":                     float64(-1),
							"mbpsReceiveRate":               float64(0),
							"mbpsSendRate":                  float64(0),
							"msRTT":                         out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["msRTT"],
							"msReceiveBuf":                  float64(0),
							"msReceiveTsbPdDelay":           float64(120),
							"msSendBuf":                     float64(0),
							"msSendTsbPdDelay":              float64(120),
							"packetsFlightSize":             float64(0),
							"packetsFlowWindow":             float64(25600),
							"packetsReceiveBuf":             float64(0),
							"packetsReceived":               float64(1),
							"packetsReceivedACK":            out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["packetsReceivedACK"],
							"packetsReceivedAvgBelatedTime": float64(0),
							"packetsReceivedBelated":        float64(0),
							"packetsReceivedDrop":           float64(0),
							"packetsReceivedKM":             float64(0),
							"packetsReceivedLoss":           float64(0),
							"packetsReceivedLossRate":       float64(0),
							"packetsReceivedNAK":            float64(0),
							"packetsReceivedRetrans":        float64(0),
							"packetsReceivedUndecrypt":      float64(0),
							"packetsReceivedUnique":         float64(1),
							"packetsReorderTolerance":       float64(0),
							"packetsRetrans":                float64(0),
							"packetsSendBuf":                float64(0),
							"packetsSendDrop":               float64(0),
							"packetsSendLoss":               float64(0),
							"packetsSendLossRate":           float64(0),
							"packetsSent":                   float64(0),
							"packetsSentACK":                out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["packetsSentACK"],
							"packetsSentKM":                 float64(0),
							"packetsSentNAK":                float64(0),
							"packetsSentUnique":             float64(0),
							"path":                          "mypath",
							"query":                         "key=val",
							"remoteAddr":                    out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["remoteAddr"],
							"state":                         "publish",
							"usPacketsSendPeriod":           float64(10.967254638671875),
							"usSndDuration":                 float64(0),
						},
					},
				}, out1)
			}

			var out2 interface{}

			if ca == "hls" {
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/get/"+
					out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["path"].(string),
					nil, &out2)
			} else {
				httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/get/"+
					out1.(map[string]interface{})["items"].([]interface{})[0].(map[string]interface{})["id"].(string),
					nil, &out2)
			}

			require.Equal(t, out1.(map[string]interface{})["items"].([]interface{})[0], out2)
		})
	}
}

func TestAPIProtocolGetNotFound(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp conns",
		"rtsp sessions",
		"rtsps conns",
		"rtsps sessions",
		"rtmp",
		"rtmps",
		"hls",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			switch ca {
			case "rtsps conns", "rtsps sessions":
				conf += "rtspTransports: [tcp]\n" +
					"rtspEncryption: strict\n" +
					"rtspServerCert: " + serverCertFpath + "\n" +
					"rtspServerKey: " + serverKeyFpath + "\n"

			case "rtmps":
				conf += "rtmpEncryption: strict\n" +
					"rtmpServerCert: " + serverCertFpath + "\n" +
					"rtmpServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			var pa string
			switch ca {
			case "rtsp conns":
				pa = "rtspconns"

			case "rtsp sessions":
				pa = "rtspsessions"

			case "rtsps conns":
				pa = "rtspsconns"

			case "rtsps sessions":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "rtmps":
				pa = "rtmpsconns"

			case "hls":
				pa = "hlsmuxers"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			func() {
				req, err := http.NewRequest(http.MethodGet, "http://localhost:9997/v3/"+pa+"/get/"+uuid.New().String(), nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)

				switch ca {
				case "rtsp conns", "rtsps conns", "rtmp", "rtmps", "srt":
					checkError(t, "connection not found", res.Body)

				case "rtsp sessions", "rtsps sessions", "webrtc":
					checkError(t, "session not found", res.Body)

				case "hls":
					checkError(t, "muxer not found", res.Body)
				}
			}()
		})
	}
}

func TestAPIProtocolKick(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp",
		"rtsps",
		"rtmp",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			if ca == "rtsps" {
				conf += "rtspTransports: [tcp]\n" +
					"rtspEncryption: strict\n" +
					"rtspServerCert: " + serverCertFpath + "\n" +
					"rtspServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			medi := test.MediaH264

			switch ca {
			case "rtsp":
				source := gortsplib.Client{}

				err := source.StartRecording("rtsp://localhost:8554/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtsps":
				source := gortsplib.Client{
					TLSConfig: &tls.Config{InsecureSkipVerify: true},
				}

				err := source.StartRecording("rtsps://localhost:8322/mypath",
					&description.Session{Medias: []*description.Media{medi}})
				require.NoError(t, err)
				defer source.Close()

			case "rtmp":
				u, err := url.Parse("rtmp://localhost:1935/mypath")
				require.NoError(t, err)

				nconn, err := net.Dial("tcp", u.Host)
				require.NoError(t, err)
				defer nconn.Close()

				conn, err := rtmp.NewClientConn(nconn, u, true)
				require.NoError(t, err)

				w, err := rtmp.NewWriter(conn, test.FormatH264, nil)
				require.NoError(t, err)

				err = w.WriteH264(2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
				require.NoError(t, err)

			case "webrtc":
				u, err := url.Parse("http://localhost:8889/mypath/whip")
				require.NoError(t, err)

				c := &whip.Client{
					HTTPClient: hc,
					URL:        u,
					Log:        test.NilLogger,
				}

				track := &webrtc.OutgoingTrack{
					Caps: pwebrtc.RTPCodecCapability{
						MimeType:    pwebrtc.MimeTypeH264,
						ClockRate:   90000,
						SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
					},
				}

				err = c.Publish(context.Background(), []*webrtc.OutgoingTrack{track})
				require.NoError(t, err)
				defer func() {
					require.Error(t, c.Close())
				}()

			case "srt":
				conf := srt.DefaultConfig()
				conf.StreamId = "publish:mypath"

				conn, err := srt.Dial("srt", "localhost:8890", conf)
				require.NoError(t, err)
				defer conn.Close()

				track := &mpegts.Track{
					Codec: &mpegts.CodecH264{},
				}

				bw := bufio.NewWriter(conn)
				w := mpegts.NewWriter(bw, []*mpegts.Track{track})
				require.NoError(t, err)

				err = w.WriteH264(track, 0, 0, [][]byte{{1}})
				require.NoError(t, err)

				err = bw.Flush()
				require.NoError(t, err)
			}

			var pa string
			switch ca {
			case "rtsp":
				pa = "rtspsessions"

			case "rtsps":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			var out1 struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out1)

			httpRequest(t, hc, http.MethodPost, "http://localhost:9997/v3/"+pa+"/kick/"+out1.Items[0].ID, nil, nil)

			var out2 struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			httpRequest(t, hc, http.MethodGet, "http://localhost:9997/v3/"+pa+"/list", nil, &out2)
			require.Empty(t, out2.Items)
		})
	}
}

func TestAPIProtocolKickNotFound(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	for _, ca := range []string{
		"rtsp",
		"rtsps",
		"rtmp",
		"webrtc",
		"srt",
	} {
		t.Run(ca, func(t *testing.T) {
			conf := "api: yes\n"

			if ca == "rtsps" {
				conf += "rtspTransports: [tcp]\n" +
					"rtspEncryption: strict\n" +
					"rtspServerCert: " + serverCertFpath + "\n" +
					"rtspServerKey: " + serverKeyFpath + "\n"
			}

			conf += "paths:\n" +
				"  all_others:\n"

			p, ok := newInstance(conf)
			require.Equal(t, true, ok)
			defer p.Close()

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc := &http.Client{Transport: tr}

			var pa string
			switch ca {
			case "rtsp":
				pa = "rtspsessions"

			case "rtsps":
				pa = "rtspssessions"

			case "rtmp":
				pa = "rtmpconns"

			case "webrtc":
				pa = "webrtcsessions"

			case "srt":
				pa = "srtconns"
			}

			func() {
				req, err := http.NewRequest(http.MethodPost, "http://localhost:9997/v3/"+pa+"/kick/"+uuid.New().String(), nil)
				require.NoError(t, err)

				res, err := hc.Do(req)
				require.NoError(t, err)
				defer res.Body.Close()

				require.Equal(t, http.StatusNotFound, res.StatusCode)

				switch ca {
				case "rtsp conns", "rtsps conns", "rtmp", "rtmps", "srt":
					checkError(t, "connection not found", res.Body)

				case "rtsp sessions", "rtsps sessions", "webrtc":
					checkError(t, "session not found", res.Body)

				case "hls":
					checkError(t, "muxer not found", res.Body)
				}
			}()
		})
	}
}
