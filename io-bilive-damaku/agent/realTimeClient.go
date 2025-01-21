package main

import (
	"context"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/tracex"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"k8s.io/klog/v2"
	"sync"
)

type config struct {
	natsx.NatsConfig
	AgentId       string `json:"agent_id" yaml:"agentId" env:"AGENT_ID"`
	SubjectPrefix string `json:"subject_prefix" yaml:"subject_prefix" env:"SUBJECT_PREFIX" envDefault:"dmCenter"`
}

var (
	cfg         = &config{}
	mq          *natsx.NatsHelper
	traceHelper = &tracex.ServiceTraceHelper{}
	client      = &DamakuCenterAgent{}
	worker      = sync.WaitGroup{}
	ctx         context.Context
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
	envx.MustLoadEnv(cfg)
	traceHelper.SetupTrace()
}

func main() {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())
	if err := mq.Open(cfg.NatsConfig); err != nil {
		klog.Errorf("Cannot connect to NATS: %s", err.Error())
		return
	}
	defer mq.Close()
	klog.Infof("using subject prefix: %s", cfg.SubjectPrefix)
	worker.Add(1)
	go waiter(cancel)
	go client.Init()
	klog.Info("fire...")
	worker.Wait()
}

func waiter(cancel context.CancelFunc) {
	utils.Wait4CtrlC()
	klog.Info("exiting...")
	cancel()
}
