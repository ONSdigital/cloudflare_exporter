package main

import (
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"gitlab.com/gitlab-org/cloudflare_exporter/cfexporter"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddress = kingpin.Flag("listen-address", "Metrics exporter listen address.").
			Short('l').Envar("CLOUDFLARE_EXPORTER_LISTEN_ADDRESS").Default(":11313").String()
	cfEmail = kingpin.Flag("cloudflare-api-email", "Email address for analytics API authentication.").
		Envar("CLOUDFLARE_API_EMAIL").Required().String()
	cfAPIKey = kingpin.Flag("cloudflare-api-key", "API key for analytics API authentication.").
			Envar("CLOUDFLARE_API_KEY").Required().String()
)

func main() {
	logger := log.New(os.Stdout, "[cloudflare-exporter] ", log.LstdFlags)
	logger.Println("starting")

	kingpin.Parse()

	exporter := cfexporter.NewExporter(*cfEmail, *cfAPIKey)
	prometheus.MustRegister(exporter)

	// TODO populate the build-time vars in
	// https://github.com/prometheus/common/blob/master/version/info.go with
	// goreleaser or something.
	prometheus.MustRegister(version.NewCollector("cloudflare_exporter"))

	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.Handler())

	logger.Printf("listening on %s\n", *listenAddress)
	logger.Fatal(http.ListenAndServe(*listenAddress, router))
}
