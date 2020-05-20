package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestTimestampedMetric_counter(t *testing.T) {
	metric := NewTimestampedMetric(
		prometheus.CounterValue, prometheus.Opts{
			Namespace:   "namespace",
			Subsystem:   "subsystem",
			Name:        "name",
			Help:        "help",
			ConstLabels: prometheus.Labels{"const": "constval"},
		},
	)
	metric.Add(1, fixedTime)
	metric.Add(3, fixedTime)
	compareMetricsFixture(t, "counter", metric)
}

func TestTimestampedMetric_gauge(t *testing.T) {
	metric := NewTimestampedMetric(
		prometheus.GaugeValue, prometheus.Opts{
			Namespace:   "namespace",
			Subsystem:   "subsystem",
			Name:        "name",
			Help:        "help",
			ConstLabels: prometheus.Labels{"const": "constval"},
		},
	)
	metric.Set(10, fixedTime)
	metric.Set(3.2, fixedTime)
	compareMetricsFixture(t, "gauge", metric)
}

func TestTimestampedMetric_countervec(t *testing.T) {
	vec := NewTimestampedMetricVec(
		prometheus.CounterValue, prometheus.Opts{
			Namespace:   "namespace",
			Subsystem:   "subsystem",
			Name:        "name",
			Help:        "help",
			ConstLabels: prometheus.Labels{"const": "constval"},
		}, []string{"l1", "l2"},
	)

	vec.WithLabelValues("foo", "bar").Add(1, fixedTime.Add(time.Second*1))
	vec.WithLabelValues("foo", "bar").Add(2, fixedTime.Add(time.Second*2))
	vec.WithLabelValues("foo", "baz").Add(10, fixedTime.Add(time.Second*3))
	vec.WithLabelValues("baz", "bar").Add(11, fixedTime.Add(time.Second*4))
	vec.WithLabelValues("banana", "potato").Add(100, fixedTime.Add(time.Second*5))

	compareMetricsFixture(t, "countervec", vec)
}

func compareMetricsFixture(t *testing.T, name string, metrics prometheus.Collector) {
	fixture, err := os.Open(filepath.Join("testdata", "timestamped_metric_fixtures", name+".metrics"))
	require.Nil(t, err)
	defer fixture.Close()

	// Formatting of this failure output is more readable without testify
	err = testutil.CollectAndCompare(metrics, fixture, "namespace_subsystem_name")
	if err != nil {
		t.Error(err)
	}
}

var (
	fixedTime = time.Date(2020, 05, 18, 10, 07, 0, 0, time.UTC)
)
