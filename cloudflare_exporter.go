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

const (
	namespace   = "cloudflare"
	apiMaxLimit = 10000
)

var (
	// arguments
	listenAddress = kingpin.Flag("listen-address", "Metrics exporter listen address.").
			Short('l').Envar("CLOUDFLARE_EXPORTER_LISTEN_ADDRESS").Default(":9199").String()
	cfEmail = kingpin.Flag("cloudflare-api-email", "email address for analytics API authentication.").
		Envar("CLOUDFLARE_API_EMAIL").Required().String()
	cfAPIKey = kingpin.Flag("cloudflare-api-key", "API key for analytics API authentication.").
			Envar("CLOUDFLARE_API_KEY").Required().String()
	cfAPIBaseURL = kingpin.Flag("cloudflare-api-base-url", "Cloudflare regular (non-analytics) API base URL").
			Envar("CLOUDFLARE_API_BASE_URL").Default("https://api.cloudflare.com/client/v4").String()
	cfAnalyticsAPIBaseURL = kingpin.Flag("cloudflare-analytics-api-base-url", "Cloudflare analytics (graphql) API base URL").
				Envar("CLOUDFLARE_ANALYTICS_API_BASE_URL").Default("https://api.cloudflare.com/client/v4/graphql").String()
	cfScrapeIntervalSeconds = kingpin.Flag("cloudflare-scrape-interval-seconds", "Interval at which to retrieve metrics from Cloudflare, separate from being scraped by prometheus").
				Envar("CLOUDFLARE_SCRAPE_INTERVAL_SECONDS").Default("300").Int()
	scrapeTimeoutSeconds = kingpin.Flag("scrape-timeout-seconds", "scrape timeout seconds").
				Envar("CLOUDFLARE_EXPORTER_SCRAPE_TIMEOUT_SECONDS").Default("30").Int()
	logLevel                 = kingpin.Flag("log-level", "log level").Envar("CLOUDFLARE_EXPORTER_LOG_LEVEL").Default("info").String()
	initialScrapeImmediately = kingpin.Flag("initial-scrape-immediately", "Scrape Cloudflare immediately at startup, or wait scrape-timeout-seconds. For development only.").
					Hidden().Envar("CLOUDFLARE_EXPORTER_INITIAL_SCRAPE_IMMEDIATELY").Default("false").Bool()
)

func main() {
	kingpin.Parse()

	logger := newPromLogger(*logLevel)
	logger.Log("msg", "starting cloudflare_exporter")

	cfExporter := &exporter{
		email: *cfEmail, apiKey: *cfAPIKey, apiBaseURL: *cfAPIBaseURL,
		graphqlClient:  graphql.NewClient(*cfAnalyticsAPIBaseURL),
		scrapeTimeout:  time.Duration(*scrapeTimeoutSeconds) * time.Second,
		scrapeInterval: time.Duration(*cfScrapeIntervalSeconds) * time.Second,
		logger:         logger,
		scrapeLock:     &sync.Mutex{},
		lastSeenBucketTimes: &lastUpdatedTimes{
			httpReqsByZone:          map[string]time.Time{},
			firewallEventsByZone:    map[string]time.Time{},
			healthCheckEventsByZone: map[string]time.Time{},
		},
	}

	prometheus.MustRegister(version.NewCollector("cloudflare_exporter"))
	registerMetrics(nil)

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
	graphqlClient  graphqlClient
	scrapeInterval time.Duration
	scrapeTimeout  time.Duration
	logger         log.Logger

	scrapeLock          *sync.Mutex
	lastSeenBucketTimes *lastUpdatedTimes
}

type lastUpdatedTimes struct {
	httpReqsByZone          map[string]time.Time
	firewallEventsByZone    map[string]time.Time
	healthCheckEventsByZone map[string]time.Time
}

type graphqlClient interface {
	Run(context.Context, *graphql.Request, interface{}) error
}

func (e *exporter) scrapeCloudflare(ctx context.Context) error {
	if err := e.initializeVectors(ctx); err != nil {
		return err
	}

	if *initialScrapeImmediately {
		// Initial scrape, the ticker below won't fire straight away.
		// Risks double counting on restart. Only useful for development.
		if err := e.scrapeCloudflareOnce(ctx); err != nil {
			level.Error(e.logger).Log("error", err)
			cfScrapeErrs.Inc()
		}
	}
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

	logger.Log("msg", "starting")
	cfScrapes.Inc()

	ctx, cancel := context.WithTimeout(ctx, e.scrapeTimeout)
	defer cancel()

	duration, err := timeOperation(func() error {
		var zones map[string]string
		zones, err := e.getZones(ctx)
		if err != nil {
			return err
		}
		zonesActive.Set(float64(len(zones)))

		return e.getZoneAnalytics(ctx, zones)
	})
	if err != nil {
		return err
	}

	cfLastSuccessTimestampSeconds.Set(float64(time.Now().Unix()))

	logger.Log("msg", "finished", "duration", duration)
	return nil
}

func (e *exporter) initializeVectors(ctx context.Context) error {
	e.logger.Log("event", "collecting_initial_country_list", "msg", "starting")

	var initialZones map[string]string
	var initialCountries map[string]struct{}
	duration, err := timeOperation(func() error {
		var err error
		initialZones, err = e.getZones(ctx)
		if err != nil {
			return err
		}
		initialCountries, err = e.getInitialCountries(ctx, initialZones)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, zone := range initialZones {
		for country := range initialCountries {
			httpCountryRequests.WithLabelValues(zone, country)
			httpCountryThreats.WithLabelValues(zone, country)
			httpCountryBytes.WithLabelValues(zone, country)
		}
	}

	e.logger.Log("event", "collecting_initial_country_list", "msg", "finished", "duration", duration)
	return nil
}

func (e *exporter) getInitialCountries(ctx context.Context, zones map[string]string) (map[string]struct{}, error) {
	initialCountriesGqlReq.Var("zones", keys(zones))
	initialCountriesGqlReq.Var("start_time", time.Now().Add(-12*time.Hour))

	var gqlResp cloudflareResp
	if err := e.makeGraphqlRequest(ctx, initialCountriesGqlReq, &gqlResp); err != nil {
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
	if err := e.getZoneAnalyticsKind(
		ctx, zones, e.lastSeenBucketTimes.httpReqsByZone, httpReqsGqlReq,
		extractZoneHTTPRequests, "scrape_http_requests",
	); err != nil {
		return err
	}
	if err := e.getZoneAnalyticsKind(
		ctx, zones, e.lastSeenBucketTimes.firewallEventsByZone, firewallEventsGqlReq,
		extractZoneFirewallEvents, "scrape_firewall_events",
	); err != nil {
		return err
	}
	if err := e.getZoneAnalyticsKind(
		ctx, zones, e.lastSeenBucketTimes.healthCheckEventsByZone, healthCheckEventsGqlReq,
		extractZoneHealthCheckEvents, "scrape_health_check_events",
	); err != nil {
		return err
	}
	return nil
}

func (e *exporter) getZoneAnalyticsKind(
	ctx context.Context, zones map[string]string, lastSeenBucketTimes map[string]time.Time,
	req *graphql.Request, extract extractFunc, event string,
) error {
	for zoneID, zoneName := range zones {
		debugLogger := level.Debug(log.With(e.logger, "zone", zoneName, "event", event))
		for {
			lastDateTimeCounted := lastSeenBucketTimes[zoneID]
			if lastDateTimeCounted == (time.Time{}) {
				lastDateTimeCounted = time.Now().Add(-e.scrapeInterval)
			}
			debugLogger.Log("msg", "starting", "last_datetime_bucket", lastDateTimeCounted.String())
			req.Var("zone", zoneID)
			// Add some grace time so that adjacent polling loops overlap in query
			// range, to avoid missing metrics. When we come to extract the zone data,
			// we exclude time buckets that occur before the lastDateTimeCounted,
			// avoiding double counting.
			req.Var("start_time", lastDateTimeCounted.Add(-5*time.Minute))
			var gqlResp cloudflareResp
			if err := e.makeGraphqlRequest(ctx, req, &gqlResp); err != nil {
				return err
			}

			if len(gqlResp.Viewer.Zones) != 1 {
				// The response length should only be zero if the zone has disappeared
				// since querying for them in this polling loop, and should never be >=2.
				return fmt.Errorf("expected 1 zone (%s), got %d", zoneName, len(gqlResp.Viewer.Zones))
			}
			zone := gqlResp.Viewer.Zones[0]
			results, lastDateTimeCounted, err := extract(zone, zones, lastDateTimeCounted)
			if err != nil {
				return err
			}
			lastSeenBucketTimes[zone.ZoneTag] = lastDateTimeCounted
			if results == 0 {
				// If we get no results, and therefore have no bucket time to record,
				// it's possible the query window will eventually exceed the API maximum
				// of 15 hours. Move it forward by one scrape interval to prevent this.
				lastSeenBucketTimes[zone.ZoneTag] = lastDateTimeCounted.Add(e.scrapeInterval)
			}
			debugLogger.Log("msg", "finished", "last_datetime_bucket", lastSeenBucketTimes[zone.ZoneTag].String())

			if results < apiMaxLimit {
				break
			}
		}
	}
	return nil
}

func (e *exporter) makeGraphqlRequest(ctx context.Context, req *graphql.Request, resp interface{}) error {
	req.Header.Set("X-AUTH-EMAIL", e.email)
	req.Header.Set("X-AUTH-KEY", e.apiKey)
	req.Var("limit", apiMaxLimit)
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

func newPromLogger(logLevel string) log.Logger {
	loggerLogLevel := &promlog.AllowedLevel{}
	if err := loggerLogLevel.Set(logLevel); err != nil {
		panic(err)
	}
	logConf := &promlog.Config{Level: loggerLogLevel, Format: &promlog.AllowedFormat{}}
	return level.Info(promlog.New(logConf))
}
