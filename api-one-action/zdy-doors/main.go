package zdy_doors

import (
	"github.com/TiyaAnlite/FocotServices/api-one-action/api"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/labstack/echo/v4"
)

type config struct {
	Key      string `json:"key" yaml:"key" env:"ZDY_KEY"`
	Phone    string `json:"phone" yaml:"phone" env:"ZDY_PHONE,required"`
	Password string `json:"password" yaml:"password" env:"ZDY_PASSWORD,required"`
}

var (
	cfg      = &config{}
	instance = &ZdyService{}
)

func init() {
	envx.MustLoadEnv(cfg)
	api.Components = append(api.Components, instance)
}

type ZdyService struct {
	token       string
	communityId int
}

func (*ZdyService) Name() string {
	return "zdy"
}

func (*ZdyService) Register(r *echo.Group, e *echo.Echo) {
	r.GET("/doors", listDoorDevs)
	r.GET("/open", openDoor)
}

func (*ZdyService) Config() *api.ComponentConfig {
	return &api.ComponentConfig{
		Auth: true,
		Key:  cfg.Key,
	}
}
