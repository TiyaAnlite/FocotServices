package main

import (
	"math"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestSuccessRatioCollectorKeepsLastValidValue(t *testing.T) {
	t.Parallel()

	metrics := NewProbeMetrics([]string{"bilibili.com"})
	metrics.RecordSuccess("bilibili.com")
	metrics.RecordTimeoutFailure("bilibili.com")

	first := gatherGaugeValue(t, metrics.Registry(), "io_net_check_probe_success_ratio", "bilibili.com")
	if math.IsNaN(first) || first != 0.5 {
		t.Fatalf("unexpected first ratio: %v", first)
	}

	second := gatherGaugeValue(t, metrics.Registry(), "io_net_check_probe_success_ratio", "bilibili.com")
	if second != 0.5 {
		t.Fatalf("expected last valid value after empty window, got %v", second)
	}
}

func TestSuccessRatioCollectorStartsWithNaN(t *testing.T) {
	t.Parallel()

	metrics := NewProbeMetrics([]string{"bilibili.com"})

	first := gatherGaugeValue(t, metrics.Registry(), "io_net_check_probe_success_ratio", "bilibili.com")
	if !math.IsNaN(first) {
		t.Fatalf("expected NaN before any valid sample, got %v", first)
	}
}

func gatherGaugeValue(t *testing.T, registry interface {
	Gather() ([]*dto.MetricFamily, error)
}, metricName, host string) float64 {
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
				if label.GetName() == "host" && label.GetValue() == host {
					if metric.GetGauge() != nil {
						return metric.GetGauge().GetValue()
					}
				}
			}
		}
	}

	t.Fatalf("metric %s with host=%s not found", metricName, host)
	return 0
}
