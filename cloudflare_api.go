package main

import (
	"encoding/json"
	"io"
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

type httpRequestsResp struct {
	Viewer struct {
		Zones []struct {
			ReqGroups []struct {
				Sum struct {
					CountryMap []struct {
						ClientCountryName string `json:"clientCountryName"`
						Requests          uint64 `json:"requests"`
						Threats           uint64 `json:"threats"`
						Bytes             uint64 `json:"bytes"`
					} `json:"countryMap"`
				} `json:"sum"`
			} `json:"httpRequests1mGroups"`
			ZoneTag string `json:"zoneTag"`
		} `json:"zones"`
	} `json:"viewer"`
}

type zonesResp struct {
	Result []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"result"`
}
