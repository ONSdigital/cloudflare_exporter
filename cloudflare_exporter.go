package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/machinebox/graphql"
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

	zones, err := e.getZones(ctx)
	if err != nil {
		return err
	}
	zonesActive.Set(float64(len(zones)))

	return e.getZoneAnalytics(ctx, zones)
}

func (e *exporter) getZoneAnalytics(ctx context.Context, zones map[string]string) error {
	req := graphql.NewRequest(`
query ($zones: [string!], $start_time: Time) {
  viewer {
    zones(filter: {zoneTag_in: $zones}) {
      zoneTag

			# Assume we don't have >10k countries, and won't need to paginate.
      httpRequests1mGroups(limit: 10000, filter: {datetime_gt: $start_time}) {
        sum {
          countryMap {
            clientCountryName
            requests
            threats
          }
        }
      }
    }
  }
}
	`)

	req.Var("zones", keys(zones))
	req.Var("start_time", time.Now().Add(-e.scrapeInterval))
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)

	gqlClient := graphql.NewClient(e.analyticsAPIBaseURL)
	var gqlResp httpRequestsResp
	if err := gqlClient.Run(ctx, req, &gqlResp); err != nil {
		return err
	}

	for _, zone := range gqlResp.Viewer.Zones {
		for _, reqGroup := range zone.ReqGroups {
			for _, country := range reqGroup.Sum.CountryMap {
				httpRequests.WithLabelValues(zones[zone.ZoneTag], country.ClientCountryName).
					Add(float64(country.Requests))
				httpThreats.WithLabelValues(zones[zone.ZoneTag], country.ClientCountryName).
					Add(float64(country.Threats))
			}
		}
	}

	return nil
}

func (e *exporter) getZones(ctx context.Context) (map[string]string, error) {
	// TODO handle >50 zones (the API maximum per page) by requesting successive
	// pages. For now, we don't anticipate having >50 zones any time soon.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.apiBaseURL+"/zones?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("expected status 200, got %d", resp.StatusCode)
		return nil, err
	}

	defer resp.Body.Close()
	zones, err := parseZoneIDs(resp.Body)
	if err != nil {
		return nil, err
	}
	return zones, nil
}

func keys(dict map[string]string) []string {
	var keys []string
	for key := range dict {
		keys = append(keys, key)
	}
	return keys
}
