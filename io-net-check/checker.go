package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"
)

type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type Checker struct {
	targets        []ProbeTarget
	resolver       IPResolver
	dialer         Dialer
	metrics        *ProbeMetrics
	maxConcurrency int
	running        atomic.Bool
}

func NewChecker(targets []ProbeTarget, metrics *ProbeMetrics, resolver IPResolver, dialer Dialer, maxConcurrency int) *Checker {
	return &Checker{
		targets:        targets,
		resolver:       resolver,
		dialer:         dialer,
		metrics:        metrics,
		maxConcurrency: maxConcurrency,
	}
}

func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.Trigger(ctx) {
				klog.Warning("probe round skipped because the previous round is still running")
			}
		}
	}
}

// Trigger starts one probe round unless the previous round is still active.
func (c *Checker) Trigger(ctx context.Context) bool {
	if !c.running.CompareAndSwap(false, true) {
		return false
	}

	go func() {
		defer c.running.Store(false)
		c.runRound(ctx)
	}()
	return true
}

func (c *Checker) runRound(ctx context.Context) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.maxConcurrency)

	for _, target := range c.targets {
		ipAddrs, err := c.resolver.LookupIPAddr(ctx, target.Host)
		if err != nil {
			klog.Errorf("resolve target %s failed: %s", target.Endpoint, err.Error())
			continue
		}

		for _, ipAddr := range ipAddrs {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(target ProbeTarget, ipAddr net.IPAddr) {
				defer wg.Done()
				defer func() { <-sem }()
				c.probeOnce(ctx, target, ipAddr)
			}(target, ipAddr)
		}
	}

	wg.Wait()
}

func (c *Checker) probeOnce(ctx context.Context, target ProbeTarget, ipAddr net.IPAddr) {
	startAt := time.Now()
	address := net.JoinHostPort(ipAddr.IP.String(), strconv.Itoa(target.Port))
	conn, err := c.dialer.DialContext(ctx, "tcp", address)
	c.metrics.ObserveDuration(target.Host, time.Since(startAt).Seconds())
	if err == nil {
		_ = conn.Close()
		c.metrics.RecordSuccess(target.Host)
		return
	}

	if IsTimeoutError(err) {
		c.metrics.RecordTimeoutFailure(target.Host)
		return
	}

	if errors.Is(err, context.Canceled) {
		return
	}
	klog.Warningf("probe target %s via %s failed: %s", target.Endpoint, ipAddr.IP.String(), err.Error())
}

func IsTimeoutError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
