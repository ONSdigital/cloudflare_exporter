package cfexporter

import "github.com/prometheus/client_golang/prometheus"

const namespace = "cloudflare"

type Exporter struct {
	Email  string
	APIKey string

	totalScrapes prometheus.Counter
}

func NewExporter(email, apiKey string) *Exporter {
	return &Exporter{
		Email:  email,
		APIKey: apiKey,
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "exporter",
			Name:      "total_scrapes",
		}),
	}
}

func (e *Exporter) Describe(descs chan<- *prometheus.Desc) {
	descs <- e.totalScrapes.Desc()
}

func (e *Exporter) Collect(metrics chan<- prometheus.Metric) {
	e.totalScrapes.Inc()
	metrics <- e.totalScrapes
}
