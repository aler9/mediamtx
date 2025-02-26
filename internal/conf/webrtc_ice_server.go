package conf

import "github.com/bluenviron/mediamtx/internal/conf/jsonwrapper"

// WebRTCICEServer is a WebRTC ICE Server.
type WebRTCICEServer struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	ClientOnly bool   `json:"clientOnly"`
}

// WebRTCICEServers is a list of WebRTCICEServer
type WebRTCICEServers []WebRTCICEServer

// UnmarshalJSON implements json.Unmarshaler.
func (s *WebRTCICEServers) UnmarshalJSON(b []byte) error {
	// remove default value before loading new value
	// https://github.com/golang/go/issues/21092
	*s = nil
	return jsonwrapper.Unmarshal(b, (*[]WebRTCICEServer)(s))
}
