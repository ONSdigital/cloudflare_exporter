package main

import (
	"encoding/json"
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

func extractZoneHTTPRequests(zoneCounts []httpRequests1mGroupsResp, lastDateTimeCounted time.Time) (httpRequestsData, time.Time, error) {
	data := httpRequestsData{
		countries:        map[string]*countryRequestData{},
		httpVersions:     map[string]uint64{},
		responseStatuses: map[int]uint64{},
		threatPaths:      map[string]uint64{},
	}
	for _, timeBucket := range zoneCounts {
		bucketTime, err := time.Parse(time.RFC3339, timeBucket.Dimensions.Datetime)
		if err != nil {
			return httpRequestsData{}, time.Time{}, err
		}

		if bucketTime.After(lastDateTimeCounted) {
			lastDateTimeCounted = bucketTime
			for _, countryData := range timeBucket.Sum.CountryMap {
				if _, ok := data.countries[countryData.ClientCountryName]; !ok {
					data.countries[countryData.ClientCountryName] = &countryRequestData{}
				}
				data.countries[countryData.ClientCountryName].requests += countryData.Requests
				data.countries[countryData.ClientCountryName].threats += countryData.Threats
				data.countries[countryData.ClientCountryName].bytes += countryData.Bytes
			}

			data.cachedRequests += timeBucket.Sum.CachedRequests
			data.cachedBytes += timeBucket.Sum.CachedBytes

			for _, httpVersionData := range timeBucket.Sum.ClientHTTPVersionMap {
				if _, ok := data.httpVersions[httpVersionData.ClientHTTPProtocol]; !ok {
					data.httpVersions[httpVersionData.ClientHTTPProtocol] = 0
				}
				data.httpVersions[httpVersionData.ClientHTTPProtocol] += httpVersionData.Requests
			}

			for _, responseStatusData := range timeBucket.Sum.ResponseStatusMap {
				if _, ok := data.responseStatuses[responseStatusData.EdgeResponseStatus]; !ok {
					data.responseStatuses[responseStatusData.EdgeResponseStatus] = 0
				}
				data.responseStatuses[responseStatusData.EdgeResponseStatus] += responseStatusData.Requests
			}

			for _, threatPathData := range timeBucket.Sum.ThreatPathingMap {
				if _, ok := data.threatPaths[threatPathData.ThreatPathingName]; !ok {
					data.threatPaths[threatPathData.ThreatPathingName] = 0
				}
				data.threatPaths[threatPathData.ThreatPathingName] += threatPathData.Requests
			}
		}
	}
	return data, lastDateTimeCounted, nil
}

type httpRequestsResp struct {
	Viewer struct {
		Zones []struct {
			ReqGroups []httpRequests1mGroupsResp `json:"httpRequests1mGroups"`
			ZoneTag   string                     `json:"zoneTag"`
		} `json:"zones"`
	} `json:"viewer"`
}

type httpRequests1mGroupsResp struct {
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
}

type zonesResp struct {
	Result []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"result"`
}

type httpRequestsData struct {
	countries        map[string]*countryRequestData
	httpVersions     map[string]uint64
	responseStatuses map[int]uint64
	threatPaths      map[string]uint64
	cachedRequests   uint64
	cachedBytes      uint64
}

type countryRequestData struct {
	requests uint64
	threats  uint64
	bytes    uint64
}
