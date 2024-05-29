package api

import "github.com/labstack/echo/v4"

var Components []ActionComponent

type ComponentConfig struct {
	Auth bool   `json:"auth" yaml:"auth"` // 启用认证，认证方式为Key
	Key  string `json:"key" yaml:"key"`   // 自定义认证密钥
}

type ActionComponent interface {
	Name() string
	Register(*echo.Group, *echo.Echo)
	Config() *ComponentConfig
}
