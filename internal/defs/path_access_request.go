package defs

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/headers"
	"github.com/bluenviron/mediamtx/internal/auth"
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

// PathAccessRequest is a path access request.
type PathAccessRequest struct {
	Name     string
	Query    string
	Publish  bool
	SkipAuth bool

	// only if skipAuth = false
	User             string
	Pass             string
	IP               net.IP
	CustomVerifyFunc func(expectedUser string, expectedPass string) bool
	Proto            auth.Protocol
	ID               *uuid.UUID
}

// ToAuthRequest converts a path access request into an authentication request.
func (r *PathAccessRequest) ToAuthRequest() *auth.Request {
	return &auth.Request{
		User: r.User,
		Pass: r.Pass,
		IP:   r.IP,
		Action: func() conf.AuthAction {
			if r.Publish {
				return conf.AuthActionPublish
			}
			return conf.AuthActionRead
		}(),
		CustomVerifyFunc: r.CustomVerifyFunc,
		Path:             r.Name,
		Protocol:         r.Proto,
		ID:               r.ID,
		Query:            r.Query,
	}
}

// FillFromRTSPRequest fills User and Pass from a RTSP request.
func (r *PathAccessRequest) FillFromRTSPRequest(rt *base.Request) {
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
func (r *PathAccessRequest) FillFromHTTPRequest(h *http.Request) {
	r.Query = h.URL.RawQuery
	r.User, r.Pass, _ = h.BasicAuth()

	// move Authorization header from headers to Query
	if h := h.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		// support passing user and password through the Authorization header
		if parts := strings.Split(strings.TrimPrefix(h, "Bearer "), ":"); len(parts) == 2 {
			r.User = parts[0]
			r.Pass = parts[1]
		} else {
			r.Query = addJWTFromAuthorization(r.Query, h)
		}
	}
}
