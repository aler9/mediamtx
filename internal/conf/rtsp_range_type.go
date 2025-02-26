package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPRangeType is the type used in the Range header.
type RTSPRangeType int

// supported values.
const (
	RTSPRangeTypeUndefined RTSPRangeType = iota
	RTSPRangeTypeClock
	RTSPRangeTypeNPT
	RTSPRangeTypeSMPTE
)

// MarshalJSON implements json.Marshaler.
func (d RTSPRangeType) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case RTSPRangeTypeClock:
		out = "clock"

	case RTSPRangeTypeNPT:
		out = "npt"

	case RTSPRangeTypeSMPTE:
		out = "smpte"

	default:
		out = ""
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPRangeType) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "clock":
		*d = RTSPRangeTypeClock

	case "npt":
		*d = RTSPRangeTypeNPT

	case "smpte":
		*d = RTSPRangeTypeSMPTE

	case "":
		*d = RTSPRangeTypeUndefined

	default:
		return fmt.Errorf("invalid rtsp range type: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPRangeType) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
