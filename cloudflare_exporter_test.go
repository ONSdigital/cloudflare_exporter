package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseZoneIDs_ReturnsMapOfNonPendingZones(t *testing.T) {
	f, err := os.Open("testdata/zones_resp.json")
	require.Nil(t, err)
	defer f.Close()
	zones, err := parseZoneIDs(f)
	require.Nil(t, err)
	assert.Equal(t, zones, map[string]string{"zone-1-id": "zone-1", "zone-2-id": "zone-2"})
}

func TestZoneAnalytics(t *testing.T) {
	for _, testCase := range []struct {
		name                       string
		metricsUnderTest           []string
		lastUpdatedTime            string
		apiRespFixturePaths        []string
		expectedMetricsFixturePath string
	}{
		{
			name: "sums HTTP request data by country for all buckets when the specified time is before them all",
			metricsUnderTest: []string{
				"cloudflare_zones_http_country_requests_total", "cloudflare_zones_http_country_threats_total",
				"cloudflare_zones_http_country_bytes_total",
			},
			lastUpdatedTime:            "1970-01-01T00:00:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_http_country_requests.metrics",
		},
		{
			name: "sums HTTP request data by country for buckets later than specified time",
			metricsUnderTest: []string{
				"cloudflare_zones_http_country_requests_total", "cloudflare_zones_http_country_threats_total",
				"cloudflare_zones_http_country_bytes_total",
			},
			lastUpdatedTime:            "2020-02-06T10:01:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_http_country_requests_later.metrics",
		},
		{
			name: "sums HTTP request cache data",
			metricsUnderTest: []string{
				"cloudflare_zones_http_cached_requests_total", "cloudflare_zones_http_cached_bytes_total",
			},
			lastUpdatedTime:            "1970-01-01T00:00:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_cache.metrics",
		},
		{
			name:                       "sums HTTP request data by protocol_version",
			metricsUnderTest:           []string{"cloudflare_zones_http_protocol_requests_total"},
			lastUpdatedTime:            "1970-01-01T00:00:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_http_protocol_requests.metrics",
		},
		{
			name:                       "sums HTTP request data by response status",
			metricsUnderTest:           []string{"cloudflare_zones_http_responses_total"},
			lastUpdatedTime:            "1970-01-01T00:00:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_http_responses.metrics",
		},
		{
			name:                       "sums HTTP request data by threat path",
			metricsUnderTest:           []string{"cloudflare_zones_http_threats_total"},
			lastUpdatedTime:            "1970-01-01T00:00:00Z",
			apiRespFixturePaths:        []string{"http_reqs_resp.json"},
			expectedMetricsFixturePath: "expected_http_threats.metrics",
		},
		{
			name:                       "sums firewall events for buckets later than specified time",
			metricsUnderTest:           []string{"cloudflare_zones_firewall_events_total"},
			lastUpdatedTime:            "2020-02-12T07:38:00Z",
			apiRespFixturePaths:        []string{"firewall_events_resp.json"},
			expectedMetricsFixturePath: "expected_firewall_events.metrics",
		},
		{
			name:                       "sums health check events for buckets later than specified time",
			metricsUnderTest:           []string{"cloudflare_zones_health_check_events_total"},
			lastUpdatedTime:            "2020-02-12T07:00:08Z",
			apiRespFixturePaths:        []string{"health_check_events_resp.json"},
			expectedMetricsFixturePath: "expected_health_check_events.metrics",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reg := prometheus.NewPedanticRegistry()
			registerMetrics(reg)

			lastUpdatedTime, err := time.Parse(time.RFC3339, testCase.lastUpdatedTime)
			require.Nil(t, err)

			cfExporter := exporter{
				logger:        newPromLogger("error"),
				scrapeLock:    &sync.Mutex{},
				graphqlClient: newFakeGraphqlClient(testCase.apiRespFixturePaths),
				lastSeenBucketTimes: &lastUpdatedTimes{
					httpReqsByZone:          map[string]time.Time{"a-zone": lastUpdatedTime},
					firewallEventsByZone:    map[string]time.Time{"a-zone": lastUpdatedTime},
					healthCheckEventsByZone: map[string]time.Time{"a-zone": lastUpdatedTime},
				},
			}
			zones := map[string]string{"a-zone": "a-zone-name"}
			require.Nil(t, cfExporter.getZoneAnalytics(context.Background(), zones))

			fixture, err := os.Open(filepath.Join("testdata", testCase.expectedMetricsFixturePath))
			require.Nil(t, err)
			defer fixture.Close()

			// This error is formatted much nicer using stdlib testing.
			err = testutil.GatherAndCompare(reg, fixture, testCase.metricsUnderTest...)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExtractZoneHTTPRequests_ReturnsUnmodifiedLastDateTimeCountedWhenNoDataReturned(t *testing.T) {
	testDataFile, err := os.Open("testdata/empty_http_reqs_resp.json")
	require.Nil(t, err)
	defer testDataFile.Close()

	var gqlResp map[string]cloudflareResp
	require.Nil(t, json.NewDecoder(testDataFile).Decode(&gqlResp))

	lastDateTimeCounted := time.Now().UTC()

	zones := map[string]string{"a-zone": "a-zone-name"}
	_, newLastDateTime, err := extractZoneHTTPRequests(gqlResp["data"].Viewer.Zones[0], zones, lastDateTimeCounted)
	require.Nil(t, err)
	assert.Equal(t, newLastDateTime, lastDateTimeCounted)
}
