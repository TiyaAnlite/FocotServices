package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"github.com/TiyaAnlite/FocotServicesCommon/utils"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

var (
	cfg        = &config{}
	checker    *Checker
	probeStats *ProbeMetrics
	httpServer *echo.Echo
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
	envx.MustReadYamlConfig(cfg, cfg.ConfigFileName)
}

func main() {
	targets, err := cfg.Validate()
	if err != nil {
		klog.Fatalf("invalid config: %s", err.Error())
	}

	hosts := collectHosts(targets)
	probeStats = NewProbeMetrics(hosts)
	checker = NewChecker(
		targets,
		probeStats,
		net.DefaultResolver,
		&net.Dialer{Timeout: cfg.DialTimeout},
		cfg.MaxConcurrency,
	)

	ctx, cancel := context.WithCancel(context.Background())

	go runHTTPServer()
	go checker.Start(ctx, cfg.Interval)

	klog.Infof("io-net-check started with %d targets, interval=%s, dialTimeout=%s, maxConcurrency=%d",
		len(targets), cfg.Interval, cfg.DialTimeout, cfg.MaxConcurrency)
	utils.Wait4CtrlC()
	cancel()
	shutdownHTTPServer()
	klog.Info("io-net-check stopped")
}

func runHTTPServer() {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.GET("/metrics", echo.WrapHandler(promhttp.HandlerFor(probeStats.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})))
	httpServer = e

	address := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)
	if err := e.Start(address); err != nil && !errors.Is(err, http.ErrServerClosed) {
		klog.Fatalf("start http server failed: %s", err.Error())
	}
}

func shutdownHTTPServer() {
	if httpServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		klog.Errorf("shutdown http server failed: %s", err.Error())
	}
}

func collectHosts(targets []ProbeTarget) []string {
	hostSet := make(map[string]struct{}, len(targets))
	hosts := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, ok := hostSet[target.Host]; ok {
			continue
		}
		hostSet[target.Host] = struct{}{}
		hosts = append(hosts, target.Host)
	}
	return hosts
}
