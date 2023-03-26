package main

import (
	proxy "github.com/TiyaAnlite/FocotServices/client-http-proxy/api"
	"github.com/TiyaAnlite/FocotServicesCommon/echox"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/otel/attribute"
	"k8s.io/klog/v2"
	"net/http"
	"strings"
)

func setupRoutes(e *echo.Echo) {
	assigned := e.Group("")

	if echox.JwtEnabled(cfg.EchoConfig) {
		jwtConfig := echox.DefaultJwtConfig(cfg.EchoConfig)
		assigned.Use(middleware.JWTWithConfig(jwtConfig))
		klog.Info("JWT enabled")
	}
	assigned.POST("/request", requestProxy)
}

func requestProxy(c echo.Context) error {
	ctx, trace := cfg.worker.Start(cfg.worker.Ctx, "requestProxy")
	defer trace.End()
	req, err := echox.CheckInput[ProxyRequest](c)
	if err != nil {
		trace.RecordError(err)
		return echox.NormalErrorResponse(c, http.StatusBadGateway, http.StatusBadRequest, err.Error())
	}
	if req.Timeout == 0 {
		req.Timeout = 10
	}
	_, mqTrace := cfg.worker.Start(ctx, "sendNATSRequest")
	defer mqTrace.End()
	mqTrace.SetAttributes(attribute.String("node", req.Node))
	resp, err := proxy.SendRequest(mq, strings.Join([]string{cfg.ServiceId, req.Node, "request"}, "."), req.Payload, req.Timeout)
	if err != nil {
		mqTrace.RecordError(err)
		klog.Errorf("At send request: %s", err.Error())
		return echox.NormalErrorResponse(c, http.StatusBadGateway, http.StatusBadRequest, err.Error())
	}
	return c.JSONBlob(resp.StatusCode, resp.Data)
}
