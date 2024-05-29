package main

import (
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/tracex"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"k8s.io/klog/v2"
	"testing"
)

type config struct {
	echox.EchoConfig
	AuthKey string `json:"auth_key" yaml:"auth_key" env:"AUTH_KEY,required"`
}

var (
	cfg         = &config{}
	traceHelper = &tracex.ServiceTraceHelper{}
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
	traceHelper.SetupTrace()
}

func main() {
	go echox.Run(&cfg.EchoConfig, setupRoutes)
	utils.Wait4CtrlC()
	klog.Info("stopping...")
	echox.Shutdown(&cfg.EchoConfig)
}
