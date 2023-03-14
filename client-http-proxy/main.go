package main

import (
	"context"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/tracex"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"strings"
	"sync"
	"testing"
	"time"
)

type config struct {
	natsx.NatsConfig
	ServiceId         string   `json:"service_id" yaml:"service_id" env:"SERVICE_ID" envDefault:"http-proxy"`
	NodeId            string   `json:"node_id" yaml:"node_id" env:"NODE_ID,required"`
	NodeGroups        []string `json:"node_group" yaml:"node_group" env:"NODE_GROUPS,required"`
	RateLimitMs       int      `json:"rate_limit_ms" yaml:"rate_limit_ms" env:"RATE_LIMIT_MS,required"`
	GzipMinLength     int      `json:"gzip_min_length" yaml:"gzip_min_length" env:"GZIP_MIN_LENGTH" envDefault:"1024"`
	HeartbeatInterval int      `json:"heart_beat_interval" yaml:"heart_beat_interval" env:"HEARTBEAT_INTERVAL" envDefault:"30"`
	worker            *worker
	uptime            time.Time
}

type worker struct {
	Ctx context.Context
	*sync.WaitGroup
	trace.Tracer
}

var (
	cfg         = &config{}
	mq          *natsx.NatsHelper
	traceHelper = &tracex.ServiceTraceHelper{}
	scheduler   = &TaskScheduler{}
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
	traceHelper.SetupTrace()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cfg.worker = &worker{
		Ctx:       ctx,
		WaitGroup: &sync.WaitGroup{},
		Tracer:    traceHelper.NewTracer(),
	}
	defer traceHelper.Shutdown(context.Background()) // Exit all 2

	initCtx, initTracer := cfg.worker.Start(ctx, "Init")
	defer initTracer.End()

	_, t1 := cfg.worker.Start(initCtx, "Connect NATS")
	defer t1.End()
	mq = &natsx.NatsHelper{}
	if err := mq.Open(cfg.NatsConfig); err != nil {
		t1.RecordError(err)
		klog.Errorf("Cannot connect to NATS: %s", err.Error())
		return
	}
	defer func() {
		if !mq.Nc.IsClosed() {
			mq.Close()
		}
	}() // Exit all 1

	for _, group := range cfg.NodeGroups {
		groupSub, err := mq.Nc.QueueSubscribe(strings.Join([]string{cfg.ServiceId, "groups", group, "request"}, "."), group, requestHandle)
		if err != nil {
			t1.RecordError(err)
			klog.Errorf(err.Error())
			return
		}
		mq.AddSubscribe(groupSub)
	}

	if err := mq.AddNatsHandler(strings.Join([]string{cfg.ServiceId, cfg.NodeId, "request"}, "."), requestHandle); err != nil {
		t1.RecordError(err)
		klog.Errorf(err.Error())
		return
	}

	if err := mq.AddNatsHandler(strings.Join([]string{cfg.ServiceId, cfg.NodeId, "meta"}, "."), metaHandle); err != nil {
		t1.RecordError(err)
		klog.Errorf(err.Error())
		return
	}

	t1.End()

	initTracer.End()
	cfg.uptime = time.Now()
	go heartbeat()
	klog.Infof("[%s]ready to accept requests, node id: %s, groups: %s", cfg.ServiceId, cfg.NodeId, cfg.NodeGroups)
	if cfg.RateLimitMs == 0 {
		go scheduler.Start()
	} else {
		go scheduler.Start(time.Duration(int64(time.Millisecond) * int64(cfg.RateLimitMs)))
	}
	// Online hook
	if err := eventHooks("online"); err != nil {
		t1.RecordError(err)
		klog.Errorf(err.Error())
		return
	}

	utils.Wait4CtrlC()
	klog.Infof("stopping...")
	cancel()
	// Offline hook
	if err := eventHooks("offline"); err != nil {
		klog.Errorf("On offline hook: %s", err.Error())
	}
	mq.Close() // Close nats first for wait scheduler
	cfg.worker.Wait()
	klog.Info("done")
}

func eventHooks(subject string) error {
	return mq.PublishJson(strings.Join([]string{cfg.ServiceId, "event", subject}, "."), getMeta())
}
