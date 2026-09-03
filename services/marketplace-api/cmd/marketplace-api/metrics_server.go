package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// The metrics endpoint runs on its own listener, separate from the API
// server, because cfg.MetricsPort differs from cfg.HTTPPort and the pods are
// annotated prometheus.io/port: "9090". Serving it off the API engine would
// also expose the whole registry through the public ingress.
//
// Until #624 nothing read cfg.MetricsPort at all: every collector the service
// registers — HTTP request counters, billing and prune counters, the MRR
// rollup collectors, carriersecrets_events_total — incremented in-process and
// was never scrapable, while the pod annotations pointed at a port with no
// listener.

// metricsMux returns the handler serving the default Prometheus registry.
// It is separate from newMetricsServer so the routing is testable without
// binding a port.
func metricsMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// newMetricsServer builds the Prometheus scrape server for the given port.
func newMetricsServer(port int) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           metricsMux(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
