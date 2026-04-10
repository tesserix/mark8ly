# Production Readiness P1 — Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Prometheus metrics (HTTP + business + system), Sentry error tracking (Go + Next.js), and audit all logging for consistency.

**Architecture:** New `internal/metrics/` package with Prometheus registry. Gin middleware for auto HTTP metrics. Sentry SDK integration. Separate metrics port (9090).

**Tech Stack:** Go 1.26, prometheus/client_golang, sentry-go, @sentry/nextjs.

---

## Task 1: Prometheus registry and metrics definitions

**Context:** The marketplace-api currently exposes `/health` and `/ready` endpoints but has no Prometheus metrics. All metrics are defined in a single registry package and exported as package-level variables for use across the codebase.

**Files to create:**
- `services/marketplace-api/internal/metrics/registry.go`
- `services/marketplace-api/internal/metrics/registry_test.go`

### Implementation

File: `services/marketplace-api/internal/metrics/registry.go`
```go
// Package metrics provides Prometheus metric definitions for marketplace-api.
// All metrics are registered on a dedicated registry (not the global default)
// so the /metrics endpoint returns only marketplace-api metrics, not Go
// runtime metrics from other libraries.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Registry is the dedicated Prometheus registry for marketplace-api.
var Registry = prometheus.NewRegistry()

// ---------------------------------------------------------------------------
// HTTP request metrics (populated by the Prometheus middleware in
// internal/middleware/prometheus.go)
// ---------------------------------------------------------------------------

// HTTPRequestsTotal counts HTTP requests by method, path pattern, and status code.
var HTTPRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	},
	[]string{"method", "path", "status"},
)

// HTTPRequestDuration measures HTTP request duration in seconds.
var HTTPRequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"method", "path"},
)

// ---------------------------------------------------------------------------
// Business metrics (populated by handlers and services)
// ---------------------------------------------------------------------------

// OrdersCreatedTotal counts orders created, labeled by store_id.
var OrdersCreatedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "orders_created_total",
		Help: "Total number of orders created.",
	},
	[]string{"store_id"},
)

// CheckoutDuration measures end-to-end checkout duration in seconds.
var CheckoutDuration = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "checkout_duration_seconds",
		Help:    "Checkout flow duration in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	},
)

// PaymentIntentCreatedTotal counts payment intents by provider and outcome.
var PaymentIntentCreatedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "payment_intent_created_total",
		Help: "Total payment intents created.",
	},
	[]string{"provider", "status"},
)

// WebhookReceivedTotal counts webhook events by provider and event type.
var WebhookReceivedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "webhook_received_total",
		Help: "Total webhook events received.",
	},
	[]string{"provider", "event_type"},
)

// TaxCalculationFallbackTotal counts tax fallback calculations by provider.
var TaxCalculationFallbackTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "tax_calculation_fallback_total",
		Help: "Total tax calculation fallbacks to default rate.",
	},
	[]string{"provider"},
)

// ---------------------------------------------------------------------------
// System metrics (populated by infrastructure components)
// ---------------------------------------------------------------------------

// DBQueryDuration measures database query duration in seconds.
var DBQueryDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "db_query_duration_seconds",
		Help:    "Database query duration in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	},
	[]string{"operation"},
)

// OutboxEventsPending is a gauge of unprocessed outbox events.
var OutboxEventsPending = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "outbox_events_pending",
		Help: "Number of pending outbox events.",
	},
)

// OutboxEventsPublishedTotal counts outbox events successfully published.
var OutboxEventsPublishedTotal = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "outbox_events_published_total",
		Help: "Total outbox events published.",
	},
)

func init() {
	// HTTP
	Registry.MustRegister(HTTPRequestsTotal)
	Registry.MustRegister(HTTPRequestDuration)

	// Business
	Registry.MustRegister(OrdersCreatedTotal)
	Registry.MustRegister(CheckoutDuration)
	Registry.MustRegister(PaymentIntentCreatedTotal)
	Registry.MustRegister(WebhookReceivedTotal)
	Registry.MustRegister(TaxCalculationFallbackTotal)

	// System
	Registry.MustRegister(DBQueryDuration)
	Registry.MustRegister(OutboxEventsPending)
	Registry.MustRegister(OutboxEventsPublishedTotal)

	// Also register Go runtime collectors on our registry.
	Registry.MustRegister(prometheus.NewGoCollector())
	Registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
}
```

### Tests

File: `services/marketplace-api/internal/metrics/registry_test.go`
```go
package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

func TestRegistry_AllMetricsRegistered(t *testing.T) {
	// Gather all registered metrics — should not panic or error.
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Registry.Gather() error: %v", err)
	}

	// Expect at minimum the Go runtime + process metrics + our custom ones.
	// Custom metrics won't appear in Gather() until they have observations,
	// but the registry should not error on gather.
	if len(families) == 0 {
		t.Error("expected at least Go runtime metric families, got 0")
	}
}

func TestHTTPRequestsTotal_Increment(t *testing.T) {
	metrics.HTTPRequestsTotal.With(prometheus.Labels{
		"method": "GET",
		"path":   "/test",
		"status": "200",
	}).Inc()

	// Verify the counter was created without panic.
	// Detailed value assertion is done via the Gather() method if needed.
}

func TestOrdersCreatedTotal_Increment(t *testing.T) {
	metrics.OrdersCreatedTotal.With(prometheus.Labels{
		"store_id": "test-store-id",
	}).Inc()
}

func TestCheckoutDuration_Observe(t *testing.T) {
	metrics.CheckoutDuration.Observe(1.5)
}

func TestWebhookReceivedTotal_Increment(t *testing.T) {
	metrics.WebhookReceivedTotal.With(prometheus.Labels{
		"provider":   "stripe",
		"event_type": "payment_intent.succeeded",
	}).Inc()
}

func TestDBQueryDuration_Observe(t *testing.T) {
	metrics.DBQueryDuration.With(prometheus.Labels{
		"operation": "SELECT",
	}).Observe(0.005)
}

func TestOutboxEventsPending_Set(t *testing.T) {
	metrics.OutboxEventsPending.Set(42)
}
```

### Go module dependency

Add `prometheus/client_golang` to the go.mod:
```bash
cd services/marketplace-api
go get github.com/prometheus/client_golang@v1.19.1
go mod tidy
```

### TDD steps
1. **RED:** Write `registry_test.go`. Run `go test ./internal/metrics/...` — fails, package does not exist.
2. **GREEN:** Create `registry.go`. Run `go get github.com/prometheus/client_golang@v1.19.1`. Tests pass.
3. **IMPROVE:** Verify `go vet ./internal/metrics/...` produces no warnings.

---

## Task 2: HTTP metrics middleware

**Context:** The Gin HTTP framework supports middleware chains. This middleware auto-instruments all routes with `http_requests_total` and `http_request_duration_seconds` metrics. It uses the route pattern (not the actual URL) as the `path` label to avoid high-cardinality issues.

**Files to create:**
- `services/marketplace-api/internal/middleware/prometheus.go`
- `services/marketplace-api/internal/middleware/prometheus_test.go`

### Implementation

File: `services/marketplace-api/internal/middleware/prometheus.go`
```go
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// Prometheus returns a Gin middleware that records HTTP request metrics.
// It uses c.FullPath() for the path label to avoid high cardinality from
// path parameters (e.g., "/products/:id" not "/products/abc-123").
func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// c.FullPath() returns the registered route pattern, e.g. "/api/v1/products/:id".
		// Falls back to "unmatched" for 404s where no route matched.
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method
		duration := time.Since(start).Seconds()

		metrics.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}
```

### Tests

File: `services/marketplace-api/internal/middleware/prometheus_test.go`
```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/middleware"
)

func TestPrometheus_RecordsMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Prometheus())
	r.GET("/api/v1/products/:id", func(c *gin.Context) {
		c.JSON(200, gin.H{"id": c.Param("id")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/products/abc-123", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify the counter was incremented.
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, f := range families {
		if f.GetName() == "http_requests_total" {
			for _, m := range f.GetMetric() {
				labels := labelMap(m.GetLabel())
				if labels["method"] == "GET" &&
					labels["path"] == "/api/v1/products/:id" &&
					labels["status"] == "200" {
					if m.GetCounter().GetValue() >= 1 {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("http_requests_total metric not found for GET /api/v1/products/:id 200")
	}
}

func TestPrometheus_UnmatchedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Prometheus())
	// No routes registered — everything is a 404.

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/nonexistent", nil)
	r.ServeHTTP(w, req)

	families, _ := metrics.Registry.Gather()
	for _, f := range families {
		if f.GetName() == "http_requests_total" {
			for _, m := range f.GetMetric() {
				labels := labelMap(m.GetLabel())
				if labels["path"] == "unmatched" && labels["status"] == "404" {
					return // pass
				}
			}
		}
	}
	t.Error("expected unmatched path metric for 404")
}

func labelMap(labels []*dto.LabelPair) map[string]string {
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.GetName()] = l.GetValue()
	}
	return m
}
```

### Go module dependency for test

```bash
cd services/marketplace-api
go get github.com/prometheus/client_model@latest
go mod tidy
```

### Wire into main.go

In `cmd/marketplace-api/main.go`, when constructing BOTH Gin engines (admin + storefront), add the Prometheus middleware:
```go
engine := gin.New()
engine.Use(gin.Recovery())
engine.Use(middleware.SecurityHeaders()) // from P0
engine.Use(middleware.Prometheus())      // ADD THIS
// ... rest of route setup
```

### TDD steps
1. **RED:** Write `prometheus_test.go`. Run tests — fails, function does not exist.
2. **GREEN:** Create `prometheus.go`. Tests pass.
3. **IMPROVE:** Verify no high-cardinality path labels: route patterns use `:param` not actual values.

---

## Task 3: Business metrics emission points

**Context:** Business metrics counters/histograms are defined in Task 1. This task adds the actual `metrics.X.Inc()` / `metrics.X.Observe()` calls at the correct points in the handler and service code.

**Files to modify:**
- `services/marketplace-api/internal/handlers/storefront/checkout_ext.go`
- `services/marketplace-api/internal/handlers/storefront/webhooks.go`
- `services/marketplace-api/internal/order/service.go`
- `services/marketplace-api/internal/outbox/publisher.go`
- `services/marketplace-api/internal/tax/repository.go` (or tax service)

### Emission points

#### orders_created_total

In `order/service.go`, after a successful order creation (the function that inserts into the `orders` table):
```go
import "github.com/mark8ly/marketplace-api/internal/metrics"

// After successful order insert:
metrics.OrdersCreatedTotal.WithLabelValues(order.StoreID.String()).Inc()
```

Find the exact location by searching for the GORM `Create` call on orders:
```bash
grep -n "\.Create\(&order\|\.Create(&o\b" services/marketplace-api/internal/order/service.go
```

#### checkout_duration_seconds

In `storefront/checkout_ext.go`, wrap the entire `Checkout` handler:
```go
import (
	"time"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

func (h *CheckoutExtHandler) Checkout(c *gin.Context) {
	start := time.Now()
	defer func() {
		metrics.CheckoutDuration.Observe(time.Since(start).Seconds())
	}()
	// ... existing checkout logic ...
}
```

#### payment_intent_created_total

In `storefront/checkout_ext.go`, after calling `gateway.CreateIntent`:
```go
intent, err := gateway.CreateIntent(ctx, input)
if err != nil {
	metrics.PaymentIntentCreatedTotal.WithLabelValues(provider, "error").Inc()
	// ... existing error handling ...
	return
}
metrics.PaymentIntentCreatedTotal.WithLabelValues(provider, "success").Inc()
```

Find the exact location:
```bash
grep -n "CreateIntent" services/marketplace-api/internal/handlers/storefront/checkout_ext.go
```

#### webhook_received_total

In `storefront/webhooks.go`, after successful webhook verification:
```go
evt, err := gateway.VerifyWebhook(ctx, body, signature)
if err != nil {
	metrics.WebhookReceivedTotal.WithLabelValues(provider, "invalid").Inc()
	// ... existing error handling ...
	return
}
metrics.WebhookReceivedTotal.WithLabelValues(provider, evt.Type).Inc()
```

#### tax_calculation_fallback_total

In the tax calculation code, wherever the fallback rate is used:
```bash
grep -n "fallback\|default.*rate\|FallbackRate" services/marketplace-api/internal/tax/
```
Add at the fallback branch:
```go
metrics.TaxCalculationFallbackTotal.WithLabelValues(provider).Inc()
```

#### outbox_events_pending + outbox_events_published_total

In `outbox/publisher.go`, in the polling loop:
```go
// After counting pending events:
metrics.OutboxEventsPending.Set(float64(pendingCount))

// After successfully publishing a batch:
metrics.OutboxEventsPublishedTotal.Add(float64(publishedCount))
```

Find exact locations:
```bash
grep -n "pending\|published\|Publish" services/marketplace-api/internal/outbox/publisher.go
```

### TDD steps
1. **RED:** Write integration tests that trigger each business event and verify the corresponding metric incremented. Example for orders:
   ```go
   func TestOrderCreation_EmitsMetric(t *testing.T) {
       // Setup: create test DB, order service
       // Act: create an order
       // Assert: metrics.OrdersCreatedTotal gathered value >= 1
   }
   ```
2. **GREEN:** Add the emission points. Tests pass.
3. **IMPROVE:** Verify no metric emission in error paths that should not count (e.g., validation failures should not increment `orders_created_total`).

---

## Task 4: /metrics endpoint on port 9090

**Context:** Prometheus metrics must be served on a separate port (9090) so they are not exposed on the public API port (8087). The metrics server runs alongside the main HTTP server.

**Files to modify:**
- `services/marketplace-api/cmd/marketplace-api/main.go`
- `services/marketplace-api/pkg/config/config.go`

### Config addition

In `services/marketplace-api/pkg/config/config.go`, add:
```go
// MetricsPort is the port for the Prometheus /metrics endpoint.
// Served on a separate port to avoid exposing metrics on the public API.
MetricsPort int `envconfig:"METRICS_PORT" default:"9090"`
```

### Metrics server in main.go

Add after the main HTTP server construction, before the graceful shutdown block:
```go
import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

// Metrics server — separate port, not exposed publicly.
metricsMux := http.NewServeMux()
metricsMux.Handle("/metrics", promhttp.HandlerFor(
	metrics.Registry,
	promhttp.HandlerOpts{EnableOpenMetrics: true},
))
metricsSrv := &http.Server{
	Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
	Handler: metricsMux,
}

go func() {
	log.Info("metrics server starting", "port", cfg.MetricsPort)
	if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("metrics server error", "err", err)
	}
}()
```

In the graceful shutdown block, add metrics server shutdown:
```go
// Existing shutdown signal handling...
// After shutting down the main server:
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
defer shutdownCancel()

if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
	log.Error("metrics server shutdown error", "err", err)
}
```

### TDD steps
1. **RED:** Start the server locally. `curl localhost:9090/metrics` returns connection refused (port not yet open).
2. **GREEN:** Add the metrics server. Restart. `curl localhost:9090/metrics` returns Prometheus text format with `go_` prefixed runtime metrics.
3. **IMPROVE:** Verify business metrics appear after triggering them: `curl localhost:9090/metrics | grep http_requests_total` shows counters after making API requests.

### Verification command
```bash
# After starting the server:
curl -s localhost:9090/metrics | head -20
# Should show:
# # HELP go_gc_duration_seconds ...
# # TYPE go_gc_duration_seconds summary
# ...

# After making an API request:
curl -s localhost:8087/health
curl -s localhost:9090/metrics | grep http_requests_total
# Should show:
# http_requests_total{method="GET",path="/health",status="200"} 1
```

---

## Task 5: Sentry Go integration

**Context:** No error tracking beyond logs. Sentry provides alerting, grouping, and context enrichment for production errors.

**Files to create:**
- `services/marketplace-api/internal/sentry/init.go`

**Files to modify:**
- `services/marketplace-api/cmd/marketplace-api/main.go`
- `services/marketplace-api/pkg/config/config.go`

### Go module dependency

```bash
cd services/marketplace-api
go get github.com/getsentry/sentry-go@latest
go get github.com/getsentry/sentry-go/gin@latest
go mod tidy
```

### Config addition

In `services/marketplace-api/pkg/config/config.go`:
```go
// SentryDSN enables Sentry error tracking when non-empty.
// Empty disables Sentry — fine for local dev and tests.
SentryDSN string `envconfig:"SENTRY_DSN" default:""`
```

### Init package

File: `services/marketplace-api/internal/sentry/init.go`
```go
// Package sentry provides Sentry error tracking initialization for marketplace-api.
package sentry

import (
	"fmt"
	"log/slog"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
)

// Config holds Sentry initialization options.
type Config struct {
	DSN         string
	Environment string // "dev", "staging", "production"
	Release     string // git SHA or version tag
}

// Init initializes the Sentry SDK. Returns a cleanup function that must
// be deferred by the caller to flush buffered events on shutdown.
// If DSN is empty, Sentry is not initialized and the cleanup is a no-op.
func Init(cfg Config, logger *slog.Logger) (cleanup func()) {
	if cfg.DSN == "" {
		logger.Info("sentry: disabled (SENTRY_DSN is empty)")
		return func() {}
	}

	err := sentrygo.Init(sentrygo.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		EnableTracing:    false, // Use OpenTelemetry for tracing, not Sentry
		TracesSampleRate: 0,
		AttachStacktrace: true,
	})
	if err != nil {
		logger.Error("sentry: init failed", "err", err)
		return func() {}
	}

	logger.Info("sentry: initialized", "env", cfg.Environment)
	return func() {
		sentrygo.Flush(2 * time.Second)
	}
}

// CaptureError sends an error to Sentry with optional key-value context.
// Use for business-critical errors that need alerting beyond log monitoring.
func CaptureError(err error, tags map[string]string) {
	if err == nil {
		return
	}
	sentrygo.WithScope(func(scope *sentrygo.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentrygo.CaptureException(err)
	})
}

// CaptureMessage sends a message to Sentry for non-error events that
// still warrant alerting (e.g., "payment fallback triggered").
func CaptureMessage(msg string, tags map[string]string) {
	sentrygo.WithScope(func(scope *sentrygo.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentrygo.CaptureMessage(msg)
	})
}

// RecoveryMiddleware returns a string suitable for Gin's recovery logging.
// The sentrygin middleware handles this, but this helper is available for
// custom panic recovery flows.
func FormatPanic(v interface{}) string {
	return fmt.Sprintf("panic recovered: %v", v)
}
```

### Wire into main.go

Add after config load, before any other initialization:
```go
import (
	sentrygin "github.com/getsentry/sentry-go/gin"
	sentrypkg "github.com/mark8ly/marketplace-api/internal/sentry"
)

// Sentry initialization.
sentryCleanup := sentrypkg.Init(sentrypkg.Config{
	DSN:         cfg.SentryDSN,
	Environment: cfg.Env,
	Release:     "marketplace-api@" + marketplaceapi.Version, // or git SHA
}, log)
defer sentryCleanup()
```

Add Sentry Gin middleware to both engines (after `gin.Recovery()`):
```go
engine := gin.New()
engine.Use(gin.Recovery())
engine.Use(sentrygin.New(sentrygin.Options{
	Repanic: true, // re-panic after capturing so gin.Recovery() also handles it
}))
// ... rest of middleware
```

### TDD steps
1. **RED:** Write a test that calls `sentry.Init` with an empty DSN — verify it returns a no-op cleanup without panicking.
2. **GREEN:** Create `init.go`. Test passes.
3. **IMPROVE:** Write an integration test that initializes Sentry with a test DSN (or mock transport), triggers a panic in a Gin handler, and verifies Sentry captured the event. This requires a Sentry test transport:
   ```go
   // In test setup:
   transport := sentrygo.NewHTTPSyncTransport()
   // Or use sentry's test transport for capturing events without sending them.
   ```

### Where to add CaptureError calls

Add `sentry.CaptureError` at critical business error paths:
- Payment intent creation failure (after logging, before returning error to client)
- Webhook signature verification failure (potential attack)
- Order creation failure (data integrity risk)
- Encryption/decryption failure (security concern)

Example in `checkout_ext.go`:
```go
intent, err := gateway.CreateIntent(ctx, input)
if err != nil {
	sentrypkg.CaptureError(err, map[string]string{
		"provider": provider,
		"store_id": storeID,
	})
	// ... existing error handling ...
}
```

---

## Task 6: Sentry Next.js integration (admin + storefront)

**Context:** Both `apps/admin` and `apps/storefront` are Next.js 16 apps. Sentry's Next.js SDK provides automatic error capturing for both server and client.

### Install dependency

```bash
cd apps/storefront
npm install @sentry/nextjs
cd ../admin
npm install @sentry/nextjs
```

### Storefront Sentry config

**Files to create in `apps/storefront/`:**

File: `apps/storefront/sentry.client.config.ts`
```typescript
import * as Sentry from "@sentry/nextjs";

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
  environment: process.env.NEXT_PUBLIC_APP_ENV ?? "development",

  // Only enable in production to avoid noise in dev.
  enabled: process.env.NODE_ENV === "production",

  // Sample 10% of transactions for performance monitoring.
  tracesSampleRate: 0.1,

  // Capture 100% of errors.
  replaysOnErrorSampleRate: 1.0,
  replaysSessionSampleRate: 0,

  integrations: [Sentry.replayIntegration()],
});
```

File: `apps/storefront/sentry.server.config.ts`
```typescript
import * as Sentry from "@sentry/nextjs";

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
  environment: process.env.NEXT_PUBLIC_APP_ENV ?? "development",
  enabled: process.env.NODE_ENV === "production",
  tracesSampleRate: 0.1,
});
```

File: `apps/storefront/sentry.edge.config.ts`
```typescript
import * as Sentry from "@sentry/nextjs";

Sentry.init({
  dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
  environment: process.env.NEXT_PUBLIC_APP_ENV ?? "development",
  enabled: process.env.NODE_ENV === "production",
  tracesSampleRate: 0.1,
});
```

**Modify:** `apps/storefront/next.config.ts` — wrap with Sentry:
```typescript
import { withSentryConfig } from "@sentry/nextjs";

// ... existing nextConfig ...

export default withSentryConfig(nextConfig, {
  // Sentry webpack plugin options
  org: "tesserix",
  project: "mark8ly-storefront",
  silent: !process.env.CI,
  // Upload source maps only in CI.
  sourcemaps: {
    disable: !process.env.CI,
  },
});
```

**Create global error handler:** `apps/storefront/app/global-error.tsx`
```typescript
"use client";

import * as Sentry from "@sentry/nextjs";
import { useEffect } from "react";

interface GlobalErrorProps {
  error: Error & { digest?: string };
  reset: () => void;
}

export default function GlobalError({ error, reset }: GlobalErrorProps) {
  useEffect(() => {
    Sentry.captureException(error);
  }, [error]);

  return (
    <html lang="en">
      <body>
        <div style={{ padding: "2rem", textAlign: "center" }}>
          <h2>Something went wrong</h2>
          <button onClick={() => reset()} type="button">
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}
```

### Admin Sentry config

Repeat the same pattern for `apps/admin/`:

**Files to create:**
- `apps/admin/sentry.client.config.ts` (same as storefront, change project name)
- `apps/admin/sentry.server.config.ts` (same as storefront)
- `apps/admin/sentry.edge.config.ts` (same as storefront)
- `apps/admin/app/global-error.tsx` (same as storefront)

**Modify:** `apps/admin/next.config.ts` — wrap with `withSentryConfig`, project name `mark8ly-admin`.

### Environment variables

Add to `.env.local.example` for both apps:
```env
NEXT_PUBLIC_SENTRY_DSN=
NEXT_PUBLIC_APP_ENV=development
SENTRY_AUTH_TOKEN=  # CI only — for source map uploads
```

### TDD steps
1. **RED:** `npm run build` in `apps/storefront` — fails if `@sentry/nextjs` is not installed.
2. **GREEN:** Install the package and create config files. Build succeeds.
3. **IMPROVE:** Verify Sentry initializes without errors in dev mode: `npm run dev`, check console for Sentry init messages (should show "Sentry disabled in development" or similar).
4. **Test error capture:** Temporarily add `throw new Error("sentry test")` to a page component, load the page, verify the error appears in the Sentry dashboard (requires a real DSN for this step — skip in CI).

---

## Task 7: Structured logging audit (Go)

**Context:** The marketplace-api uses `log/slog` consistently (verified: `main.go` creates logger via `logger.New(cfg.Env)`, all handlers accept `*slog.Logger`). This task audits for any stray `fmt.Println`, `log.Println`, or inconsistent log usage.

**No new files.** This is an audit + fix task.

### Audit commands

Run these searches and fix any findings:

```bash
# 1. Find any fmt.Println in Go production code (exclude test files):
grep -rn "fmt\.Println\|fmt\.Printf" services/marketplace-api/ \
  --include="*.go" \
  | grep -v "_test.go" \
  | grep -v "vendor/"

# 2. Find any log.Println (standard library log, not slog):
grep -rn "\"log\"" services/marketplace-api/ \
  --include="*.go" \
  | grep -v "_test.go" \
  | grep -v "vendor/" \
  | grep -v "log/slog"

# 3. Find any bare log.Fatal/log.Panic outside of main.go:
grep -rn "log\.Fatal\|log\.Panic" services/marketplace-api/ \
  --include="*.go" \
  | grep -v "_test.go" \
  | grep -v "cmd/"

# 4. Find errors logged without context:
grep -rn 'logger\.Error("[^"]*")' services/marketplace-api/ \
  --include="*.go" \
  | grep -v "_test.go"
# These should have at least one structured key-value pair.

# 5. Find potential PII in info-level logs:
grep -rn 'slog\.String("email"\|slog\.String("customer_email"' services/marketplace-api/ \
  --include="*.go" \
  | grep -v "_test.go" \
  | grep -v "Debug"
# Email at info level is a PII concern — should be debug only.
```

### Fix patterns

For each finding:

**fmt.Println/Printf:** Replace with `logger.Info("message", "key", value)` or `logger.Debug(...)`.

**log.Println:** Replace with `slog.Info(...)` or inject the structured logger.

**Errors without context:** Add structured fields:
```go
// BEFORE:
logger.Error("failed to create order")

// AFTER:
logger.Error("failed to create order",
    "store_id", storeID,
    "err", err,
)
```

**PII at info level:** Downgrade to debug:
```go
// BEFORE:
logger.Info("customer lookup", "email", email)

// AFTER:
logger.Debug("customer lookup", "email", email)
```

### TDD steps
1. Run all 5 audit commands. Document findings.
2. Fix each finding.
3. Re-run audit commands — all return zero results.
4. Run `go build ./...` to verify no compilation errors from changes.
5. Run `go test ./...` to verify no test regressions.

---

## Task 8: Structured logging audit (TypeScript)

**Context:** Next.js apps should not have `console.log` in production code. The project CLAUDE.md says there is an existing hook for this, but we need to verify and audit.

**No new files.** This is an audit + fix task.

### Audit commands

```bash
# 1. Find console.log in production TypeScript/TSX (exclude test files):
grep -rn "console\.log" apps/storefront/app/ apps/storefront/components/ apps/storefront/lib/ \
  --include="*.ts" --include="*.tsx" \
  | grep -v ".test." \
  | grep -v "__tests__"

grep -rn "console\.log" apps/admin/app/ apps/admin/components/ apps/admin/lib/ \
  --include="*.ts" --include="*.tsx" \
  | grep -v ".test." \
  | grep -v "__tests__"

# 2. Find console.error that should use a logger:
grep -rn "console\.error" apps/storefront/ apps/admin/ \
  --include="*.ts" --include="*.tsx" \
  | grep -v "node_modules" \
  | grep -v ".test."

# 3. Check if a logger utility exists:
find apps/storefront/lib apps/admin/lib -name "logger*" 2>/dev/null
```

### Fix patterns

**console.log:** Remove entirely or replace with a logger utility if the information is genuinely useful:
```typescript
// BEFORE:
console.log("Products loaded:", products.length);

// AFTER — if debugging info is needed:
// Remove entirely. Use the logger only in server-side code.

// AFTER — if server-side and genuinely useful:
import { logger } from "@/lib/logger";
logger.info("products loaded", { count: products.length });
```

**console.error in API routes:** Replace with logger:
```typescript
// BEFORE:
console.error("API error:", error);

// AFTER:
import { logger } from "@/lib/logger";
logger.error("API error", { error: error instanceof Error ? error.message : String(error) });
```

### TDD steps
1. Run all audit commands. Document findings.
2. Fix each finding.
3. Re-run audit commands — all return zero results.
4. Run `npm run build` in both apps to verify no compilation errors.
5. Run `npm run lint` if available.

---

## Execution order

Tasks can be parallelized as follows:

| Wave | Tasks | Rationale |
|------|-------|-----------|
| 1 | Task 1, Task 7, Task 8 | Independent: metrics definitions, Go log audit, TS log audit |
| 2 | Task 2, Task 5, Task 6 | Task 2 depends on Task 1 (metrics package). Tasks 5 and 6 are independent Sentry integrations. |
| 3 | Task 3 | Depends on Tasks 1 and 2 (metrics definitions + middleware must exist). Requires reading handler code to find emission points. |
| 4 | Task 4 | Depends on Task 1 (registry). Can technically run in wave 2 but logically makes sense after the middleware (Task 2) is wired. |

## Verification checklist

After all tasks:
- [ ] `go build ./...` compiles without errors
- [ ] `go test ./...` passes all tests including new metrics tests
- [ ] `curl localhost:9090/metrics` returns Prometheus text format
- [ ] `curl localhost:9090/metrics | grep http_requests_total` shows counter after API calls
- [ ] `curl localhost:9090/metrics | grep orders_created_total` shows counter after order creation
- [ ] Sentry test event captured (verify in Sentry dashboard or mock transport)
- [ ] `grep -rn "fmt\.Println" services/marketplace-api/ --include="*.go" | grep -v _test.go` returns nothing
- [ ] `grep -rn "console\.log" apps/storefront/app/ apps/admin/app/ --include="*.ts" --include="*.tsx"` returns nothing
- [ ] `npm run build` succeeds for both admin and storefront (Sentry wrapping works)
- [ ] No PII in info-level Go logs (email fields are debug-only)

## Infra notes (for tesserix-k8s — out of scope here)

- Add `SENTRY_DSN` to marketplace-api ExternalSecret
- Add `NEXT_PUBLIC_SENTRY_DSN` to admin and storefront configmaps
- Add `SENTRY_AUTH_TOKEN` to CI secrets for source map uploads
- Add `METRICS_PORT=9090` to marketplace-api configmap (or rely on default)
- Add Prometheus ServiceMonitor or PodMonitor to scrape port 9090
- Verify Grafana can query the new `http_requests_total` metric
- Add container port 9090 to the Knative Service spec for metrics scraping
