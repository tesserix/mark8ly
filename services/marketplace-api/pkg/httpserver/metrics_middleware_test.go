package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/mode"
)

// internal/middleware.Prometheus() existed and populated HTTPRequestsTotal /
// HTTPRequestDuration, but was installed on NO router — so the service
// exported no request, latency or error metrics at all, and
// http_requests_total had zero series in production (mark8ly#624).
//
// It belongs here rather than in main.go's two mode branches: this is where
// every engine already gets its baseline middleware, it runs before any
// route is registered (gin's Use is not retroactive — the same trap that
// silently dropped customer ids in storefront/routes.go), and a future
// engine inherits it instead of forgetting it.

func serveOnce(t *testing.T, r *gin.Engine, method, route, url string) {
	t.Helper()
	r.Handle(method, route, func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("request returned %d, want 200", rec.Code)
	}
}

func TestNew_EnginesRecordRequestMetrics(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Admin, log)

	const route = "/metrics-mw-probe/:id"
	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, route, "200"))

	serveOnce(t, e.Admin, http.MethodGet, route, "/metrics-mw-probe/abc")

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, route, "200"))
	if after != before+1 {
		t.Errorf("http_requests_total = %v, want %v — Prometheus middleware is not installed on the engine", after, before+1)
	}
}

func TestMergedForBoth_RecordsRequestMetrics(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := MergedForBoth("test", log)

	const route = "/metrics-mw-merged-probe"
	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, route, "200"))

	serveOnce(t, r, http.MethodGet, route, route)

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, route, "200"))
	if after != before+1 {
		t.Errorf("http_requests_total = %v, want %v — Prometheus middleware is not installed on the merged engine", after, before+1)
	}
}

// The path label must be the ROUTE PATTERN, never the raw URL. Labelling by
// raw path would give every product handle and order id its own time series
// and blow up cardinality — the standard way this middleware goes wrong.
func TestEngine_MetricsPathLabelIsRoutePatternNotRawURL(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Storefront, log)

	const route = "/metrics-mw-cardinality/:handle"
	serveOnce(t, e.Storefront, http.MethodGet, route, "/metrics-mw-cardinality/some-unique-handle")

	if got := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, route, "200")); got < 1 {
		t.Errorf("no series for the route pattern %q; got %v", route, got)
	}
	raw := "/metrics-mw-cardinality/some-unique-handle"
	if got := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues(http.MethodGet, raw, "200")); got != 0 {
		t.Errorf("a series exists for the RAW path %q (%v) — path label must be the route pattern", raw, got)
	}
}
