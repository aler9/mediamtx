package auth

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/google/uuid"
)

func addJWTFromAuthorization(rawQuery string, auth string) string {
	jwt := strings.TrimPrefix(auth, "Bearer ")
	if rawQuery != "" {
		if v, err := url.ParseQuery(rawQuery); err == nil && v.Get("jwt") == "" {
			v.Set("jwt", jwt)
			return v.Encode()
		}
	}
	return url.Values{"jwt": []string{jwt}}.Encode()
}

// Protocol is a protocol.
type Protocol string

// protocols.
const (
	ProtocolRTSP   Protocol = "rtsp"
	ProtocolRTMP   Protocol = "rtmp"
	ProtocolHLS    Protocol = "hls"
	ProtocolWebRTC Protocol = "webrtc"
	ProtocolSRT    Protocol = "srt"
)

// Request is an authentication request.
type Request struct {
	User             string
	Pass             string
	IP               net.IP
	Action           conf.AuthAction
	CustomVerifyFunc func(expectedUser string, expectedPass string) bool

	// only for ActionPublish, ActionRead, ActionPlayback
	Path     string
	Protocol Protocol
	ID       *uuid.UUID
	Query    string
}

// FillFromRTSPRequest fills User and Pass from a RTSP request.
func (r *Request) FillFromRTSPRequest(rt *base.Request) {
	var rtspAuthHeader headers.Authorization
	err := rtspAuthHeader.Unmarshal(rt.Header["Authorization"])
	if err == nil {
		r.User = rtspAuthHeader.Username
		if rtspAuthHeader.Method == headers.AuthMethodBasic {
			r.Pass = rtspAuthHeader.BasicPass
		}
	}
}

// FillFromHTTPRequest fills Query, User and Pass from an HTTP request.
func (r *Request) FillFromHTTPRequest(h *http.Request) {
	r.Query = h.URL.RawQuery
	r.User, r.Pass, _ = h.BasicAuth()

	if h := h.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		// support passing user and password through the Authorization header
		if parts := strings.Split(strings.TrimPrefix(h, "Bearer "), ":"); len(parts) == 2 {
			r.User = parts[0]
			r.Pass = parts[1]
		} else { // move Authorization header to Query
			r.Query = addJWTFromAuthorization(r.Query, h)
		}
	}
}
