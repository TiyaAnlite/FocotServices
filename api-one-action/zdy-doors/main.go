package zdy_doors

import (
	"github.com/TiyaAnlite/FocotServices/api-one-action/api"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/labstack/echo/v4"
	"k8s.io/klog/v2"
)

type config struct {
	Key string `json:"key" yaml:"key" env:"ZDY_KEY"`
}

var (
	cfg      = &config{}
	instance = &ZdyDoors{}
)

func init() {
	envx.MustLoadEnv(cfg)
	api.Components = append(api.Components, instance)
}

type ZdyDoors struct {
}

func (i *ZdyDoors) Name() string {
	return "zdy"
}

func (i *ZdyDoors) Register(r *echo.Group, e *echo.Echo) {
	klog.Infof("zdy running")
}

func (i *ZdyDoors) Config() *api.ComponentConfig {
	return &api.ComponentConfig{
		Auth: true,
		Key:  cfg.Key,
	}
}
