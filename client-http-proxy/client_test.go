package main

import (
	. "github.com/TiyaAnlite/FocotServices/client-http-proxy/api"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"strings"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	mq = &natsx.NatsHelper{}
	if err := mq.Open(cfg.NatsConfig); err != nil {
		t.Logf("Cannot connect to NATS: %s", err.Error())
	}
	request := NewRequest(
		WithRequestHost("api.focot.cn"),
		WithRequestPath("/public/ip"),
	)
	var resp ProxyPackedResponse
	if err := mq.RequestJson(strings.Join([]string{cfg.ServiceId, cfg.NodeId, "request"}, "."), &request, &resp, time.Second*3); err != nil {
		t.Fatal(err)
	}
	respData, err := PrepareUnPackedResponse(&resp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("data: %s", string(respData.Data))
}
