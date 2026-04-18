# P17 — Subscription Observability: Metrics, Dashboards, Alerts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire every metric the preceding plans emit into one importable package, ship Prometheus recording + alert rules, and commit four Grafana dashboards as version-controlled JSON. Operators get a "Subscription Health" business overview, an ops-focused "Webhook Pipeline", a "Trial Activation Funnel", and an "Arbitrage Operations" board. Alerting covers the nine failure modes in spec §21.3 plus DR reconciliation (§22.3).

**Architecture:** A new `internal/metrics/subscription.go` in `services/marketplace-api` centralises every Prometheus collector the subscription system emits. Callers (state machine, webhook dispatcher, trial scheduler, arbitrage flagger, dunning) import typed helpers instead of constructing collectors inline. Recording + alert rules live under `tesserix-infra/k8s/cluster/prometheus/rules/` as a `PrometheusRule` CRD. Grafana dashboards ship as JSON under `tesserix-infra/grafana/dashboards/`, loaded by the Grafana sidecar provisioner via a ConfigMap labelled `grafana_dashboard=1`. Alertmanager receivers fan out to `#security-alerts` Slack, `#platform-alerts` Slack, and PagerDuty by severity + custom labels. A daily `reconciliation-cron` Kubernetes CronJob runs the Stripe drift check and pushes `reconciliation.drift{source}` to Pushgateway.

**Tech Stack:** Go 1.26, `github.com/prometheus/client_golang` v1.19.1 (already in use), Prometheus 2.x + Prometheus Operator (`PrometheusRule` CRD), Alertmanager, Grafana 10.x (schema v38), Stripe SDK `github.com/stripe/stripe-go/v76`, Kubernetes CronJob + Pushgateway.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §21 (observability: metrics, logs, alerts, dashboards), §22 (DR + Stripe reconciliation), §26.2 (strategic risks incl. trial activation), §28 (success criteria backing).

**Depends on:** **P1** (data model + `subscription.state.count`, `EmitStateTransition`), **P2** (webhooks + `webhook.processed`/`webhook.failed`/orphan histogram), **P4** (downgrade cron + `downgrade_blocked_at_cron`), **P5** (trial funnel + day-30 counters), **P6** (dunning + `payment_failed`, `payment_action_required`), **P7** (arbitrage counters), **P9** (campaign email counter), **P13** (break-glass audit emit).

Every metric named in this plan is emitted by one of the plans above. P17 is wiring — it centralises definitions, names them consistently, and composes dashboards/alerts.

**Related plans:**
- **P18** (capacity planning) consumes `subscription.mrr_*` rollups
- **P19** (SLO formalisation) extends with burn-rate alerts — out of scope here

---

## Scope Check

In scope:
1. Centralised `internal/metrics/subscription.go` — single source of truth for collector names, label sets, and help text.
2. FX rate job: daily `cmd/fx-rate-refresh` + `fx_rates` table + `mrr_usd` rollup collector.
3. `PrometheusRule` YAML: recording rules + 12 alert rules.
4. Alertmanager receivers + routing tree (`#security-alerts`, PagerDuty, default severity).
5. Four Grafana dashboards as JSON.
6. Daily Stripe reconciliation CronJob + `reconciliation.drift` + its alert.
7. billing_archive expiry gauge.
8. Log-shipping verification (Cloud Logging smoke in CI).
9. CI PII-leak grep guard.

Out of scope:
- SLO definitions beyond the alerts listed (P19).
- Custom OpenTelemetry tracing (existing setup covers it).
- Per-tenant merchant-facing dashboards — internal only.
- Metric retention config (central Prometheus tier).
- PagerDuty rotations (ops runbook).

---

## File Structure

### Create
- `services/marketplace-api/internal/metrics/subscription.go` — typed collectors
- `services/marketplace-api/internal/metrics/subscription_test.go`
- `services/marketplace-api/internal/metrics/fx.go` — FX repo + fetcher
- `services/marketplace-api/internal/metrics/fx_test.go`
- `services/marketplace-api/internal/metrics/mrr_rollup.go` — scrape-time USD rollup
- `services/marketplace-api/cmd/fx-rate-refresh/main.go`
- `services/marketplace-api/cmd/reconciliation-cron/main.go`
- `services/marketplace-api/internal/reconciliation/stripe_drift.go`
- `services/marketplace-api/internal/reconciliation/stripe_drift_test.go`
- `services/marketplace-api/internal/reconciliation/billing_archive_expiry.go`
- `services/marketplace-api/migrations/20260418_fx_rates.sql`
- `tesserix-infra/k8s/cluster/prometheus/rules/subscription.yaml`
- `tesserix-infra/k8s/cluster/prometheus/rules/subscription_test.yaml` (promtool)
- `tesserix-infra/k8s/cluster/alertmanager/routes.yaml`
- `tesserix-infra/grafana/dashboards/subscription-health.json`
- `tesserix-infra/grafana/dashboards/webhook-pipeline.json`
- `tesserix-infra/grafana/dashboards/trial-activation-funnel.json`
- `tesserix-infra/grafana/dashboards/arbitrage-operations.json`
- `tesserix-infra/k8s/cluster/grafana/dashboards/kustomization.yaml`
- `tesserix-infra/k8s/apps/marketplace/marketplace-api/reconciliation-cronjob.yaml`
- `tesserix-infra/k8s/apps/marketplace/marketplace-api/fx-rate-refresh-cronjob.yaml`
- `.github/workflows/pii-leak-guard.yml`
- `scripts/ci/grep-pii-logs.sh`
- `scripts/ci/verify-cloud-logging.sh`
- `scripts/ci/verify-dashboard-queries.sh`

### Modify
- `services/marketplace-api/cmd/marketplace-api/main.go` — register collectors at startup
- `services/marketplace-api/internal/subscription/statemachine/machine.go` — emit `subscription_state_transitioned_total`
- `services/marketplace-api/internal/billing/dispatch/handlers.go` — emit webhook counters + duration histogram
- `services/marketplace-api/internal/dunning/dunning.go`
- `services/marketplace-api/internal/arbitrage/flagger.go`
- `services/marketplace-api/internal/subscription/trial/scheduler.go`
- `services/marketplace-api/internal/downgrade/cron.go`
- `services/marketplace-api/internal/campaigns/dispatcher.go`
- `.github/workflows/ci.yml` (marketplace-api) — dashboard validation step

### Delete
None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Centralised collectors + tests | — |
| 2 | Wire collectors into emit sites | 1, P1-P9, P13 |
| 3 | FX refresh + `mrr_usd` rollup | 1 |
| 4 | Prometheus recording + alert rules | 2, 3 |
| 5 | Alertmanager receivers + routes | 4 |
| 6 | "Subscription Health" dashboard | 2, 3, 4 |
| 7 | "Webhook Pipeline" dashboard | 2, 4 |
| 8 | "Trial Activation Funnel" dashboard | 2 |
| 9 | "Arbitrage Operations" dashboard | 2 |
| 10 | Stripe reconciliation CronJob | 4 |
| 11 | billing_archive expiry gauge | 1 |
| 12 | Log-shipping Cloud Logging smoke | 2 |
| 13 | CI PII-leak guard | — |
| 14 | Dashboard schema + PromQL reference validation | 6-9 |
| 15 | ArgoCD sync for rules + dashboards | 4, 5, 6-9, 10 |

---

## Reusable patterns

**A. Collector registration.** Package-level `NewSubscriptionCollectors(reg prometheus.Registerer) *SubscriptionCollectors` exposes typed accessors. Tests use a fresh `prometheus.NewRegistry()` to avoid cross-test pollution.

**B. Label cardinality.** Only `status`, `event_type`, `reason`, `plan`, `currency`, `source`, `severity` appear as labels. `store_id`/`tenant_id` are **never** labels — they go into logs. `campaign_email_sent{store_id}` is the documented exception (bounded by §3.x campaign rate-limits); revisit in P18 if series count grows past 5k.

**C. Recording rule naming.** `subscription:<aggregate>:<window>` (e.g. `subscription:active_count:by_plan`). Alert rules reference recording rules, not raw counters, so panel queries + alert queries never diverge.

**D. Grafana JSON discipline.** Every dashboard commits with explicit `"uid"`, `"schemaVersion": 38`, manually-bumped `"version"`. Panel queries reference recording rules by name — a rename forces a dashboard update in the same PR.

**E. Alertmanager routing.** Severity-based top level: `critical → PagerDuty`, `warning → #platform-alerts`, `info → #platform-alerts` (24h repeat). `security: "true"` label branches to `#security-alerts` independent of severity. Custom `pagerduty_route: subscription` override for orphan-webhook + reconciliation drift so ownership routing is explicit.

**F. CronJob + Pushgateway.** `fx-rate-refresh` and `reconciliation-cron` are `batch/v1` CronJobs (not Knative). They push metrics to Pushgateway on completion; Prometheus scrapes Pushgateway. Keeps scale-to-zero intact for the main API.

---

## Task 1: Centralised metrics package

**Files:**
- Create: `services/marketplace-api/internal/metrics/subscription.go`
- Create: `services/marketplace-api/internal/metrics/subscription_test.go`

**Spec references:** §21.1.

- [ ] **Step 1: Failing tests — every named metric registered with expected labels**

```go
package metrics_test

import (
    "strings"
    "testing"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/testutil"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/metrics"
)

func TestSubscriptionCollectors_Names(t *testing.T) {
    want := []string{
        "subscription_state_count", "subscription_state_transitioned_total",
        "subscription_mrr_inr", "subscription_mrr_usd", "subscription_mrr_eur",
        "subscription_mrr_gbp", "subscription_mrr_cad", "subscription_mrr_aud", "subscription_mrr_sgd",
        "subscription_mrr_usd_rollup",
        "subscription_trial_expired_today", "subscription_trial_activated_day_30",
        "subscription_trial_product_created_day_30",
        "subscription_payment_failed", "subscription_payment_action_required",
        "subscription_arbitrage_flagged", "subscription_arbitrage_false_positive_cleared",
        "subscription_downgrade_blocked_at_cron",
        "campaign_email_sent",
        "webhook_processed", "webhook_failed",
        "webhook_orphan_resolved_after_seconds", "webhook_processing_duration_seconds",
        "reconciliation_drift", "billing_archive_expiry_soon",
    }
    reg := prometheus.NewRegistry()
    _ = metrics.NewSubscriptionCollectors(reg)
    mf, err := reg.Gather()
    require.NoError(t, err)
    got := map[string]struct{}{}
    for _, m := range mf { got[m.GetName()] = struct{}{} }
    for _, n := range want { require.Contains(t, got, n, "missing %s", n) }
}

// Additional tests (same package, one per concern):
// - TestLabelHygiene_NoTenantStoreOnStateCount — set + gather + assert no tenant_id/store_id label on subscription_state_count
// - TestCounterMonotonicity — Inc twice, assert testutil.ToFloat64 == 2
// - TestMetricHelp_NonEmpty — every gathered metric has non-empty Help text
```

- [ ] **Step 2: Run — expect FAIL** (`go test ./internal/metrics/... -v`)

- [ ] **Step 3: Write `subscription.go`**

Structure: one `SubscriptionCollectors` aggregate struct with typed sub-groups (`State`, `MRR`, `Trial`, `Payment`, `Arbitrage`, `Downgrade`, `Campaign`, `Webhook`, `DR`). `NewSubscriptionCollectors(reg prometheus.Registerer)` constructs every collector, registers them all, returns the aggregate.

```go
// Package metrics centralises Prometheus collectors for the subscription
// subsystem. Rules in tesserix-infra/k8s/cluster/prometheus/rules/subscription.yaml
// mirror these names exactly — any rename requires a rule update in the same PR.
package metrics

import "github.com/prometheus/client_golang/prometheus"

type SubscriptionCollectors struct {
    State     *stateCollectors
    MRR       *mrrCollectors
    Trial     *trialCollectors
    Payment   *paymentCollectors
    Arbitrage *arbitrageCollectors
    Downgrade *downgradeCollectors
    Campaign  *campaignCollectors
    Webhook   *webhookCollectors
    DR        *drCollectors
}

type stateCollectors struct {
    Count        *prometheus.GaugeVec   // labels: status
    Transitioned *prometheus.CounterVec // labels: from, to
}
func (s *stateCollectors) Set(status string, v float64) { s.Count.WithLabelValues(status).Set(v) }

type mrrCollectors struct {
    PerCurrency map[string]prometheus.Gauge // subscription_mrr_{currency}
    USDRollup   prometheus.Gauge            // daily FX-normalised rollup
}
type trialCollectors struct {
    ExpiredToday, ActivatedDay30, ProductCreatedDay30 prometheus.Counter
}
type paymentCollectors struct {
    Failed         *prometheus.CounterVec // labels: plan, currency
    ActionRequired prometheus.Counter
}
type arbitrageCollectors struct{ Flagged, FalsePositiveCleared prometheus.Counter }
type downgradeCollectors struct{ BlockedAtCron prometheus.Counter }
type campaignCollectors struct {
    EmailSent *prometheus.CounterVec // labels: store_id — see §B cardinality note
}
type webhookCollectors struct {
    Processed           *prometheus.CounterVec   // labels: event_type
    Failed              *prometheus.CounterVec   // labels: event_type, reason
    OrphanResolvedAfter prometheus.Histogram     // buckets: 1,5,15,60,300,1800,3600,14400,86400 sec
    ProcessingDuration  *prometheus.HistogramVec // labels: event_type; buckets: 0.05..10s
}
type drCollectors struct {
    ReconciliationDrift *prometheus.CounterVec // labels: source (stripe|cnpg|gcs)
    BillingArchiveSoon  prometheus.Gauge
}

var supportedCurrencies = []string{"inr", "usd", "eur", "gbp", "cad", "aud", "sgd"}

// NewSubscriptionCollectors constructs + registers every collector.
// See appendix of this task for full construction code (one collector per
// helper, each using NewGaugeVec/NewCounterVec/NewHistogram with Name + Help +
// labels matching the struct comments above). Help text is mandatory —
// TestMetricHelp_NonEmpty asserts every collector has non-empty Help.
func NewSubscriptionCollectors(reg prometheus.Registerer) *SubscriptionCollectors {
    // … construct each sub-collectors group with the names + help text
    //    documented in struct comments above; call reg.MustRegister for every
    //    collector returned in allCollectors()
}
```

**Concrete Name + Help pairs (authoritative):**

| Name | Help |
|---|---|
| `subscription_state_count` | Subscription count per status. Scraped snapshot. |
| `subscription_state_transitioned_total` | Successful subscription state transitions. |
| `subscription_mrr_{inr\|usd\|eur\|gbp\|cad\|aud\|sgd}` | MRR in {currency} (native billing, no FX). |
| `subscription_mrr_usd_rollup` | All-currency MRR normalised to USD via daily mid-market FX. |
| `subscription_trial_expired_today` | Trials expired in today's cron. Alerted if stale >25h. |
| `subscription_trial_activated_day_30` | Trials activated within 30d of signup. |
| `subscription_trial_product_created_day_30` | Trials that published a product within 30d. |
| `subscription_payment_failed` | Failed payment attempts (invoice.payment_failed). |
| `subscription_payment_action_required` | Subscriptions moved to payment_action_required (SCA/3DS). |
| `subscription_arbitrage_flagged` | Signups flagged by the arbitrage detector. |
| `subscription_arbitrage_false_positive_cleared` | Flagged signups cleared as false-positive. |
| `subscription_downgrade_blocked_at_cron` | Downgrades blocked at renewal per §4.5.1. |
| `campaign_email_sent` | Campaign emails dispatched, labelled by store_id. |
| `webhook_processed` | Stripe webhooks processed successfully. |
| `webhook_failed` | Stripe webhooks that failed. |
| `webhook_orphan_resolved_after_seconds` | Seconds from orphan sighting to resolution. |
| `webhook_processing_duration_seconds` | Time from webhook ingress to ACK. |
| `reconciliation_drift` | Records out-of-sync with source of truth. |
| `billing_archive_expiry_soon` | billing_archive rows with archive_expires_at in next 30d. |

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/metrics/subscription{,_test}.go
git commit -m "feat(metrics): centralised subscription collectors with typed accessors"
```

---

## Task 2: Wire collectors into emit sites

**Files:** modify state machine, dispatch, dunning, arbitrage, trial scheduler, downgrade cron, campaign dispatcher, main.go.

**Purpose:** Replace scattered `prometheus.NewCounter` calls with the typed accessors from Task 1. Mechanical rewrite; per-module existing tests updated for the new constructor arg.

- [ ] **Step 1: Failing test — state machine emits transitioned counter**

```go
//go:build integration

func TestStatemachine_EmitsTransitionCounter(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    reg := prometheus.NewRegistry()
    subs := metrics.NewSubscriptionCollectors(reg)
    em := audit.NewEmitter(audit.NewRecorderForTesting())

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, Metrics: subs,
        TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusActive, To: subscription.StatusCancelScheduled,
        Actor: "user:x", Reason: "merchant_cancelled",
    }))
    require.Equal(t, float64(1),
        testutil.ToFloat64(subs.State.Transitioned.WithLabelValues("active", "cancel_scheduled")))
}
```

- [ ] **Step 2: Extend `statemachine.TransitionInput` with `Metrics *metrics.SubscriptionCollectors` (nil-safe)**

After CAS succeeds in `Transition`:

```go
if in.Metrics != nil {
    in.Metrics.State.Transitioned.WithLabelValues(string(in.From), string(in.To)).Inc()
}
```

- [ ] **Step 3: Webhook dispatcher — instrument every branch**

```go
// internal/billing/dispatch/handlers.go
func (d *Dispatcher) handle(c *gin.Context, event stripe.Event) {
    start := time.Now()
    defer func() {
        d.metrics.Webhook.ProcessingDuration.WithLabelValues(event.Type).Observe(time.Since(start).Seconds())
    }()
    if err := d.route(c, event); err != nil {
        d.metrics.Webhook.Failed.WithLabelValues(event.Type, classifyReason(err)).Inc()
        return
    }
    d.metrics.Webhook.Processed.WithLabelValues(event.Type).Inc()
}
```

- [ ] **Step 4: Dunning / trial scheduler / arbitrage / downgrade / campaigns**

Mechanical: each package accepts `*metrics.SubscriptionCollectors` via constructor and calls typed accessors at natural counting points. Per-module unit tests pass a `prometheus.NewRegistry()` + fresh collectors.

- [ ] **Step 5: `cmd/marketplace-api/main.go` wiring**

```go
reg := prometheus.DefaultRegisterer
subsMetrics := metrics.NewSubscriptionCollectors(reg)
// Pass subsMetrics into webhook dispatcher, statemachine input, dunning, trial, etc.
```

- [ ] **Step 6: Full test suite green** — `go test -tags=integration ./internal/... -count=1`

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/{cmd,internal}/...
git commit -m "feat(metrics): wire centralised subscription collectors into all emit points"
```

---

## Task 3: FX refresh + `mrr_usd` rollup

**Files:**
- Create: `services/marketplace-api/migrations/20260418_fx_rates.sql`
- Create: `services/marketplace-api/internal/metrics/fx.go` (+ `fx_test.go`)
- Create: `services/marketplace-api/internal/metrics/mrr_rollup.go`
- Create: `services/marketplace-api/cmd/fx-rate-refresh/main.go`
- Create: `tesserix-infra/k8s/apps/marketplace/marketplace-api/fx-rate-refresh-cronjob.yaml`

**Purpose:** `subscription_mrr_usd_rollup = Σ (mrr_{c} × fx[c→USD])`. Daily ECB reference rates → `fx_rates` table; the marketplace-api computes the rollup on every Prometheus scrape via a custom collector.

- [ ] **Step 1: Migration**

```sql
CREATE TABLE fx_rates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    base_currency   CHAR(3) NOT NULL,
    quote_currency  CHAR(3) NOT NULL,
    rate            NUMERIC(18,9) NOT NULL,
    as_of_date      DATE NOT NULL,
    source          VARCHAR(50) NOT NULL DEFAULT 'ecb',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (base_currency, quote_currency, as_of_date)
);
CREATE INDEX idx_fx_rates_latest ON fx_rates (base_currency, quote_currency, as_of_date DESC);
```

- [ ] **Step 2: `fx.go` — fetcher interface + repo**

```go
package metrics

import (
    "context"
    "fmt"
    "time"
    "gorm.io/gorm"
)

type FXFetcher interface {
    Fetch(ctx context.Context, base string, quotes []string) (map[string]float64, error)
}

type FXRepo struct{ DB *gorm.DB }

type fxRateRow struct {
    BaseCurrency, QuoteCurrency string
    Rate                        float64
    AsOfDate                    time.Time
    Source                      string
}

func (r *FXRepo) Latest(ctx context.Context, base, quote string) (float64, error) {
    var out fxRateRow
    err := r.DB.WithContext(ctx).Raw(`
        SELECT base_currency, quote_currency, rate, as_of_date, source FROM fx_rates
        WHERE base_currency = ? AND quote_currency = ?
        ORDER BY as_of_date DESC LIMIT 1`, base, quote).Scan(&out).Error
    if err != nil { return 0, fmt.Errorf("fx latest: %w", err) }
    if out.Rate == 0 { return 0, fmt.Errorf("no fx rate for %s/%s", base, quote) }
    return out.Rate, nil
}

func (r *FXRepo) Upsert(ctx context.Context, rows []fxRateRow) error {
    for _, row := range rows {
        if err := r.DB.WithContext(ctx).Exec(`
            INSERT INTO fx_rates (base_currency, quote_currency, rate, as_of_date, source)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT (base_currency, quote_currency, as_of_date)
            DO UPDATE SET rate = EXCLUDED.rate, source = EXCLUDED.source`,
            row.BaseCurrency, row.QuoteCurrency, row.Rate, row.AsOfDate, row.Source).Error; err != nil {
            return fmt.Errorf("fx upsert: %w", err)
        }
    }
    return nil
}
```

- [ ] **Step 3: `mrr_rollup.go` — scrape-time custom collector**

```go
package metrics

import (
    "context"
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type mrrRollupCollector struct {
    fx   *FXRepo
    sub  subscription.Repo
    desc *prometheus.Desc
}

func NewMRRRollupCollector(fx *FXRepo, sub subscription.Repo) prometheus.Collector {
    return &mrrRollupCollector{
        fx: fx, sub: sub,
        desc: prometheus.NewDesc("subscription_mrr_usd_rollup_live",
            "All-currency MRR normalised to USD, computed at scrape time.", nil, nil),
    }
}

func (c *mrrRollupCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *mrrRollupCollector) Collect(ch chan<- prometheus.Metric) {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    perCurrency, err := c.sub.MRRByCurrency(ctx)
    if err != nil { return }
    var totalUSD float64
    for currency, amount := range perCurrency {
        if currency == "USD" { totalUSD += amount; continue }
        rate, err := c.fx.Latest(ctx, currency, "USD")
        if err != nil { continue }  // undercount > wrong rollup
        totalUSD += amount * rate
    }
    ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, totalUSD)
}
```

- [ ] **Step 4: `cmd/fx-rate-refresh/main.go`** — pulls ECB daily reference rates, upserts. Fetcher impl `NewECBFetcher()` hits `https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml`, transforms via USD-base, upserts one row per supported quote currency. On error: log + exit 1 (CronJob `backoffLimit: 2` retries). Pushes completion metric `fx_rate_refresh_last_success_timestamp` to Pushgateway.

- [ ] **Step 5: CronJob manifest**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: fx-rate-refresh
  namespace: marketplace
spec:
  schedule: "15 6 * * *"        # 06:15 UTC — after ECB publishes
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          serviceAccountName: marketplace-api
          containers:
            - name: fx-rate-refresh
              image: asia-south1-docker.pkg.dev/tesserix-app/services/marketplace-api:latest
              command: ["/app/fx-rate-refresh"]
              envFrom: [{ secretRef: { name: marketplace-api-secrets } }]
              resources:
                requests: { cpu: 50m, memory: 128Mi }
                limits:   { cpu: 200m, memory: 256Mi }
```

- [ ] **Step 6: Tests**
- `TestFXRepo_LatestRate` — table-driven, inserts + reads most-recent row.
- `TestFXRepo_Upsert_Idempotent` — same (base, quote, date) twice leaves single row with latest rate.
- `TestMRRRollup_SumsAllCurrencies` — mocked repo returns {USD:100, INR:8300, EUR:90}; FX rates stubbed; assert totalUSD ≈ 290.
- `TestMRRRollup_MissingFX_SkipsQuote` — FX repo returns error for one currency; collector still emits a value for remaining.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/metrics/{fx,mrr_rollup}*.go \
        services/marketplace-api/cmd/fx-rate-refresh/ \
        services/marketplace-api/migrations/20260418_fx_rates.sql \
        tesserix-infra/k8s/apps/marketplace/marketplace-api/fx-rate-refresh-cronjob.yaml
git commit -m "feat(metrics): daily ECB FX refresh + scrape-time MRR USD rollup"
```

---

## Task 4: Prometheus recording + alert rules

**Files:**
- Create: `tesserix-infra/k8s/cluster/prometheus/rules/subscription.yaml`
- Create: `tesserix-infra/k8s/cluster/prometheus/rules/subscription_test.yaml`

**Spec references:** §21.3 (alerts), §21.1 (metrics), §22.3 (reconciliation), §26.2 (trial activation).

- [ ] **Step 1: PrometheusRule CRD**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: subscription-rules
  namespace: monitoring
  labels:
    prometheus: kube-prometheus
    role: alert-rules
    app.kubernetes.io/part-of: subscription-observability
spec:
  groups:
    # -----------------------------------------------------------------------
    # Recording rules — derived series for dashboards + alert predicates
    # -----------------------------------------------------------------------
    - name: subscription.recording
      interval: 30s
      rules:
        - record: subscription:active_count:by_plan
          expr: sum by (plan) (subscription_state_count{status="active"})
        - record: subscription:active_count:total
          expr: sum(subscription_state_count{status=~"active|payment_action_required|past_due|cancel_scheduled"})
        - record: subscription:trial_count:total
          expr: sum(subscription_state_count{status="trialing"})
        - record: subscription:failed_payment_rate_24h
          expr: |
            sum(rate(subscription_payment_failed[24h]))
              / clamp_min(subscription:active_count:total, 1)
        - record: subscription:trial_signups_daily
          expr: |
            increase(sum(subscription_state_transitioned_total{from="signup", to="trialing"})[24h:5m])
        - record: subscription:trial_activation_day_30_rate
          expr: |
            increase(subscription_trial_activated_day_30[30d])
              / clamp_min(
                  increase(sum(subscription_state_transitioned_total{from="signup", to="trialing"})[30d:5m]),
                  1)
        - record: subscription:webhook_processing_p50
          expr: histogram_quantile(0.50, sum by (le, event_type) (rate(webhook_processing_duration_seconds_bucket[5m])))
        - record: subscription:webhook_processing_p95
          expr: histogram_quantile(0.95, sum by (le, event_type) (rate(webhook_processing_duration_seconds_bucket[5m])))
        - record: subscription:webhook_processing_p99
          expr: histogram_quantile(0.99, sum by (le, event_type) (rate(webhook_processing_duration_seconds_bucket[5m])))
        - record: subscription:webhook_failure_rate_1h
          expr: |
            sum(rate(webhook_failed[1h]))
              / clamp_min(sum(rate(webhook_processed[1h])) + sum(rate(webhook_failed[1h])), 1)
        - record: subscription:arbitrage_flag_rate_1h
          expr: rate(subscription_arbitrage_flagged[1h])
        - record: subscription:arbitrage_flag_baseline_24h
          expr: rate(subscription_arbitrage_flagged[24h])

    # -----------------------------------------------------------------------
    # Alert rules — 12 alerts covering §21.3 + DR (§22.3) + strategic risk
    # -----------------------------------------------------------------------
    - name: subscription.alerts
      interval: 30s
      rules:
        - alert: TrialSchedulerDeadMansSwitch
          expr: time() - max(timestamp(subscription_trial_expired_today)) > (25 * 3600)
          for: 0m
          labels:
            severity: critical
            component: subscription
            runbook: https://runbooks.tesserix.internal/subscription/trial-scheduler
          annotations:
            summary: "Trial expiry cron has not reported in >25h"
            description: "subscription_trial_expired_today is stale. P5 scheduler outage — trials won't transition."

        - alert: FailedPaymentSpike
          expr: subscription:failed_payment_rate_24h > 0.05
          for: 30m
          labels: { severity: warning, component: subscription }
          annotations:
            summary: "Failed-payment rate >5% of active subs (24h)"
            description: "Rate: {{ $value | humanizePercentage }}. Check Stripe incidents or card-decline pattern."

        - alert: WebhookP95HighLatency
          expr: max(subscription:webhook_processing_p95) > 5
          for: 10m
          labels: { severity: warning, component: webhook }
          annotations:
            summary: "Stripe webhook P95 >5s"
            description: "P95 = {{ $value }}s. Likely DB contention or downstream timeout. Stripe retries at >10s."

        - alert: WebhookFailureRateHigh
          expr: subscription:webhook_failure_rate_1h > 0.01
          for: 30m
          labels: { severity: warning, component: webhook }
          annotations:
            summary: "Webhook failure rate >1% over 1h"
            description: "Current: {{ $value | humanizePercentage }}. Break down by (event_type, reason)."

        - alert: TrialSignupAnomaly
          expr: subscription:trial_signups_daily > 50
          for: 15m
          labels: { severity: warning, component: subscription, security: "true" }
          annotations:
            summary: "Trial signups >50 in rolling 24h — possible abuse"
            description: "Check arbitrage flags + IP distribution. §5.1 flags >50/day anomalous at our stage."

        - alert: BreakGlassAdminUsed
          expr: increase(audit_breakglass_used_total[5m]) > 0
          for: 0m
          labels: { severity: critical, component: subscription, security: "true" }
          annotations:
            summary: "Break-glass admin access used"
            description: "P13 break-glass override invoked. Post-incident review required. Check actor + reason."

        - alert: ArbitrageFlagSpike
          expr: subscription:arbitrage_flag_rate_1h > (5 * subscription:arbitrage_flag_baseline_24h)
          for: 15m
          labels: { severity: warning, component: subscription, security: "true" }
          annotations:
            summary: "Arbitrage flag rate >5× baseline"
            description: "1h rate = {{ $value }}. Possible abuse campaign or detector regression."

        - alert: OrphanWebhookUnresolved
          expr: |
            histogram_quantile(0.99,
              sum by (le) (rate(webhook_orphan_resolved_after_seconds_bucket[1h]))
            ) > 3600
          for: 15m
          labels: { severity: critical, component: webhook, pagerduty_route: subscription }
          annotations:
            summary: "Orphan webhooks unresolved >1h at P99"
            description: "Orphan events linger >1h. Check manual_review_required queue + P2 dispatcher health."

        - alert: TrialActivationRateLow
          expr: subscription:trial_activation_day_30_rate < 0.30
          for: 24h
          labels: { severity: info, component: subscription, strategy: "true" }
          annotations:
            summary: "Day-30 trial activation <30% (rolling 30d)"
            description: "Activation = {{ $value | humanizePercentage }}. Strategic risk #8 (§26.2). Weekly review."

        - alert: ReconciliationDriftStripe
          expr: |
            increase(reconciliation_drift{source="stripe"}[24h])
              > 0.001 * subscription:active_count:total
          for: 30m
          labels: { severity: critical, component: reconciliation, pagerduty_route: subscription }
          annotations:
            summary: "Stripe ↔ DB subscription drift >0.1% of active subs"
            description: "Failed webhook or direct Stripe dashboard mutation. See reconciliation-cron logs."

        - alert: BillingArchiveExpirySoon
          expr: billing_archive_expiry_soon > 0
          for: 6h
          labels: { severity: info, component: compliance }
          annotations:
            summary: "{{ $value }} billing_archive rows expire within 30d"
            description: "Operational awareness. Confirm purge cron ran last window."

        - alert: DowngradeBlockedAtCronSpike
          expr: increase(subscription_downgrade_blocked_at_cron[24h]) > 5
          for: 1h
          labels: { severity: warning, component: subscription }
          annotations:
            summary: "Downgrade cron blocked >5 downgrades in 24h"
            description: "§4.5.1 firing more than expected. Check tenant states; possible plan-selection UX issue."
```

- [ ] **Step 2: `promtool` unit tests**

```yaml
# subscription_test.yaml
rule_files: [subscription.yaml]
evaluation_interval: 1m
tests:
  - interval: 1m
    input_series:
      - series: 'subscription_state_count{status="active"}'
        values: '100x5'
      - series: 'subscription_state_count{status="past_due"}'
        values: '5x5'
      - series: 'subscription_state_count{status="payment_action_required"}'
        values: '2x5'
      - series: 'subscription_payment_failed_total'
        values: '0 2 4 6 8'
    alert_rule_test:
      - eval_time: 2m
        alertname: FailedPaymentSpike
        exp_alerts: []
      - eval_time: 35m
        alertname: FailedPaymentSpike
        exp_alerts:
          - exp_labels:
              severity: warning
              component: subscription
              alertname: FailedPaymentSpike
            exp_annotations:
              summary: "Failed-payment rate >5% of active subs (24h)"

  - interval: 1m
    input_series:
      - series: 'subscription_trial_expired_today'
        values: '1'     # absent thereafter
    alert_rule_test:
      - eval_time: 30h
        alertname: TrialSchedulerDeadMansSwitch
        exp_alerts:
          - exp_labels:
              severity: critical
              component: subscription
              alertname: TrialSchedulerDeadMansSwitch
```

Run: `promtool test rules tesserix-infra/k8s/cluster/prometheus/rules/subscription_test.yaml`

- [ ] **Step 3: Commit**

```bash
git add tesserix-infra/k8s/cluster/prometheus/rules/
git commit -m "feat(observability): Prometheus recording + alert rules for subscription (§21.3)"
```

---

## Task 5: Alertmanager receivers + routing

**Files:**
- Create: `tesserix-infra/k8s/cluster/alertmanager/routes.yaml`

- [ ] **Step 1: AlertmanagerConfig**

```yaml
apiVersion: monitoring.coreos.com/v1alpha1
kind: AlertmanagerConfig
metadata:
  name: subscription-routes
  namespace: monitoring
spec:
  route:
    receiver: platform-default
    groupBy: [alertname, component]
    groupWait: 30s
    groupInterval: 5m
    repeatInterval: 4h
    routes:
      # Security-flagged → #security-alerts (continue:true allows further routing).
      - matchers: [{ name: security, value: "true" }]
        receiver: security-slack
        continue: true
      # Explicit PagerDuty for subscription-owned criticals.
      - matchers: [{ name: pagerduty_route, value: subscription }]
        receiver: pagerduty-subscription
        groupWait: 10s
        repeatInterval: 1h
      # Severity fan-out.
      - matchers: [{ name: severity, value: critical }]
        receiver: pagerduty-platform
        groupWait: 10s
        repeatInterval: 1h
      - matchers: [{ name: severity, value: warning }]
        receiver: platform-slack
      - matchers: [{ name: severity, value: info }]
        receiver: platform-slack
        repeatInterval: 24h

  receivers:
    # 5 receivers; full YAML bodies in the committed file.
    # - platform-default: Slack #platform-alerts, formatted title [{{.Status|toUpper}}] {{alertname}},
    #     text iterates {{range .Alerts}} summary + description {{end}}; sendResolved: true.
    # - platform-slack: bare Slack #platform-alerts sender (Alertmanager default body); sendResolved: true.
    # - security-slack: Slack #security-alerts, title '[SECURITY] {{alertname}}', text adds Component + Runbook fields.
    # - pagerduty-platform: PD routing-key `platform`; severity from {{.CommonLabels.severity}}; description from summary.
    # - pagerduty-subscription: PD routing-key `subscription`; severity: critical fixed;
    #     details include firing count + runbook label.
    # All SecretRefs point at the `slack-webhook` Secret (keys: platform, security) and `pagerduty-keys` Secret
    # (keys: platform, subscription). ESO syncs these from GCP Secret Manager.
```

- [ ] **Step 2: Dry-run the routing tree**

```bash
amtool config routes test --config.file=/tmp/alertmanager.yml \
  alertname=OrphanWebhookUnresolved severity=critical pagerduty_route=subscription
# Expect: pagerduty-subscription

amtool config routes test --config.file=/tmp/alertmanager.yml \
  alertname=BreakGlassAdminUsed severity=critical security=true
# Expect: security-slack + pagerduty-platform (security route continues)
```

- [ ] **Step 3: Commit**

```bash
git add tesserix-infra/k8s/cluster/alertmanager/routes.yaml
git commit -m "feat(observability): Alertmanager routes — #security-alerts, PagerDuty, severity fan-out"
```

---

## Task 6: "Subscription Health" dashboard

**Files:**
- Create: `tesserix-infra/grafana/dashboards/subscription-health.json`

**Top-level JSON shape** (identical for all four dashboards — only `uid`, `title`, `tags`, `time`, `refresh`, and `panels` vary):

```json
{
  "uid": "subscription-health-v1",
  "title": "Subscription Health",
  "tags": ["subscription", "business"],
  "schemaVersion": 38,
  "version": 1,
  "refresh": "30s",
  "time": { "from": "now-7d", "to": "now" },
  "templating": { "list": [{ "name": "plan", "type": "query",
    "datasource": { "type": "prometheus", "uid": "prometheus" },
    "query": "label_values(subscription_state_count, plan)",
    "includeAll": true, "multi": true }] },
  "panels": [ /* see table below */ ],
  "links": [
    { "title": "Webhook Pipeline", "url": "/d/webhook-pipeline-v1" },
    { "title": "Trial Activation Funnel", "url": "/d/trial-funnel-v1" },
    { "title": "Arbitrage Operations", "url": "/d/arbitrage-ops-v1" }
  ]
}
```

**Panels (8 total):**

| id | title | type | gridPos (x,y,w,h) | expr | notes |
|---|---|---|---|---|---|
| 1 | Total Active Subscriptions | stat | 0,0,6,4 | `subscription:active_count:total` | — |
| 2 | Active by Plan | bargauge | 6,0,12,4 | `subscription:active_count:by_plan` | `legendFormat: {{plan}}` |
| 3 | MRR — USD rollup | stat | 18,0,6,4 | `subscription_mrr_usd_rollup` | unit: `currencyUSD` |
| 4 | MRR by currency | timeseries | 0,4,12,8 | 7 targets, one per currency | legend: INR/USD/EUR/GBP/CAD/AUD/SGD |
| 5 | Failed Payment Rate (24h) | timeseries | 12,4,12,8 | `subscription:failed_payment_rate_24h` | thresholds: green<0.03, yellow<0.05, red≥0.05; unit `percentunit` |
| 6 | Trial Activation Day-30 | stat | 0,12,6,4 | `subscription:trial_activation_day_30_rate` | thresholds: red<0.30, yellow<0.50, green≥0.50 |
| 7 | Webhook Processing P95 by event | heatmap | 6,12,12,8 | `subscription:webhook_processing_p95` | `legendFormat: {{event_type}}` |
| 8 | Upcoming Expirations | timeseries | 18,12,6,8 | `subscription_state_count{status="cancel_scheduled"}` | — |

Each panel follows the shape `{ "id": N, "title": "…", "type": "…", "gridPos": {…}, "targets": [{ "expr": "…", "refId": "A", "legendFormat": "…" }], "fieldConfig": { "defaults": { "unit": "…", "thresholds": {…} } } }`.

- [ ] **Step 2: Commit**

```bash
git add tesserix-infra/grafana/dashboards/subscription-health.json
git commit -m "feat(observability): 'Subscription Health' Grafana dashboard"
```

---

## Task 7: "Webhook Pipeline" dashboard

**Files:**
- Create: `tesserix-infra/grafana/dashboards/webhook-pipeline.json`

**Top-level:** `uid: "webhook-pipeline-v1"`, `title: "Webhook Pipeline"`, `tags: ["subscription","webhook","ops"]`, `schemaVersion: 38`, `version: 1`, `refresh: "30s"`, `time: { "from":"now-6h","to":"now" }`.

**Panels (8 total):**

| id | title | type | gridPos (x,y,w,h) | expr |
|---|---|---|---|---|
| 1 | Ingress Rate by event_type | timeseries | 0,0,18,8 | `sum by (event_type) (rate(webhook_processed[5m]) + rate(webhook_failed[5m]))` |
| 2 | Signature Verification Failures | stat | 18,0,6,4 | `sum(rate(webhook_failed{reason="invalid_signature"}[5m]))` |
| 3 | Orphan Queue Depth | stat | 18,4,6,4 | `webhook_orphan_queue_depth` |
| 4 | P50 / P95 / P99 processing time | timeseries | 0,8,18,8 | 3 targets: `subscription:webhook_processing_{p50,p95,p99}` with legendFormat `{p50\|p95\|p99} {{event_type}}` |
| 5 | Retry attempts histogram | bargauge | 18,8,6,8 | `sum by (attempt) (rate(webhook_retries_total[1h]))` |
| 6 | Orphan resolution latency | heatmap | 0,16,12,8 | `sum by (le) (rate(webhook_orphan_resolved_after_seconds_bucket[1h]))` (format: heatmap) |
| 7 | Top 5 failing (event_type, reason) | table | 12,16,12,8 | `topk(5, sum by (event_type, reason) (rate(webhook_failed[1h])))` (instant) |
| 8 | Manual review pending | stat | 0,24,6,4 | `webhook_manual_review_pending` |

- [ ] **Commit**

```bash
git add tesserix-infra/grafana/dashboards/webhook-pipeline.json
git commit -m "feat(observability): 'Webhook Pipeline' Grafana dashboard"
```

---

## Task 8: "Trial Activation Funnel" dashboard

**Files:**
- Create: `tesserix-infra/grafana/dashboards/trial-activation-funnel.json`

**Funnel stages:** signup → trialing → tax_id validated → first product created → card added → activated.

**Top-level:** `uid: "trial-funnel-v1"`, `title: "Trial Activation Funnel"`, `tags: ["subscription","trial","growth"]`, `time: { "from":"now-30d","to":"now" }`, `refresh: "5m"`.

**Panels (5 total):**

| id | title | type | gridPos | expr(s) |
|---|---|---|---|---|
| 1 | Funnel (last 30d) | bargauge | 0,0,24,8 | 5 targets, one per stage: (a) `increase(sum(subscription_state_transitioned_total{from="signup",to="trialing"})[30d:5m])`, (b) `increase(subscription_tax_id_validated_total[30d])`, (c) `increase(subscription_trial_product_created_day_30[30d])`, (d) `increase(subscription_card_added_during_trial_total[30d])`, (e) `increase(subscription_trial_activated_day_30[30d])`. Legends: "1. Trialing" … "5. Activated" |
| 2 | Stage-to-stage conversion % | stat | 0,8,24,6 | 4 targets, each `(stage_N) / clamp_min(stage_N-1, 1)` for the 4 adjacent pairs; legends "Trial→Tax ID", "Tax ID→Product", "Product→Card", "Card→Active" |
| 3 | Daily stage counts | timeseries | 0,14,24,10 | 3 targets: `increase(subscription_trial_product_created_day_30[1d])`, `increase(subscription_card_added_during_trial_total[1d])`, `increase(subscription_trial_activated_day_30[1d])` |
| 4 | Day-30 activation rate (rolling 30d) | stat | 0,24,12,4 | `subscription:trial_activation_day_30_rate`; thresholds red<0.30 yellow<0.50 green≥0.50; unit `percentunit` |
| 5 | Trials expiring in next 7d | stat | 12,24,12,4 | `subscription_trial_expiring_7d` |

- [ ] **Commit**

```bash
git add tesserix-infra/grafana/dashboards/trial-activation-funnel.json
git commit -m "feat(observability): 'Trial Activation Funnel' Grafana dashboard"
```

---

## Task 9: "Arbitrage Operations" dashboard

**Files:**
- Create: `tesserix-infra/grafana/dashboards/arbitrage-operations.json`

**Top-level:** `uid: "arbitrage-ops-v1"`, `title: "Arbitrage Operations"`, `tags: ["subscription","abuse","ops"]`, `time: { "from":"now-30d","to":"now" }`, `refresh: "1m"`.

**Panels (4 total):**

| id | title | type | gridPos | expr(s) |
|---|---|---|---|---|
| 1 | Flags per week | timeseries | 0,0,12,8 | `increase(subscription_arbitrage_flagged[7d])` |
| 2 | Resolution mix (30d) | piechart | 12,0,12,8 | 3 targets: `increase(subscription_arbitrage_false_positive_cleared[30d])` "Cleared", `increase(subscription_arbitrage_blocked_total[30d])` "Blocked", `subscription_arbitrage_appeal_queue_depth` "Pending" |
| 3 | Flag rate vs baseline | timeseries | 0,8,24,8 | 3 targets: `subscription:arbitrage_flag_rate_1h` "1h rate", `subscription:arbitrage_flag_baseline_24h` "24h baseline", `5 * subscription:arbitrage_flag_baseline_24h` "5× threshold" |
| 4 | False-positive ratio (7d) | stat | 0,16,12,4 | `sum(rate(subscription_arbitrage_false_positive_cleared[7d])) / clamp_min(sum(rate(subscription_arbitrage_flagged[7d])), 1)`; thresholds green<0.30 yellow<0.60 red≥0.60; unit `percentunit` |

- [ ] **Commit**

```bash
git add tesserix-infra/grafana/dashboards/arbitrage-operations.json
git commit -m "feat(observability): 'Arbitrage Operations' Grafana dashboard"
```

---

## Task 10: Stripe reconciliation CronJob

**Files:**
- Create: `services/marketplace-api/internal/reconciliation/stripe_drift{,_test}.go`
- Create: `services/marketplace-api/cmd/reconciliation-cron/main.go`
- Create: `tesserix-infra/k8s/apps/marketplace/marketplace-api/reconciliation-cronjob.yaml`

**Purpose:** Daily job LEFT JOIN's Stripe active subscriptions vs `store_subscriptions.stripe_subscription_id`. Each mismatch increments `reconciliation_drift{source="stripe"}`. Alerts fire per Task 4 when drift > 0.1% of active subs.

- [ ] **Step 1: Failing test**

```go
//go:build integration

func TestStripeDriftCheck_FlagsMismatch(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    reg := prometheus.NewRegistry()
    subs := metrics.NewSubscriptionCollectors(reg)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_local_only",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    fakeStripe := &fakeStripeLister{subs: []string{"sub_stripe_only"}}
    checker := reconciliation.NewStripeDriftChecker(db, fakeStripe, subs)
    require.NoError(t, checker.Run(context.Background()))

    drift := testutil.ToFloat64(subs.DR.ReconciliationDrift.WithLabelValues("stripe"))
    require.Equal(t, float64(2), drift,
        "sub_local_only (absent in Stripe) + sub_stripe_only (absent in DB) should both count")
}
```

- [ ] **Step 2: Implementation**

```go
package reconciliation

type StripeLister interface {
    ListActive(ctx context.Context) ([]string, error)
}

type StripeDriftChecker struct {
    db *gorm.DB; stripe StripeLister; metrics *metrics.SubscriptionCollectors
}

func NewStripeDriftChecker(db *gorm.DB, s StripeLister, m *metrics.SubscriptionCollectors) *StripeDriftChecker {
    return &StripeDriftChecker{db: db, stripe: s, metrics: m}
}

func (c *StripeDriftChecker) Run(ctx context.Context) error {
    stripeIDs, err := c.stripe.ListActive(ctx)
    if err != nil { return fmt.Errorf("stripe list: %w", err) }
    inStripe := toSet(stripeIDs)

    var dbIDs []string
    if err := c.db.WithContext(ctx).Raw(`
        SELECT stripe_subscription_id FROM store_subscriptions
        WHERE status IN ('active','past_due','payment_action_required','cancel_scheduled','trialing')
          AND stripe_subscription_id IS NOT NULL`).Pluck("stripe_subscription_id", &dbIDs).Error; err != nil {
        return fmt.Errorf("db pluck: %w", err)
    }
    inDB := toSet(dbIDs)

    var drift int
    for id := range inDB  { if _, ok := inStripe[id]; !ok { drift++ } }
    for id := range inStripe { if _, ok := inDB[id]; !ok { drift++ } }
    for i := 0; i < drift; i++ {
        c.metrics.DR.ReconciliationDrift.WithLabelValues("stripe").Inc()
    }
    return nil
}

// Live Stripe lister uses stripe-go v76 sub.List with Status="active" + limit=100 pagination.
// See sibling file stripe_lister_live.go.
```

- [ ] **Step 3: `cmd/reconciliation-cron/main.go`** — builds db + stripe client + metrics, invokes `checker.Run`, then `billing_archive` expiry recorder (Task 11). On success pushes to Pushgateway:

```go
pusher := push.New("http://pushgateway.monitoring.svc.cluster.local:9091", "reconciliation-cron").Gatherer(reg)
if err := pusher.Push(); err != nil { slog.Error("pushgateway push failed", "err", err) }
```

- [ ] **Step 4: CronJob manifest**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: reconciliation-cron
  namespace: marketplace
spec:
  schedule: "30 3 * * *"         # 03:30 UTC daily — after Stripe's batch
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 5
  jobTemplate:
    spec:
      backoffLimit: 2
      template:
        spec:
          restartPolicy: OnFailure
          serviceAccountName: marketplace-api
          containers:
            - name: reconciliation-cron
              image: asia-south1-docker.pkg.dev/tesserix-app/services/marketplace-api:latest
              command: ["/app/reconciliation-cron"]
              envFrom: [{ secretRef: { name: marketplace-api-secrets } }]
              resources:
                requests: { cpu: 100m, memory: 256Mi }
                limits:   { cpu: 500m, memory: 512Mi }
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/reconciliation/ \
        services/marketplace-api/cmd/reconciliation-cron/ \
        tesserix-infra/k8s/apps/marketplace/marketplace-api/reconciliation-cronjob.yaml
git commit -m "feat(reconciliation): daily Stripe drift check + reconciliation.drift counter (§22.3)"
```

---

## Task 11: billing_archive expiry gauge

**Files:**
- Create: `services/marketplace-api/internal/reconciliation/billing_archive_expiry.go`

Invoked from `reconciliation-cron` `main.go` after the Stripe drift check.

```go
// BillingArchiveExpiryRecorder.Run sets the billing_archive_expiry_soon gauge.
func (r *BillingArchiveExpiryRecorder) Run(ctx context.Context) error {
    var count int64
    threshold := time.Now().Add(30 * 24 * time.Hour)
    if err := r.db.WithContext(ctx).Raw(`
        SELECT COUNT(*) FROM billing_archive
        WHERE archive_expires_at <= ? AND archive_expires_at > now()`,
        threshold).Scan(&count).Error; err != nil {
        return err
    }
    r.metrics.DR.BillingArchiveSoon.Set(float64(count))
    return nil
}
```

Constructor `NewBillingArchiveExpiryRecorder(db, metrics)` is the standard shape. **Test:** `TestBillingArchiveExpiry_Counts30dWindow` — inserts rows at 10d/25d/60d; expect gauge = 2.

- [ ] **Commit**

```bash
git add services/marketplace-api/internal/reconciliation/billing_archive_expiry.go
git commit -m "feat(reconciliation): billing_archive 30d expiry gauge"
```

---

## Task 12: Log-shipping Cloud Logging smoke

**Files:**
- Create: `scripts/ci/verify-cloud-logging.sh`
- Modify: `.github/workflows/` in marketplace-api — add post-deploy smoke step

P1 already emits structured `subscription.state_transition` logs via `EmitStateTransition`. This task proves they reach Cloud Logging.

- [ ] **Step 1: Smoke script** — `scripts/ci/verify-cloud-logging.sh`. Flow:
  1. Generate `TRACE_ID=$(uuidgen)`
  2. `curl -fsS -X POST https://marketplace-api.tesserix.internal/_debug/emit-state-transition` with header `X-Trace-Id: $TRACE_ID`, bearer token, body `{"from":"active","to":"cancel_scheduled","actor":"test:ci","reason":"observability_smoke"}`
  3. `sleep 15` (Cloud Logging ingestion lag)
  4. `gcloud logging read --project="$CLOUD_LOGGING_PROJECT" --format=json --limit=1 "jsonPayload.trace_id=\"$TRACE_ID\" AND jsonPayload.event=\"subscription.state_transition\""`
  5. Assert result non-empty; `jq -e '.[] | select(.jsonPayload.from_status=="active" and .jsonPayload.to_status=="cancel_scheduled")'`
  6. Exit 1 on any miss, print `PASS` otherwise.

- [ ] **Step 2: `_debug/emit-state-transition` endpoint** — guarded behind `ENABLE_DEBUG_ENDPOINTS=true` build flag + P13 break-glass internal auth header. Production never sets the flag.

- [ ] **Step 3: CI wiring**

```yaml
- name: Cloud Logging smoke
  run: bash scripts/ci/verify-cloud-logging.sh
  env:
    CLOUD_LOGGING_PROJECT: tesserix-app-staging
    TEST_TOKEN: ${{ secrets.CI_SMOKE_BEARER_STAGING }}
```

- [ ] **Step 4: Commit**

```bash
git add scripts/ci/verify-cloud-logging.sh .github/workflows/post-deploy-smoke.yml
git commit -m "test(observability): post-deploy Cloud Logging smoke for state-transition logs"
```

---

## Task 13: CI PII-leak guard

**Files:**
- Create: `.github/workflows/pii-leak-guard.yml`
- Create: `scripts/ci/grep-pii-logs.sh`

**Purpose:** Block PRs that add log statements with suspected PII fields. Conservative grep over diff-added lines only.

- [ ] **Step 1: Grep script**

```bash
#!/usr/bin/env bash
# scripts/ci/grep-pii-logs.sh
set -euo pipefail

BASE_SHA="${GITHUB_BASE_REF:-origin/main}"
DIFF=$(git diff --unified=0 "${BASE_SHA}"...HEAD -- '*.go')

PII_FIELDS='(?:email|phone|tax[_-]?id|card|cvv|address|passport|ssn|dob|date[_-]?of[_-]?birth|national[_-]?id|pan[_-]?number|aadhaar|password|token|refresh[_-]?token|access[_-]?token)'
LOG_CALL='(?:slog\.(?:Debug|Info|Warn|Error)|logrus\.(?:Debug|Info|Warn|Error)|WithField\s*\(|Logger\.(?:Info|Warn|Error|Debug))'

VIOLATIONS=$(echo "${DIFF}" | grep -E "^\+" | grep -E "${LOG_CALL}" | grep -iE "${PII_FIELDS}" || true)

if [[ -n "${VIOLATIONS}" ]]; then
    echo "FAIL: possible PII leak in log statements:"
    echo "${VIOLATIONS}"
    echo ""
    echo "False positive (hashed value)? Apply PR label 'pii-audit-cleared' and re-run CI."
    exit 1
fi
echo "PASS: no suspected PII in added log lines."
```

- [ ] **Step 2: Workflow**

```yaml
name: PII Leak Guard
on:
  pull_request:
    paths: ['services/**/*.go']

jobs:
  pii-grep:
    runs-on: ubuntu-latest
    if: "!contains(github.event.pull_request.labels.*.name, 'pii-audit-cleared')"
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - name: Grep for PII in added log lines
        run: bash scripts/ci/grep-pii-logs.sh
```

- [ ] **Step 3: Escape hatch** — label `pii-audit-cleared` skips the job. Reviewer must verify the flagged line is truly safe (e.g. hashed value) before applying.

- [ ] **Step 4: Local sanity check**

```bash
# Expected FAIL:
echo '+slog.Info("signed up", "email", u.Email)' | bash scripts/ci/grep-pii-logs.sh || echo "failed as expected"
# Expected PASS:
echo '+slog.Info("signed up", "user_hash", hash(u.Email))' | bash scripts/ci/grep-pii-logs.sh
```

- [ ] **Step 5: Commit**

```bash
git add scripts/ci/grep-pii-logs.sh .github/workflows/pii-leak-guard.yml
git commit -m "ci(security): PII-leak guard — grep added log lines for sensitive fields"
```

---

## Task 14: Dashboard schema + PromQL reference validation

**Files:**
- Create: `scripts/ci/verify-dashboard-queries.sh`
- Modify: `.github/workflows/ci.yml` (marketplace-api)

- [ ] **Step 1: Schema check in CI**

```yaml
- name: Validate Grafana dashboards
  run: |
    for f in tesserix-infra/grafana/dashboards/*.json; do
      echo "Validating ${f}"
      jq empty "${f}" || exit 1
      jq -e '.uid and .title and (.schemaVersion >= 37) and (.version | type == "number") and .panels' "${f}" \
        || { echo "Missing required field in ${f}"; exit 1; }
      if grep -iE 'todo|fixme|wip' "${f}" > /dev/null; then
        echo "FAIL: marker in ${f}"; exit 1
      fi
    done
    bash scripts/ci/verify-dashboard-queries.sh
```

- [ ] **Step 2: PromQL reference check** — `scripts/ci/verify-dashboard-queries.sh` extracts every metric name referenced by dashboard JSON (`jq -r '.panels[].targets[].expr' ... | grep -oE '[a-z_][a-z0-9_:]*' | sort -u`) and asserts each is in either `KNOWN_METRICS` (the full list from Task 1 — every collector Name) or `RECORDING_RULES` (the list from Task 4's `subscription.recording` group). PromQL keywords + label values (`sum|rate|increase|histogram_quantile|clamp_min|topk|time|label_values|by|le|count|max|min|avg|status|active|past_due|payment_action_required|cancel_scheduled|trialing|signup|to|from|event_type|reason|invalid_signature|attempt|plan|currency|source|stripe|cnpg|gcs`) short-circuit via regex. Any token not matching either list fails CI with `UNKNOWN: <ref>`. Run locally: `bash scripts/ci/verify-dashboard-queries.sh`.

- [ ] **Step 3: Commit**

```bash
git add scripts/ci/verify-dashboard-queries.sh .github/workflows/ci.yml
git commit -m "ci(observability): validate dashboard JSON + PromQL reference set"
```

---

## Task 15: ArgoCD sync for rules + dashboards

**Files:**
- Create: `tesserix-infra/k8s/cluster/grafana/dashboards/kustomization.yaml`
- Modify: `tesserix-infra/k8s/argocd/appsets/cluster-addons.yaml`

- [ ] **Step 1: ArgoCD Application for PrometheusRule + Alertmanager**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: subscription-observability
  namespace: argocd
spec:
  project: cluster-addons
  source:
    repoURL: https://github.com/tesserix/tesserix-infra.git
    targetRevision: main
    path: k8s/cluster/prometheus/rules
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [ServerSideApply=true]
```

- [ ] **Step 2: Grafana dashboard ConfigMap via Kustomize**

```yaml
# tesserix-infra/k8s/cluster/grafana/dashboards/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: monitoring
configMapGenerator:
  - name: subscription-dashboards
    options:
      labels:
        grafana_dashboard: "1"
    files:
      - ../../../../grafana/dashboards/subscription-health.json
      - ../../../../grafana/dashboards/webhook-pipeline.json
      - ../../../../grafana/dashboards/trial-activation-funnel.json
      - ../../../../grafana/dashboards/arbitrage-operations.json
generatorOptions:
  disableNameSuffixHash: true
```

Grafana's sidecar (`grafana-sc-dashboard`) watches ConfigMaps with label `grafana_dashboard=1` and auto-loads them.

- [ ] **Step 3: Post-sync verification**

```bash
argocd app sync subscription-observability --timeout 300
kubectl -n monitoring get prometheusrule subscription-rules -o yaml | grep 'alertname' | head -5
kubectl -n monitoring get configmap subscription-dashboards -o jsonpath='{.data}' | jq 'keys'
# Expected: ["arbitrage-operations.json","subscription-health.json","trial-activation-funnel.json","webhook-pipeline.json"]
kubectl -n monitoring logs deploy/grafana -c grafana-sc-dashboard | grep -i 'subscription'
```

- [ ] **Step 4: Commit**

```bash
git add tesserix-infra/k8s/argocd/ tesserix-infra/k8s/cluster/grafana/dashboards/kustomization.yaml
git commit -m "infra(observability): ArgoCD sync for PrometheusRule + Grafana dashboards"
```

---

## Final verification

- [ ] `go build ./...` in `services/marketplace-api` — clean.
- [ ] `go test -tags=integration ./internal/metrics/... ./internal/reconciliation/...` — green.
- [ ] `promtool test rules tesserix-infra/k8s/cluster/prometheus/rules/subscription_test.yaml` — green.
- [ ] `amtool config check tesserix-infra/k8s/cluster/alertmanager/routes.yaml` — clean.
- [ ] `jq empty tesserix-infra/grafana/dashboards/*.json` — all 4 parse.
- [ ] `bash scripts/ci/verify-dashboard-queries.sh` — PASS.
- [ ] `argocd app sync subscription-observability` — synced + healthy.
- [ ] Grafana `/d/subscription-health-v1` — all 8 panels render with data.
- [ ] Grafana `/d/webhook-pipeline-v1` — orphan queue depth shows a recent value.
- [ ] Grafana `/d/trial-funnel-v1` — funnel bars render descending.
- [ ] Grafana `/d/arbitrage-ops-v1` — baseline vs 1h rate plotted.
- [ ] Synthetic `BreakGlassAdminUsed` in staging → message lands in `#security-alerts`.
- [ ] Synthetic `OrphanWebhookUnresolved` → PagerDuty test incident fires.
- [ ] `reconciliation-cron` in staging emits `reconciliation_drift{source="stripe"} == 0` when Stripe + DB agree.
- [ ] PII-leak guard: test branch adding `slog.Info("signup", "email", e.Email)` — CI fails as expected.
- [ ] `scripts/ci/verify-cloud-logging.sh` in staging — PASS.

## What's now unlocked

- **P18** (capacity planning) derives per-tier MRR / merchant counts from `subscription:active_count:by_plan` + `subscription_mrr_usd_rollup`.
- **P19** (SLO formalisation) layers burn-rate alerts on the same recording rules.
- **P20** (runbooks) binds each alert's `runbook` label to a page under `runbooks.tesserix.internal/subscription/<alert-name>`.
- **P21** (weekly business review) auto-generates a Notion page from "Subscription Health" headlines.

## Execution handoff

P17 completes the observability plane for the v2.3 subscription subsystem:
- Every spec-named metric lives in one Go package with typed accessors.
- Every spec-named alert is codified as a Prometheus rule with a routed Alertmanager receiver.
- Four version-controlled Grafana dashboards cover business, ops, growth, and abuse perspectives.
- Stripe drift reconciliation + billing_archive expiry + Cloud Logging smoke cover the DR surface (§22).
- CI gates block PRs that leak PII into logs or reference undeclared metrics in dashboards.

Hand off to **P18** (capacity planning), which consumes the rollup gauges.
