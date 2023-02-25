package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/levigross/grequests"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, tracer := cfg.worker.Start(cfg.worker.Ctx, "request-handle")
	request := NewRequest()
	if err := json.Unmarshal(msg.Data, request); err != nil {
		klog.Errorf("Cannot parse proxy request: %s", err.Error())
		tracer.RecordError(err)
		tracer.End()
	} else {
		go doRequest(ctx, tracer, request, msg)
	}
}

func doRequest(ctx context.Context, tracer trace.Span, request *Request, msg *nats.Msg) {
	cfg.worker.Add(1)
	defer cfg.worker.Done()
	defer tracer.End()
	ro := request.BuildRequestOptions()
	url := fmt.Sprintf("%s://%s%s", request.Protocol, request.Host, request.Path)
	klog.Infof("%s %s", request.Method, url)

	_, reqTrace := cfg.worker.Start(ctx, "do-request")
	defer reqTrace.End()
	reqTrace.SetAttributes(attribute.String("http.request.method", request.Method), attribute.String("http.request.url", url))
	rate := time.Now().UnixMilli()
	resp, err := grequests.Req(request.Method, url, ro)
	klog.Infof("%s %s - %d - %dms", request.Method, url, resp.StatusCode, time.Now().UnixMilli()-rate)
	if err != nil {
		klog.Errorf("Error at create request: %s", err.Error())
		reqTrace.RecordError(err)
		reqStr, _ := json.Marshal(request)
		reqTrace.SetAttributes(attribute.String("http.request", string(reqStr))) // Dump fll request
		return
	}
	reqTrace.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.Error != nil {
		klog.Errorf("Error at do request: %s", err.Error())
		reqTrace.RecordError(err)
		return
	}
	reqTrace.End()

	_, packTrace := cfg.worker.Start(ctx, "response-pack")
	defer packTrace.End()
	respData, err := PackResponse(resp, request)
	if err != nil {
		packTrace.RecordError(err)
		packTrace.SetAttributes(attribute.String("http.response.content", resp.String()))
		return
	}
	packTrace.End()

	_, respTrace := cfg.worker.Start(ctx, "nats-respond")
	defer respTrace.End()
	if err := msg.Respond(respData); err != nil {
		klog.Errorf("Error at respond msg: %s", err.Error())
		respTrace.RecordError(err)
		return
	}
}
