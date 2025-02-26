// Package jsonwrapper contains a JSON unmarshaler.
package jsonwrapper

import (
	"bytes"
	"encoding/json"
	"io"
)

// Unmarshal decodes JSON.
// It returns an error if a non-existing field is found.
func Unmarshal(buf []byte, dest any) error {
	return Decode(bytes.NewReader(buf), dest)
}

// Decode decodes JSON.
// It returns an error if a non-existing field is found.
func Decode(r io.Reader, dest any) error {
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	return d.Decode(dest)
}
