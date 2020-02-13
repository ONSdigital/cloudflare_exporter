package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	zonesActive                   prometheus.Gauge
	httpRequests                  *prometheus.CounterVec
	httpThreats                   *prometheus.CounterVec
	httpBytes                     *prometheus.CounterVec
	httpCachedRequests            *prometheus.CounterVec
	httpCachedBytes               *prometheus.CounterVec
	firewallEvents                *prometheus.CounterVec
	healthCheckEvents             *prometheus.CounterVec
	cfScrapes                     prometheus.Counter
	cfScrapeErrs                  prometheus.Counter
	cfLastSuccessTimestampSeconds prometheus.Gauge
)

func registerMetrics(reg prometheus.Registerer) {
	// zone metrics
	zonesActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "active",
			Help:      "Number of active zones in the target Cloudflare account",
		},
	)
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_requests_total",
			Help:      "Number of HTTP requests made by clients",
		},
		[]string{"zone", "client_country_name", "client_http_protocol", "edge_response_status", "threat_pathing_name"},
	)
	httpThreats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_threats_total",
			Help:      "Number of HTTP threats made by clients",
		},
		[]string{"zone", "client_country_name"},
	)
	httpBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_bytes_total",
			Help:      "Number of HTTP bytes received by clients",
		},
		[]string{"zone", "client_country_name"},
	)
	httpCachedRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_cached_requests_total",
			Help:      "Number of cached HTTP requests served.",
		},
		[]string{"zone"},
	)
	httpCachedBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_cached_bytes_total",
			Help:      "Number of cached HTTP bytes served.",
		},
		[]string{"zone"},
	)
	firewallEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "firewall_events_total",
			Help:      "Number of firewall events.",
		},
		[]string{"zone", "action", "source", "ruleID"},
	)
	healthCheckEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "health_check_events_total",
			Help:      "Number of health check events.",
		},
		[]string{"zone", "failure_reason", "health_check_name", "health_status", "origin_response_status", "region", "scope"},
	)

	// graphql metrics
	cfScrapes = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "scrapes_total",
			Help:      "Number of times this exporter has scraped cloudflare",
		},
	)
	cfScrapeErrs = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "scrape_errors_total",
			Help:      "Number of times this exporter has failed to scrape cloudflare",
		},
	)
	cfLastSuccessTimestampSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "last_success_timestamp_seconds",
			Help:      "Time that the analytics data was last updated.",
		},
	)

	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	reg.MustRegister(zonesActive)
	reg.MustRegister(httpRequests)
	reg.MustRegister(httpThreats)
	reg.MustRegister(httpBytes)
	reg.MustRegister(httpCachedRequests)
	reg.MustRegister(httpCachedBytes)
	reg.MustRegister(firewallEvents)
	reg.MustRegister(healthCheckEvents)
	reg.MustRegister(cfScrapes)
	reg.MustRegister(cfScrapeErrs)
	reg.MustRegister(cfLastSuccessTimestampSeconds)
}
