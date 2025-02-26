package conf

import (
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

var reDays = regexp.MustCompile("^(-?[0-9]+)d")

// Duration is a duration. It differs from the standard duration in these ways:
// - it is unmarshaled/marshaled from/to a string (instead of a number)
// - it supports days
type Duration time.Duration

func (d Duration) marshalInternal() string {
	negative := false
	if d < 0 {
		negative = true
		d = -d
	}

	day := Duration(86400 * time.Second)
	days := d / day
	nonDays := d % day

	ret := ""
	if negative {
		ret += "-"
	}

	if days > 0 {
		ret += strconv.FormatInt(int64(days), 10) + "d"
	}

	if nonDays != 0 {
		ret += time.Duration(nonDays).String()
	}

	return ret
}

// MarshalJSON implements json.Marshaler.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.marshalInternal())
}

func (d *Duration) unmarshalInternal(in string) error {
	negative := false
	days := int64(0)

	m := reDays.FindStringSubmatch(in)
	if m != nil {
		days, _ = strconv.ParseInt(m[1], 10, 64)
		if days < 0 {
			negative = true
			days = -days
		}

		in = in[len(m[0]):]
	}

	var nonDays time.Duration

	if len(in) != 0 {
		var err error
		nonDays, err = time.ParseDuration(in)
		if err != nil {
			return err
		}
	}

	nonDays += time.Duration(days) * 24 * time.Hour
	if negative {
		nonDays = -nonDays
	}

	*d = Duration(nonDays)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	err := d.unmarshalInternal(in)
	if err != nil {
		return err
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Duration) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
