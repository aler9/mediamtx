package conf

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/bluenviron/mediamtx/internal/logger"
)

func createTempFile(byts []byte) (string, error) {
	tmpf, err := os.CreateTemp(os.TempDir(), "rtsp-")
	if err != nil {
		return "", err
	}
	defer tmpf.Close()

	_, err = tmpf.Write(byts)
	if err != nil {
		return "", err
	}

	return tmpf.Name(), nil
}

func TestConfFromFile(t *testing.T) {
	func() {
		tmpf, err := createTempFile([]byte("logLevel: debug\n" +
			"paths:\n" +
			"  cam1:\n" +
			"    runOnDemandStartTimeout: 5s\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf, confPath, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, tmpf, confPath)

		require.Equal(t, LogLevel(logger.Debug), conf.LogLevel)

		pa, ok := conf.Paths["cam1"]
		require.Equal(t, true, ok)
		require.Equal(t, &Path{
			Name:                       "cam1",
			Source:                     "publisher",
			SourceOnDemandStartTimeout: 10 * Duration(time.Second),
			SourceOnDemandCloseAfter:   10 * Duration(time.Second),
			RecordPath:                 "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f",
			RecordFormat:               RecordFormatFMP4,
			RecordPartDuration:         Duration(1 * time.Second),
			RecordSegmentDuration:      3600000000000,
			RecordDeleteAfter:          86400000000000,
			OverridePublisher:          true,
			RPICameraWidth:             1920,
			RPICameraHeight:            1080,
			RPICameraContrast:          1,
			RPICameraSaturation:        1,
			RPICameraSharpness:         1,
			RPICameraExposure:          "normal",
			RPICameraAWB:               "auto",
			RPICameraAWBGains:          []float64{0, 0},
			RPICameraDenoise:           "off",
			RPICameraMetering:          "centre",
			RPICameraFPS:               30,
			RPICameraAfMode:            "continuous",
			RPICameraAfRange:           "normal",
			RPICameraAfSpeed:           "normal",
			RPICameraTextOverlay:       "%Y-%m-%d %H:%M:%S - MediaMTX",
			RPICameraCodec:             "auto",
			RPICameraIDRPeriod:         60,
			RPICameraBitrate:           5000000,
			RPICameraProfile:           "main",
			RPICameraLevel:             "4.1",
			RunOnDemandStartTimeout:    5 * Duration(time.Second),
			RunOnDemandCloseAfter:      10 * Duration(time.Second),
		}, pa)
	}()

	func() {
		tmpf, err := createTempFile([]byte(``))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		_, _, err = Load(tmpf, nil, nil)
		require.NoError(t, err)
	}()

	func() {
		tmpf, err := createTempFile([]byte(`paths:`))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		_, _, err = Load(tmpf, nil, nil)
		require.NoError(t, err)
	}()

	func() {
		tmpf, err := createTempFile([]byte(
			"paths:\n" +
				"  mypath:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		_, _, err = Load(tmpf, nil, nil)
		require.NoError(t, err)
	}()
}

func TestConfFromFileAndEnv(t *testing.T) {
	// global parameter
	t.Setenv("RTSP_PROTOCOLS", "tcp")

	// path parameter
	t.Setenv("MTX_PATHS_CAM1_SOURCE", "rtsp://testing")

	// deprecated global parameter
	t.Setenv("MTX_RTMPDISABLE", "yes")

	// deprecated path parameter
	t.Setenv("MTX_PATHS_CAM2_DISABLEPUBLISHEROVERRIDE", "yes")

	tmpf, err := createTempFile([]byte("{}"))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, confPath, err := Load(tmpf, nil, nil)
	require.NoError(t, err)
	require.Equal(t, tmpf, confPath)

	require.Equal(t, RTSPTransports{gortsplib.TransportTCP: {}}, conf.RTSPTransports)
	require.Equal(t, false, conf.RTMP)

	pa, ok := conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, "rtsp://testing", pa.Source)

	pa, ok = conf.Paths["cam2"]
	require.Equal(t, true, ok)
	require.Equal(t, false, pa.OverridePublisher)
}

func TestConfFromEnvOnly(t *testing.T) {
	t.Setenv("MTX_PATHS_CAM1_SOURCE", "rtsp://testing")

	conf, confPath, err := Load("", nil, nil)
	require.NoError(t, err)
	require.Equal(t, "", confPath)

	pa, ok := conf.Paths["cam1"]
	require.Equal(t, true, ok)
	require.Equal(t, "rtsp://testing", pa.Source)
}

func TestConfEncryption(t *testing.T) {
	key := "testing123testin"
	plaintext := "paths:\n" +
		"  path1:\n" +
		"  path2:\n"

	encryptedConf := func() string {
		var secretKey [32]byte
		copy(secretKey[:], key)

		var nonce [24]byte
		_, err := io.ReadFull(rand.Reader, nonce[:])
		require.NoError(t, err)

		encrypted := secretbox.Seal(nonce[:], []byte(plaintext), &nonce, &secretKey)
		return base64.StdEncoding.EncodeToString(encrypted)
	}()

	t.Setenv("RTSP_CONFKEY", key)

	tmpf, err := createTempFile([]byte(encryptedConf))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, confPath, err := Load(tmpf, nil, nil)
	require.NoError(t, err)
	require.Equal(t, tmpf, confPath)

	_, ok := conf.Paths["path1"]
	require.Equal(t, true, ok)

	_, ok = conf.Paths["path2"]
	require.Equal(t, true, ok)
}

func TestConfDeprecatedAuth(t *testing.T) {
	tmpf, err := createTempFile([]byte(
		"paths:\n" +
			"  cam:\n" +
			"    readUser: myuser\n" +
			"    readPass: mypass\n"))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, _, err := Load(tmpf, nil, nil)
	require.NoError(t, err)

	require.Equal(t, AuthInternalUsers{
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
		{
			User: "any",
			IPs:  IPNetworks{mustParseCIDR("0.0.0.0/0")},
			Permissions: []AuthInternalUserPermission{
				{
					Action: AuthActionPublish,
					Path:   "cam",
				},
			},
		},
		{
			User: "myuser",
			Pass: "mypass",
			IPs:  IPNetworks{mustParseCIDR("0.0.0.0/0")},
			Permissions: []AuthInternalUserPermission{
				{
					Action: AuthActionRead,
					Path:   "cam",
				},
			},
		},
	}, conf.AuthInternalUsers)
}

func TestConfErrors(t *testing.T) {
	for _, ca := range []struct {
		name string
		conf string
		err  string
	}{
		{
			"duplicate parameter",
			"paths:\n" +
				"paths:\n",
			"yaml: unmarshal errors:\n  line 2: key \"paths\" already set in map",
		},
		{
			"non existent parameter",
			`invalid: param`,
			"json: unknown field \"invalid\"",
		},
		{
			"invalid readTimeout",
			"readTimeout: 0s\n",
			"'readTimeout' must be greater than zero",
		},
		{
			"invalid writeTimeout",
			"writeTimeout: 0s\n",
			"'writeTimeout' must be greater than zero",
		},
		{
			"invalid writeQueueSize",
			"writeQueueSize: 1001\n",
			"'writeQueueSize' must be a power of two",
		},
		{
			"invalid udpMaxPayloadSize",
			"udpMaxPayloadSize: 5000\n",
			"'udpMaxPayloadSize' must be less than 1472",
		},
		{
			"invalid strict encryption 1",
			"rtspEncryption: strict\n" +
				"rtspTransports: [udp]\n",
			"strict encryption cannot be used with the UDP transport protocol",
		},
		{
			"invalid strict encryption 2",
			"rtspEncryption: strict\n" +
				"rtspTransports: [multicast]\n",
			"strict encryption cannot be used with the UDP-multicast transport protocol",
		},
		{
			"invalid ICE server",
			"webrtcICEServers: [testing]\n",
			"invalid ICE server: 'testing'",
		},
		{
			"non existent parameter in path",
			"paths:\n" +
				"  mypath:\n" +
				"    invalid: parameter\n",
			"json: unknown field \"invalid\"",
		},
		{
			"non existent parameter in auth",
			"authInternalUsers:\n" +
				"- users: test\n",
			"json: unknown field \"users\"",
		},
		{
			"invalid path name",
			"paths:\n" +
				"  '':\n" +
				"    source: publisher\n",
			"invalid path name '': cannot be empty",
		},
		{
			"double raspberry pi camera",
			"paths:\n" +
				"  cam1:\n" +
				"    source: rpiCamera\n" +
				"  cam2:\n" +
				"    source: rpiCamera\n",
			"'rpiCamera' with same camera ID 0 is used as source in two paths, 'cam1' and 'cam2'",
		},
		{
			"invalid srt publish passphrase",
			"paths:\n" +
				"  mypath:\n" +
				"    srtPublishPassphrase: a\n",
			`invalid 'srtPublishPassphrase': must be between 10 and 79 characters`,
		},
		{
			"invalid srt read passphrase",
			"paths:\n" +
				"  mypath:\n" +
				"    srtReadPassphrase: a\n",
			`invalid 'readRTPassphrase': must be between 10 and 79 characters`,
		},
		{
			"all_others aliases",
			"paths:\n" +
				"  all:\n" +
				"  all_others:\n",
			`all_others, all and '~^.*$' are aliases`,
		},
		{
			"all_others aliases",
			"paths:\n" +
				"  all_others:\n" +
				"  ~^.*$:\n",
			`all_others, all and '~^.*$' are aliases`,
		},
		{
			"playback",
			"playback: yes\n" +
				"paths:\n" +
				"  my_path:\n" +
				"    recordPath: ./recordings/%path/%Y-%m-%d_%H-%M-%S",
			`record path './recordings/%path/%Y-%m-%d_%H-%M-%S' is missing one of the` +
				` mandatory elements for the playback server to work: %Y %m %d %H %M %S %f`,
		},
		{
			"jwt claim key empty",
			"authMethod: jwt\n" +
				"authJWTJWKS: https://not-real.com\n" +
				"authJWTClaimKey: \"\"",
			"'authJWTClaimKey' is empty",
		},
		{
			"invalid rtsp auth methods",
			"rtspAuthMethods: []",
			"at least one 'rtspAuthMethods' must be provided",
		},
		{
			"invalid fallback",
			"paths:\n" +
				"  my_path:\n" +
				"    fallback: invalid://invalid",
			`'invalid://invalid' is not a valid RTSP URL`,
		},
		{
			"invalid source redirect",
			"paths:\n" +
				"  my_path:\n" +
				"    source: redirect\n" +
				"    sourceRedirect: invalid://invalid",
			`'invalid://invalid' is not a valid RTSP URL`,
		},
		{
			"useless source redirect",
			"paths:\n" +
				"  my_path:\n" +
				"    sourceRedirect: invalid://invalid",
			`'sourceRedirect' is useless when source is not 'redirect'`,
		},
		{
			"invalid user",
			"authInternalUsers:\n" +
				"- user:\n" +
				"  pass: test\n" +
				"  permissions:\n" +
				"  - action: publish\n",
			"empty usernames are not supported",
		},
		{
			"invalid pass",
			"authInternalUsers:\n" +
				"- user: any\n" +
				"  pass: test\n" +
				"  permissions:\n" +
				"  - action: publish\n",
			`using a password with 'any' user is not supported`,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			tmpf, err := createTempFile([]byte(ca.conf))
			require.NoError(t, err)
			defer os.Remove(tmpf)

			_, _, err = Load(tmpf, nil, nil)
			require.EqualError(t, err, ca.err)
		})
	}
}

func TestSampleConfFile(t *testing.T) {
	func() {
		conf1, confPath1, err := Load("../../mediamtx.yml", nil, nil)
		require.NoError(t, err)
		require.Equal(t, "../../mediamtx.yml", confPath1)
		conf1.Paths = make(map[string]*Path)
		conf1.OptionalPaths = nil

		conf2, confPath2, err := Load("", nil, nil)
		require.NoError(t, err)
		require.Equal(t, "", confPath2)

		require.Equal(t, conf1, conf2)
	}()

	func() {
		conf1, confPath1, err := Load("../../mediamtx.yml", nil, nil)
		require.NoError(t, err)
		require.Equal(t, "../../mediamtx.yml", confPath1)

		tmpf, err := createTempFile([]byte("paths:\n  all_others:"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf2, confPath2, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, tmpf, confPath2)

		require.Equal(t, conf1.Paths, conf2.Paths)
	}()
}

// needed due to https://github.com/golang/go/issues/21092
func TestConfOverrideDefaultSlices(t *testing.T) {
	tmpf, err := createTempFile([]byte(
		"authInternalUsers:\n" +
			"  - user: user1\n" +
			"  - user: user2\n" +
			"authHTTPExclude:\n" +
			"  - path: ''\n"))
	require.NoError(t, err)
	defer os.Remove(tmpf)

	conf, _, err := Load(tmpf, nil, nil)
	require.NoError(t, err)

	require.Equal(t, AuthInternalUsers{
		{
			User: "user1",
		},
		{
			User: "user2",
		},
	}, conf.AuthInternalUsers)

	require.Equal(t, AuthInternalUserPermissions{
		{},
	}, conf.AuthHTTPExclude)
}
