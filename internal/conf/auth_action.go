package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// AuthAction is an authentication action.
type AuthAction string

// auth actions
const (
	AuthActionPublish  AuthAction = "publish"
	AuthActionRead     AuthAction = "read"
	AuthActionPlayback AuthAction = "playback"
	AuthActionAPI      AuthAction = "api"
	AuthActionMetrics  AuthAction = "metrics"
	AuthActionPprof    AuthAction = "pprof"
)

// MarshalJSON implements json.Marshaler.
func (d AuthAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *AuthAction) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case string(AuthActionPublish),
		string(AuthActionRead),
		string(AuthActionPlayback),
		string(AuthActionAPI),
		string(AuthActionMetrics),
		string(AuthActionPprof):
		*d = AuthAction(in)

	default:
		return fmt.Errorf("invalid auth action: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *AuthAction) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
