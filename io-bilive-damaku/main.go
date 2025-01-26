package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/dbx"
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/natsx"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
	"sync"
	"testing"
)

type envConfig struct {
	echox.EchoConfig
	natsx.NatsConfig
	dbx.DBConfig
	dbx.RedisConfig
	ConfigFile string `json:"config_file" env:"CONFIG_FILE" envDefault:"config.yaml"`
}

var (
	envCfg    = &envConfig{}
	cfg       = &Config{}
	mq        = &natsx.NatsHelper{}
	db        = &dbx.GormHelper{}
	rdb       = &dbx.RedisHelper{}
	ctx       *CenterContext
	worker    = &sync.WaitGroup{}
	providers []RoomProvider
)

func init() {
	testing.Init()
	klog.InitFlags(nil)
	flag.Parse()
	envx.MustLoadEnv(envCfg)
	envx.MustReadYamlConfig(cfg, envCfg.ConfigFile)
	if err := mq.Open(envCfg.NatsConfig); err != nil {
		klog.Fatalf("Cannot connect to NATS: %s", err.Error())
	}
	if err := db.Open(&envCfg.DBConfig, dbx.PostgresProvider); err != nil {
		klog.Fatalf("Cannot connect to db: %s", err.Error())
	}
	if err := rdb.Open(&envCfg.RedisConfig); err != nil {
		klog.Fatalf("Cannot connect to redis: %s", err.Error())
	}
}

func main() {
	// build global context
	ctx = &CenterContext{
		Context:  context.Background(),
		Worker:   worker,
		Registry: prometheus.NewRegistry(),
		MQ:       mq,
		DB:       db,
		RDB:      rdb,
	}
	go echox.Run(&envCfg.EchoConfig, setupRoutes)
	providerInit()
	klog.Info("fire...")
	utils.Wait4CtrlC()
}

func setupRoutes(e *echo.Echo) {
	ctx.Echo = e
	e.GET("/metrics", echoprometheus.NewHandlerWithConfig(echoprometheus.HandlerConfig{Gatherer: ctx.Registry}))
}

func providerInit() {
	for _, p := range cfg.Provider {
		var addingProvider RoomProvider
		switch p.Type {
		case "room":
			addingProvider = &StaticConfigProvider{}
		case "api":
			addingProvider = &ApiConfigProvider{}
		}
		if err := json.Unmarshal(p.RawMessage, &addingProvider); err != nil {
			klog.Fatalf("Cannot unmarshal provider(%s) config: %s", p.Type, err.Error())
		}
		if err := addingProvider.Init(ctx); err != nil {
			klog.Fatalf("Provider init error: %s", err.Error())
		}
		providers = append(providers, addingProvider)
	}
}
