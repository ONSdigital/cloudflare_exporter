# Cloudflare exporter

A Prometheus exporter for Cloudflare metrics. Consumes metrics from the [graphql
analytics API](https://developers.cloudflare.com/analytics/graphql-api/).

## Usage

Download a release or build the binary. Run `cloudflare_exporter --help`.
Alternatively, read the `kingpin` command line flags / env vars in
[cloudflare_exporter.go](cloudflare_exporter.go).

Example kubernetes deployment configuration
[here](https://gitlab.com/gitlab-com/gl-infra/k8s-workloads/gitlab-helmfiles/-/blob/master/releases/cloudflare-exporter/values.yaml.gotmpl).

Note that as per [the Cloudflare
docs](https://developers.cloudflare.com/analytics/graphql-api/getting-started/#setting-up-authentication-in-GraphiQL),
you'll need to use an account's API key, not an API token.

## What does this do?

The Cloudflare analytics API exposes [several data
sets](https://developers.cloudflare.com/analytics/graphql-api/features/data-sets/),
not all of which the exporter supports yet. In general, these data sets must be
queried over a finite (and recent) time window. Results usually consist of
counts in time buckets, and are partitioned by some dimensions that differ from
dataset to dataset. The exporter keeps track of these counters in memory,
incrementing them with values from newer time buckets it obtains by scraping
Cloudflare periodically, and exposes them to Prometheus.

Some Cloudflare analytics datasets can be partitioned by many dimensions. We map
these dimensions onto Prometheus labels. If we were to map every possible
dimension, then some of the resultant labels would have very high cardinality,
which is known to cause [performance issues with
Prometheus](https://prometheus.io/docs/practices/naming/#labels).  An example of
such a dimension would be `clientIP` in `firewallEventsAdaptiveGroups`.
Currently, the exporter is not flexible with regards to what it exposes as
labels, and decisions about what cardinality is too high are based on the GitLab
infrastructure team's use case. It's not impossible that this will change in the
future.

## Contributing

Feel free to open an issue and/or a merge request. Please check the list of
existing issues and MRs first.
