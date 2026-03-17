package main

import (
	"math"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type ProbeMetrics struct {
	registry      *prometheus.Registry
	successTotal  *prometheus.CounterVec
	failureTotal  *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	successWindow *SuccessRatioCollector
}

func NewProbeMetrics(hosts []string) *ProbeMetrics {
	m := &ProbeMetrics{
		registry: prometheus.NewRegistry(),
		successTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "io_net_check_probe_success_total",
			Help: "Total successful TCP probe attempts grouped by host.",
		}, []string{"host"}),
		failureTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "io_net_check_probe_failure_total",
			Help: "Total timed out TCP probe attempts grouped by host.",
		}, []string{"host"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "io_net_check_probe_duration_seconds",
			Help:    "Duration of each TCP probe attempt grouped by host.",
			Buckets: prometheus.DefBuckets,
		}, []string{"host"}),
		successWindow: NewSuccessRatioCollector(hosts),
	}

	m.registry.MustRegister(m.successTotal, m.failureTotal, m.duration, m.successWindow)
	return m
}

func (m *ProbeMetrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *ProbeMetrics) RecordSuccess(host string) {
	m.successTotal.WithLabelValues(host).Inc()
	m.successWindow.RecordSuccess(host)
}

func (m *ProbeMetrics) RecordTimeoutFailure(host string) {
	m.failureTotal.WithLabelValues(host).Inc()
	m.successWindow.RecordFailure(host)
}

func (m *ProbeMetrics) ObserveDuration(host string, seconds float64) {
	m.duration.WithLabelValues(host).Observe(seconds)
}

type SuccessRatioCollector struct {
	desc  *prometheus.Desc
	mu    sync.Mutex
	state map[string]*successRatioState
}

type successRatioState struct {
	success      uint64
	failure      uint64
	lastValue    float64
	hasLastValue bool
}

func NewSuccessRatioCollector(hosts []string) *SuccessRatioCollector {
	c := &SuccessRatioCollector{
		desc: prometheus.NewDesc(
			"io_net_check_probe_success_ratio",
			"Success ratio since the previous metrics scrape grouped by host.",
			[]string{"host"},
			nil,
		),
		state: make(map[string]*successRatioState, len(hosts)),
	}
	for _, host := range hosts {
		c.ensureHostLocked(host)
	}
	return c
}

func (c *SuccessRatioCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect resets the window after each scrape so the gauge reflects the latest scrape interval only.
func (c *SuccessRatioCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hosts := make([]string, 0, len(c.state))
	for host := range c.state {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	for _, host := range hosts {
		state := c.state[host]
		ratio := math.NaN()
		total := state.success + state.failure
		if total > 0 {
			ratio = float64(state.success) / float64(total)
			state.lastValue = ratio
			state.hasLastValue = true
		} else if state.hasLastValue {
			ratio = state.lastValue
		}
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, ratio, host)
		state.success = 0
		state.failure = 0
	}
}

func (c *SuccessRatioCollector) RecordSuccess(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureHostLocked(host).success++
}

func (c *SuccessRatioCollector) RecordFailure(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureHostLocked(host).failure++
}

func (c *SuccessRatioCollector) ensureHostLocked(host string) *successRatioState {
	if state, ok := c.state[host]; ok {
		return state
	}
	state := &successRatioState{}
	c.state[host] = state
	return state
}
