package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RecordFormat is the recordFormat parameter.
type RecordFormat int

// supported values.
const (
	RecordFormatFMP4 RecordFormat = iota
	RecordFormatMPEGTS
)

// MarshalJSON implements json.Marshaler.
func (d RecordFormat) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case RecordFormatMPEGTS:
		out = "mpegts"

	default:
		out = "fmp4"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RecordFormat) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "mpegts":
		*d = RecordFormatMPEGTS

	case "fmp4":
		*d = RecordFormatFMP4

	default:
		return fmt.Errorf("invalid record format '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RecordFormat) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
