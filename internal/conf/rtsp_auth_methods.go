package conf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/auth"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPAuthMethods is the rtspAuthMethods parameter.
type RTSPAuthMethods []auth.VerifyMethod

// MarshalJSON implements json.Marshaler.
func (d RTSPAuthMethods) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		switch v {
		case auth.VerifyMethodBasic:
			out[i] = "basic"

		default:
			out[i] = "digest"
		}
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPAuthMethods) UnmarshalJSON(b []byte) error {
	var in []string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = nil

	for _, v := range in {
		switch v {
		case "basic":
			*d = append(*d, auth.VerifyMethodBasic)

		case "digest":
			*d = append(*d, auth.VerifyMethodDigestMD5)

		default:
			return fmt.Errorf("invalid authentication method: '%s'", v)
		}
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPAuthMethods) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return d.UnmarshalJSON(byts)
}
