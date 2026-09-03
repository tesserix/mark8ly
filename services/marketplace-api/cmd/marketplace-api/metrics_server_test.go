package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// The service registers dozens of Prometheus collectors but, until #624,
// served none of them: MetricsPort was declared in config and read by
// nothing, so the prometheus.io/* pod annotations pointed at a port with no
// listener. These tests pin that the handler exists and actually exposes the
// default registry.

func TestMetricsMux_ServesDefaultRegistry(t *testing.T) {
	// Touch a counter so the registry has at least one of our own series to
	// find, rather than only the Go runtime collectors.
	metrics.CarrierSecretCounter("gsm_fallback_read", 1)

	rec := httptest.NewRecorder()
	metricsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "# HELP") {
		t.Error("response has no # HELP lines; it is not a Prometheus exposition")
	}
	// The specific series that #608 added and #621 depends on being readable.
	if !strings.Contains(body, "carriersecrets_events_total") {
		t.Error("carriersecrets_events_total missing from /metrics; #621 cannot gather its evidence")
	}
}

func TestMetricsMux_UnknownPathIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	metricsMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("GET / on the metrics mux = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNewMetricsServer_UsesConfiguredPort(t *testing.T) {
	srv := newMetricsServer(9090)
	if srv == nil {
		t.Fatal("newMetricsServer returned nil")
	}
	if srv.Addr != ":9090" {
		t.Errorf("Addr = %q, want %q", srv.Addr, ":9090")
	}
	if srv.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout is 0; the listener is exposed to slowloris")
	}
}
