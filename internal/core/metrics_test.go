package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	srt "github.com/datarhei/gosrt"
	"github.com/pion/rtp"
	pwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/protocols/rtmp"
	"github.com/bluenviron/mediamtx/internal/protocols/webrtc"
	"github.com/bluenviron/mediamtx/internal/protocols/whip"
	"github.com/bluenviron/mediamtx/internal/test"
)

func httpPullFile(t *testing.T, hc *http.Client, u string) []byte {
	res, err := hc.Get(u)
	require.NoError(t, err)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("bad status code: %v", res.StatusCode)
	}

	byts, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	return byts
}

func TestMetrics(t *testing.T) {
	serverCertFpath, err := test.CreateTempFile(test.TLSCertPub)
	require.NoError(t, err)
	defer os.Remove(serverCertFpath)

	serverKeyFpath, err := test.CreateTempFile(test.TLSCertKey)
	require.NoError(t, err)
	defer os.Remove(serverKeyFpath)

	p, ok := newInstance("api: yes\n" +
		"hlsAlwaysRemux: yes\n" +
		"metrics: yes\n" +
		"webrtcServerCert: " + serverCertFpath + "\n" +
		"webrtcServerKey: " + serverKeyFpath + "\n" +
		"rtspEncryption: optional\n" +
		"rtspServerCert: " + serverCertFpath + "\n" +
		"rtspServerKey: " + serverKeyFpath + "\n" +
		"rtmpEncryption: optional\n" +
		"rtmpServerCert: " + serverCertFpath + "\n" +
		"rtmpServerKey: " + serverKeyFpath + "\n" +
		"paths:\n" +
		"  all_others:\n")
	require.Equal(t, true, ok)
	defer p.Close()

	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Transport: tr}

	t.Run("initial", func(t *testing.T) {
		bo := httpPullFile(t, hc, "http://localhost:9998/metrics")

		require.Equal(t, `paths 0
hls_muxers 0
hls_muxers_bytes_sent 0
rtsp_conns 0
rtsp_conns_bytes_received 0
rtsp_conns_bytes_sent 0
rtsp_sessions 0
rtsp_sessions_bytes_received 0
rtsp_sessions_bytes_sent 0
rtsp_sessions_rtp_packets_received 0
rtsp_sessions_rtp_packets_sent 0
rtsp_sessions_rtp_packets_lost 0
rtsp_sessions_rtp_packets_in_error 0
rtsp_sessions_rtp_packets_jitter 0
rtsp_sessions_rtcp_packets_received 0
rtsp_sessions_rtcp_packets_sent 0
rtsp_sessions_rtcp_packets_in_error 0
rtsps_conns 0
rtsps_conns_bytes_received 0
rtsps_conns_bytes_sent 0
rtsps_sessions 0
rtsps_sessions_bytes_received 0
rtsps_sessions_bytes_sent 0
rtsps_sessions_rtp_packets_received 0
rtsps_sessions_rtp_packets_sent 0
rtsps_sessions_rtp_packets_lost 0
rtsps_sessions_rtp_packets_in_error 0
rtsps_sessions_rtp_packets_jitter 0
rtsps_sessions_rtcp_packets_received 0
rtsps_sessions_rtcp_packets_sent 0
rtsps_sessions_rtcp_packets_in_error 0
rtmp_conns 0
rtmp_conns_bytes_received 0
rtmp_conns_bytes_sent 0
rtmps_conns 0
rtmps_conns_bytes_received 0
rtmps_conns_bytes_sent 0
srt_conns 0
srt_conns_packets_sent 0
srt_conns_packets_received 0
srt_conns_packets_sent_unique 0
srt_conns_packets_received_unique 0
srt_conns_packets_send_loss 0
srt_conns_packets_received_loss 0
srt_conns_packets_retrans 0
srt_conns_packets_received_retrans 0
srt_conns_packets_sent_ack 0
srt_conns_packets_received_ack 0
srt_conns_packets_sent_nak 0
srt_conns_packets_received_nak 0
srt_conns_packets_sent_km 0
srt_conns_packets_received_km 0
srt_conns_us_snd_duration 0
srt_conns_packets_send_drop 0
srt_conns_packets_received_drop 0
srt_conns_packets_received_undecrypt 0
srt_conns_bytes_sent 0
srt_conns_bytes_received 0
srt_conns_bytes_sent_unique 0
srt_conns_bytes_received_unique 0
srt_conns_bytes_received_loss 0
srt_conns_bytes_retrans 0
srt_conns_bytes_received_retrans 0
srt_conns_bytes_send_drop 0
srt_conns_bytes_received_drop 0
srt_conns_bytes_received_undecrypt 0
srt_conns_us_packets_send_period 0
srt_conns_packets_flow_window 0
srt_conns_packets_flight_size 0
srt_conns_ms_rtt 0
srt_conns_mbps_send_rate 0
srt_conns_mbps_receive_rate 0
srt_conns_mbps_link_capacity 0
srt_conns_bytes_avail_send_buf 0
srt_conns_bytes_avail_receive_buf 0
srt_conns_mbps_max_bw 0
srt_conns_bytes_mss 0
srt_conns_packets_send_buf 0
srt_conns_bytes_send_buf 0
srt_conns_ms_send_buf 0
srt_conns_ms_send_tsb_pd_delay 0
srt_conns_packets_receive_buf 0
srt_conns_bytes_receive_buf 0
srt_conns_ms_receive_buf 0
srt_conns_ms_receive_tsb_pd_delay 0
srt_conns_packets_reorder_tolerance 0
srt_conns_packets_received_avg_belated_time 0
srt_conns_packets_send_loss_rate 0
srt_conns_packets_received_loss_rate 0
webrtc_sessions 0
webrtc_sessions_bytes_received 0
webrtc_sessions_bytes_sent 0
`, string(bo))
	})

	t.Run("with data", func(t *testing.T) {
		terminate := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(6)

		go func() {
			defer wg.Done()
			source := gortsplib.Client{}
			err := source.StartRecording("rtsp://localhost:8554/rtsp_path",
				&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
			require.NoError(t, err)
			defer source.Close()
			<-terminate
		}()

		go func() {
			defer wg.Done()
			source2 := gortsplib.Client{TLSConfig: &tls.Config{InsecureSkipVerify: true}}
			err := source2.StartRecording("rtsps://localhost:8322/rtsps_path",
				&description.Session{Medias: []*description.Media{test.UniqueMediaH264()}})
			require.NoError(t, err)
			defer source2.Close()
			<-terminate
		}()

		go func() {
			defer wg.Done()
			u, err := url.Parse("rtmp://localhost:1935/rtmp_path")
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

			<-terminate
		}()

		go func() {
			defer wg.Done()
			u, err := url.Parse("rtmps://localhost:1936/rtmps_path")
			require.NoError(t, err)

			nconn, err := tls.Dial("tcp", u.Host, &tls.Config{InsecureSkipVerify: true})
			require.NoError(t, err)
			defer nconn.Close() //nolint:errcheck

			conn, err := rtmp.NewClientConn(nconn, u, true)
			require.NoError(t, err)

			w, err := rtmp.NewWriter(conn, test.FormatH264, nil)
			require.NoError(t, err)

			err = w.WriteH264(2*time.Second, 2*time.Second, [][]byte{{5, 2, 3, 4}})
			require.NoError(t, err)

			<-terminate
		}()

		go func() {
			defer wg.Done()

			su, err := url.Parse("http://localhost:8889/webrtc_path/whip")
			require.NoError(t, err)

			tr := &http.Transport{}
			defer tr.CloseIdleConnections()
			hc2 := &http.Client{Transport: tr}

			s := &whip.Client{
				HTTPClient: hc2,
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

			err = s.Publish(context.Background(), []*webrtc.OutgoingTrack{track})
			require.NoError(t, err)
			defer checkClose(t, s.Close)

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
			<-terminate
		}()

		go func() {
			defer wg.Done()

			srtConf := srt.DefaultConfig()
			address, err := srtConf.UnmarshalURL("srt://localhost:8890?streamid=publish:srt_path")
			require.NoError(t, err)

			err = srtConf.Validate()
			require.NoError(t, err)

			publisher, err := srt.Dial("srt", address, srtConf)
			require.NoError(t, err)
			defer publisher.Close()

			track := &mpegts.Track{
				Codec: &mpegts.CodecH264{},
			}

			bw := bufio.NewWriter(publisher)
			w := mpegts.NewWriter(bw, []*mpegts.Track{track})
			require.NoError(t, err)

			err = w.WriteH264(track, 0, 0, [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{0x05, 1}, // IDR
			})
			require.NoError(t, err)

			err = bw.Flush()
			require.NoError(t, err)
			<-terminate
		}()

		time.Sleep(500*time.Millisecond + 2*time.Second)

		bo := httpPullFile(t, hc, "http://localhost:9998/metrics")

		require.Regexp(t,
			`^paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths\{name=".*?",state="ready"\} 1`+"\n"+
				`paths_bytes_received\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`paths_bytes_sent\{name=".*?",state="ready"\} [0-9]+`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`hls_muxers\{name=".*?"\} 1`+"\n"+
				`hls_muxers_bytes_sent\{name=".*?"\} 0`+"\n"+
				`rtsp_conns\{id=".*?"\} 1`+"\n"+
				`rtsp_conns_bytes_received\{id=".*?"\} [0-9]+`+"\n"+
				`rtsp_conns_bytes_sent\{id=".*?"\} [0-9]+`+"\n"+
				`rtsp_sessions\{id=".*?",state="publish"\} 1`+"\n"+
				`rtsp_sessions_bytes_received\{id=".*?",state="publish"\} 0`+"\n"+
				`rtsp_sessions_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtp_packets_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtp_packets_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtp_packets_lost\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtp_packets_in_error\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtp_packets_jitter\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtcp_packets_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtcp_packets_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsp_sessions_rtcp_packets_in_error\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_conns\{id=".*?"\} 1`+"\n"+
				`rtsps_conns_bytes_received\{id=".*?"\} [0-9]+`+"\n"+
				`rtsps_conns_bytes_sent\{id=".*?"\} [0-9]+`+"\n"+
				`rtsps_sessions\{id=".*?",state="publish"\} 1`+"\n"+
				`rtsps_sessions_bytes_received\{id=".*?",state="publish"\} 0`+"\n"+
				`rtsps_sessions_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtp_packets_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtp_packets_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtp_packets_lost\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtp_packets_in_error\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtp_packets_jitter\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtcp_packets_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtcp_packets_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtsps_sessions_rtcp_packets_in_error\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtmp_conns\{id=".*?",state="publish"\} 1`+"\n"+
				`rtmp_conns_bytes_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtmp_conns_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtmps_conns\{id=".*?",state="publish"\} 1`+"\n"+
				`rtmps_conns_bytes_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`rtmps_conns_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns\{id=".*?",state="publish"\} 1`+"\n"+
				`srt_conns_packets_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_sent_unique\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_unique\{id=".*?",state="publish"\} 1`+"\n"+
				`srt_conns_packets_send_loss\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_loss\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_retrans\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_retrans\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_sent_ack\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_ack\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_sent_nak\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_nak\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_sent_km\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_km\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_us_snd_duration\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_send_drop\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_drop\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_undecrypt\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_sent\{id=".*?",state="publish"\} 0`+"\n"+
				`srt_conns_bytes_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_sent_unique\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_received_unique\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_received_loss\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_retrans\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_received_retrans\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_send_drop\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_received_drop\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_received_undecrypt\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_us_packets_send_period\{id=".*?",state="publish"\} \d+\.\d+`+"\n"+
				`srt_conns_packets_flow_window\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_flight_size\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_ms_rtt\{id=".*?",state="publish"\} \d+\.\d+`+"\n"+
				`srt_conns_mbps_send_rate\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_mbps_receive_rate\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_mbps_link_capacity\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_avail_send_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_avail_receive_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_mbps_max_bw\{id=".*?",state="publish"\} -1`+"\n"+
				`srt_conns_bytes_mss\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_send_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_send_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_ms_send_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_ms_send_tsb_pd_delay\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_receive_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_bytes_receive_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_ms_receive_buf\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_ms_receive_tsb_pd_delay\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_reorder_tolerance\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_avg_belated_time\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_send_loss_rate\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`srt_conns_packets_received_loss_rate\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`webrtc_sessions\{id=".*?",state="publish"\} 1`+"\n"+
				`webrtc_sessions_bytes_received\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				`webrtc_sessions_bytes_sent\{id=".*?",state="publish"\} [0-9]+`+"\n"+
				"$",
			string(bo))

		close(terminate)
		wg.Wait()
	})

	t.Run("servers disabled", func(t *testing.T) {
		httpRequest(t, hc, http.MethodPatch, "http://localhost:9997/v3/config/global/patch", map[string]interface{}{
			"rtsp":   false,
			"rtmp":   false,
			"srt":    false,
			"hls":    false,
			"webrtc": false,
		}, nil)

		time.Sleep(500 * time.Millisecond)

		bo := httpPullFile(t, hc, "http://localhost:9998/metrics")

		require.Equal(t, "paths 0\n", string(bo))
	})
}
