package conf

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"
	"github.com/matthewhartstonge/argon2"
)

var (
	rePlainCredential = regexp.MustCompile(`^[a-zA-Z0-9!\$\(\)\*\+\.;<=>\[\]\^_\-\{\}@#&]+$`)
	reBase64          = regexp.MustCompile(`^sha256:[a-zA-Z0-9\+/=]+$`)
)

const plainCredentialSupportedChars = "A-Z,0-9,!,$,(,),*,+,.,;,<,=,>,[,],^,_,-,\",\",@,#,&"

func sha256Base64(in string) string {
	h := sha256.New()
	h.Write([]byte(in))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Credential is a parameter that is used as username or password.
type Credential string

// MarshalJSON implements json.Marshaler.
func (d Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Credential) UnmarshalJSON(b []byte) error {
	var in string
	if err := jsonwrapper.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = Credential(in)

	return d.validate()
}

// UnmarshalEnv implements env.Unmarshaler.
func (d *Credential) UnmarshalEnv(_ string, v string) error {
	return d.UnmarshalJSON([]byte(`"` + v + `"`))
}

// IsSha256 returns true if the credential is a sha256 hash.
func (d Credential) IsSha256() bool {
	return strings.HasPrefix(string(d), "sha256:")
}

// IsArgon2 returns true if the credential is an argon2 hash.
func (d Credential) IsArgon2() bool {
	return strings.HasPrefix(string(d), "argon2:")
}

// IsHashed returns true if the credential is a sha256 or argon2 hash.
func (d Credential) IsHashed() bool {
	return d.IsSha256() || d.IsArgon2()
}

// Check returns true if the given value matches the credential.
func (d Credential) Check(guess string) bool {
	if d.IsSha256() {
		return string(d)[len("sha256:"):] == sha256Base64(guess)
	}

	if d.IsArgon2() {
		// TODO: remove matthewhartstonge/argon2 when this PR gets merged into mainline Go:
		// https://go-review.googlesource.com/c/crypto/+/502515
		ok, err := argon2.VerifyEncoded([]byte(guess), []byte(string(d)[len("argon2:"):]))
		return ok && err == nil
	}

	if d != "" {
		return string(d) == guess
	}

	return true
}

func (d Credential) validate() error {
	if d != "" {
		switch {
		case d.IsSha256():
			if !reBase64.MatchString(string(d)) {
				return fmt.Errorf("credential contains unsupported characters, sha256 hash must be base64 encoded")
			}
		case d.IsArgon2():
			// TODO: remove matthewhartstonge/argon2 when this PR gets merged into mainline Go:
			// https://go-review.googlesource.com/c/crypto/+/502515
			_, err := argon2.Decode([]byte(string(d)[len("argon2:"):]))
			if err != nil {
				return fmt.Errorf("invalid argon2 hash: %w", err)
			}
		default:
			if !rePlainCredential.MatchString(string(d)) {
				return fmt.Errorf("credential contains unsupported characters. Supported are: %s", plainCredentialSupportedChars)
			}
		}
	}
	return nil
}
