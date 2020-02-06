package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// zone metrics
	zonesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "active",
			Help:      "Number of active zones in the target Cloudflare account",
		},
	)
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_requests_total",
			Help:      "Number of HTTP requests made by clients",
		},
		[]string{"zone", "client_country_name"},
	)
	httpThreats = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_threats_total",
			Help:      "Number of HTTP threats made by clients",
		},
		[]string{"zone", "client_country_name"},
	)
	httpBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "zones",
			Name:      "http_bytes_total",
			Help:      "Number of HTTP bytes received by clients",
		},
		[]string{"zone", "client_country_name"},
	)

	// graphql metrics
	cfScrapes = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "scrapes_total",
			Help:      "Number of times this exporter has scraped cloudflare",
		},
	)
	cfScrapeErrs = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "scrape_errors_total",
			Help:      "Number of times this exporter has failed to scrape cloudflare",
		},
	)
	cfLastSuccessTimestampSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "graphql",
			Name:      "last_success_timestamp_seconds",
			Help:      "Time that the analytics data was last updated.",
		},
	)
)
