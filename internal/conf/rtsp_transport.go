package conf

import (
	"encoding/json"
	"fmt"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// RTSPTransport is the rtspTransport parameter.
type RTSPTransport struct {
	*gortsplib.Transport
}

// MarshalJSON implements json.Marshaler.
func (d RTSPTransport) MarshalJSON() ([]byte, error) {
	var out string

	if d.Transport == nil {
		out = "automatic"
	} else {
		switch *d.Transport {
		case gortsplib.TransportUDP:
			out = "udp"

		case gortsplib.TransportUDPMulticast:
			out = "multicast"

		default:
			out = "tcp"
		}
	}

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *RTSPTransport) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	switch in {
	case "udp":
		v := gortsplib.TransportUDP
		d.Transport = &v

	case "multicast":
		v := gortsplib.TransportUDPMulticast
		d.Transport = &v

	case "tcp":
		v := gortsplib.TransportTCP
		d.Transport = &v

	case "automatic":
		d.Transport = nil

	default:
		return fmt.Errorf("invalid transport '%s'", in)
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *RTSPTransport) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}
