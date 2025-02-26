package conf

import (
	"fmt"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
)

// AuthInternalUserPermission is a permission of a user.
type AuthInternalUserPermission struct {
	Action AuthAction `json:"action"`
	Path   string     `json:"path"`
}

// AuthInternalUser is an user.
type AuthInternalUser struct {
	User        Credential                   `json:"user"`
	Pass        Credential                   `json:"pass"`
	IPs         IPNetworks                   `json:"ips"`
	Permissions []AuthInternalUserPermission `json:"permissions"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *AuthInternalUser) UnmarshalJSON(b []byte) error {
	type alias AuthInternalUser
	if err := jsonwrapper.Unmarshal(b, (*alias)(d)); err != nil {
		return err
	}

	// https://github.com/bluenviron/gortsplib/blob/55556f1ecfa2bd51b29fe14eddd70512a0361cbd/server_conn.go#L155-L156
	if d.User == "" {
		return fmt.Errorf("empty usernames are not supported")
	}

	if d.User == "any" && d.Pass != "" {
		return fmt.Errorf("using a password with 'any' user is not supported")
	}

	return nil
}

// AuthInternalUsers is a list of AuthInternalUser
type AuthInternalUsers []AuthInternalUser

// UnmarshalJSON implements json.Unmarshaler.
func (s *AuthInternalUsers) UnmarshalJSON(b []byte) error {
	// remove default value before loading new value
	// https://github.com/golang/go/issues/21092
	*s = nil
	return jsonwrapper.Unmarshal(b, (*[]AuthInternalUser)(s))
}

// AuthInternalUserPermissions is a list of AuthInternalUserPermission
type AuthInternalUserPermissions []AuthInternalUserPermission

// UnmarshalJSON implements json.Unmarshaler.
func (s *AuthInternalUserPermissions) UnmarshalJSON(b []byte) error {
	// remove default value before loading new value
	// https://github.com/golang/go/issues/21092
	*s = nil
	return jsonwrapper.Unmarshal(b, (*[]AuthInternalUserPermission)(s))
}
