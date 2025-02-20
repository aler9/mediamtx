// Package conf contains the struct that holds the configuration of the software.
package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/auth"

	"github.com/bluenviron/mediamtx/internal/conf/decrypt"
	"github.com/bluenviron/mediamtx/internal/conf/env"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/conf/yamlwrapper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// ErrPathNotFound is returned when a path is not found.
var ErrPathNotFound = errors.New("path not found")

func sortedKeys(paths map[string]*OptionalPath) []string {
	ret := make([]string, len(paths))
	i := 0
	for name := range paths {
		ret[i] = name
		i++
	}
	sort.Strings(ret)
	return ret
}

func firstThatExists(paths []string) string {
	for _, pa := range paths {
		_, err := os.Stat(pa)
		if err == nil {
			return pa
		}
	}
	return ""
}

func contains(list []auth.VerifyMethod, item auth.VerifyMethod) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

func copyStructFields(dest interface{}, source interface{}) {
	rvsource := reflect.ValueOf(source).Elem()
	rvdest := reflect.ValueOf(dest)
	nf := rvsource.NumField()
	var zero reflect.Value

	for i := 0; i < nf; i++ {
		fnew := rvsource.Field(i)
		f := rvdest.Elem().FieldByName(rvsource.Type().Field(i).Name)
		if f == zero {
			continue
		}

		if fnew.Kind() == reflect.Pointer {
			if !fnew.IsNil() {
				if f.Kind() == reflect.Ptr {
					f.Set(fnew)
				} else {
					f.Set(fnew.Elem())
				}
			}
		} else {
			f.Set(fnew)
		}
	}
}

func mustParseCIDR(v string) net.IPNet {
	_, ne, err := net.ParseCIDR(v)
	if err != nil {
		panic(err)
	}
	if ipv4 := ne.IP.To4(); ipv4 != nil {
		return net.IPNet{IP: ipv4, Mask: ne.Mask[len(ne.Mask)-4 : len(ne.Mask)]}
	}
	return *ne
}

func anyPathHasDeprecatedCredentials(pathDefaults Path, paths map[string]*OptionalPath) bool {
	if pathDefaults.PublishUser != nil ||
		pathDefaults.PublishPass != nil ||
		pathDefaults.PublishIPs != nil ||
		pathDefaults.ReadUser != nil ||
		pathDefaults.ReadPass != nil ||
		pathDefaults.ReadIPs != nil {
		return true
	}

	for _, pa := range paths {
		if pa != nil {
			rva := reflect.ValueOf(pa.Values).Elem()
			if rva.FieldByName("PublishUser").Interface().(*Credential) != nil ||
				rva.FieldByName("PublishPass").Interface().(*Credential) != nil ||
				rva.FieldByName("PublishIPs").Interface().(*IPNetworks) != nil ||
				rva.FieldByName("ReadUser").Interface().(*Credential) != nil ||
				rva.FieldByName("ReadPass").Interface().(*Credential) != nil ||
				rva.FieldByName("ReadIPs").Interface().(*IPNetworks) != nil {
				return true
			}
		}
	}
	return false
}

var defaultAuthInternalUsers = AuthInternalUsers{
	{
		User: "any",
		Pass: "",
		Permissions: []AuthInternalUserPermission{
			{
				Action: AuthActionPublish,
			},
			{
				Action: AuthActionRead,
			},
			{
				Action: AuthActionPlayback,
			},
		},
	},
	{
		User: "any",
		Pass: "",
		IPs:  IPNetworks{mustParseCIDR("127.0.0.1/32"), mustParseCIDR("::1/128")},
		Permissions: []AuthInternalUserPermission{
			{
				Action: AuthActionAPI,
			},
			{
				Action: AuthActionMetrics,
			},
			{
				Action: AuthActionPprof,
			},
		},
	},
}

// Conf is a configuration.
// WARNING: Avoid using slices directly due to https://github.com/golang/go/issues/21092
type Conf struct {
	// General
	LogLevel            LogLevel        `json:"logLevel"`
	LogDestinations     LogDestinations `json:"logDestinations"`
	LogFile             string          `json:"logFile"`
	ReadTimeout         Duration        `json:"readTimeout"`
	WriteTimeout        Duration        `json:"writeTimeout"`
	ReadBufferCount     *int            `json:"readBufferCount,omitempty"` // deprecated
	WriteQueueSize      int             `json:"writeQueueSize"`
	UDPMaxPayloadSize   int             `json:"udpMaxPayloadSize"`
	RunOnConnect        string          `json:"runOnConnect"`
	RunOnConnectRestart bool            `json:"runOnConnectRestart"`
	RunOnDisconnect     string          `json:"runOnDisconnect"`
	GopCache            bool            `json:"gopCache"`

	// Authentication
	AuthMethod                AuthMethod                  `json:"authMethod"`
	AuthInternalUsers         AuthInternalUsers           `json:"authInternalUsers"`
	AuthHTTPAddress           string                      `json:"authHTTPAddress"`
	ExternalAuthenticationURL *string                     `json:"externalAuthenticationURL,omitempty"` // deprecated
	AuthHTTPExclude           AuthInternalUserPermissions `json:"authHTTPExclude"`
	AuthJWTJWKS               string                      `json:"authJWTJWKS"`
	AuthJWTClaimKey           string                      `json:"authJWTClaimKey"`

	// Control API
	API               bool       `json:"api"`
	APIAddress        string     `json:"apiAddress"`
	APIEncryption     bool       `json:"apiEncryption"`
	APIServerKey      string     `json:"apiServerKey"`
	APIServerCert     string     `json:"apiServerCert"`
	APIAllowOrigin    string     `json:"apiAllowOrigin"`
	APITrustedProxies IPNetworks `json:"apiTrustedProxies"`

	// Metrics
	Metrics               bool       `json:"metrics"`
	MetricsAddress        string     `json:"metricsAddress"`
	MetricsEncryption     bool       `json:"metricsEncryption"`
	MetricsServerKey      string     `json:"metricsServerKey"`
	MetricsServerCert     string     `json:"metricsServerCert"`
	MetricsAllowOrigin    string     `json:"metricsAllowOrigin"`
	MetricsTrustedProxies IPNetworks `json:"metricsTrustedProxies"`

	// PPROF
	PPROF               bool       `json:"pprof"`
	PPROFAddress        string     `json:"pprofAddress"`
	PPROFEncryption     bool       `json:"pprofEncryption"`
	PPROFServerKey      string     `json:"pprofServerKey"`
	PPROFServerCert     string     `json:"pprofServerCert"`
	PPROFAllowOrigin    string     `json:"pprofAllowOrigin"`
	PPROFTrustedProxies IPNetworks `json:"pprofTrustedProxies"`

	// Playback
	Playback               bool       `json:"playback"`
	PlaybackAddress        string     `json:"playbackAddress"`
	PlaybackEncryption     bool       `json:"playbackEncryption"`
	PlaybackServerKey      string     `json:"playbackServerKey"`
	PlaybackServerCert     string     `json:"playbackServerCert"`
	PlaybackAllowOrigin    string     `json:"playbackAllowOrigin"`
	PlaybackTrustedProxies IPNetworks `json:"playbackTrustedProxies"`

	// RTSP server
	RTSP              bool             `json:"rtsp"`
	RTSPDisable       *bool            `json:"rtspDisable,omitempty"` // deprecated
	Protocols         *RTSPTransports  `json:"protocols,omitempty"`   // deprecated
	RTSPTransports    RTSPTransports   `json:"rtspTransports"`
	Encryption        *Encryption      `json:"encryption,omitempty"` // deprecated
	RTSPEncryption    Encryption       `json:"rtspEncryption"`
	RTSPAddress       string           `json:"rtspAddress"`
	RTSPSAddress      string           `json:"rtspsAddress"`
	RTPAddress        string           `json:"rtpAddress"`
	RTCPAddress       string           `json:"rtcpAddress"`
	MulticastIPRange  string           `json:"multicastIPRange"`
	MulticastRTPPort  int              `json:"multicastRTPPort"`
	MulticastRTCPPort int              `json:"multicastRTCPPort"`
	ServerKey         *string          `json:"serverKey,omitempty"`
	ServerCert        *string          `json:"serverCert,omitempty"`
	RTSPServerKey     string           `json:"rtspServerKey"`
	RTSPServerCert    string           `json:"rtspServerCert"`
	AuthMethods       *RTSPAuthMethods `json:"authMethods,omitempty"` // deprecated
	RTSPAuthMethods   RTSPAuthMethods  `json:"rtspAuthMethods"`

	// RTMP server
	RTMP           bool       `json:"rtmp"`
	RTMPDisable    *bool      `json:"rtmpDisable,omitempty"` // deprecated
	RTMPAddress    string     `json:"rtmpAddress"`
	RTMPEncryption Encryption `json:"rtmpEncryption"`
	RTMPSAddress   string     `json:"rtmpsAddress"`
	RTMPServerKey  string     `json:"rtmpServerKey"`
	RTMPServerCert string     `json:"rtmpServerCert"`

	// HLS server
	HLS                bool       `json:"hls"`
	HLSDisable         *bool      `json:"hlsDisable,omitempty"` // deprecated
	HLSAddress         string     `json:"hlsAddress"`
	HLSEncryption      bool       `json:"hlsEncryption"`
	HLSServerKey       string     `json:"hlsServerKey"`
	HLSServerCert      string     `json:"hlsServerCert"`
	HLSAllowOrigin     string     `json:"hlsAllowOrigin"`
	HLSTrustedProxies  IPNetworks `json:"hlsTrustedProxies"`
	HLSAlwaysRemux     bool       `json:"hlsAlwaysRemux"`
	HLSVariant         HLSVariant `json:"hlsVariant"`
	HLSSegmentCount    int        `json:"hlsSegmentCount"`
	HLSSegmentDuration Duration   `json:"hlsSegmentDuration"`
	HLSPartDuration    Duration   `json:"hlsPartDuration"`
	HLSSegmentMaxSize  StringSize `json:"hlsSegmentMaxSize"`
	HLSDirectory       string     `json:"hlsDirectory"`
	HLSMuxerCloseAfter Duration   `json:"hlsMuxerCloseAfter"`

	// WebRTC server
	WebRTC                      bool             `json:"webrtc"`
	WebRTCDisable               *bool            `json:"webrtcDisable,omitempty"` // deprecated
	WebRTCAddress               string           `json:"webrtcAddress"`
	WebRTCEncryption            bool             `json:"webrtcEncryption"`
	WebRTCServerKey             string           `json:"webrtcServerKey"`
	WebRTCServerCert            string           `json:"webrtcServerCert"`
	WebRTCAllowOrigin           string           `json:"webrtcAllowOrigin"`
	WebRTCTrustedProxies        IPNetworks       `json:"webrtcTrustedProxies"`
	WebRTCLocalUDPAddress       string           `json:"webrtcLocalUDPAddress"`
	WebRTCLocalTCPAddress       string           `json:"webrtcLocalTCPAddress"`
	WebRTCIPsFromInterfaces     bool             `json:"webrtcIPsFromInterfaces"`
	WebRTCIPsFromInterfacesList []string         `json:"webrtcIPsFromInterfacesList"`
	WebRTCAdditionalHosts       []string         `json:"webrtcAdditionalHosts"`
	WebRTCICEServers2           WebRTCICEServers `json:"webrtcICEServers2"`
	WebRTCHandshakeTimeout      Duration         `json:"webrtcHandshakeTimeout"`
	WebRTCTrackGatherTimeout    Duration         `json:"webrtcTrackGatherTimeout"`
	WebRTCSTUNGatherTimeout     Duration         `json:"webrtcSTUNGatherTimeout"`
	WebRTCICEUDPMuxAddress      *string          `json:"webrtcICEUDPMuxAddress,omitempty"`  // deprecated
	WebRTCICETCPMuxAddress      *string          `json:"webrtcICETCPMuxAddress,omitempty"`  // deprecated
	WebRTCICEHostNAT1To1IPs     *[]string        `json:"webrtcICEHostNAT1To1IPs,omitempty"` // deprecated
	WebRTCICEServers            *[]string        `json:"webrtcICEServers,omitempty"`        // deprecated

	// SRT server
	SRT        bool   `json:"srt"`
	SRTAddress string `json:"srtAddress"`

	// Record (deprecated)
	Record                *bool         `json:"record,omitempty"`                // deprecated
	RecordPath            *string       `json:"recordPath,omitempty"`            // deprecated
	RecordFormat          *RecordFormat `json:"recordFormat,omitempty"`          // deprecated
	RecordPartDuration    *Duration     `json:"recordPartDuration,omitempty"`    // deprecated
	RecordSegmentDuration *Duration     `json:"recordSegmentDuration,omitempty"` // deprecated
	RecordDeleteAfter     *Duration     `json:"recordDeleteAfter,omitempty"`     // deprecated

	// Path defaults
	PathDefaults Path `json:"pathDefaults"`

	// Paths
	OptionalPaths map[string]*OptionalPath `json:"paths"`
	Paths         map[string]*Path         `json:"-"` // filled by Check()
}

func (conf *Conf) setDefaults() {
	// General
	conf.LogLevel = LogLevel(logger.Info)
	conf.LogDestinations = LogDestinations{logger.DestinationStdout}
	conf.LogFile = "mediamtx.log"
	conf.ReadTimeout = 10 * Duration(time.Second)
	conf.WriteTimeout = 10 * Duration(time.Second)
	conf.WriteQueueSize = 512
	conf.UDPMaxPayloadSize = 1472

	// Authentication
	conf.AuthInternalUsers = defaultAuthInternalUsers
	conf.AuthHTTPExclude = []AuthInternalUserPermission{
		{
			Action: AuthActionAPI,
		},
		{
			Action: AuthActionMetrics,
		},
		{
			Action: AuthActionPprof,
		},
	}
	conf.AuthJWTClaimKey = "mediamtx_permissions"

	// Control API
	conf.APIAddress = ":9997"
	conf.APIServerKey = "server.key"
	conf.APIServerCert = "server.crt"
	conf.APIAllowOrigin = "*"

	// Metrics
	conf.MetricsAddress = ":9998"
	conf.MetricsServerKey = "server.key"
	conf.MetricsServerCert = "server.crt"
	conf.MetricsAllowOrigin = "*"

	// PPROF
	conf.PPROFAddress = ":9999"
	conf.PPROFServerKey = "server.key"
	conf.PPROFServerCert = "server.crt"
	conf.PPROFAllowOrigin = "*"

	// Playback server
	conf.PlaybackAddress = ":9996"
	conf.PlaybackServerKey = "server.key"
	conf.PlaybackServerCert = "server.crt"
	conf.PlaybackAllowOrigin = "*"

	// RTSP server
	conf.RTSP = true
	conf.RTSPTransports = RTSPTransports{
		gortsplib.TransportUDP:          {},
		gortsplib.TransportUDPMulticast: {},
		gortsplib.TransportTCP:          {},
	}
	conf.RTSPAddress = ":8554"
	conf.RTSPSAddress = ":8322"
	conf.RTPAddress = ":8000"
	conf.RTCPAddress = ":8001"
	conf.MulticastIPRange = "224.1.0.0/16"
	conf.MulticastRTPPort = 8002
	conf.MulticastRTCPPort = 8003
	conf.RTSPServerKey = "server.key"
	conf.RTSPServerCert = "server.crt"
	conf.RTSPAuthMethods = RTSPAuthMethods{auth.VerifyMethodBasic}

	// RTMP server
	conf.RTMP = true
	conf.RTMPAddress = ":1935"
	conf.RTMPSAddress = ":1936"
	conf.RTMPServerKey = "server.key"
	conf.RTMPServerCert = "server.crt"

	// HLS
	conf.HLS = true
	conf.HLSAddress = ":8888"
	conf.HLSServerKey = "server.key"
	conf.HLSServerCert = "server.crt"
	conf.HLSAllowOrigin = "*"
	conf.HLSVariant = HLSVariant(gohlslib.MuxerVariantLowLatency)
	conf.HLSSegmentCount = 7
	conf.HLSSegmentDuration = 1 * Duration(time.Second)
	conf.HLSPartDuration = 200 * Duration(time.Millisecond)
	conf.HLSSegmentMaxSize = 50 * 1024 * 1024
	conf.HLSMuxerCloseAfter = 60 * Duration(time.Second)

	// WebRTC server
	conf.WebRTC = true
	conf.WebRTCAddress = ":8889"
	conf.WebRTCServerKey = "server.key"
	conf.WebRTCServerCert = "server.crt"
	conf.WebRTCAllowOrigin = "*"
	conf.WebRTCLocalUDPAddress = ":8189"
	conf.WebRTCIPsFromInterfaces = true
	conf.WebRTCIPsFromInterfacesList = []string{}
	conf.WebRTCAdditionalHosts = []string{}
	conf.WebRTCICEServers2 = []WebRTCICEServer{}
	conf.WebRTCHandshakeTimeout = 10 * Duration(time.Second)
	conf.WebRTCTrackGatherTimeout = 2 * Duration(time.Second)
	conf.WebRTCSTUNGatherTimeout = 5 * Duration(time.Second)

	// SRT server
	conf.SRT = true
	conf.SRTAddress = ":8890"

	conf.PathDefaults.setDefaults()
}

// Load loads a Conf.
func Load(fpath string, defaultConfPaths []string, l logger.Writer) (*Conf, string, error) {
	conf := &Conf{}

	fpath, err := conf.loadFromFile(fpath, defaultConfPaths)
	if err != nil {
		return nil, "", err
	}

	err = env.Load("RTSP", conf) // legacy prefix
	if err != nil {
		return nil, "", err
	}

	err = env.Load("MTX", conf)
	if err != nil {
		return nil, "", err
	}

	err = conf.Validate(l)
	if err != nil {
		return nil, "", err
	}

	return conf, fpath, nil
}

func (conf *Conf) loadFromFile(fpath string, defaultConfPaths []string) (string, error) {
	if fpath == "" {
		fpath = firstThatExists(defaultConfPaths)

		// when the configuration file is not explicitly set,
		// it is optional.
		if fpath == "" {
			conf.setDefaults()
			return "", nil
		}
	}

	byts, err := os.ReadFile(fpath)
	if err != nil {
		return "", err
	}

	if key, ok := os.LookupEnv("RTSP_CONFKEY"); ok { // legacy format
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return "", err
		}
	}

	if key, ok := os.LookupEnv("MTX_CONFKEY"); ok {
		byts, err = decrypt.Decrypt(key, byts)
		if err != nil {
			return "", err
		}
	}

	err = yamlwrapper.Unmarshal(byts, conf)
	if err != nil {
		return "", err
	}

	return fpath, nil
}

// Clone clones the configuration.
func (conf Conf) Clone() *Conf {
	enc, err := json.Marshal(conf)
	if err != nil {
		panic(err)
	}

	var dest Conf
	err = json.Unmarshal(enc, &dest)
	if err != nil {
		panic(err)
	}

	return &dest
}

type nilLogger struct{}

func (nilLogger) Log(_ logger.Level, _ string, _ ...interface{}) {
}

// Validate checks the configuration for errors.
func (conf *Conf) Validate(l logger.Writer) error {
	if l == nil {
		l = &nilLogger{}
	}

	// General

	if conf.ReadTimeout <= 0 {
		return fmt.Errorf("'readTimeout' must be greater than zero")
	}
	if conf.WriteTimeout <= 0 {
		return fmt.Errorf("'writeTimeout' must be greater than zero")
	}
	if conf.ReadBufferCount != nil {
		l.Log(logger.Warn, "parameter 'readBufferCount' is deprecated and has been replaced with 'writeQueueSize'")
		conf.WriteQueueSize = *conf.ReadBufferCount
	}
	if (conf.WriteQueueSize & (conf.WriteQueueSize - 1)) != 0 {
		return fmt.Errorf("'writeQueueSize' must be a power of two")
	}
	if conf.UDPMaxPayloadSize > 1472 {
		return fmt.Errorf("'udpMaxPayloadSize' must be less than 1472")
	}

	// Authentication

	if conf.ExternalAuthenticationURL != nil {
		l.Log(logger.Warn, "parameter 'externalAuthenticationURL' is deprecated "+
			"and has been replaced with 'authMethod' and 'authHTTPAddress'")
		conf.AuthMethod = AuthMethodHTTP
		conf.AuthHTTPAddress = *conf.ExternalAuthenticationURL
	}
	if conf.AuthHTTPAddress != "" &&
		!strings.HasPrefix(conf.AuthHTTPAddress, "http://") &&
		!strings.HasPrefix(conf.AuthHTTPAddress, "https://") {
		return fmt.Errorf("'externalAuthenticationURL' must be a HTTP URL")
	}
	if conf.AuthJWTJWKS != "" &&
		!strings.HasPrefix(conf.AuthJWTJWKS, "http://") &&
		!strings.HasPrefix(conf.AuthJWTJWKS, "https://") {
		return fmt.Errorf("'authJWTJWKS' must be a HTTP URL")
	}
	deprecatedCredentialsMode := false
	if anyPathHasDeprecatedCredentials(conf.PathDefaults, conf.OptionalPaths) {
		l.Log(logger.Warn, "you are using one or more authentication-related deprecated parameters "+
			"(publishUser, publishPass, publishIPs, readUser, readPass, readIPs). "+
			"These have been replaced by 'authInternalUsers'")

		if conf.AuthInternalUsers != nil && !reflect.DeepEqual(conf.AuthInternalUsers, defaultAuthInternalUsers) {
			return fmt.Errorf("authInternalUsers and legacy credentials " +
				"(publishUser, publishPass, publishIPs, readUser, readPass, readIPs) cannot be used together")
		}

		conf.AuthInternalUsers = []AuthInternalUser{
			{
				User: "any",
				Permissions: []AuthInternalUserPermission{
					{
						Action: AuthActionPlayback,
					},
				},
			},
			{
				User: "any",
				IPs:  IPNetworks{mustParseCIDR("127.0.0.1/32"), mustParseCIDR("::1/128")},
				Permissions: []AuthInternalUserPermission{
					{
						Action: AuthActionAPI,
					},
					{
						Action: AuthActionMetrics,
					},
					{
						Action: AuthActionPprof,
					},
				},
			},
		}
		deprecatedCredentialsMode = true
	}
	switch conf.AuthMethod {
	case AuthMethodHTTP:
		if conf.AuthHTTPAddress == "" {
			return fmt.Errorf("'authHTTPAddress' is empty")
		}

	case AuthMethodJWT:
		if conf.AuthJWTJWKS == "" {
			return fmt.Errorf("'authJWTJWKS' is empty")
		}
		if conf.AuthJWTClaimKey == "" {
			return fmt.Errorf("'authJWTClaimKey' is empty")
		}
	}

	// RTSP

	if conf.RTSPDisable != nil {
		l.Log(logger.Warn, "parameter 'rtspDisabled' is deprecated and has been replaced with 'rtsp'")
		conf.RTSP = !*conf.RTSPDisable
	}
	if conf.Protocols != nil {
		l.Log(logger.Warn, "parameter 'protocols' is deprecated and has been replaced with 'rtspTransports'")
		conf.RTSPTransports = *conf.Protocols
	}
	if conf.Encryption != nil {
		l.Log(logger.Warn, "parameter 'encryption' is deprecated and has been replaced with 'rtspEncryption'")
		conf.RTSPEncryption = *conf.Encryption
	}
	if conf.RTSPEncryption == EncryptionStrict {
		if _, ok := conf.RTSPTransports[gortsplib.TransportUDP]; ok {
			return fmt.Errorf("strict encryption cannot be used with the UDP transport protocol")
		}
		if _, ok := conf.RTSPTransports[gortsplib.TransportUDPMulticast]; ok {
			return fmt.Errorf("strict encryption cannot be used with the UDP-multicast transport protocol")
		}
	}
	if conf.AuthMethods != nil {
		l.Log(logger.Warn, "parameter 'authMethods' is deprecated and has been replaced with 'rtspAuthMethods'")
		conf.RTSPAuthMethods = *conf.AuthMethods
	}
	if contains(conf.RTSPAuthMethods, auth.VerifyMethodDigestMD5) {
		if conf.AuthMethod != AuthMethodInternal {
			return fmt.Errorf("when RTSP digest is enabled, the only supported auth method is 'internal'")
		}
		for _, user := range conf.AuthInternalUsers {
			if user.User.IsHashed() || user.Pass.IsHashed() {
				return fmt.Errorf("when RTSP digest is enabled, hashed credentials cannot be used")
			}
		}
	}
	if conf.ServerCert != nil {
		l.Log(logger.Warn, "parameter 'serverCert' is deprecated and has been replaced with 'rtspServerCert'")
		conf.RTSPServerCert = *conf.ServerCert
	}
	if conf.ServerKey != nil {
		l.Log(logger.Warn, "parameter 'serverKey' is deprecated and has been replaced with 'rtspServerKey'")
		conf.RTSPServerKey = *conf.ServerKey
	}
	if len(conf.RTSPAuthMethods) == 0 {
		return fmt.Errorf("at least one 'rtspAuthMethods' must be provided")
	}

	// RTMP

	if conf.RTMPDisable != nil {
		l.Log(logger.Warn, "parameter 'rtmpDisabled' is deprecated and has been replaced with 'rtmp'")
		conf.RTMP = !*conf.RTMPDisable
	}

	// HLS

	if conf.HLSDisable != nil {
		l.Log(logger.Warn, "parameter 'hlsDisable' is deprecated and has been replaced with 'hls'")
		conf.HLS = !*conf.HLSDisable
	}

	// WebRTC

	if conf.WebRTCDisable != nil {
		l.Log(logger.Warn, "parameter 'webrtcDisable' is deprecated and has been replaced with 'webrtc'")
		conf.WebRTC = !*conf.WebRTCDisable
	}
	if conf.WebRTCICEUDPMuxAddress != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEUDPMuxAdderss' is deprecated "+
			"and has been replaced with 'webrtcLocalUDPAddress'")
		conf.WebRTCLocalUDPAddress = *conf.WebRTCICEUDPMuxAddress
	}
	if conf.WebRTCICETCPMuxAddress != nil {
		l.Log(logger.Warn, "parameter 'webrtcICETCPMuxAddress' is deprecated "+
			"and has been replaced with 'webrtcLocalTCPAddress'")
		conf.WebRTCLocalTCPAddress = *conf.WebRTCICETCPMuxAddress
	}
	if conf.WebRTCICEHostNAT1To1IPs != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEHostNAT1To1IPs' is deprecated "+
			"and has been replaced with 'webrtcAdditionalHosts'")
		conf.WebRTCAdditionalHosts = *conf.WebRTCICEHostNAT1To1IPs
	}
	if conf.WebRTCICEServers != nil {
		l.Log(logger.Warn, "parameter 'webrtcICEServers' is deprecated "+
			"and has been replaced with 'webrtcICEServers2'")

		for _, server := range *conf.WebRTCICEServers {
			parts := strings.Split(server, ":")
			if len(parts) == 5 {
				conf.WebRTCICEServers2 = append(conf.WebRTCICEServers2, WebRTCICEServer{
					URL:      parts[0] + ":" + parts[3] + ":" + parts[4],
					Username: parts[1],
					Password: parts[2],
				})
			} else {
				conf.WebRTCICEServers2 = append(conf.WebRTCICEServers2, WebRTCICEServer{
					URL: server,
				})
			}
		}
	}
	for _, server := range conf.WebRTCICEServers2 {
		if !strings.HasPrefix(server.URL, "stun:") &&
			!strings.HasPrefix(server.URL, "turn:") &&
			!strings.HasPrefix(server.URL, "turns:") {
			return fmt.Errorf("invalid ICE server: '%s'", server.URL)
		}
	}
	if conf.WebRTCLocalUDPAddress == "" &&
		conf.WebRTCLocalTCPAddress == "" &&
		len(conf.WebRTCICEServers2) == 0 {
		return fmt.Errorf("at least one between 'webrtcLocalUDPAddress'," +
			" 'webrtcLocalTCPAddress' or 'webrtcICEServers2' must be filled")
	}
	if conf.WebRTCLocalUDPAddress != "" || conf.WebRTCLocalTCPAddress != "" {
		if !conf.WebRTCIPsFromInterfaces && len(conf.WebRTCAdditionalHosts) == 0 {
			return fmt.Errorf("at least one between 'webrtcIPsFromInterfaces' or 'webrtcAdditionalHosts' must be filled")
		}
	}

	// Record (deprecated)

	if conf.Record != nil {
		l.Log(logger.Warn, "parameter 'record' is deprecated "+
			"and has been replaced with 'pathDefaults.record'")
		conf.PathDefaults.Record = *conf.Record
	}
	if conf.RecordPath != nil {
		l.Log(logger.Warn, "parameter 'recordPath' is deprecated "+
			"and has been replaced with 'pathDefaults.recordPath'")
		conf.PathDefaults.RecordPath = *conf.RecordPath
	}
	if conf.RecordFormat != nil {
		l.Log(logger.Warn, "parameter 'recordFormat' is deprecated "+
			"and has been replaced with 'pathDefaults.recordFormat'")
		conf.PathDefaults.RecordFormat = *conf.RecordFormat
	}
	if conf.RecordPartDuration != nil {
		l.Log(logger.Warn, "parameter 'recordPartDuration' is deprecated "+
			"and has been replaced with 'pathDefaults.recordPartDuration'")
		conf.PathDefaults.RecordPartDuration = *conf.RecordPartDuration
	}
	if conf.RecordSegmentDuration != nil {
		l.Log(logger.Warn, "parameter 'recordSegmentDuration' is deprecated "+
			"and has been replaced with 'pathDefaults.recordSegmentDuration'")
		conf.PathDefaults.RecordSegmentDuration = *conf.RecordSegmentDuration
	}
	if conf.RecordDeleteAfter != nil {
		l.Log(logger.Warn, "parameter 'recordDeleteAfter' is deprecated "+
			"and has been replaced with 'pathDefaults.recordDeleteAfter'")
		conf.PathDefaults.RecordDeleteAfter = *conf.RecordDeleteAfter
	}

	hasAllOthers := false
	for name := range conf.OptionalPaths {
		if name == "all" || name == "all_others" || name == "~^.*$" {
			if hasAllOthers {
				return fmt.Errorf("all_others, all and '~^.*$' are aliases")
			}
			hasAllOthers = true
		}
	}

	conf.Paths = make(map[string]*Path)

	for _, name := range sortedKeys(conf.OptionalPaths) {
		optional := conf.OptionalPaths[name]
		if optional == nil {
			optional = &OptionalPath{
				Values: newOptionalPathValues(),
			}
			conf.OptionalPaths[name] = optional
		}

		pconf := newPath(&conf.PathDefaults, optional)
		conf.Paths[name] = pconf
	}

	for _, name := range sortedKeys(conf.OptionalPaths) {
		err := conf.Paths[name].validate(conf, name, deprecatedCredentialsMode, l)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (conf *Conf) UnmarshalJSON(b []byte) error {
	conf.setDefaults()
	type alias Conf
	return jsonwrapper.Unmarshal(b, (*alias)(conf))
}

// Global returns the global part of Conf.
func (conf *Conf) Global() *Global {
	g := &Global{
		Values: newGlobalValues(),
	}
	copyStructFields(g.Values, conf)
	return g
}

// PatchGlobal patches the global configuration.
func (conf *Conf) PatchGlobal(optional *OptionalGlobal) {
	copyStructFields(conf, optional.Values)
}

// PatchPathDefaults patches path default settings.
func (conf *Conf) PatchPathDefaults(optional *OptionalPath) {
	copyStructFields(&conf.PathDefaults, optional.Values)
}

// AddPath adds a path.
func (conf *Conf) AddPath(name string, p *OptionalPath) error {
	if _, ok := conf.OptionalPaths[name]; ok {
		return fmt.Errorf("path already exists")
	}

	if conf.OptionalPaths == nil {
		conf.OptionalPaths = make(map[string]*OptionalPath)
	}

	conf.OptionalPaths[name] = p
	return nil
}

// PatchPath patches a path.
func (conf *Conf) PatchPath(name string, optional2 *OptionalPath) error {
	optional, ok := conf.OptionalPaths[name]
	if !ok {
		return ErrPathNotFound
	}

	copyStructFields(optional.Values, optional2.Values)
	return nil
}

// ReplacePath replaces a path.
func (conf *Conf) ReplacePath(name string, optional2 *OptionalPath) error {
	if conf.OptionalPaths == nil {
		conf.OptionalPaths = make(map[string]*OptionalPath)
	}

	conf.OptionalPaths[name] = optional2
	return nil
}

// RemovePath removes a path.
func (conf *Conf) RemovePath(name string) error {
	if _, ok := conf.OptionalPaths[name]; !ok {
		return ErrPathNotFound
	}

	delete(conf.OptionalPaths, name)
	return nil
}
