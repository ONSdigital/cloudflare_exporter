package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/oklog/run"
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
	cfScrapeIntervalSeconds = kingpin.Flag("cloudflare-scrape-interval-seconds", "Interval at which to retrieve metrics from Cloudflare, separate from being scraped by prometheus").
				Envar("CLOUDFLARE_SCRAPE_INTERVAL_MINUTES").Default("300").Int()
	scrapeTimeoutSeconds = kingpin.Flag("scrape-timeout-seconds", "scrape timeout seconds").
				Envar("CLOUDFLARE_EXPORTER_SCRAPE_TIMEOUT_SECONDS").Default("30").Int()

	// zone metrics
	zoneCount = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Subsystem: "zones", Name: "count", Help: "Number of zones in the target Cloudflare account"})

	// exporter metrics
	totalScrapes = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "total_scrapes", Help: "Number of times this exporter has been scraped"})
	cfScrapes    = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "cloudflare_scrapes", Help: "Number of times this exporter has scraped cloudflare"})
	cfScrapeErrs = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Subsystem: "exporter", Name: "cloudflare_scrape_errors", Help: "Number of times this exporter has failed to scrape cloudflare"})
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
		scrapeInterval:      time.Duration(*cfScrapeIntervalSeconds) * time.Second,
	}
	prometheus.MustRegister(cfExporter)

	// TODO populate the build-time vars in
	// https://github.com/prometheus/common/blob/master/version/info.go with
	// goreleaser or something.
	prometheus.MustRegister(version.NewCollector("cloudflare_exporter"))

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())

	runGroup := run.Group{}

	logger.Printf("listening on %s\n", *listenAddress)
	serverSocket, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		logger.Fatal(err)
	}
	runGroup.Add(func() error {
		return http.Serve(serverSocket, router)
	}, func(error) {
		serverSocket.Close()
	})

	cfScrapeCtx, cancelCfScrape := context.WithCancel(context.Background())
	runGroup.Add(func() error {
		return cfExporter.scrapeCloudflare(cfScrapeCtx, logger)
	}, func(error) {
		cancelCfScrape()
	})

	if err := runGroup.Run(); err != nil {
		logger.Fatal(err)
	}
}

type exporter struct {
	email               string
	apiKey              string
	apiBaseURL          string
	analyticsAPIBaseURL string
	scrapeInterval      time.Duration
	scrapeTimeout       time.Duration
}

func (e *exporter) scrapeCloudflare(ctx context.Context, logger *log.Logger) error {
	ticker := time.Tick(e.scrapeInterval)
	for {
		select {
		case <-ticker:
			if err := e.scrapeCloudflareOnce(ctx, logger); err != nil {
				// Returning an error here would cause the exporter to crash. If it
				// crashloops but prometheus manages to scrape it in between crashes, we
				// might never notice that we are not updating our cached metrics.
				// Instead, we should alert on the exporter_cloudflare_scrape_errors
				// metric.
				logger.Println(err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (e *exporter) scrapeCloudflareOnce(ctx context.Context, logger *log.Logger) (err error) {
	defer func() {
		if err != nil {
			cfScrapeErrs.Inc()
		}
	}()

	logger.Println("scraping Cloudflare")
	cfScrapes.Inc()

	ctx, cancel := context.WithTimeout(ctx, e.scrapeTimeout)
	defer cancel()

	// TODO handle >50 zones (the API maximum per page) by requesting successive
	// pages. For now, we don't anticipate having >50 zones any time soon.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.apiBaseURL+"/zones?per_page=50", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("expected status 200, got %d", resp.StatusCode)
		return err
	}

	defer resp.Body.Close()
	zones, err := parseZoneIDs(resp.Body)
	if err != nil {
		return err
	}

	zoneCount.Set(float64(len(zones)))
	return nil
}

func (e *exporter) Describe(descs chan<- *prometheus.Desc) {
	descs <- zoneCount.Desc()
	descs <- totalScrapes.Desc()
	descs <- cfScrapes.Desc()
	descs <- cfScrapeErrs.Desc()
}

func (e *exporter) Collect(metrics chan<- prometheus.Metric) {
	// TODO does this need to be mutex-ed?
	totalScrapes.Inc()
	metrics <- totalScrapes

	metrics <- zoneCount
	metrics <- cfScrapes
	metrics <- cfScrapeErrs
}

func parseZoneIDs(apiRespBody io.Reader) (map[string]string, error) {
	var zoneList zonesResp
	if err := json.NewDecoder(apiRespBody).Decode(&zoneList); err != nil {
		return nil, err
	}
	zones := map[string]string{}
	for _, zone := range zoneList.Result {
		zones[zone.ID] = zone.Name
	}
	return zones, nil
}

type zonesResp struct {
	Result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"result"`
}
