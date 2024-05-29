package main

import (
	"fmt"
	"github.com/TiyaAnlite/FocotServices/api-one-action/api"
	_ "github.com/TiyaAnlite/FocotServices/api-one-action/zdy-doors"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"k8s.io/klog/v2"
)

func setupRoutes(e *echo.Echo) {
	authed := e.Group("", useAuth(cfg.AuthKey))
	for _, component := range api.Components {
		c := component.Config()
		name := component.Name()
		if c == nil {
			klog.Infof("[%s]skipping component because return nil config", name)
			continue
		}
		klog.Infof("[Component Install]%s", name)
		var compRoute *echo.Group
		if c.Auth {
			if c.Key != "" {
				// root => name => auth
				compRoute = e.Group(fmt.Sprintf("/%s", name), useAuth(c.Key))
			} else {
				// root(authed) => name
				compRoute = authed.Group(fmt.Sprintf("/%s", name))
			}
		} else {
			// root => name
			compRoute = e.Group(fmt.Sprintf("/%s", name))
		}
		component.Register(compRoute, e)
	}
}

func useAuth(key string) echo.MiddlewareFunc {
	return middleware.KeyAuth(func(auth string, c echo.Context) (bool, error) {
		return key == auth, nil
	})
}
