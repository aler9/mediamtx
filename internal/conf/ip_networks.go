package conf

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// IPNetworks is a parameter that contains a list of IP networks.
type IPNetworks []net.IPNet

// MarshalJSON implements json.Marshaler.
func (d IPNetworks) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for i, v := range d {
		out[i] = v.String()
	}

	sort.Strings(out)

	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *IPNetworks) UnmarshalJSON(b []byte) error {
	var in []string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = nil

	if len(in) == 0 {
		return nil
	}

	for _, t := range in {
		if _, ipnet, err := net.ParseCIDR(t); err == nil {
			if ipv4 := ipnet.IP.To4(); ipv4 != nil {
				*d = append(*d, net.IPNet{IP: ipv4, Mask: ipnet.Mask[len(ipnet.Mask)-4 : len(ipnet.Mask)]})
			} else {
				*d = append(*d, *ipnet)
			}
		} else if ip := net.ParseIP(t); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				*d = append(*d, net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)})
			} else {
				*d = append(*d, net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
			}
		} else {
			return fmt.Errorf("unable to parse IP/CIDR '%s'", t)
		}
	}

	return nil
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *IPNetworks) UnmarshalEnv(_ string, v string) error {
	byts, _ := json.Marshal(strings.Split(v, ","))
	return d.UnmarshalJSON(byts)
}

// ToTrustedProxies converts IPNetworks into a string slice for SetTrustedProxies.
func (d *IPNetworks) ToTrustedProxies() []string {
	ret := make([]string, len(*d))
	for i, entry := range *d {
		ret[i] = entry.String()
	}
	return ret
}

// Contains checks whether the IP is part of one of the networks.
func (d IPNetworks) Contains(ip net.IP) bool {
	for _, network := range d {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
