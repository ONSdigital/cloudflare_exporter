package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/machinebox/graphql"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
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
	logLevel = kingpin.Flag("log-level", "log level").Envar("CLOUDFLARE_EXPORTER_LOG_LEVEL").Default("info").String()
)

func main() {
	kingpin.Parse()

	loggerLogLevel := &promlog.AllowedLevel{}
	if err := loggerLogLevel.Set(*logLevel); err != nil {
		panic(err)
	}
	logConf := &promlog.Config{Level: loggerLogLevel, Format: &promlog.AllowedFormat{}}
	logger := level.Info(promlog.New(logConf))
	logger.Log("msg", "starting cloudflare_exporter")

	cfExporter := &exporter{
		email: *cfEmail, apiKey: *cfAPIKey, apiBaseURL: *cfAPIBaseURL,
		graphqlClient:  graphql.NewClient(*cfAnalyticsAPIBaseURL),
		scrapeTimeout:  time.Duration(*scrapeTimeoutSeconds) * time.Second,
		scrapeInterval: time.Duration(*cfScrapeIntervalSeconds) * time.Second,
		logger:         logger,
		scrapeLock:     &sync.Mutex{},
	}

	// TODO populate the build-time vars in
	// https://github.com/prometheus/common/blob/master/version/info.go with
	// goreleaser or something.
	prometheus.MustRegister(version.NewCollector("cloudflare_exporter"))

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())

	runGroup := run.Group{}

	logger.Log("msg", "listening", "addr", *listenAddress)
	serverSocket, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		level.Error(logger).Log("error", err)
		os.Exit(1)
	}
	runGroup.Add(func() error {
		return http.Serve(serverSocket, router)
	}, func(error) {
		logger.Log("msg", "closing server socket")
		serverSocket.Close()
	})

	cfScrapeCtx, cancelCfScrape := context.WithCancel(context.Background())
	runGroup.Add(func() error {
		logger.Log("msg", "starting Cloudflare scrape loop")
		return cfExporter.scrapeCloudflare(cfScrapeCtx)
	}, func(error) {
		logger.Log("msg", "ending Cloudflare scrape loop")
		cancelCfScrape()
	})

	if err := runGroup.Run(); err != nil {
		level.Error(logger).Log("error", err)
		os.Exit(1)
	}
}

type exporter struct {
	email          string
	apiKey         string
	apiBaseURL     string
	graphqlClient  *graphql.Client
	scrapeInterval time.Duration
	scrapeTimeout  time.Duration
	logger         log.Logger

	scrapeLock           *sync.Mutex
	lastDateTimesCounted struct {
		httpReqsByCountryByZone map[string]time.Time
	}
}

func (e *exporter) scrapeCloudflare(ctx context.Context) error {
	initialCountryListStart := time.Now()
	e.logger.Log("event", "collecting_initial_country_list", "msg", "starting")
	initialZones, err := e.getZones(ctx)
	if err != nil {
		return err
	}
	initialCountries, err := e.getInitialCountries(ctx, initialZones)
	if err != nil {
		return err
	}
	for _, zone := range initialZones {
		for country := range initialCountries {
			httpRequests.WithLabelValues(zone, country)
			httpThreats.WithLabelValues(zone, country)
			httpBytes.WithLabelValues(zone, country)
		}
	}
	initialCountryListDuration := float64(time.Since(initialCountryListStart)) / float64(time.Second)
	e.logger.Log("event", "collecting_initial_country_list", "msg", "finished", "duration", initialCountryListDuration)

	ticker := time.Tick(e.scrapeInterval)
	for {
		select {
		case <-ticker:
			if err := e.scrapeCloudflareOnce(ctx); err != nil {
				// Returning an error here would cause the exporter to crash. If it
				// crashloops but prometheus manages to scrape it in between crashes, we
				// might never notice that we are not updating our cached metrics.
				// Instead, we should alert on the exporter_cloudflare_scrape_errors
				// metric.
				level.Error(e.logger).Log("error", err)
				cfScrapeErrs.Inc()
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (e *exporter) scrapeCloudflareOnce(ctx context.Context) error {
	e.scrapeLock.Lock()
	defer e.scrapeLock.Unlock()

	logger := log.With(e.logger, "event", "scraping_cloudflare")
	start := time.Now()

	logger.Log("msg", "starting")
	cfScrapes.Inc()

	ctx, cancel := context.WithTimeout(ctx, e.scrapeTimeout)
	defer cancel()

	zones, err := e.getZones(ctx)
	if err != nil {
		return err
	}
	zonesActive.Set(float64(len(zones)))

	if err := e.getZoneAnalytics(ctx, zones); err != nil {
		return err
	}

	cfLastSuccessTimestampSeconds.Set(float64(time.Now().Unix()))

	duration := float64(time.Since(start)) / float64(time.Second)
	logger.Log("msg", "finished", "duration", duration)
	return nil
}

func (e *exporter) getInitialCountries(ctx context.Context, zones map[string]string) (map[string]struct{}, error) {
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
          }
        }
      }
    }
  }
}
	`)

	req.Var("zones", keys(zones))
	req.Var("start_time", time.Now().Add(-12*time.Hour))

	var gqlResp httpRequestsResp
	if err := e.makeGraphqlRequest(ctx, req, &gqlResp); err != nil {
		return nil, err
	}

	// Quick n dirty HashSet
	// Values will be unique within a zone, but we have a list of zones.
	countries := map[string]struct{}{}
	for _, zone := range gqlResp.Viewer.Zones {
		for _, reqGroup := range zone.ReqGroups {
			for _, country := range reqGroup.Sum.CountryMap {
				countries[country.ClientCountryName] = struct{}{}
			}
		}
	}
	return countries, nil
}

func (e *exporter) getZoneAnalytics(ctx context.Context, zones map[string]string) error {
	req := graphql.NewRequest(`
query ($zones: [string!], $start_time: Time) {
  viewer {
    zones(filter: {zoneTag_in: $zones}) {
      zoneTag

      httpRequests1mGroups(limit: 10000, filter: {datetime_gt: $start_time}, orderBy: [datetime_ASC]) {
        sum {
          countryMap {
            clientCountryName
            requests
            threats
            bytes
          }
        }
        dimensions {
          datetime
        }
      }
    }
  }
}
	`)

	// We add a few minutes to the scrape interval so that the first few buckets
	// returned are the same as the last few in the previous scrape. By keeping
	// track of the last bucket time counted, we can be sure not to double count
	// or miss buckets. However, on the first scrape, we don't know how long the
	// exporter has been down. Not using a grace time errs on the side of missing
	// metrics rather than double counting, but this is arbitrary.
	graceTime := time.Minute * 5
	if e.lastDateTimesCounted.httpReqsByCountryByZone == nil {
		e.logger.Log("msg", "first scrape, not using grace time")
		graceTime = 0
		e.lastDateTimesCounted.httpReqsByCountryByZone = map[string]time.Time{}
	}

	req.Var("zones", keys(zones))
	req.Var("start_time", time.Now().Add(-(e.scrapeInterval + graceTime)))

	var gqlResp httpRequestsResp
	if err := e.makeGraphqlRequest(ctx, req, &gqlResp); err != nil {
		return err
	}

	for _, zone := range gqlResp.Viewer.Zones {
		debugLogger := level.Debug(log.With(e.logger, "zone", zones[zone.ZoneTag], "action", "parse_zones"))
		debugLogger.Log("msg", "starting", "last_datetime_bucket", e.lastDateTimesCounted.httpReqsByCountryByZone[zone.ZoneTag])
		countries, lastDateTimeCounted, err := extractZoneHTTPRequests(zone.ReqGroups, e.lastDateTimesCounted.httpReqsByCountryByZone[zone.ZoneTag])
		if err != nil {
			return err
		}
		e.lastDateTimesCounted.httpReqsByCountryByZone[zone.ZoneTag] = lastDateTimeCounted
		debugLogger.Log("msg", "finished", "last_datetime_bucket", lastDateTimeCounted.String())
		for country, countryData := range countries {
			httpRequests.WithLabelValues(zones[zone.ZoneTag], country).
				Add(float64(countryData.requests))
			httpThreats.WithLabelValues(zones[zone.ZoneTag], country).
				Add(float64(countryData.threats))
			httpBytes.WithLabelValues(zones[zone.ZoneTag], country).
				Add(float64(countryData.bytes))
		}
	}

	return nil
}

func (e *exporter) makeGraphqlRequest(ctx context.Context, req *graphql.Request, resp interface{}) error {
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)
	return e.graphqlClient.Run(ctx, req, &resp)
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
