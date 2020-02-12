package main

import "github.com/machinebox/graphql"

var (
	httpReqsGqlReq = graphql.NewRequest(`
query ($zone: String!, $start_time: Time!, $limit: Int!) {
  viewer {
    zones(filter: {zoneTag: $zone}) {
      zoneTag

      httpRequests1mGroups(limit: $limit, filter: {datetime_gt: $start_time}, orderBy: [datetime_ASC]) {
        sum {
          countryMap {
            clientCountryName
            requests
            threats
            bytes
          }
          cachedRequests
          cachedBytes
          clientHTTPVersionMap{
            clientHTTPProtocol
            requests
          }
          responseStatusMap{
            edgeResponseStatus
            requests
          }
          threatPathingMap{
            requests
            threatPathingName
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

	initialCountriesGqlReq = graphql.NewRequest(`
query ($zones: [String!], $start_time: Time!, $limit: Int!) {
  viewer {
    zones(filter: {zoneTag_in: $zones}) {
      zoneTag

      httpRequests1mGroups(limit: $limit, filter: {datetime_gt: $start_time}) {
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
)
