package main

import (
	"context"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/tracex"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sync"
	"testing"
	"time"
)

type config struct {
	natsx.NatsConfig
	ConfigFileName  string            `json:"config_file_name" yaml:"configFileName" env:"CONFIG_FILE_NAME" envDefault:"config.yaml"`
	StreamName      string            `json:"stream_name" yaml:"streamName" env:"STREAM_NAME" envDefault:"biliLiveRecorder"`
	RefreshInterval int               `json:"refresh_interval" yaml:"refreshInterval" env:"REFRESH_INTERVAL" envDefault:"1"`
	DataMaxAge      int               `json:"data_max_age" yaml:"dataMaxAge" env:"DATA_MAX_AGE" envDefault:"5"`
	Recorders       map[string]string `json:"recorders" yaml:"recorders"`
	worker          *worker
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
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
	envx.MustReadYamlConfig(cfg, cfg.ConfigFileName)
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
	klog.Infof("Stream name: %s", cfg.StreamName)
	if _, err := mq.Js.AddStream(&nats.StreamConfig{
		Name:    cfg.StreamName,
		MaxMsgs: 1,
		MaxAge:  time.Duration(int64(time.Second) * int64(cfg.DataMaxAge)),
	}); err != nil {
		t1.RecordError(err)
		klog.Errorf("Cannot create stream: %s", err.Error())
		return
	}
	defer func() {
		if !mq.Nc.IsClosed() {
			mq.Close()
		}
	}() // Exit all 1
	t1.End()
	initTracer.End()

	go doSync()

	utils.Wait4CtrlC()
	klog.Infof("stopping...")
	cancel()
	cfg.worker.Wait()
	klog.Info("done")
}
