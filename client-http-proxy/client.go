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
	"strings"
	"time"
)

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

const (
	TASK_ON_PROCESS = iota
	TASK_SCHEDULED
	TASK_FINISHED
	TASK_FAILED
)

type taskStatus int

type RequestProcessMsg struct {
	Status    taskStatus    `json:"status"`
	RequestId string        `json:"request_id"`
	TraceId   trace.TraceID `json:"trace_id"`
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
	if request.ResponseHeaders {
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

func getMeta() *MetaResponse {
	return &MetaResponse{
		NodeId:            cfg.NodeId,
		Groups:            cfg.NodeGroups,
		RateLimitMs:       cfg.RateLimitMs,
		GzipMinLength:     cfg.GzipMinLength,
		Uptime:            int(time.Now().Sub(cfg.uptime).Seconds()),
		HeartbeatInterval: cfg.HeartbeatInterval,
	}
}

func heartbeat() {
	cfg.worker.Add(1)
	defer cfg.worker.Done()
	ticker := time.NewTicker(time.Duration(int64(time.Second) * int64(cfg.HeartbeatInterval)))
	klog.Infof("Heartbeat alive")
	for {
		select {
		case <-ticker.C:
			if err := eventHooks("heartbeat"); err != nil {
				klog.Errorf("On heartbeat: %s", err.Error())
			}
		case <-cfg.worker.Ctx.Done():
			ticker.Stop()
			klog.Info("Heartbeat end")
			return
		}
	}
}

func metaHandle(msg *nats.Msg) {
	data, _ := json.Marshal(getMeta())
	if err := msg.Respond(data); err != nil {
		klog.Errorf("On respond meta: %s", err.Error())
	}
}

func requestHandle(msg *nats.Msg) {
	ctx, tracer := cfg.worker.Start(cfg.worker.Ctx, "request-handle")
	request := NewRequest()
	if err := json.Unmarshal(msg.Data, request); err != nil {
		klog.Errorf("Cannot parse proxy request: %s", err.Error())
		tracer.RecordError(err)
		tracer.End()
	} else {
		tracer.SetAttributes(attribute.String("http.request.request_id", request.RequestId))
		if err := updateTaskStatus(TASK_ON_PROCESS, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
			tracer.RecordError(err)
			tracer.End()
			return
		}
		_, limitTrace := cfg.worker.Start(ctx, "wait-schedule")
		limitTrace.SetAttributes(attribute.Int("schedule.rate_limit_ms", cfg.RateLimitMs))
		scheduler.AddTask(func() {
			doRequest(ctx, tracer, limitTrace, request, msg)
		})
	}
}

// waitingRequest request chan waiter
func waitingRequest(requestFunc func() (*grequests.Response, error), respChan chan<- *grequests.Response, errChan chan<- error) {
	resp, err := requestFunc()
	if err != nil {
		errChan <- err
	} else {
		respChan <- resp
	}
}

func updateTaskStatus(newStatus taskStatus, requestId string, traceId trace.TraceID) error {
	if err := mq.PublishJson(strings.Join([]string{cfg.ServiceId, "task", requestId}, "."), &RequestProcessMsg{newStatus, requestId, traceId}); err != nil {
		klog.ErrorfDepth(1, "Failed update task status: %s", err.Error())
		return err
	}
	return nil
}

func doRequest(ctx context.Context, tracer trace.Span, limitTracer trace.Span, request *Request, msg *nats.Msg) {
	if err := updateTaskStatus(TASK_SCHEDULED, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
		limitTracer.RecordError(err)
		limitTracer.End()
		return
	}
	limitTracer.End()
	cfg.worker.Add(1)
	defer cfg.worker.Done()
	defer tracer.End()

	ro := request.BuildRequestOptions()
	ro.Context = ctx
	url := fmt.Sprintf("%s://%s%s", request.Protocol, request.Host, request.Path)
	klog.Infof("%s %s", request.Method, url)

	_, reqTrace := cfg.worker.Start(ctx, "do-request")
	defer reqTrace.End()
	reqTrace.SetAttributes(attribute.String("http.request.host", request.Host), attribute.String("http.request.method", request.Method), attribute.String("http.request.url", url))
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	respChan := make(chan *grequests.Response, 1)
	errChan := make(chan error, 1)
	rate := time.Now().UnixMilli()
	waitingRequest(func() (*grequests.Response, error) {
		return grequests.Req(request.Method, url, ro)
	}, respChan, errChan)
	var resp *grequests.Response
	for {
		select {
		case resp = <-respChan:
			if resp.Error != nil {
				klog.Errorf("Error at do request: %s", resp.Error.Error())
				reqTrace.RecordError(resp.Error)
				msg.Nak()
				if err := updateTaskStatus(TASK_FAILED, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
					reqTrace.RecordError(err)
				}
				return
			}
			goto ProcessResp
		case err := <-errChan:
			klog.Errorf("Error at create request: %s", err.Error())
			reqTrace.RecordError(err)
			reqStr, _ := json.Marshal(request)
			reqTrace.SetAttributes(attribute.String("http.request.raw", string(reqStr))) // Dump full request
			if err := updateTaskStatus(TASK_FAILED, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
				reqTrace.RecordError(err)
			}
			msg.Term()
			return
		case <-ticker.C:
			msg.InProgress()
		}
	}
ProcessResp:
	klog.Infof("%s %s - %d - %dms", request.Method, url, resp.StatusCode, time.Now().UnixMilli()-rate)
	reqTrace.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))

	reqTrace.End()

	_, packTrace := cfg.worker.Start(ctx, "response-compress")
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
		if err := updateTaskStatus(TASK_FAILED, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
			respTrace.RecordError(err)
		}
		return
	}
	if err := updateTaskStatus(TASK_FINISHED, request.RequestId, tracer.SpanContext().TraceID()); err != nil {
		respTrace.RecordError(err)
	}
}
