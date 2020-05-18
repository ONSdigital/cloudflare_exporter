package main

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsMaxAge = 15 * time.Minute
)

func NewTimestampedMetric(
	valueType prometheus.ValueType, opts prometheus.Opts,
) *TimestampedMetric {
	fqName := strings.Join(
		[]string{opts.Namespace, opts.Subsystem, opts.Name}, "_",
	)
	return &TimestampedMetric{
		desc:      prometheus.NewDesc(fqName, opts.Help, nil, opts.ConstLabels),
		valueType: valueType,
	}
}

type TimestampedMetric struct {
	desc        *prometheus.Desc
	valueType   prometheus.ValueType
	value       float64
	labelValues []string
	timestamp   time.Time
}

func (m *TimestampedMetric) Set(value float64, timestamp time.Time) {
	m.value = value
	m.timestamp = timestamp
}

func (m *TimestampedMetric) Add(value float64, timestamp time.Time) {
	m.value += value
	m.timestamp = timestamp
}

func (m *TimestampedMetric) Describe(descs chan<- *prometheus.Desc) {
	descs <- m.desc
}

func (m *TimestampedMetric) Collect(metrics chan<- prometheus.Metric) {
	// Freshly registered metrics that have not been set should be stamped with
	// the current time (prometheus default behavior).
	timestamp := m.timestamp
	if timestamp == (time.Time{}) {
		timestamp = time.Now().UTC()
	}

	// Do not report timestamped metrics older than 15m
	// Prometheus complains about "Error on ingesting samples that are too old or
	// are too far into the future"
	if time.Now().UTC().Add(-metricsMaxAge).After(m.timestamp) {
		return
	}

	metrics <- prometheus.NewMetricWithTimestamp(timestamp, prometheus.MustNewConstMetric(
		m.desc, m.valueType, m.value, m.labelValues...,
	))
}

func NewTimestampedMetricVec(
	valueType prometheus.ValueType, opts prometheus.Opts, variableLabels []string,
) *TimestampedMetricVec {
	fqName := strings.Join(
		[]string{opts.Namespace, opts.Subsystem, opts.Name}, "_",
	)
	return &TimestampedMetricVec{
		desc:      prometheus.NewDesc(fqName, opts.Help, variableLabels, opts.ConstLabels),
		valueType: valueType,
		metrics:   map[string]*TimestampedMetric{},
	}
}

type TimestampedMetricVec struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
	metrics   map[string]*TimestampedMetric
}

func (m *TimestampedMetricVec) WithLabelValues(labelValues ...string) *TimestampedMetric {
	labelHash := hashLabels(labelValues)
	if m.metrics[labelHash] == nil {
		metric := &TimestampedMetric{
			desc:        m.desc,
			valueType:   m.valueType,
			labelValues: labelValues,
		}
		m.metrics[labelHash] = metric
	}
	return m.metrics[labelHash]
}

func (m *TimestampedMetricVec) Describe(descs chan<- *prometheus.Desc) {
	descs <- m.desc
}

func (m *TimestampedMetricVec) Collect(metrics chan<- prometheus.Metric) {
	for _, metric := range m.metrics {
		metric.Collect(metrics)
	}
}

func hashLabels(labels []string) string {
	hash := sha1.Sum([]byte(strings.Join(labels, "!!!")))
	return hex.EncodeToString(hash[:])
}
