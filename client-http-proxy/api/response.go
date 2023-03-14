package api

import "net/http"

type ProxyPackedResponse struct {
	Ok      bool   `json:"ok,omitempty"`
	Payload []byte `json:"payload,omitempty"`
	Gzip    bool   `json:"gzip,omitempty"`
}

type ProxyResponse struct {
	StatusCode int         `json:"status_code,omitempty"`
	Data       []byte      `json:"data,omitempty"`
	Header     http.Header `json:"header,omitempty"`
}

type MetaResponse struct {
	NodeId            string   `json:"node_id"`
	Groups            []string `json:"groups"`
	RateLimitMs       int      `json:"rate_limit_ms"`
	GzipMinLength     int      `json:"gzip_min_length"`
	Uptime            int      `json:"uptime"`
	HeartbeatInterval int      `json:"heartbeat_interval"`
}
