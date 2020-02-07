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

func extractZoneHTTPRequests(zoneCounts []httpRequests1mGroupsResp, lastDateTimeCounted time.Time) (map[string]*countryRequestData, time.Time, error) {
	countries := map[string]*countryRequestData{}
	var bucketTime time.Time
	for _, timeBucket := range zoneCounts {
		var err error
		bucketTime, err = time.Parse(time.RFC3339, timeBucket.Dimensions.Datetime)
		if err != nil {
			return nil, time.Time{}, err
		}

		if bucketTime.After(lastDateTimeCounted) {
			for _, countryData := range timeBucket.Sum.CountryMap {
				if _, ok := countries[countryData.ClientCountryName]; !ok {
					countries[countryData.ClientCountryName] = &countryRequestData{}
				}
				countries[countryData.ClientCountryName].requests += countryData.Requests
				countries[countryData.ClientCountryName].threats += countryData.Threats
				countries[countryData.ClientCountryName].bytes += countryData.Bytes
			}
		}
	}
	return countries, bucketTime, nil
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
	} `json:"sum"`
}

type zonesResp struct {
	Result []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"result"`
}

type countryRequestData struct {
	requests uint64
	threats  uint64
	bytes    uint64
}
