package main

import proxy "github.com/TiyaAnlite/FocotServices/client-http-proxy/api"

type ProxyRequest struct {
	Node    string         `json:"node"`
	Timeout int            `json:"timeout"`
	Payload *proxy.Request `json:"payload"`
}
