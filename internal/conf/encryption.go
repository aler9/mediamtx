package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// Encryption is the rtspEncryption / rtmpEncryption parameter.
type Encryption int

// values.
const (
	EncryptionNo Encryption = iota
	EncryptionOptional
	EncryptionStrict
)

// MarshalJSON implements json.Marshaler.
func (d Encryption) MarshalJSON() ([]byte, error) {
	var out string

	switch d {
	case EncryptionNo:
		out = "no"

	case EncryptionOptional:
		out = "optional"

	default:
		out = "strict"
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Encryption) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "no", "false":
		*d = EncryptionNo

	case "optional":
		*d = EncryptionOptional

	case "strict", "yes", "true":
		*d = EncryptionStrict

	default:
		return fmt.Errorf("invalid encryption: '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Encryption) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
