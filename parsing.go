package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

func parseZoneIDs(apiRespBody io.Reader) (map[string]string, error) {
	var zoneList zonesResp
	if err := json.NewDecoder(apiRespBody).Decode(&zoneList); err != nil {
		return nil, err
	}
	zones := map[string]string{}
	for _, zone := range zoneList.Result {
		if zone.Status != "pending" {
			zones[zone.ID] = zone.Name
		}
	}
	return zones, nil
}

func extractZoneHTTPRequests(zone zoneResp, zoneNames map[string]string, lastDateTimeCounted time.Time) (time.Time, error) {
	for _, timeBucket := range zone.ReqGroups {
		bucketTime, err := time.Parse(time.RFC3339, timeBucket.Dimensions.Datetime)
		if err != nil {
			return time.Time{}, err
		}

		if bucketTime.After(lastDateTimeCounted) {
			lastDateTimeCounted = bucketTime
			for _, countryData := range timeBucket.Sum.CountryMap {
				httpRequests.WithLabelValues(zoneNames[zone.ZoneTag], countryData.ClientCountryName, "", "", "").
					Add(float64(countryData.Requests))
				httpThreats.WithLabelValues(zoneNames[zone.ZoneTag], countryData.ClientCountryName).
					Add(float64(countryData.Threats))
				httpBytes.WithLabelValues(zoneNames[zone.ZoneTag], countryData.ClientCountryName).
					Add(float64(countryData.Bytes))
			}

			httpCachedRequests.WithLabelValues(zoneNames[zone.ZoneTag]).Add(float64(timeBucket.Sum.CachedRequests))
			httpCachedBytes.WithLabelValues(zoneNames[zone.ZoneTag]).Add(float64(timeBucket.Sum.CachedBytes))

			for _, httpVersionData := range timeBucket.Sum.ClientHTTPVersionMap {
				httpRequests.WithLabelValues(zoneNames[zone.ZoneTag], "", httpVersionData.ClientHTTPProtocol, "", "").
					Add(float64(httpVersionData.Requests))
			}

			for _, responseStatusData := range timeBucket.Sum.ResponseStatusMap {
				httpRequests.WithLabelValues(zoneNames[zone.ZoneTag], "", "", fmt.Sprintf("%d", responseStatusData.EdgeResponseStatus), "").
					Add(float64(responseStatusData.Requests))
			}

			for _, threatPathData := range timeBucket.Sum.ThreatPathingMap {
				httpRequests.WithLabelValues(zoneNames[zone.ZoneTag], "", "", "", threatPathData.ThreatPathingName).
					Add(float64(threatPathData.Requests))
			}
		}
	}
	return lastDateTimeCounted, nil
}

type httpRequestsResp struct {
	Viewer struct {
		Zones []zoneResp `json:"zones"`
	} `json:"viewer"`
}

type zoneResp struct {
	ReqGroups []struct {
		Dimensions struct {
			Datetime string `json:"datetime"`
		} `json:"dimensions"`
		Sum struct {
			CountryMap []struct {
				ClientCountryName string `json:"clientCountryName"`
				Requests          uint64 `json:"requests"`
				Threats           uint64 `json:"threats"`
				Bytes             uint64 `json:"bytes"`
			} `json:"countryMap"`
			CachedBytes          uint64 `json:"cachedBytes"`
			CachedRequests       uint64 `json:"cachedRequests"`
			ClientHTTPVersionMap []struct {
				ClientHTTPProtocol string `json:"clientHTTPProtocol"`
				Requests           uint64 `json:"requests"`
			} `json:"clientHTTPVersionMap"`
			ResponseStatusMap []struct {
				EdgeResponseStatus int    `json:"edgeResponseStatus"`
				Requests           uint64 `json:"requests"`
			} `json:"responseStatusMap"`
			ThreatPathingMap []struct {
				ThreatPathingName string `json:"threatPathingName"`
				Requests          uint64 `json:"requests"`
			} `json:"threatPathingMap"`
		} `json:"sum"`
	} `json:"httpRequests1mGroups"`
	ZoneTag string `json:"zoneTag"`
}

type zonesResp struct {
	Result []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"result"`
}
