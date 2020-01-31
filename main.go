package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

const namespace = "cloudflare"

var (
	// arguments
	listenAddress = kingpin.Flag("listen-address", "Metrics exporter listen address.").
			Short('l').Envar("CLOUDFLARE_EXPORTER_LISTEN_ADDRESS").Default(":11313").String()
	cfEmail = kingpin.Flag("cloudflare-api-email", "email address for analytics API authentication.").
		Envar("CLOUDFLARE_API_EMAIL").Required().String()
	cfAPIKey = kingpin.Flag("cloudflare-api-key", "API key for analytics API authentication.").
			Envar("CLOUDFLARE_API_KEY").Required().String()
	cfAPIBaseURL = kingpin.Flag("cloudflare-api-base-url", "Cloudflare regular (non-analytics) API base URL").
			Envar("CLOUDFLARE_API_BASE_URL").Default("https://api.cloudflare.com/client/v4").String()
	cfAnalyticsAPIBaseURL = kingpin.Flag("cloudflare-analytics-api-base-url", "Cloudflare analytics (graphql) API base URL").
				Envar("CLOUDFLARE_ANALYTICS_API_BASE_URL").Default("https://api.cloudflare.com/client/v4/graphql").String()
	scrapeTimeoutSeconds = kingpin.Flag("scrape-timeout-seconds", "scrape timeout seconds").
				Envar("CLOUDFLARE_EXPORTER_SCRAPE_TIMEOUT_SECONDS").Default("30").Int()

	// metric descriptions
	zoneCount = prometheus.NewDesc(prometheus.BuildFQName(namespace, "zones", "count"), "Number of zones in the target Cloudflare account", nil, nil)

	// direct instrumentation counters
	totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "total_scrapes", Help: "Number of times this exporter has been scraped"})
)

func main() {
	// TODO structured logger
	logger := log.New(os.Stdout, "[cloudflare-exporter] ", log.LstdFlags)
	logger.Println("starting")

	kingpin.Parse()

	cfExporter := &exporter{
		email: *cfEmail, apiKey: *cfAPIKey, apiBaseURL: *cfAPIBaseURL,
		analyticsAPIBaseURL: *cfAnalyticsAPIBaseURL,
		scrapeTimeout:       time.Duration(*scrapeTimeoutSeconds) * time.Second,
	}
	prometheus.MustRegister(cfExporter)

	// TODO populate the build-time vars in
	// https://github.com/prometheus/common/blob/master/version/info.go with
	// goreleaser or something.
	prometheus.MustRegister(version.NewCollector("cloudflare_exporter"))

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())

	logger.Printf("listening on %s\n", *listenAddress)
	logger.Fatal(http.ListenAndServe(*listenAddress, router))
}

type exporter struct {
	email               string
	apiKey              string
	apiBaseURL          string
	analyticsAPIBaseURL string
	scrapeTimeout       time.Duration
}

func (e *exporter) Describe(descs chan<- *prometheus.Desc) {
	descs <- zoneCount
	descs <- totalScrapes.Desc()
}

func (e *exporter) Collect(metrics chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), e.scrapeTimeout)
	defer cancel()

	e.collectZones(ctx, metrics)

	// TODO does this need to be mutex-ed?
	totalScrapes.Inc()
	metrics <- totalScrapes
}

func (e *exporter) collectZones(ctx context.Context, metrics chan<- prometheus.Metric) {
	// TODO handle >50 zones (the API maximum per page) by requesting successive
	// pages. For now, we don't anticipate having >50 zones any time soon.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.apiBaseURL+"/zones?per_page=50", nil)
	if err != nil {
		metrics <- prometheus.NewInvalidMetric(zoneCount, err)
		return
	}
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		metrics <- prometheus.NewInvalidMetric(zoneCount, err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		metrics <- prometheus.NewInvalidMetric(zoneCount, fmt.Errorf("expected status 200, got %d", resp.StatusCode))
		return
	}

	defer resp.Body.Close()
	var zoneList zonesResp
	if err := json.NewDecoder(resp.Body).Decode(&zoneList); err != nil {
		metrics <- prometheus.NewInvalidMetric(zoneCount, err)
		return
	}
	var zones []string
	for _, zone := range zoneList.Result {
		zones = append(zones, zone.ID)
	}
	metrics <- prometheus.MustNewConstMetric(zoneCount, prometheus.GaugeValue, float64(len(zones)))
}

type zonesResp struct {
	Result []struct {
		ID string `json:"id"`
	} `json:"result"`
}
