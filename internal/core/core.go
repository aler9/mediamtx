// Package core contains the main struct of the software.
package core

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/bluenviron/gortsplib/v4"
	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/api"
	"github.com/bluenviron/mediamtx/internal/auth"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/confwatcher"
	"github.com/bluenviron/mediamtx/internal/externalcmd"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/metrics"
	"github.com/bluenviron/mediamtx/internal/playback"
	"github.com/bluenviron/mediamtx/internal/pprof"
	"github.com/bluenviron/mediamtx/internal/recordcleaner"
	"github.com/bluenviron/mediamtx/internal/rlimit"
	"github.com/bluenviron/mediamtx/internal/servers/hls"
	"github.com/bluenviron/mediamtx/internal/servers/rtmp"
	"github.com/bluenviron/mediamtx/internal/servers/rtsp"
	"github.com/bluenviron/mediamtx/internal/servers/srt"
	"github.com/bluenviron/mediamtx/internal/servers/webrtc"
)

//go:generate go run ./versiongetter

//go:embed VERSION
var version []byte

var defaultConfPaths = []string{
	"rtsp-simple-server.yml",
	"mediamtx.yml",
	"/usr/local/etc/mediamtx.yml",
	"/usr/etc/mediamtx.yml",
	"/etc/mediamtx/mediamtx.yml",
}

var cli struct {
	Version  bool   `help:"print version"`
	Confpath string `arg:"" default:""`
}

func atLeastOneRecordDeleteAfter(pathConfs map[string]*conf.Path) bool {
	for _, e := range pathConfs {
		if e.RecordDeleteAfter != 0 {
			return true
		}
	}
	return false
}

// Core is an instance of MediaMTX.
type Core struct {
	ctx             context.Context
	ctxCancel       func()
	confPath        string
	conf            *conf.Conf
	logger          *logger.Logger
	externalCmdPool *externalcmd.Pool
	authManager     *auth.Manager
	metrics         *metrics.Metrics
	pprof           *pprof.PPROF
	recordCleaner   *recordcleaner.Cleaner
	playbackServer  *playback.Server
	pathManager     *pathManager
	rtspServer      *rtsp.Server
	rtspsServer     *rtsp.Server
	rtmpServer      *rtmp.Server
	rtmpsServer     *rtmp.Server
	hlsServer       *hls.Server
	webRTCServer    *webrtc.Server
	srtServer       *srt.Server
	api             *api.API
	confWatcher     *confwatcher.ConfWatcher

	// in
	chAPIConfigSet chan *conf.Conf

	// out
	done chan struct{}
}

// New allocates a Core.
func New(args []string) (*Core, bool) {
	parser, err := kong.New(&cli,
		kong.Description("MediaMTX "+string(version)),
		kong.UsageOnError(),
		kong.ValueFormatter(func(value *kong.Value) string {
			switch value.Name {
			case "confpath":
				return "path to a config file. The default is mediamtx.yml."

			default:
				return kong.DefaultHelpValueFormatter(value)
			}
		}))
	if err != nil {
		panic(err)
	}

	_, err = parser.Parse(args)
	parser.FatalIfErrorf(err)

	if cli.Version {
		fmt.Println(string(version))
		os.Exit(0)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	p := &Core{
		ctx:            ctx,
		ctxCancel:      ctxCancel,
		chAPIConfigSet: make(chan *conf.Conf),
		done:           make(chan struct{}),
	}

	tempLogger, _ := logger.New(logger.Warn, []logger.Destination{logger.DestinationStdout}, "")

	p.conf, p.confPath, err = conf.Load(cli.Confpath, defaultConfPaths, tempLogger)
	if err != nil {
		fmt.Printf("ERR: %s\n", err)
		return nil, false
	}

	err = p.createResources(true)
	if err != nil {
		if p.logger != nil {
			p.Log(logger.Error, "%s", err)
		} else {
			fmt.Printf("ERR: %s\n", err)
		}
		p.closeResources(nil, false)
		return nil, false
	}

	go p.run()

	return p, true
}

// Close closes Core and waits for all goroutines to return.
func (p *Core) Close() {
	p.ctxCancel()
	<-p.done
}

// Wait waits for the Core to exit.
func (p *Core) Wait() {
	<-p.done
}

// Log implements logger.Writer.
func (p *Core) Log(level logger.Level, format string, args ...interface{}) {
	p.logger.Log(level, format, args...)
}

func (p *Core) run() {
	defer close(p.done)

	confChanged := func() chan struct{} {
		if p.confWatcher != nil {
			return p.confWatcher.Watch()
		}
		return make(chan struct{})
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

outer:
	for {
		select {
		case <-confChanged:
			p.Log(logger.Info, "reloading configuration (file changed)")

			newConf, _, err := conf.Load(p.confPath, nil, p.logger)
			if err != nil {
				p.Log(logger.Error, "%s", err)
				break outer
			}

			err = p.reloadConf(newConf, false)
			if err != nil {
				p.Log(logger.Error, "%s", err)
				break outer
			}

		case newConf := <-p.chAPIConfigSet:
			p.Log(logger.Info, "reloading configuration (API request)")

			err := p.reloadConf(newConf, true)
			if err != nil {
				p.Log(logger.Error, "%s", err)
				break outer
			}

		case <-interrupt:
			p.Log(logger.Info, "shutting down gracefully")
			break outer

		case <-p.ctx.Done():
			break outer
		}
	}

	p.ctxCancel()

	p.closeResources(nil, false)
}

func (p *Core) createResources(initial bool) error {
	var err error

	if p.logger == nil {
		p.logger, err = logger.New(
			logger.Level(p.conf.LogLevel),
			p.conf.LogDestinations,
			p.conf.LogFile,
		)
		if err != nil {
			return err
		}
	}

	if initial {
		p.Log(logger.Info, "MediaMTX %s", version)

		if p.confPath != "" {
			a, _ := filepath.Abs(p.confPath)
			p.Log(logger.Info, "configuration loaded from %s", a)
		} else {
			list := make([]string, len(defaultConfPaths))
			for i, pa := range defaultConfPaths {
				a, _ := filepath.Abs(pa)
				list[i] = a
			}

			p.Log(logger.Warn,
				"configuration file not found (looked in %s), using an empty configuration",
				strings.Join(list, ", "))
		}

		// on Linux, try to raise the number of file descriptors that can be opened
		// to allow the maximum possible number of clients.
		rlimit.Raise() //nolint:errcheck

		gin.SetMode(gin.ReleaseMode)

		p.externalCmdPool = externalcmd.NewPool()
	}

	if p.authManager == nil {
		p.authManager = &auth.Manager{
			Method:        p.conf.AuthMethod,
			InternalUsers: p.conf.AuthInternalUsers,
			HTTPAddress:   p.conf.AuthHTTPAddress,
			HTTPExclude:   p.conf.AuthHTTPExclude,
			JWTJWKS:       p.conf.AuthJWTJWKS,
			JWTClaimKey:   p.conf.AuthJWTClaimKey,
			ReadTimeout:   time.Duration(p.conf.ReadTimeout),
		}
	}

	if p.conf.Metrics &&
		p.metrics == nil {
		i := &metrics.Metrics{
			Address:        p.conf.MetricsAddress,
			Encryption:     p.conf.MetricsEncryption,
			ServerKey:      p.conf.MetricsServerKey,
			ServerCert:     p.conf.MetricsServerCert,
			AllowOrigin:    p.conf.MetricsAllowOrigin,
			TrustedProxies: p.conf.MetricsTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.metrics = i
	}

	if p.conf.PPROF &&
		p.pprof == nil {
		i := &pprof.PPROF{
			Address:        p.conf.PPROFAddress,
			Encryption:     p.conf.PPROFEncryption,
			ServerKey:      p.conf.PPROFServerKey,
			ServerCert:     p.conf.PPROFServerCert,
			AllowOrigin:    p.conf.PPROFAllowOrigin,
			TrustedProxies: p.conf.PPROFTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.pprof = i
	}

	if p.recordCleaner == nil &&
		atLeastOneRecordDeleteAfter(p.conf.Paths) {
		p.recordCleaner = &recordcleaner.Cleaner{
			PathConfs: p.conf.Paths,
			Parent:    p,
		}
		p.recordCleaner.Initialize()
	}

	if p.conf.Playback &&
		p.playbackServer == nil {
		i := &playback.Server{
			Address:        p.conf.PlaybackAddress,
			Encryption:     p.conf.PlaybackEncryption,
			ServerKey:      p.conf.PlaybackServerKey,
			ServerCert:     p.conf.PlaybackServerCert,
			AllowOrigin:    p.conf.PlaybackAllowOrigin,
			TrustedProxies: p.conf.PlaybackTrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			PathConfs:      p.conf.Paths,
			AuthManager:    p.authManager,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.playbackServer = i
	}

	if p.pathManager == nil {
		p.pathManager = &pathManager{
			logLevel:          p.conf.LogLevel,
			authManager:       p.authManager,
			rtspAddress:       p.conf.RTSPAddress,
			readTimeout:       p.conf.ReadTimeout,
			writeTimeout:      p.conf.WriteTimeout,
			writeQueueSize:    p.conf.WriteQueueSize,
			udpMaxPayloadSize: p.conf.UDPMaxPayloadSize,
			pathConfs:         p.conf.Paths,
			externalCmdPool:   p.externalCmdPool,
			parent:            p,
			gopCache:          p.conf.GopCache,
		}
		p.pathManager.initialize()

		if p.metrics != nil {
			p.metrics.SetPathManager(p.pathManager)
		}
	}

	if p.conf.RTSP &&
		(p.conf.RTSPEncryption == conf.EncryptionNo ||
			p.conf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspServer == nil {
		_, useUDP := p.conf.RTSPTransports[gortsplib.TransportUDP]
		_, useMulticast := p.conf.RTSPTransports[gortsplib.TransportUDPMulticast]

		i := &rtsp.Server{
			Address:             p.conf.RTSPAddress,
			AuthMethods:         p.conf.RTSPAuthMethods,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			WriteQueueSize:      p.conf.WriteQueueSize,
			UseUDP:              useUDP,
			UseMulticast:        useMulticast,
			RTPAddress:          p.conf.RTPAddress,
			RTCPAddress:         p.conf.RTCPAddress,
			MulticastIPRange:    p.conf.MulticastIPRange,
			MulticastRTPPort:    p.conf.MulticastRTPPort,
			MulticastRTCPPort:   p.conf.MulticastRTCPPort,
			IsTLS:               false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         p.conf.RTSPAddress,
			Transports:          p.conf.RTSPTransports,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtspServer = i

		if p.metrics != nil {
			p.metrics.SetRTSPServer(p.rtspServer)
		}
	}

	if p.conf.RTSP &&
		(p.conf.RTSPEncryption == conf.EncryptionStrict ||
			p.conf.RTSPEncryption == conf.EncryptionOptional) &&
		p.rtspsServer == nil {
		i := &rtsp.Server{
			Address:             p.conf.RTSPSAddress,
			AuthMethods:         p.conf.RTSPAuthMethods,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			WriteQueueSize:      p.conf.WriteQueueSize,
			UseUDP:              false,
			UseMulticast:        false,
			RTPAddress:          "",
			RTCPAddress:         "",
			MulticastIPRange:    "",
			MulticastRTPPort:    0,
			MulticastRTCPPort:   0,
			IsTLS:               true,
			ServerCert:          p.conf.RTSPServerCert,
			ServerKey:           p.conf.RTSPServerKey,
			RTSPAddress:         p.conf.RTSPAddress,
			Transports:          p.conf.RTSPTransports,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtspsServer = i

		if p.metrics != nil {
			p.metrics.SetRTSPSServer(p.rtspsServer)
		}
	}

	if p.conf.RTMP &&
		(p.conf.RTMPEncryption == conf.EncryptionNo ||
			p.conf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpServer == nil {
		i := &rtmp.Server{
			Address:             p.conf.RTMPAddress,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			IsTLS:               false,
			ServerCert:          "",
			ServerKey:           "",
			RTSPAddress:         p.conf.RTSPAddress,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtmpServer = i

		if p.metrics != nil {
			p.metrics.SetRTMPServer(p.rtmpServer)
		}
	}

	if p.conf.RTMP &&
		(p.conf.RTMPEncryption == conf.EncryptionStrict ||
			p.conf.RTMPEncryption == conf.EncryptionOptional) &&
		p.rtmpsServer == nil {
		i := &rtmp.Server{
			Address:             p.conf.RTMPSAddress,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			IsTLS:               true,
			ServerCert:          p.conf.RTMPServerCert,
			ServerKey:           p.conf.RTMPServerKey,
			RTSPAddress:         p.conf.RTSPAddress,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.rtmpsServer = i

		if p.metrics != nil {
			p.metrics.SetRTMPSServer(p.rtmpsServer)
		}
	}

	if p.conf.HLS &&
		p.hlsServer == nil {
		i := &hls.Server{
			Address:         p.conf.HLSAddress,
			Encryption:      p.conf.HLSEncryption,
			ServerKey:       p.conf.HLSServerKey,
			ServerCert:      p.conf.HLSServerCert,
			AllowOrigin:     p.conf.HLSAllowOrigin,
			TrustedProxies:  p.conf.HLSTrustedProxies,
			AlwaysRemux:     p.conf.HLSAlwaysRemux,
			Variant:         p.conf.HLSVariant,
			SegmentCount:    p.conf.HLSSegmentCount,
			SegmentDuration: p.conf.HLSSegmentDuration,
			PartDuration:    p.conf.HLSPartDuration,
			SegmentMaxSize:  p.conf.HLSSegmentMaxSize,
			Directory:       p.conf.HLSDirectory,
			ReadTimeout:     p.conf.ReadTimeout,
			MuxerCloseAfter: p.conf.HLSMuxerCloseAfter,
			PathManager:     p.pathManager,
			Parent:          p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.hlsServer = i

		p.pathManager.setHLSServer(p.hlsServer)

		if p.metrics != nil {
			p.metrics.SetHLSServer(p.hlsServer)
		}
	}

	if p.conf.WebRTC &&
		p.webRTCServer == nil {
		i := &webrtc.Server{
			Address:               p.conf.WebRTCAddress,
			Encryption:            p.conf.WebRTCEncryption,
			ServerKey:             p.conf.WebRTCServerKey,
			ServerCert:            p.conf.WebRTCServerCert,
			AllowOrigin:           p.conf.WebRTCAllowOrigin,
			TrustedProxies:        p.conf.WebRTCTrustedProxies,
			ReadTimeout:           p.conf.ReadTimeout,
			LocalUDPAddress:       p.conf.WebRTCLocalUDPAddress,
			LocalTCPAddress:       p.conf.WebRTCLocalTCPAddress,
			IPsFromInterfaces:     p.conf.WebRTCIPsFromInterfaces,
			IPsFromInterfacesList: p.conf.WebRTCIPsFromInterfacesList,
			AdditionalHosts:       p.conf.WebRTCAdditionalHosts,
			ICEServers:            p.conf.WebRTCICEServers2,
			HandshakeTimeout:      p.conf.WebRTCHandshakeTimeout,
			STUNGatherTimeout:     p.conf.WebRTCSTUNGatherTimeout,
			TrackGatherTimeout:    p.conf.WebRTCTrackGatherTimeout,
			ExternalCmdPool:       p.externalCmdPool,
			PathManager:           p.pathManager,
			Parent:                p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.webRTCServer = i

		if p.metrics != nil {
			p.metrics.SetWebRTCServer(p.webRTCServer)
		}
	}

	if p.conf.SRT &&
		p.srtServer == nil {
		i := &srt.Server{
			Address:             p.conf.SRTAddress,
			RTSPAddress:         p.conf.RTSPAddress,
			ReadTimeout:         p.conf.ReadTimeout,
			WriteTimeout:        p.conf.WriteTimeout,
			UDPMaxPayloadSize:   p.conf.UDPMaxPayloadSize,
			RunOnConnect:        p.conf.RunOnConnect,
			RunOnConnectRestart: p.conf.RunOnConnectRestart,
			RunOnDisconnect:     p.conf.RunOnDisconnect,
			ExternalCmdPool:     p.externalCmdPool,
			PathManager:         p.pathManager,
			Parent:              p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.srtServer = i

		if p.metrics != nil {
			p.metrics.SetSRTServer(p.srtServer)
		}
	}

	if p.conf.API &&
		p.api == nil {
		i := &api.API{
			Address:        p.conf.APIAddress,
			Encryption:     p.conf.APIEncryption,
			ServerKey:      p.conf.APIServerKey,
			ServerCert:     p.conf.APIServerCert,
			AllowOrigin:    p.conf.APIAllowOrigin,
			TrustedProxies: p.conf.APITrustedProxies,
			ReadTimeout:    p.conf.ReadTimeout,
			Conf:           p.conf,
			AuthManager:    p.authManager,
			PathManager:    p.pathManager,
			RTSPServer:     p.rtspServer,
			RTSPSServer:    p.rtspsServer,
			RTMPServer:     p.rtmpServer,
			RTMPSServer:    p.rtmpsServer,
			HLSServer:      p.hlsServer,
			WebRTCServer:   p.webRTCServer,
			SRTServer:      p.srtServer,
			Parent:         p,
		}
		err = i.Initialize()
		if err != nil {
			return err
		}
		p.api = i
	}

	if initial && p.confPath != "" {
		p.confWatcher, err = confwatcher.New(p.confPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *Core) closeResources(newConf *conf.Conf, calledByAPI bool) {
	closeLogger := newConf == nil ||
		newConf.LogLevel != p.conf.LogLevel ||
		!reflect.DeepEqual(newConf.LogDestinations, p.conf.LogDestinations) ||
		newConf.LogFile != p.conf.LogFile

	closeAuthManager := newConf == nil ||
		newConf.AuthMethod != p.conf.AuthMethod ||
		newConf.AuthHTTPAddress != p.conf.AuthHTTPAddress ||
		!reflect.DeepEqual(newConf.AuthHTTPExclude, p.conf.AuthHTTPExclude) ||
		newConf.AuthJWTJWKS != p.conf.AuthJWTJWKS ||
		newConf.AuthJWTClaimKey != p.conf.AuthJWTClaimKey ||
		newConf.ReadTimeout != p.conf.ReadTimeout
	if !closeAuthManager && !reflect.DeepEqual(newConf.AuthInternalUsers, p.conf.AuthInternalUsers) {
		p.authManager.ReloadInternalUsers(newConf.AuthInternalUsers)
	}

	closeMetrics := newConf == nil ||
		newConf.Metrics != p.conf.Metrics ||
		newConf.MetricsAddress != p.conf.MetricsAddress ||
		newConf.MetricsEncryption != p.conf.MetricsEncryption ||
		newConf.MetricsServerKey != p.conf.MetricsServerKey ||
		newConf.MetricsServerCert != p.conf.MetricsServerCert ||
		newConf.MetricsAllowOrigin != p.conf.MetricsAllowOrigin ||
		!reflect.DeepEqual(newConf.MetricsTrustedProxies, p.conf.MetricsTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		closeAuthManager ||
		closeLogger

	closePPROF := newConf == nil ||
		newConf.PPROF != p.conf.PPROF ||
		newConf.PPROFAddress != p.conf.PPROFAddress ||
		newConf.PPROFEncryption != p.conf.PPROFEncryption ||
		newConf.PPROFServerKey != p.conf.PPROFServerKey ||
		newConf.PPROFServerCert != p.conf.PPROFServerCert ||
		newConf.PPROFAllowOrigin != p.conf.PPROFAllowOrigin ||
		!reflect.DeepEqual(newConf.PPROFTrustedProxies, p.conf.PPROFTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		closeAuthManager ||
		closeLogger

	closeRecorderCleaner := newConf == nil ||
		atLeastOneRecordDeleteAfter(newConf.Paths) != atLeastOneRecordDeleteAfter(p.conf.Paths) ||
		closeLogger
	if !closeRecorderCleaner && p.recordCleaner != nil && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.recordCleaner.ReloadPathConfs(newConf.Paths)
	}

	closePlaybackServer := newConf == nil ||
		newConf.Playback != p.conf.Playback ||
		newConf.PlaybackAddress != p.conf.PlaybackAddress ||
		newConf.PlaybackEncryption != p.conf.PlaybackEncryption ||
		newConf.PlaybackServerKey != p.conf.PlaybackServerKey ||
		newConf.PlaybackServerCert != p.conf.PlaybackServerCert ||
		newConf.PlaybackAllowOrigin != p.conf.PlaybackAllowOrigin ||
		!reflect.DeepEqual(newConf.PlaybackTrustedProxies, p.conf.PlaybackTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		closeAuthManager ||
		closeLogger
	if !closePlaybackServer && p.playbackServer != nil && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.playbackServer.ReloadPathConfs(newConf.Paths)
	}

	closePathManager := newConf == nil ||
		newConf.LogLevel != p.conf.LogLevel ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.UDPMaxPayloadSize != p.conf.UDPMaxPayloadSize ||
		newConf.GopCache != p.conf.GopCache ||
		closeMetrics ||
		closeAuthManager ||
		closeLogger
	if !closePathManager && !reflect.DeepEqual(newConf.Paths, p.conf.Paths) {
		p.pathManager.ReloadPathConfs(newConf.Paths)
	}

	closeRTSPServer := newConf == nil ||
		newConf.RTSP != p.conf.RTSP ||
		newConf.RTSPEncryption != p.conf.RTSPEncryption ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, p.conf.RTSPAuthMethods) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.RTPAddress != p.conf.RTPAddress ||
		newConf.RTCPAddress != p.conf.RTCPAddress ||
		newConf.MulticastIPRange != p.conf.MulticastIPRange ||
		newConf.MulticastRTPPort != p.conf.MulticastRTPPort ||
		newConf.MulticastRTCPPort != p.conf.MulticastRTCPPort ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPTransports, p.conf.RTSPTransports) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTSPSServer := newConf == nil ||
		newConf.RTSP != p.conf.RTSP ||
		newConf.RTSPEncryption != p.conf.RTSPEncryption ||
		newConf.RTSPSAddress != p.conf.RTSPSAddress ||
		!reflect.DeepEqual(newConf.RTSPAuthMethods, p.conf.RTSPAuthMethods) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.WriteQueueSize != p.conf.WriteQueueSize ||
		newConf.RTSPServerCert != p.conf.RTSPServerCert ||
		newConf.RTSPServerKey != p.conf.RTSPServerKey ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		!reflect.DeepEqual(newConf.RTSPTransports, p.conf.RTSPTransports) ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPServer := newConf == nil ||
		newConf.RTMP != p.conf.RTMP ||
		newConf.RTMPEncryption != p.conf.RTMPEncryption ||
		newConf.RTMPAddress != p.conf.RTMPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeRTMPSServer := newConf == nil ||
		newConf.RTMP != p.conf.RTMP ||
		newConf.RTMPEncryption != p.conf.RTMPEncryption ||
		newConf.RTMPSAddress != p.conf.RTMPSAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.RTMPServerCert != p.conf.RTMPServerCert ||
		newConf.RTMPServerKey != p.conf.RTMPServerKey ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeHLSServer := newConf == nil ||
		newConf.HLS != p.conf.HLS ||
		newConf.HLSAddress != p.conf.HLSAddress ||
		newConf.HLSEncryption != p.conf.HLSEncryption ||
		newConf.HLSServerKey != p.conf.HLSServerKey ||
		newConf.HLSServerCert != p.conf.HLSServerCert ||
		newConf.HLSAllowOrigin != p.conf.HLSAllowOrigin ||
		!reflect.DeepEqual(newConf.HLSTrustedProxies, p.conf.HLSTrustedProxies) ||
		newConf.HLSAlwaysRemux != p.conf.HLSAlwaysRemux ||
		newConf.HLSVariant != p.conf.HLSVariant ||
		newConf.HLSSegmentCount != p.conf.HLSSegmentCount ||
		newConf.HLSSegmentDuration != p.conf.HLSSegmentDuration ||
		newConf.HLSPartDuration != p.conf.HLSPartDuration ||
		newConf.HLSSegmentMaxSize != p.conf.HLSSegmentMaxSize ||
		newConf.HLSDirectory != p.conf.HLSDirectory ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.HLSMuxerCloseAfter != p.conf.HLSMuxerCloseAfter ||
		closePathManager ||
		closeMetrics ||
		closeLogger

	closeWebRTCServer := newConf == nil ||
		newConf.WebRTC != p.conf.WebRTC ||
		newConf.WebRTCAddress != p.conf.WebRTCAddress ||
		newConf.WebRTCEncryption != p.conf.WebRTCEncryption ||
		newConf.WebRTCServerKey != p.conf.WebRTCServerKey ||
		newConf.WebRTCServerCert != p.conf.WebRTCServerCert ||
		newConf.WebRTCAllowOrigin != p.conf.WebRTCAllowOrigin ||
		!reflect.DeepEqual(newConf.WebRTCTrustedProxies, p.conf.WebRTCTrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WebRTCLocalUDPAddress != p.conf.WebRTCLocalUDPAddress ||
		newConf.WebRTCLocalTCPAddress != p.conf.WebRTCLocalTCPAddress ||
		newConf.WebRTCIPsFromInterfaces != p.conf.WebRTCIPsFromInterfaces ||
		!reflect.DeepEqual(newConf.WebRTCIPsFromInterfacesList, p.conf.WebRTCIPsFromInterfacesList) ||
		!reflect.DeepEqual(newConf.WebRTCAdditionalHosts, p.conf.WebRTCAdditionalHosts) ||
		!reflect.DeepEqual(newConf.WebRTCICEServers2, p.conf.WebRTCICEServers2) ||
		newConf.WebRTCHandshakeTimeout != p.conf.WebRTCHandshakeTimeout ||
		newConf.WebRTCSTUNGatherTimeout != p.conf.WebRTCSTUNGatherTimeout ||
		newConf.WebRTCTrackGatherTimeout != p.conf.WebRTCTrackGatherTimeout ||
		closeMetrics ||
		closePathManager ||
		closeLogger

	closeSRTServer := newConf == nil ||
		newConf.SRT != p.conf.SRT ||
		newConf.SRTAddress != p.conf.SRTAddress ||
		newConf.RTSPAddress != p.conf.RTSPAddress ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		newConf.WriteTimeout != p.conf.WriteTimeout ||
		newConf.UDPMaxPayloadSize != p.conf.UDPMaxPayloadSize ||
		newConf.RunOnConnect != p.conf.RunOnConnect ||
		newConf.RunOnConnectRestart != p.conf.RunOnConnectRestart ||
		newConf.RunOnDisconnect != p.conf.RunOnDisconnect ||
		closePathManager ||
		closeLogger

	closeAPI := newConf == nil ||
		newConf.API != p.conf.API ||
		newConf.APIAddress != p.conf.APIAddress ||
		newConf.APIEncryption != p.conf.APIEncryption ||
		newConf.APIServerKey != p.conf.APIServerKey ||
		newConf.APIServerCert != p.conf.APIServerCert ||
		newConf.APIAllowOrigin != p.conf.APIAllowOrigin ||
		!reflect.DeepEqual(newConf.APITrustedProxies, p.conf.APITrustedProxies) ||
		newConf.ReadTimeout != p.conf.ReadTimeout ||
		closeAuthManager ||
		closePathManager ||
		closeRTSPServer ||
		closeRTSPSServer ||
		closeRTMPServer ||
		closeHLSServer ||
		closeWebRTCServer ||
		closeSRTServer ||
		closeLogger

	if newConf == nil && p.confWatcher != nil {
		p.confWatcher.Close()
		p.confWatcher = nil
	}

	if p.api != nil {
		if closeAPI {
			p.api.Close()
			p.api = nil
		} else if !calledByAPI { // avoid a loop
			p.api.ReloadConf(newConf)
		}
	}

	if closeSRTServer && p.srtServer != nil {
		if p.metrics != nil {
			p.metrics.SetSRTServer(nil)
		}

		p.srtServer.Close()
		p.srtServer = nil
	}

	if closeWebRTCServer && p.webRTCServer != nil {
		if p.metrics != nil {
			p.metrics.SetWebRTCServer(nil)
		}

		p.webRTCServer.Close()
		p.webRTCServer = nil
	}

	if closeHLSServer && p.hlsServer != nil {
		if p.metrics != nil {
			p.metrics.SetHLSServer(nil)
		}

		p.pathManager.setHLSServer(nil)

		p.hlsServer.Close()
		p.hlsServer = nil
	}

	if closeRTMPSServer && p.rtmpsServer != nil {
		if p.metrics != nil {
			p.metrics.SetRTMPSServer(nil)
		}

		p.rtmpsServer.Close()
		p.rtmpsServer = nil
	}

	if closeRTMPServer && p.rtmpServer != nil {
		if p.metrics != nil {
			p.metrics.SetRTMPServer(nil)
		}

		p.rtmpServer.Close()
		p.rtmpServer = nil
	}

	if closeRTSPSServer && p.rtspsServer != nil {
		if p.metrics != nil {
			p.metrics.SetRTSPSServer(nil)
		}

		p.rtspsServer.Close()
		p.rtspsServer = nil
	}

	if closeRTSPServer && p.rtspServer != nil {
		if p.metrics != nil {
			p.metrics.SetRTSPServer(nil)
		}

		p.rtspServer.Close()
		p.rtspServer = nil
	}

	if closePathManager && p.pathManager != nil {
		if p.metrics != nil {
			p.metrics.SetPathManager(nil)
		}

		p.pathManager.close()
		p.pathManager = nil
	}

	if closePlaybackServer && p.playbackServer != nil {
		p.playbackServer.Close()
		p.playbackServer = nil
	}

	if closeRecorderCleaner && p.recordCleaner != nil {
		p.recordCleaner.Close()
		p.recordCleaner = nil
	}

	if closePPROF && p.pprof != nil {
		p.pprof.Close()
		p.pprof = nil
	}

	if closeMetrics && p.metrics != nil {
		p.metrics.Close()
		p.metrics = nil
	}

	if closeAuthManager && p.authManager != nil {
		p.authManager = nil
	}

	if newConf == nil && p.externalCmdPool != nil {
		p.Log(logger.Info, "waiting for running hooks")
		p.externalCmdPool.Close()
	}

	if closeLogger && p.logger != nil {
		p.logger.Close()
		p.logger = nil
	}
}

func (p *Core) reloadConf(newConf *conf.Conf, calledByAPI bool) error {
	p.closeResources(newConf, calledByAPI)
	p.conf = newConf
	return p.createResources(false)
}

// APIConfigSet is called by api.
func (p *Core) APIConfigSet(conf *conf.Conf) {
	select {
	case p.chAPIConfigSet <- conf:
	case <-p.ctx.Done():
	}
}
