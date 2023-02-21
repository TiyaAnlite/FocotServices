package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"github.com/nats-io/nats.go"
	"k8s.io/klog/v2"
	"net/http"
	"time"
)

type ProxyPackedResponse struct {
	Ok      bool   `json:"ok"`
	Payload []byte `json:"payload"`
	Gzip    bool   `json:"gzip"`
}

type ProxyResponse struct {
	StatusCode int         `json:"status_code"`
	Data       []byte      `json:"data"`
	Header     http.Header `json:"header"`
}

// PrepareUnPackedResponse 返回包数据预处理
func PrepareUnPackedResponse(response *ProxyPackedResponse) (*ProxyResponse, error) {
	payload := response.Payload
	if response.Gzip {
		buf := bytes.NewReader(response.Payload)
		reader, err := gzip.NewReader(buf)
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		var b []byte
		_, err = reader.Read(b)
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		payload = b
	}
	var resp ProxyResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		klog.Errorf("Error at unmarshal payload: %s", err.Error())
		return nil, err
	}

	return &resp, nil
}

func PackResponse(response *grequests.Response, request *Request) ([]byte, error) {
	// Payload
	proxyResp := &ProxyResponse{
		StatusCode: response.StatusCode,
		Data:       response.Bytes(),
	}
	if request.Headers {
		proxyResp.Header = response.Header
	}
	payload, err := json.Marshal(proxyResp)
	if err != nil {
		klog.Errorf("Error at marshal resp data: %s", err.Error())
		klog.Errorf("response content: %s", response.String())
		return nil, err
	}

	// Gzip
	gzipFlag := false
	if len(payload) >= cfg.GzipMinLength {
		var buf bytes.Buffer
		gzipFlag = true
		gzipWriter := gzip.NewWriter(&buf)
		_, err = gzipWriter.Write(payload)
		if err != nil {
			_ = gzipWriter.Close()
			klog.Errorf("Error at gzip payload: %s", err.Error())
			return nil, err
		}
		if err := gzipWriter.Close(); err != nil {
			klog.Errorf("Error at close gzip writer: %s", err.Error())
			return nil, err
		}
		payload = buf.Bytes()
	}

	// Pack
	respData := &ProxyPackedResponse{
		Ok:      response.Ok,
		Payload: payload,
		Gzip:    gzipFlag,
	}
	packetData, err := json.Marshal(respData)
	if err != nil {
		klog.Errorf("Error at marshal compressed data: %s", err.Error())
		return nil, err
	}
	return packetData, nil
}

func requestHandle(msg *nats.Msg) {
	request := NewRequest()
	if err := json.Unmarshal(msg.Data, request); err != nil {
		klog.Errorf("Cannot parse proxy request: %s", err.Error())
	} else {
		go doRequest(request, msg)
	}
}

func doRequest(request *Request, msg *nats.Msg) {
	cfg.worker.Add(1)
	defer cfg.worker.Done()
	ro := &grequests.RequestOptions{
		Params:  request.Params,
		Data:    request.Data,
		JSON:    request.JSON,
		Context: cfg.worker.ctx,
	}
	url := fmt.Sprintf("%s://%s%s", request.Protocol, request.Host, request.Path)
	klog.Infof("%s %s", request.Method, url)
	rate := time.Now().UnixMilli()
	resp, err := grequests.Req(request.Method, url, ro)
	klog.Infof("%s %s - %d - %dms", request.Method, url, resp.StatusCode, time.Now().UnixMilli()-rate)
	if err != nil {
		klog.Errorf("Error at create request: %s", err.Error())
		return
	}
	if resp.Error != nil {
		klog.Errorf("Error at parse response: %s", err.Error())
		return
	}
	respData, err := PackResponse(resp, request)
	if err == nil {
		if err := msg.Respond(respData); err != nil {
			klog.Errorf("Error at respond msg: %s", err.Error())
			return
		}
	}
}
