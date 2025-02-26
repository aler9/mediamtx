package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// HLSVariant is the hlsVariant parameter.
type HLSVariant gohlslib.MuxerVariant

// MarshalJSON implements json.Marshaler.
func (d HLSVariant) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case HLSVariant(gohlslib.MuxerVariantMPEGTS):
		out = "mpegts"

	case HLSVariant(gohlslib.MuxerVariantFMP4):
		out = "fmp4"

	default:
		out = "lowLatency"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *HLSVariant) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "mpegts":
		*d = HLSVariant(gohlslib.MuxerVariantMPEGTS)

	case "fmp4":
		*d = HLSVariant(gohlslib.MuxerVariantFMP4)

	case "lowLatency":
		*d = HLSVariant(gohlslib.MuxerVariantLowLatency)

	default:
		return fmt.Errorf("invalid HLS variant: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *HLSVariant) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
