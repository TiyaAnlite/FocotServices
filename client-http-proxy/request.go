package main

import "github.com/levigross/grequests"

type Request struct {
	Protocol string            `json:"protocol" default:"https"`
	Host     string            `json:"host"`
	Path     string            `json:"path"`
	Method   string            `json:"method" default:"GET"`
	Params   map[string]string `json:"params"`
	Data     map[string]string `json:"data"`
	JSON     any               `json:"json"`
	Headers  bool              `json:"headers"`
}

type RequestOption func(*Request)

func (request *Request) BuildRequestOptions() *grequests.RequestOptions {
	return &grequests.RequestOptions{
		Params: request.Params,
		Data:   request.Data,
		JSON:   request.JSON,
	}
}

func NewRequest(opts ...RequestOption) *Request {
	request := &Request{
		Protocol: "https",
		Method:   "GET",
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
		request.Headers = true
	}
}
