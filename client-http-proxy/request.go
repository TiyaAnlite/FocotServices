package main

import (
	"github.com/levigross/grequests"
	"net/http"
	"strconv"
)

type Request struct {
	Protocol        string            `json:"protocol,omitempty" default:"https"`
	Host            string            `json:"host,omitempty"`
	Path            string            `json:"path,omitempty"`
	Method          string            `json:"method,omitempty" default:"GET"`
	Params          map[string]string `json:"params,omitempty"`
	Data            map[string]string `json:"data,omitempty"`
	JSON            any               `json:"json,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Cookies         []*http.Cookie    `json:"cookies,omitempty"`
	UserAgent       string            `json:"user_agent,omitempty"`
	ResponseHeaders bool              `json:"response_headers,omitempty"`
	RequestId       string            `json:"request_id,omitempty"`
}

type RequestOption func(*Request)

func (request *Request) BuildRequestOptions() *grequests.RequestOptions {
	return &grequests.RequestOptions{
		Params:    request.Params,
		Data:      request.Data,
		JSON:      request.JSON,
		Headers:   request.Headers,
		Cookies:   request.Cookies,
		UserAgent: request.UserAgent,
	}
}

func NewRequest(opts ...RequestOption) *Request {
	request := &Request{
		Protocol:  "https",
		Method:    "GET",
		RequestId: strconv.FormatInt(snowFlake.NextVal(), 10),
	}
	for _, opt := range opts {
		opt(request)
	}
	return request
}

func WithHttpRequest() RequestOption {
	return func(request *Request) {
		request.Protocol = "http"
	}
}

func WithHttpsRequest() RequestOption {
	return func(request *Request) {
		request.Protocol = "https"
	}
}

func WithGetRequest() RequestOption {
	return func(request *Request) {
		request.Method = "GET"
	}
}

func WithPostRequest() RequestOption {
	return func(request *Request) {
		request.Method = "POST"
	}
}

func WithGetRequestParams(params map[string]string) RequestOption {
	return func(request *Request) {
		request.Method = "GET"
		request.Params = params
	}
}

func WithRequestParams(params map[string]string) RequestOption {
	return func(request *Request) {
		request.Params = params
	}
}

func WithPostRequestData(data map[string]string) RequestOption {
	return func(request *Request) {
		request.Method = "POST"
		request.Data = data
	}
}

func WithPostRequestJson(jsonData any) RequestOption {
	return func(request *Request) {
		request.Method = "POST"
		request.JSON = jsonData
	}
}

func WithRequestHost(host string) RequestOption {
	return func(request *Request) {
		request.Host = host
	}
}

func WithRequestPath(url string) RequestOption {
	return func(request *Request) {
		request.Path = url
	}
}

func WithHeaderEchoRequest() RequestOption {
	return func(request *Request) {
		request.ResponseHeaders = true
	}
}
