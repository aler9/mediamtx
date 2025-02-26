// Package metrics contains the metrics provider.
package metrics

import (
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/api"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/protocols/httpp"
	"github.com/bluenviron/mediamtx/internal/restrictnetwork"
)

func interfaceIsEmpty(i interface{}) bool {
	return reflect.ValueOf(i).Kind() != reflect.Ptr || reflect.ValueOf(i).IsNil()
}

func metric(key string, tags string, value int64) string {
	return key + tags + " " + strconv.FormatInt(value, 10) + "\n"
}

func metricFloat(key string, tags string, value float64) string {
	return key + tags + " " + strconv.FormatFloat(value, 'f', -1, 64) + "\n"
}

type metricsAuthManager interface {
	Authenticate(req *auth.Request) error
}

type metricsParent interface {
	logger.Writer
}

// Metrics is a metrics provider.
type Metrics struct {
	Address        string
	Encryption     bool
	ServerKey      string
	ServerCert     string
	AllowOrigin    string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	AuthManager    metricsAuthManager
	Parent         metricsParent

	httpServer   *httpp.Server
	mutex        sync.Mutex
	pathManager  api.PathManager
	rtspServer   api.RTSPServer
	rtspsServer  api.RTSPServer
	rtmpServer   api.RTMPServer
	rtmpsServer  api.RTMPServer
	srtServer    api.SRTServer
	hlsManager   api.HLSServer
	webRTCServer api.WebRTCServer
}

// Initialize initializes metrics.
func (m *Metrics) Initialize() error {
	router := gin.New()
	router.SetTrustedProxies(m.TrustedProxies.ToTrustedProxies()) //nolint:errcheck

	router.Use(m.middlewareOrigin)
	router.Use(m.middlewareAuth)

	router.GET("/metrics", m.onMetrics)

	network, address := restrictnetwork.Restrict("tcp", m.Address)

	m.httpServer = &httpp.Server{
		Network:     network,
		Address:     address,
		ReadTimeout: time.Duration(m.ReadTimeout),
		Encryption:  m.Encryption,
		ServerCert:  m.ServerCert,
		ServerKey:   m.ServerKey,
		Handler:     router,
		Parent:      m,
	}
	err := m.httpServer.Initialize()
	if err != nil {
		return err
	}

	m.Log(logger.Info, "listener opened on "+address)

	return nil
}

// Close closes Metrics.
func (m *Metrics) Close() {
	m.Log(logger.Info, "listener is closing")
	m.httpServer.Close()
}

// Log implements logger.Writer.
func (m *Metrics) Log(level logger.Level, format string, args ...interface{}) {
	m.Parent.Log(level, "[metrics] "+format, args...)
}

func (m *Metrics) middlewareOrigin(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", m.AllowOrigin)
	ctx.Header("Access-Control-Allow-Credentials", "true")

	// preflight requests
	if ctx.Request.Method == http.MethodOptions &&
		ctx.Request.Header.Get("Access-Control-Request-Method") != "" {
		ctx.Header("Access-Control-Allow-Methods", "OPTIONS, GET")
		ctx.Header("Access-Control-Allow-Headers", "Authorization")
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
}

func (m *Metrics) middlewareAuth(ctx *gin.Context) {
	req := &auth.Request{
		IP:     net.ParseIP(ctx.ClientIP()),
		Action: conf.AuthActionMetrics,
	}
	req.FillFromHTTPRequest(ctx.Request)

	err := m.AuthManager.Authenticate(req)
	if err != nil {
		if err.(auth.Error).AskCredentials { //nolint:errorlint
			ctx.Header("WWW-Authenticate", `Basic realm="mediamtx"`)
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// wait some seconds to mitigate brute force attacks
		<-time.After(auth.PauseAfterError)

		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
}

func (m *Metrics) onMetrics(ctx *gin.Context) {
	out := ""

	data, err := m.pathManager.APIPathsList()
	if err == nil && len(data.Items) != 0 {
		for _, i := range data.Items {
			var state string
			if i.Ready {
				state = "ready"
			} else {
				state = "notReady"
			}

			tags := "{name=\"" + i.Name + "\",state=\"" + state + "\"}"
			out += metric("paths", tags, 1)
			out += metric("paths_bytes_received", tags, int64(i.BytesReceived))
			out += metric("paths_bytes_sent", tags, int64(i.BytesSent))
		}
	} else {
		out += metric("paths", "", 0)
	}

	if !interfaceIsEmpty(m.hlsManager) {
		data, err := m.hlsManager.APIMuxersList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{name=\"" + i.Path + "\"}"
				out += metric("hls_muxers", tags, 1)
				out += metric("hls_muxers_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("hls_muxers", "", 0)
			out += metric("hls_muxers_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtspServer) { //nolint:dupl
		func() {
			data, err := m.rtspServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\"}"
					out += metric("rtsp_conns", tags, 1)
					out += metric("rtsp_conns_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsp_conns_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsp_conns", "", 0)
				out += metric("rtsp_conns_bytes_received", "", 0)
				out += metric("rtsp_conns_bytes_sent", "", 0)
			}
		}()

		func() {
			data, err := m.rtspServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
					out += metric("rtsp_sessions", tags, 1)
					out += metric("rtsp_sessions_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsp_sessions_bytes_sent", tags, int64(i.BytesSent))
					out += metric("rtsp_sessions_rtp_packets_received", tags, int64(i.RTPPacketsReceived))
					out += metric("rtsp_sessions_rtp_packets_sent", tags, int64(i.RTPPacketsSent))
					out += metric("rtsp_sessions_rtp_packets_lost", tags, int64(i.RTPPacketsLost))
					out += metric("rtsp_sessions_rtp_packets_in_error", tags, int64(i.RTPPacketsInError))
					out += metricFloat("rtsp_sessions_rtp_packets_jitter", tags, i.RTPPacketsJitter)
					out += metric("rtsp_sessions_rtcp_packets_received", tags, int64(i.RTCPPacketsReceived))
					out += metric("rtsp_sessions_rtcp_packets_sent", tags, int64(i.RTCPPacketsSent))
					out += metric("rtsp_sessions_rtcp_packets_in_error", tags, int64(i.RTCPPacketsInError))
				}
			} else {
				out += metric("rtsp_sessions", "", 0)
				out += metric("rtsp_sessions_bytes_received", "", 0)
				out += metric("rtsp_sessions_bytes_sent", "", 0)
				out += metric("rtsp_sessions_rtp_packets_received", "", 0)
				out += metric("rtsp_sessions_rtp_packets_sent", "", 0)
				out += metric("rtsp_sessions_rtp_packets_lost", "", 0)
				out += metric("rtsp_sessions_rtp_packets_in_error", "", 0)
				out += metricFloat("rtsp_sessions_rtp_packets_jitter", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_received", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_sent", "", 0)
				out += metric("rtsp_sessions_rtcp_packets_in_error", "", 0)
			}
		}()
	}

	if !interfaceIsEmpty(m.rtspsServer) { //nolint:dupl
		func() {
			data, err := m.rtspsServer.APIConnsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\"}"
					out += metric("rtsps_conns", tags, 1)
					out += metric("rtsps_conns_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsps_conns_bytes_sent", tags, int64(i.BytesSent))
				}
			} else {
				out += metric("rtsps_conns", "", 0)
				out += metric("rtsps_conns_bytes_received", "", 0)
				out += metric("rtsps_conns_bytes_sent", "", 0)
			}
		}()

		func() {
			data, err := m.rtspsServer.APISessionsList()
			if err == nil && len(data.Items) != 0 {
				for _, i := range data.Items {
					tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
					out += metric("rtsps_sessions", tags, 1)
					out += metric("rtsps_sessions_bytes_received", tags, int64(i.BytesReceived))
					out += metric("rtsps_sessions_bytes_sent", tags, int64(i.BytesSent))
					out += metric("rtsps_sessions_rtp_packets_received", tags, int64(i.RTPPacketsReceived))
					out += metric("rtsps_sessions_rtp_packets_sent", tags, int64(i.RTPPacketsSent))
					out += metric("rtsps_sessions_rtp_packets_lost", tags, int64(i.RTPPacketsLost))
					out += metric("rtsps_sessions_rtp_packets_in_error", tags, int64(i.RTPPacketsInError))
					out += metricFloat("rtsps_sessions_rtp_packets_jitter", tags, i.RTPPacketsJitter)
					out += metric("rtsps_sessions_rtcp_packets_received", tags, int64(i.RTCPPacketsReceived))
					out += metric("rtsps_sessions_rtcp_packets_sent", tags, int64(i.RTCPPacketsSent))
					out += metric("rtsps_sessions_rtcp_packets_in_error", tags, int64(i.RTCPPacketsInError))
				}
			} else {
				out += metric("rtsps_sessions", "", 0)
				out += metric("rtsps_sessions_bytes_received", "", 0)
				out += metric("rtsps_sessions_bytes_sent", "", 0)
				out += metric("rtsps_sessions_rtp_packets_received", "", 0)
				out += metric("rtsps_sessions_rtp_packets_sent", "", 0)
				out += metric("rtsps_sessions_rtp_packets_lost", "", 0)
				out += metric("rtsps_sessions_rtp_packets_in_error", "", 0)
				out += metricFloat("rtsps_sessions_rtp_packets_jitter", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_received", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_sent", "", 0)
				out += metric("rtsps_sessions_rtcp_packets_in_error", "", 0)
			}
		}()
	}

	if !interfaceIsEmpty(m.rtmpServer) {
		data, err := m.rtmpServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("rtmp_conns", tags, 1)
				out += metric("rtmp_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("rtmp_conns_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("rtmp_conns", "", 0)
			out += metric("rtmp_conns_bytes_received", "", 0)
			out += metric("rtmp_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.rtmpsServer) {
		data, err := m.rtmpsServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("rtmps_conns", tags, 1)
				out += metric("rtmps_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("rtmps_conns_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("rtmps_conns", "", 0)
			out += metric("rtmps_conns_bytes_received", "", 0)
			out += metric("rtmps_conns_bytes_sent", "", 0)
		}
	}

	if !interfaceIsEmpty(m.srtServer) {
		data, err := m.srtServer.APIConnsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("srt_conns", tags, 1)
				out += metric("srt_conns_packets_sent", tags, int64(i.PacketsSent))
				out += metric("srt_conns_packets_received", tags, int64(i.PacketsReceived))
				out += metric("srt_conns_packets_sent_unique", tags, int64(i.PacketsSentUnique))
				out += metric("srt_conns_packets_received_unique", tags, int64(i.PacketsReceivedUnique))
				out += metric("srt_conns_packets_send_loss", tags, int64(i.PacketsSendLoss))
				out += metric("srt_conns_packets_received_loss", tags, int64(i.PacketsReceivedLoss))
				out += metric("srt_conns_packets_retrans", tags, int64(i.PacketsRetrans))
				out += metric("srt_conns_packets_received_retrans", tags, int64(i.PacketsReceivedRetrans))
				out += metric("srt_conns_packets_sent_ack", tags, int64(i.PacketsSentACK))
				out += metric("srt_conns_packets_received_ack", tags, int64(i.PacketsReceivedACK))
				out += metric("srt_conns_packets_sent_nak", tags, int64(i.PacketsSentNAK))
				out += metric("srt_conns_packets_received_nak", tags, int64(i.PacketsReceivedNAK))
				out += metric("srt_conns_packets_sent_km", tags, int64(i.PacketsSentKM))
				out += metric("srt_conns_packets_received_km", tags, int64(i.PacketsReceivedKM))
				out += metric("srt_conns_us_snd_duration", tags, int64(i.UsSndDuration))
				out += metric("srt_conns_packets_send_drop", tags, int64(i.PacketsSendDrop))
				out += metric("srt_conns_packets_received_drop", tags, int64(i.PacketsReceivedDrop))
				out += metric("srt_conns_packets_received_undecrypt", tags, int64(i.PacketsReceivedUndecrypt))
				out += metric("srt_conns_bytes_sent", tags, int64(i.BytesSent))
				out += metric("srt_conns_bytes_received", tags, int64(i.BytesReceived))
				out += metric("srt_conns_bytes_sent_unique", tags, int64(i.BytesSentUnique))
				out += metric("srt_conns_bytes_received_unique", tags, int64(i.BytesReceivedUnique))
				out += metric("srt_conns_bytes_received_loss", tags, int64(i.BytesReceivedLoss))
				out += metric("srt_conns_bytes_retrans", tags, int64(i.BytesRetrans))
				out += metric("srt_conns_bytes_received_retrans", tags, int64(i.BytesReceivedRetrans))
				out += metric("srt_conns_bytes_send_drop", tags, int64(i.BytesSendDrop))
				out += metric("srt_conns_bytes_received_drop", tags, int64(i.BytesReceivedDrop))
				out += metric("srt_conns_bytes_received_undecrypt", tags, int64(i.BytesReceivedUndecrypt))
				out += metricFloat("srt_conns_us_packets_send_period", tags, i.UsPacketsSendPeriod)
				out += metric("srt_conns_packets_flow_window", tags, int64(i.PacketsFlowWindow))
				out += metric("srt_conns_packets_flight_size", tags, int64(i.PacketsFlightSize))
				out += metricFloat("srt_conns_ms_rtt", tags, i.MsRTT)
				out += metricFloat("srt_conns_mbps_send_rate", tags, i.MbpsSendRate)
				out += metricFloat("srt_conns_mbps_receive_rate", tags, i.MbpsReceiveRate)
				out += metricFloat("srt_conns_mbps_link_capacity", tags, i.MbpsLinkCapacity)
				out += metric("srt_conns_bytes_avail_send_buf", tags, int64(i.BytesAvailSendBuf))
				out += metric("srt_conns_bytes_avail_receive_buf", tags, int64(i.BytesAvailReceiveBuf))
				out += metricFloat("srt_conns_mbps_max_bw", tags, i.MbpsMaxBW)
				out += metric("srt_conns_bytes_mss", tags, int64(i.ByteMSS))
				out += metric("srt_conns_packets_send_buf", tags, int64(i.PacketsSendBuf))
				out += metric("srt_conns_bytes_send_buf", tags, int64(i.BytesSendBuf))
				out += metric("srt_conns_ms_send_buf", tags, int64(i.MsSendBuf))
				out += metric("srt_conns_ms_send_tsb_pd_delay", tags, int64(i.MsSendTsbPdDelay))
				out += metric("srt_conns_packets_receive_buf", tags, int64(i.PacketsReceiveBuf))
				out += metric("srt_conns_bytes_receive_buf", tags, int64(i.BytesReceiveBuf))
				out += metric("srt_conns_ms_receive_buf", tags, int64(i.MsReceiveBuf))
				out += metric("srt_conns_ms_receive_tsb_pd_delay", tags, int64(i.MsReceiveTsbPdDelay))
				out += metric("srt_conns_packets_reorder_tolerance", tags, int64(i.PacketsReorderTolerance))
				out += metric("srt_conns_packets_received_avg_belated_time", tags, int64(i.PacketsReceivedAvgBelatedTime))
				out += metricFloat("srt_conns_packets_send_loss_rate", tags, i.PacketsSendLossRate)
				out += metricFloat("srt_conns_packets_received_loss_rate", tags, i.PacketsReceivedLossRate)
			}
		} else {
			out += metric("srt_conns", "", 0)
			out += metric("srt_conns_packets_sent", "", 0)
			out += metric("srt_conns_packets_received", "", 0)
			out += metric("srt_conns_packets_sent_unique", "", 0)
			out += metric("srt_conns_packets_received_unique", "", 0)
			out += metric("srt_conns_packets_send_loss", "", 0)
			out += metric("srt_conns_packets_received_loss", "", 0)
			out += metric("srt_conns_packets_retrans", "", 0)
			out += metric("srt_conns_packets_received_retrans", "", 0)
			out += metric("srt_conns_packets_sent_ack", "", 0)
			out += metric("srt_conns_packets_received_ack", "", 0)
			out += metric("srt_conns_packets_sent_nak", "", 0)
			out += metric("srt_conns_packets_received_nak", "", 0)
			out += metric("srt_conns_packets_sent_km", "", 0)
			out += metric("srt_conns_packets_received_km", "", 0)
			out += metric("srt_conns_us_snd_duration", "", 0)
			out += metric("srt_conns_packets_send_drop", "", 0)
			out += metric("srt_conns_packets_received_drop", "", 0)
			out += metric("srt_conns_packets_received_undecrypt", "", 0)
			out += metric("srt_conns_bytes_sent", "", 0)
			out += metric("srt_conns_bytes_received", "", 0)
			out += metric("srt_conns_bytes_sent_unique", "", 0)
			out += metric("srt_conns_bytes_received_unique", "", 0)
			out += metric("srt_conns_bytes_received_loss", "", 0)
			out += metric("srt_conns_bytes_retrans", "", 0)
			out += metric("srt_conns_bytes_received_retrans", "", 0)
			out += metric("srt_conns_bytes_send_drop", "", 0)
			out += metric("srt_conns_bytes_received_drop", "", 0)
			out += metric("srt_conns_bytes_received_undecrypt", "", 0)
			out += metricFloat("srt_conns_us_packets_send_period", "", 0)
			out += metric("srt_conns_packets_flow_window", "", 0)
			out += metric("srt_conns_packets_flight_size", "", 0)
			out += metricFloat("srt_conns_ms_rtt", "", 0)
			out += metricFloat("srt_conns_mbps_send_rate", "", 0)
			out += metricFloat("srt_conns_mbps_receive_rate", "", 0)
			out += metricFloat("srt_conns_mbps_link_capacity", "", 0)
			out += metric("srt_conns_bytes_avail_send_buf", "", 0)
			out += metric("srt_conns_bytes_avail_receive_buf", "", 0)
			out += metricFloat("srt_conns_mbps_max_bw", "", 0)
			out += metric("srt_conns_bytes_mss", "", 0)
			out += metric("srt_conns_packets_send_buf", "", 0)
			out += metric("srt_conns_bytes_send_buf", "", 0)
			out += metric("srt_conns_ms_send_buf", "", 0)
			out += metric("srt_conns_ms_send_tsb_pd_delay", "", 0)
			out += metric("srt_conns_packets_receive_buf", "", 0)
			out += metric("srt_conns_bytes_receive_buf", "", 0)
			out += metric("srt_conns_ms_receive_buf", "", 0)
			out += metric("srt_conns_ms_receive_tsb_pd_delay", "", 0)
			out += metric("srt_conns_packets_reorder_tolerance", "", 0)
			out += metric("srt_conns_packets_received_avg_belated_time", "", 0)
			out += metricFloat("srt_conns_packets_send_loss_rate", "", 0)
			out += metricFloat("srt_conns_packets_received_loss_rate", "", 0)
		}
	}

	if !interfaceIsEmpty(m.webRTCServer) {
		data, err := m.webRTCServer.APISessionsList()
		if err == nil && len(data.Items) != 0 {
			for _, i := range data.Items {
				tags := "{id=\"" + i.ID.String() + "\",state=\"" + string(i.State) + "\"}"
				out += metric("webrtc_sessions", tags, 1)
				out += metric("webrtc_sessions_bytes_received", tags, int64(i.BytesReceived))
				out += metric("webrtc_sessions_bytes_sent", tags, int64(i.BytesSent))
			}
		} else {
			out += metric("webrtc_sessions", "", 0)
			out += metric("webrtc_sessions_bytes_received", "", 0)
			out += metric("webrtc_sessions_bytes_sent", "", 0)
		}
	}

	ctx.Writer.WriteHeader(http.StatusOK)
	io.WriteString(ctx.Writer, out) //nolint:errcheck
}

// SetPathManager is called by core.
func (m *Metrics) SetPathManager(s api.PathManager) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pathManager = s
}

// SetHLSServer is called by core.
func (m *Metrics) SetHLSServer(s api.HLSServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.hlsManager = s
}

// SetRTSPServer is called by core.
func (m *Metrics) SetRTSPServer(s api.RTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspServer = s
}

// SetRTSPSServer is called by core.
func (m *Metrics) SetRTSPSServer(s api.RTSPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtspsServer = s
}

// SetRTMPServer is called by core.
func (m *Metrics) SetRTMPServer(s api.RTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpServer = s
}

// SetRTMPSServer is called by core.
func (m *Metrics) SetRTMPSServer(s api.RTMPServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.rtmpsServer = s
}

// SetSRTServer is called by core.
func (m *Metrics) SetSRTServer(s api.SRTServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.srtServer = s
}

// SetWebRTCServer is called by core.
func (m *Metrics) SetWebRTCServer(s api.WebRTCServer) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.webRTCServer = s
}
