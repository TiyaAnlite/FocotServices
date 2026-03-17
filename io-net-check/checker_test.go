package main

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestCheckerRunRoundRecordsMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewProbeMetrics([]string{"bilibili.com"})
	checker := NewChecker(
		[]ProbeTarget{{Endpoint: "https://bilibili.com", Host: "bilibili.com", Port: 443}},
		metrics,
		fakeResolver{
			results: map[string][]net.IPAddr{
				"bilibili.com": {
					{IP: net.ParseIP("1.1.1.1")},
					{IP: net.ParseIP("1.1.1.2")},
					{IP: net.ParseIP("1.1.1.3")},
				},
			},
		},
		fakeDialer{dial: func(_ context.Context, _, address string) (net.Conn, error) {
			switch address {
			case "1.1.1.1:443":
				return dummyConn{}, nil
			case "1.1.1.2:443":
				return nil, timeoutErr{}
			default:
				return nil, errors.New("connection refused")
			}
		}},
		2,
	)

	checker.runRound(context.Background())

	if got := testutil.ToFloat64(metrics.successTotal.WithLabelValues("bilibili.com")); got != 1 {
		t.Fatalf("unexpected success counter: %v", got)
	}
	if got := testutil.ToFloat64(metrics.failureTotal.WithLabelValues("bilibili.com")); got != 1 {
		t.Fatalf("unexpected failure counter: %v", got)
	}
	if got := gatherGaugeValue(t, metrics.Registry(), "io_net_check_probe_success_ratio", "bilibili.com"); got != 0.5 {
		t.Fatalf("unexpected success ratio: %v", got)
	}
	if got := gatherHistogramCount(t, metrics.Registry(), "io_net_check_probe_duration_seconds", "bilibili.com"); got != 3 {
		t.Fatalf("unexpected histogram sample count: %d", got)
	}
}

func TestCheckerRunRoundSkipsResolverErrors(t *testing.T) {
	t.Parallel()

	metrics := NewProbeMetrics([]string{"bilibili.com"})
	checker := NewChecker(
		[]ProbeTarget{{Endpoint: "https://bilibili.com", Host: "bilibili.com", Port: 443}},
		metrics,
		fakeResolver{errs: map[string]error{"bilibili.com": errors.New("lookup failed")}},
		fakeDialer{dial: func(_ context.Context, _, _ string) (net.Conn, error) {
			t.Fatal("dial should not be called when resolve fails")
			return nil, nil
		}},
		1,
	)

	checker.runRound(context.Background())

	if got := testutil.ToFloat64(metrics.successTotal.WithLabelValues("bilibili.com")); got != 0 {
		t.Fatalf("unexpected success counter: %v", got)
	}
	if got := testutil.ToFloat64(metrics.failureTotal.WithLabelValues("bilibili.com")); got != 0 {
		t.Fatalf("unexpected failure counter: %v", got)
	}
	if got := gatherHistogramCount(t, metrics.Registry(), "io_net_check_probe_duration_seconds", "bilibili.com"); got != 0 {
		t.Fatalf("unexpected histogram sample count: %d", got)
	}
	if got := gatherGaugeValue(t, metrics.Registry(), "io_net_check_probe_success_ratio", "bilibili.com"); !math.IsNaN(got) {
		t.Fatalf("expected NaN ratio for empty window, got %v", got)
	}
}

func TestCheckerTriggerSkipsWhileRunning(t *testing.T) {
	t.Parallel()

	enter := make(chan struct{}, 1)
	release := make(chan struct{})
	metrics := NewProbeMetrics([]string{"bilibili.com"})
	checker := NewChecker(
		[]ProbeTarget{{Endpoint: "https://bilibili.com", Host: "bilibili.com", Port: 443}},
		metrics,
		fakeResolver{
			results: map[string][]net.IPAddr{
				"bilibili.com": {{IP: net.ParseIP("1.1.1.1")}},
			},
		},
		fakeDialer{dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			enter <- struct{}{}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return dummyConn{}, nil
			}
		}},
		1,
	)

	if !checker.Trigger(context.Background()) {
		t.Fatal("first trigger should start a round")
	}
	<-enter
	if checker.Trigger(context.Background()) {
		t.Fatal("second trigger should be skipped while round is running")
	}

	close(release)
	waitUntil(t, time.Second, func() bool {
		return !checker.running.Load()
	})
}

func gatherHistogramCount(t *testing.T, registry interface {
	Gather() ([]*dto.MetricFamily, error)
}, metricName, host string) uint64 {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics failed: %v", err)
	}
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "host" && label.GetValue() == host && metric.GetHistogram() != nil {
					return metric.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

type fakeResolver struct {
	results map[string][]net.IPAddr
	errs    map[string]error
}

func (r fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if err, ok := r.errs[host]; ok {
		return nil, err
	}
	return r.results[host], nil
}

type fakeDialer struct {
	active atomic.Int64
	dial   func(ctx context.Context, network, address string) (net.Conn, error)
}

func (d fakeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.active.Add(1)
	defer d.active.Add(-1)
	return d.dial(ctx, network, address)
}

type dummyConn struct{}

func (dummyConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (dummyConn) Write(b []byte) (int, error)        { return len(b), nil }
func (dummyConn) Close() error                       { return nil }
func (dummyConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (dummyConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (dummyConn) SetDeadline(_ time.Time) error      { return nil }
func (dummyConn) SetReadDeadline(_ time.Time) error  { return nil }
func (dummyConn) SetWriteDeadline(_ time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return "tcp" }
func (a dummyAddr) String() string  { return string(a) }

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
