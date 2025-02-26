package main

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/conf/yamlwrapper"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/stretchr/testify/require"
)

func TestAPIDocs(t *testing.T) {
	byts, err := os.ReadFile("../../apidocs/openapi.yaml")
	require.NoError(t, err)

	var raw map[string]interface{}
	err = yamlwrapper.Unmarshal(byts, &raw)
	require.NoError(t, err)

	components := raw["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})

	for _, ca := range []struct {
		yamlKey  string
		goStruct interface{}
	}{
		{
			"AuthInternalUser",
			conf.AuthInternalUser{},
		},
		{
			"AuthInternalUserPermission",
			conf.AuthInternalUserPermission{},
		},
		{
			"GlobalConf",
			conf.Conf{},
		},
		{
			"PathConf",
			conf.Path{},
		},
		{
			"PathConfList",
			defs.APIPathConfList{},
		},
		{
			"Path",
			defs.APIPath{},
		},
		{
			"PathList",
			defs.APIPathList{},
		},
		{
			"PathSource",
			defs.APIPathSourceOrReader{},
		},
		{
			"PathReader",
			defs.APIPathSourceOrReader{},
		},
		{
			"HLSMuxer",
			defs.APIHLSMuxer{},
		},
		{
			"HLSMuxerList",
			defs.APIHLSMuxerList{},
		},
		{
			"Recording",
			defs.APIRecording{},
		},
		{
			"RecordingList",
			defs.APIRecordingList{},
		},
		{
			"RecordingSegment",
			defs.APIRecordingSegment{},
		},
		{
			"RTMPConn",
			defs.APIRTMPConn{},
		},
		{
			"RTMPConnList",
			defs.APIRTMPConnList{},
		},
		{
			"RTSPConn",
			defs.APIRTSPConn{},
		},
		{
			"RTSPConnList",
			defs.APIRTSPConnsList{},
		},
		{
			"RTSPSession",
			defs.APIRTSPSession{},
		},
		{
			"RTSPSessionList",
			defs.APIRTSPSessionList{},
		},
		{
			"SRTConn",
			defs.APISRTConn{},
		},
		{
			"SRTConnList",
			defs.APISRTConnList{},
		},
		{
			"WebRTCSession",
			defs.APIWebRTCSession{},
		},
		{
			"WebRTCSessionList",
			defs.APIWebRTCSessionList{},
		},
	} {
		t.Run(ca.yamlKey, func(t *testing.T) {
			yamlContent := schemas[ca.yamlKey].(map[string]interface{})
			props := yamlContent["properties"].(map[string]interface{})
			key1 := make([]string, len(props))
			i := 0
			for key := range props {
				key1[i] = key
				i++
			}

			var key2 []string
			ty := reflect.TypeOf(ca.goStruct)
			for i := 0; i < ty.NumField(); i++ {
				sf := ty.Field(i)
				js := sf.Tag.Get("json")
				if js != "-" && js != "paths" && js != "pathDefaults" && !strings.Contains(js, ",omitempty") {
					key2 = append(key2, js)
				}
			}

			sort.Strings(key1)
			sort.Strings(key2)

			require.Equal(t, key1, key2)
		})
	}
}
