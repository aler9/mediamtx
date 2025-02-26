package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/bluenviron/mediamtx/internal/logger"
)

// LogLevel is the logLevel parameter.
type LogLevel logger.Level

// MarshalJSON implements json.Marshaler.
func (d LogLevel) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case LogLevel(logger.Error):
		out = "error"

	case LogLevel(logger.Warn):
		out = "warn"

	case LogLevel(logger.Info):
		out = "info"

	default:
		out = "debug"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *LogLevel) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "error":
		*d = LogLevel(logger.Error)

	case "warn":
		*d = LogLevel(logger.Warn)

	case "info":
		*d = LogLevel(logger.Info)

	case "debug":
		*d = LogLevel(logger.Debug)

	default:
		return fmt.Errorf("invalid log level: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *LogLevel) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
