package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

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

func TestExtractZoneHTTPRequests(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		lastDateTimeCounted string
		expectedData        map[string]*countryRequestData
	}{
		{
			name:                "sums data by country for buckets later than specified time",
			lastDateTimeCounted: "2020-02-06T10:01:00Z",
			expectedData: map[string]*countryRequestData{
				"CZ": &countryRequestData{requests: 3, threats: 0, bytes: 400},
				"DE": &countryRequestData{requests: 3, threats: 1, bytes: 400},
				"GB": &countryRequestData{requests: 24, threats: 0, bytes: 200},
			},
		},
		{
			name:                "sums data by country for all buckets when the specified time is before them all",
			lastDateTimeCounted: "1970-01-01T00:00:00Z",
			expectedData: map[string]*countryRequestData{
				"CZ": &countryRequestData{requests: 4, threats: 0, bytes: 500},
				"DE": &countryRequestData{requests: 27, threats: 1, bytes: 600},
				"GB": &countryRequestData{requests: 24, threats: 0, bytes: 200},
			},
		},
	} {
		t.Run(testCase.name, (func(t *testing.T) {
			testDataFile, err := os.Open("testdata/http_reqs_by_country_resp.json")
			require.Nil(t, err)
			defer testDataFile.Close()

			var gqlResp map[string]httpRequestsResp
			require.Nil(t, json.NewDecoder(testDataFile).Decode(&gqlResp))

			lastDateTimeCounted, err := time.Parse(time.RFC3339, testCase.lastDateTimeCounted)
			require.Nil(t, err)

			countries, newLastDateTime, err := extractZoneHTTPRequests(gqlResp["data"].Viewer.Zones[0].ReqGroups, lastDateTimeCounted)
			require.Nil(t, err)
			assert.Equal(t, countries, testCase.expectedData)
			assert.Equal(t, newLastDateTime, time.Date(2020, time.February, 6, 10, 3, 0, 0, time.UTC))
		}))
	}
}

func TestExtractZoneHTTPRequests_ReturnsUnmodifiedLastDateTimeCountedWhenNoDataReturned(t *testing.T) {
	testDataFile, err := os.Open("testdata/empty_http_reqs_by_country_resp.json")
	require.Nil(t, err)
	defer testDataFile.Close()

	var gqlResp map[string]httpRequestsResp
	require.Nil(t, json.NewDecoder(testDataFile).Decode(&gqlResp))

	lastDateTimeCounted := time.Now()

	_, newLastDateTime, err := extractZoneHTTPRequests(gqlResp["data"].Viewer.Zones[0].ReqGroups, lastDateTimeCounted)
	require.Nil(t, err)
	assert.Equal(t, newLastDateTime, lastDateTimeCounted)
}
