package main

import (
	"context"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"k8s.io/klog/v2"
	"strings"
	"sync"
	"testing"
)

type config struct {
	natsx.NatsConfig
	ServiceId     string `json:"service_id" yaml:"service_id" env:"SERVICE_ID" envDefault:"http-proxy"`
	NodeId        string `json:"node_id" yaml:"node_id" env:"NODE_ID,required"`
	GzipMinLength int    `json:"gzip_min_length" yaml:"gzip_min_length" env:"GZIP_MIN_LENGTH" envDefault:"1024"`
	worker        *worker
}

type worker struct {
	ctx context.Context
	*sync.WaitGroup
}

var (
	cfg = &config{}
	mq  *natsx.NatsHelper
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	cfg.worker = &worker{
		ctx:       ctx,
		WaitGroup: &sync.WaitGroup{},
	}

	mq = &natsx.NatsHelper{}
	if err := mq.Open(cfg.NatsConfig); err != nil {
		klog.Fatal("Cannot connect to NATS: %s", err.Error())
	}

	if err := mq.AddNatsHandler(strings.Join([]string{cfg.ServiceId, "groups", "request"}, "."), requestHandle); err != nil {
		klog.Fatal(err.Error())
	}
	if err := mq.AddNatsHandler(strings.Join([]string{cfg.ServiceId, cfg.NodeId, "request"}, "."), requestHandle); err != nil {
		klog.Fatal(err.Error())
	}

	klog.Infof("[%s]ready to accept requests, node id: %s", cfg.ServiceId, cfg.NodeId)

	utils.Wait4CtrlC()
	klog.Infof("stopping...")
	mq.Close()
	cancel()
	cfg.worker.Wait()
	klog.Info("done")
}
