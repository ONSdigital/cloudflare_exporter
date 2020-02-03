package main

import "github.com/prometheus/client_golang/prometheus"

var (
	// zone metrics
	zoneCount    = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Subsystem: "zones", Name: "active_count", Help: "Number of active zones in the target Cloudflare account"})
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: namespace, Subsystem: "zones", Name: "http_requests", Help: "Number of HTTP requests made by clients"},
		[]string{"zone", "client_country_name"},
	)
	httpThreats = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: namespace, Subsystem: "zones", Name: "http_threats", Help: "Number of HTTP threats made by clients"},
		[]string{"zone", "client_country_name"},
	)

	// exporter metrics
	cfScrapes    = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "cloudflare_scrapes", Help: "Number of times this exporter has scraped cloudflare"})
	cfScrapeErrs = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "cloudflare_scrape_errors", Help: "Number of times this exporter has failed to scrape cloudflare"})
)

func registerMetrics() {
	prometheus.MustRegister(zoneCount)
	prometheus.MustRegister(httpRequests)
	prometheus.MustRegister(httpThreats)
	prometheus.MustRegister(cfScrapes)
	prometheus.MustRegister(cfScrapeErrs)
}
