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

type extractFunc func(zoneResp, map[string]string, time.Time) (int, time.Time, error)

func extractZoneHTTPRequests(zone zoneResp, zoneNames map[string]string, lastDateTimeCounted time.Time) (int, time.Time, error) {
	for _, timeBucket := range zone.ReqGroups {
		bucketTime, err := time.Parse(time.RFC3339, timeBucket.Dimensions.Datetime)
		if err != nil {
			return len(zone.ReqGroups), time.Time{}, err
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
	return len(zone.ReqGroups), lastDateTimeCounted, nil
}

func extractZoneFirewallEvents(zone zoneResp, zoneNames map[string]string, lastDateTimeCounted time.Time) (int, time.Time, error) {
	for _, firewallEventGroup := range zone.FirewallEventsAdaptiveGroups {
		eventTime, err := time.Parse(time.RFC3339, firewallEventGroup.Dimensions.Datetime)
		if err != nil {
			return len(zone.FirewallEventsAdaptiveGroups), time.Time{}, err
		}

		if eventTime.After(lastDateTimeCounted) {
			lastDateTimeCounted = eventTime
			firewallEvents.WithLabelValues(
				zoneNames[zone.ZoneTag], firewallEventGroup.Dimensions.Action,
				firewallEventGroup.Dimensions.Source, firewallEventGroup.Dimensions.RuleID,
			).Add(float64(firewallEventGroup.Count))
		}
	}
	return len(zone.FirewallEventsAdaptiveGroups), lastDateTimeCounted, nil
}

func extractZoneHealthCheckEvents(zone zoneResp, zoneNames map[string]string, lastDateTimeCounted time.Time) (int, time.Time, error) {
	for _, healthCheckEventsGroup := range zone.HealthCheckEventsGroups {
		eventTime, err := time.Parse(time.RFC3339, healthCheckEventsGroup.Dimensions.Datetime)
		if err != nil {
			return len(zone.HealthCheckEventsGroups), time.Time{}, err
		}

		if eventTime.After(lastDateTimeCounted) {
			lastDateTimeCounted = eventTime
			healthCheckEvents.WithLabelValues(
				zoneNames[zone.ZoneTag], healthCheckEventsGroup.Dimensions.FailureReason,
				healthCheckEventsGroup.Dimensions.HealthCheckName, healthCheckEventsGroup.Dimensions.HealthStatus,
				fmt.Sprintf("%d", healthCheckEventsGroup.Dimensions.OriginResponseStatus),
				healthCheckEventsGroup.Dimensions.Region, healthCheckEventsGroup.Dimensions.Scope,
			).Add(float64(healthCheckEventsGroup.Count))
		}
	}
	return len(zone.HealthCheckEventsGroups), lastDateTimeCounted, nil
}

type cloudflareResp struct {
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

	FirewallEventsAdaptiveGroups []struct {
		Count      uint64 `json:"count"`
		Dimensions struct {
			Action   string `json:"action"`
			Datetime string `json:"datetime"`
			RuleID   string `json:"ruleId"`
			Source   string `json:"source"`
		} `json:"dimensions"`
	} `json:"firewallEventsAdaptiveGroups"`

	HealthCheckEventsGroups []struct {
		Count      uint64 `json:"count"`
		Dimensions struct {
			Datetime             string `json:"datetime"`
			FailureReason        string `json:"failureReason"`
			HealthCheckName      string `json:"healthCheckName"`
			HealthStatus         string `json:"healthStatus"`
			OriginResponseStatus int    `json:"originResponseStatus"`
			Region               string `json:"region"`
			Scope                string `json:"scope"`
		} `json:"dimensions"`
	} `json:"healthCheckEventsGroups"`

	ZoneTag string `json:"zoneTag"`
}

type zonesResp struct {
	Result []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"result"`
}
