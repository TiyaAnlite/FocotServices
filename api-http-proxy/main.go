package main

import (
	"context"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/tracex"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/klog/v2"
	"sync"
	"testing"
)

type config struct {
	echox.EchoConfig
	natsx.NatsConfig
	ServiceId string `json:"service_id" yaml:"service_id" env:"SERVICE_ID" envDefault:"http-proxy"`
	worker    *worker
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

	go echox.Run(&cfg.EchoConfig, setupRoutes)

	utils.Wait4CtrlC()
	klog.Infof("stopping...")
	cancel()
	cfg.worker.Wait()
	klog.Info("done")
}
