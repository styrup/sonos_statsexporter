package main

import (
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}
func main() {
	//This section will start the HTTP server and expose
	//any metrics on the /metrics endpoint.
	port := getEnv("PORT", "8080")
	http.Handle("/metrics", promhttp.Handler())
	log.Info("Beginning to serve on port :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
