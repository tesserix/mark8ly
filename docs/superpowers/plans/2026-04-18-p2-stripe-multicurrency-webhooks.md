# P2 — Stripe Multi-Currency + Webhook Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the raw-HTTP Stripe shim with a hardened multi-currency billing client — 8 `currency_options`-based Price objects, server-generated idempotency keys everywhere, a dedicated webhook dispatcher backed by the `stripe_webhook_events` table from P1, orphan re-attempt and dead-letter crons with the §17.7 SLA, and log sanitization that closes the current PII leak.

**Architecture:** `internal/payment/stripe.go` is split into a per-resource client (`internal/billing/stripe/{customer,price,checkout,portal,subscription,invoice,webhook}.go`), each with sanitized errors. Price objects are bootstrapped once via a one-shot job that reads `internal/billing/pricing/catalog.go` and POSTs idempotently to Stripe; the job is re-runnable (does nothing if prices exist). The webhook endpoint moves to `internal/handlers/webhooks/stripe.go`, verifies the signature on the raw request body, inserts into `stripe_webhook_events` with `ON CONFLICT (event_id) DO NOTHING`, then dispatches to per-event handlers inside a transaction that holds `pg_advisory_xact_lock(hashtext(store_id))` via `subscription.WithAdvisoryLock` (P1). `billing_currency` is bound on `checkout.session.completed` and frozen for the billing term. AU prices use `tax_behavior: exclusive` exclusively. Orphan events (store_id nullable) are re-attempted every 5 min up to 6 times (30 min) before flagging `manual_review_required = true`; PagerDuty fires at 1h unresolved.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL (CNPG), `net/http` (existing pattern — no Stripe Go SDK), `robfig/cron/v3` (already on the project), GCP Cloud Scheduler (optional for the orphan cron — local cron loop at v1), Stripe API v2026-01 (latest at time of plan).

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §4.2.1 (currency_options architecture), §4.2.2 (split-currency deferred), §17.6–17.8 (webhooks, idempotency, orphan SLA), §18.1 (PCI-A), §18.3 (secret mgmt), §18.5 (webhook hardening), §18.6 (log sanitization), §19.4 (AU tax_behavior exclusive), §4.7 (payment_action_required).

**Depends on:** P1 (data model, `stripe_webhook_events` table, `WithAdvisoryLock`, `EmitStateTransition`, `webhookevents.StripeWebhookEvent` GORM model).

**Related plans (NOT in scope here):**
- **P3** — state machine + plan gates (consumes webhook-produced transitions)
- **P5/P6** — trial card-add deferred-charge flow and dunning (consume this webhook dispatcher)
- **P9** — campaign email budget (orthogonal)

---

## Scope Check

In scope:
1. Pricing catalog (Go source of truth for all 13 currencies × 4 plans × 2 periods).
2. One-shot Stripe bootstrap job that creates/idempotently-updates Products + Prices.
3. Per-resource Stripe clients: Customer, Checkout Session, Customer Portal, Subscription read, Invoice read.
4. Idempotency keys on every outbound Stripe call (§17.8).
5. AU `tax_behavior: exclusive` explicit override (§19.4).
6. Webhook endpoint (HMAC-SHA256 signature verification on raw body, 512 KB MaxBytesReader, event-type allowlist).
7. `stripe_webhook_events` INSERT ON CONFLICT + per-event-type dispatcher.
8. `billing_currency` binding on `checkout.session.completed`.
9. Orphan re-attempt cron (5 min, 6-retry cap, 24h dead-letter) + manual-review flag.
10. PagerDuty alert path stub (POST to a configured URL; full alerting is observability plan P17).
11. Log sanitization: redact request bodies, Stripe error bodies, PII fields — keep only `error.code`, `error.type`, `event.id`, `event.type`.
12. Integration test suite against Stripe in `test mode` using Stripe's fixture cards + the `stripe listen` CLI for webhook signature generation.

Out of scope:
- State-machine transitions per webhook event → P3 wires `webhook → state-change` handlers.
- Trial-card-add deferred-charge logic → P5.
- `invoice.payment_action_required` handling and recovery → P6.
- Split-currency settlement accounts → §4.2.2 activation at $200k ARR (not v1).
- MRR normalization cron → P17 observability.
- Promo codes → P10.
- Refund flow → P10.

---

## File Structure

### Create

**Pricing catalog**
- `services/marketplace-api/internal/billing/pricing/catalog.go` — typed pricing table: developed + PPP markets × plan × period
- `services/marketplace-api/internal/billing/pricing/catalog_test.go`
- `services/marketplace-api/cmd/pricing-dump/main.go` — tiny CSV dumper used by Task 1 Step 5 for reviewer sanity check

**Stripe clients (split per resource)**
- `services/marketplace-api/internal/billing/stripe/client.go` — shared HTTP client + `doForm` + `Idempotency-Key` header helper + sanitized error types
- `services/marketplace-api/internal/billing/stripe/client_test.go`
- `services/marketplace-api/internal/billing/stripe/product.go` — CreateProduct / UpsertProduct by `metadata.plan`
- `services/marketplace-api/internal/billing/stripe/price.go` — CreatePrice with `currency_options`, list/lookup by `lookup_key`
- `services/marketplace-api/internal/billing/stripe/customer.go` — CreateCustomer (idempotency: `customer:<store_id>`)
- `services/marketplace-api/internal/billing/stripe/checkout.go` — CreateCheckoutSession for subscriptions with `customer_update[shipping]` + `currency` + `locale`
- `services/marketplace-api/internal/billing/stripe/portal.go` — CreatePortalSession (idempotency: `portal:<store_id>:<5-min bucket>`)
- `services/marketplace-api/internal/billing/stripe/subscription.go` — Get + Cancel + Update (subscription ID flows)
- `services/marketplace-api/internal/billing/stripe/invoice.go` — Get + List (for reconciliation + `payment_action_required` URL retrieval)
- `services/marketplace-api/internal/billing/stripe/signature.go` — `VerifySignature(raw []byte, header string, secret string, now time.Time) (eventID, eventType string, err error)`
- `services/marketplace-api/internal/billing/stripe/logging.go` — `SanitizeError(err) error`, `SanitizeEvent(event) loggableFields`
- Unit tests for each file: `*_test.go`

**Bootstrap job**
- `services/marketplace-api/cmd/billing-bootstrap/main.go` — idempotent CLI that creates Products + Prices in Stripe; safe to re-run

**Webhook dispatch**
- `services/marketplace-api/internal/handlers/webhooks/stripe.go` — Gin handler mounted at `/webhooks/stripe-billing`
- `services/marketplace-api/internal/handlers/webhooks/stripe_test.go`
- `services/marketplace-api/internal/billing/dispatch/dispatcher.go` — `Dispatch(ctx, event)` routes event-type → handler func
- `services/marketplace-api/internal/billing/dispatch/handlers.go` — minimal per-event handlers that only update `store_subscriptions` columns + emit audit event; state transitions stay in P3
- `services/marketplace-api/internal/billing/dispatch/orphan_resolver.go` — queries for `store_id IS NULL` events + looks up via `stripe_customer_id`
- `services/marketplace-api/internal/billing/dispatch/cron.go` — `StartOrphanCron(ctx, db, interval)` using `robfig/cron/v3`
- `services/marketplace-api/internal/billing/dispatch/*_test.go`

**Webhook events repo**
- `services/marketplace-api/internal/webhookevents/repository.go` — `InsertIfNew`, `GetUnprocessedOrphans`, `MarkProcessed`, `IncrementRetry`, `FlagManualReview`
- `services/marketplace-api/internal/webhookevents/repository_test.go`

### Modify

- `services/marketplace-api/internal/subscription/service.go` — delegate to new `billing/stripe` clients; drop the current `StripeClient` interface
- `services/marketplace-api/internal/handlers/admin/subscription.go` — use new clients; thread `billing_currency` selection through `CreateCheckout`
- `services/marketplace-api/internal/handlers/admin/routes.go` — remove inline webhook route (moves to `handlers/webhooks/stripe.go`)
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire new webhook handler + start orphan cron
- `services/marketplace-api/pkg/config/config.go` — add `StripeAllowedEventTypes`, `WebhookMaxBodyBytes`, `OrphanRetryMaxCount`, `OrphanRetryIntervalSeconds`, `PagerDutyWebhookURL`

### Delete / deprecate

- `services/marketplace-api/internal/payment/stripe.go` — legacy raw-HTTP shim used by orders flow remains for now but is **not** used by subscription billing. Flag it with a file-level comment pointing to `internal/billing/stripe/*` for new billing work.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Pricing catalog + tests | — |
| 2 | Stripe shared client + idempotency helper + log sanitization | — |
| 3 | Product + Price clients + `currency_options` | 1, 2 |
| 4 | Bootstrap CLI + idempotent Stripe bootstrap | 3 |
| 5 | Customer + Checkout clients + AU `tax_behavior: exclusive` | 2 |
| 6 | Customer Portal client with 5-min bucket idempotency | 2 |
| 7 | Subscription + Invoice clients (reads only in P2) | 2 |
| 8 | Signature verification on raw body | 2 |
| 9 | `webhookevents.Repository` | — (P1 gave the model) |
| 10 | Webhook handler (`handlers/webhooks/stripe.go`) | 8, 9 |
| 11 | Event dispatcher + per-event handlers | 9, 10 |
| 12 | `billing_currency` binding on `checkout.session.completed` | 11 |
| 13 | Orphan resolver | 9, 11 |
| 14 | Orphan cron + dead-letter + PagerDuty hook | 13 |
| 15 | Wire everything into `main.go` + remove legacy handler webhook | 10, 14 |
| 16 | Integration test suite (Stripe test mode + signed fixtures) | all |

Each task is one atomic commit boundary.

---

## Reusable patterns

**A. Idempotency-Key header** — Stripe accepts `Idempotency-Key: <server-generated-value>` on any POST. The key is server-generated, cacheable for 24h. §17.8 specifies:
- `customer:<store_id>` (never retried with same key)
- `checkout:<store_id>:<plan>:<period>:<day_bucket>` — `day_bucket = floor(unix_ts / 86400)`
- `subscription:<store_id>:<plan>:<billing_period>`
- `portal:<store_id>:<5_min_bucket>` — `5_min_bucket = floor(unix_ts / 300)`
- `refund:<invoice_id>` (P10)

**B. Signature verification** — Stripe sends `Stripe-Signature: t=<ts>,v1=<sig>,v1=<sig>…`. The signed payload is `<ts>.<raw_body>`. Reject if `|now - t| > 5 min`. Use `hmac.Equal` for constant-time compare. Existing `VerifyWebhook` in `internal/payment/stripe.go:268` is a valid reference implementation — we move it to the new location + add unit tests.

**C. `currency_options` shape** — a single Price object holds `unit_amount` in a default currency plus a `currency_options` map keyed by ISO currency. Stripe enforces that all currencies on one Price ID quote the same plan tier — that's why PPP markets use **separate** Price IDs (PPP `unit_amount` for India can't coexist with developed-market `unit_amount` on one Price).

**D. AU `tax_behavior: exclusive`** — AU Prices must set `tax_behavior=exclusive`. Stripe default is `inclusive`. **Never skip this override** (§19.4). The bootstrap job sets it; our integration test asserts the final Price has the right behavior.

**E. Request size cap** — every Stripe webhook endpoint uses `r.Body = http.MaxBytesReader(w, r.Body, 512<<10)` to reject >512 KB payloads (§18.5).

**F. Sanitized error** — wrap every Stripe error as:
```go
type StripeAPIError struct {
    Code       string // from error.code
    Type       string // from error.type ("invalid_request_error", ...)
    HTTPStatus int
    // No body, no headers, no request payload.
}
func (e *StripeAPIError) Error() string {
    return fmt.Sprintf("stripe: %s/%s (status %d)", e.Type, e.Code, e.HTTPStatus)
}
```

**G. Audit-event emission** — every webhook handler that mutates state ends with `audit.EmitStateTransition(c, ...)` (from P1 Task 14). Non-mutating webhook acks emit a generic `audit.webhook_received` instead.

---

## Task 1: Pricing catalog

**Files:**
- Create: `services/marketplace-api/internal/billing/pricing/catalog.go`
- Create: `services/marketplace-api/internal/billing/pricing/catalog_test.go`

**Spec references:** §4.1, §4.1.1 (PPP), §4.1.2 (app add-on USD only), §9 feature matrix.

- [ ] **Step 1: Write failing test**

Create `internal/billing/pricing/catalog_test.go`:

```go
package pricing_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/pricing"
)

func TestCatalog_DevelopedStarterMonthly_19USD(t *testing.T) {
    p, ok := pricing.LookupBaseline(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierDeveloped)
    require.True(t, ok)
    require.Equal(t, int64(1900), p.UnitAmountMinor) // $19.00 in cents
    require.Equal(t, "usd", p.Currency)
}

func TestCatalog_IndiaPPPStarterMonthly_999INR(t *testing.T) {
    p, ok := pricing.LookupPPPOption(pricing.PlanStarter, pricing.PeriodMonthly, "inr")
    require.True(t, ok)
    require.Equal(t, int64(99900), p.UnitAmountMinor) // ₹999.00 in paise
}

func TestCatalog_CurrencyOptionsConsistency(t *testing.T) {
    // Developed Price object must have currency_options for every developed currency.
    opts, ok := pricing.DevelopedCurrencyOptions(pricing.PlanStudio, pricing.PeriodAnnual)
    require.True(t, ok)
    expected := []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"}
    for _, c := range expected {
        _, present := opts[c]
        require.True(t, present, "developed Price must carry currency_options for %s", c)
    }
}

func TestCatalog_AUTaxBehaviorExclusive(t *testing.T) {
    opts, _ := pricing.DevelopedCurrencyOptions(pricing.PlanStarter, pricing.PeriodMonthly)
    au := opts["aud"]
    require.Equal(t, "exclusive", au.TaxBehavior, "AU Price options must be tax_behavior=exclusive (§19.4)")
}

func TestCatalog_ProMonthlyPremium_20Percent(t *testing.T) {
    annualMo := pricing.MustGet(pricing.PlanPro, pricing.PeriodAnnual, "usd").UnitAmountMinor / 12
    monthly  := pricing.MustGet(pricing.PlanPro, pricing.PeriodMonthly, "usd").UnitAmountMinor
    ratio := float64(monthly) / float64(annualMo)
    require.InDelta(t, 1.20, ratio, 0.01, "Pro monthly must be ~+20%% of annual-equivalent monthly")
}
```

- [ ] **Step 2: Run — expect build failure**

```bash
cd services/marketplace-api
go test ./internal/billing/pricing/... -v
```

- [ ] **Step 3: Write `catalog.go`**

```go
package pricing

// Plan is the billing plan name; mirrors subscription.SubscriptionPlan.
type Plan string

const (
    PlanStarter Plan = "starter"
    PlanStudio  Plan = "studio"
    PlanPro     Plan = "pro"
    // PlanTrial is no-charge; no Price object.
    // PlanMarketplace is hidden; no Price object in v1.
)

type Period string

const (
    PeriodMonthly Period = "monthly"
    PeriodAnnual  Period = "annual"
)

type Tier string

const (
    TierDeveloped Tier = "developed"
    TierPPP       Tier = "ppp"
)

// Amount is a minor-unit currency amount plus optional per-currency tax behavior.
// For AU, TaxBehavior must be "exclusive" (§19.4); elsewhere "unspecified" (Stripe default).
type Amount struct {
    Currency        string // lowercase ISO 4217
    UnitAmountMinor int64  // cents, paise, sen, etc.
    TaxBehavior     string // "exclusive" | "unspecified"
}

type PriceDescriptor struct {
    Plan       Plan
    Period     Period
    Tier       Tier
    Currency   string          // baseline currency for the Stripe Price (usd for developed, native for PPP)
    Baseline   Amount          // default unit_amount
    Options    map[string]Amount // currency_options keyed by lowercase currency
    LookupKey  string          // Stripe price.lookup_key for bootstrap idempotency
}

// Canonical table. Source of truth for every Stripe Price object.
// All minor units. Annual = 12 * monthly * 0.80 rounded to native convention where needed.
var developedMonthlyStarter = PriceDescriptor{
    Plan: PlanStarter, Period: PeriodMonthly, Tier: TierDeveloped, Currency: "usd",
    Baseline: Amount{Currency: "usd", UnitAmountMinor: 1900},
    LookupKey: "mark8ly_starter_monthly_developed_v1",
    Options: map[string]Amount{
        "usd": {Currency: "usd", UnitAmountMinor: 1900},
        "cad": {Currency: "cad", UnitAmountMinor: 2500},
        "gbp": {Currency: "gbp", UnitAmountMinor: 1500},
        "eur": {Currency: "eur", UnitAmountMinor: 1700},
        "aud": {Currency: "aud", UnitAmountMinor: 2900, TaxBehavior: "exclusive"},
        "nzd": {Currency: "nzd", UnitAmountMinor: 2900},
        "sgd": {Currency: "sgd", UnitAmountMinor: 2500},
    },
}
// ... (full table: starter annual, studio monthly/annual, pro monthly/annual — all developed)
// Pro monthly: $119 USD (not $99). Annual: $1188 → $99/mo equivalent, 20% premium encoded in monthly.

var pppMonthlyStarter = []PriceDescriptor{
    {Plan: PlanStarter, Period: PeriodMonthly, Tier: TierPPP, Currency: "inr",
        Baseline: Amount{Currency: "inr", UnitAmountMinor: 99900},
        LookupKey: "mark8ly_starter_monthly_ppp_inr_v1",
        Options: map[string]Amount{"inr": {Currency: "inr", UnitAmountMinor: 99900}}},
    {Plan: PlanStarter, Period: PeriodMonthly, Tier: TierPPP, Currency: "myr",
        Baseline: Amount{Currency: "myr", UnitAmountMinor: 5900},
        LookupKey: "mark8ly_starter_monthly_ppp_myr_v1",
        Options: map[string]Amount{"myr": {Currency: "myr", UnitAmountMinor: 5900}}},
    // ... thb, php, idr, vnd
}
// ... mirror for studio + pro across all PPP currencies

// Lookup helpers.
func LookupBaseline(p Plan, period Period, tier Tier) (Amount, bool) { /* ... */ }
func LookupPPPOption(p Plan, period Period, currency string) (Amount, bool) { /* ... */ }
func DevelopedCurrencyOptions(p Plan, period Period) (map[string]Amount, bool) { /* ... */ }

// MustGet panics in tests when the row is missing; use LookupBaseline in production.
func MustGet(p Plan, period Period, currency string) Amount {
    if a, ok := LookupPPPOption(p, period, currency); ok {
        return a
    }
    if opts, ok := DevelopedCurrencyOptions(p, period); ok {
        if a, ok := opts[currency]; ok {
            return a
        }
    }
    panic("pricing: missing amount for " + string(p) + "/" + string(period) + "/" + currency)
}

// AllDescriptors returns every PriceDescriptor — used by the bootstrap job.
func AllDescriptors() []PriceDescriptor { /* concatenate developed + ppp */ }
```

Fill the tables carefully against §4.1 and §4.1.1. Every amount lives as a constant in this file; there must be **no** duplication in the bootstrap job.

**Pro monthly rule (explicit, because §4.1 only tabulates annual):** For every currency, `Pro monthly = round(annual_price / 12 × 1.20)` rounded to a sensible native-currency increment. Concretely:
- USD: `$1,188 / 12 × 1.20 = $118.80 → $119` (matches spec §3).
- CAD: `C$1,619 / 12 × 1.20 = C$161.90 → C$162` (or C$165 if rounding to nearest 5).
- GBP: `£948 / 12 × 1.20 = £94.80 → £95`.
- EUR: `€1,068 / 12 × 1.20 = €106.80 → €107`.
- AUD: `A$1,788 / 12 × 1.20 = A$178.80 → A$179` (GST-exclusive).
- NZD: `NZ$1,788 / 12 × 1.20 = NZ$179`.
- SGD: `S$1,548 / 12 × 1.20 = S$154.80 → S$155`.
- INR: `₹65,999 / 12 × 1.20 = ₹6,599.90 → ₹6,599` (matches `₹5,499/mo eq × 1.20 = ₹6,598.80`).
- MYR/THB/PHP/IDR/VND: apply the same formula; round to the nearest native increment that the developed-tier table uses.

If a rounded value lands within 1% of the next "cleaner" increment (e.g. $119 vs $120), prefer the cleaner increment but preserve the +20% guarantee — the test `TestCatalog_ProMonthlyPremium_20Percent` uses `InDelta 0.01`, which gives you room to round up.

**Success criterion 43** asserts Pro monthly at $119; use that test as the source of truth for USD. Extend the same test across every currency so rounding errors don't slip through.

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/billing/pricing/... -v
```

- [ ] **Step 5: Snapshot the catalog**

Generate a human-readable dump for reviewer sanity check:

```bash
go run ./cmd/pricing-dump > /tmp/pricing-v1.txt
```

Write a tiny `services/marketplace-api/cmd/pricing-dump/main.go` that prints every descriptor as CSV (plan,period,tier,currency,unit_amount,tax_behavior,lookup_key) — one line per currency_option within each descriptor. Include the file in this commit. Keep `/tmp/pricing-v1.txt` out of git — it's a one-shot artifact for the reviewer.

Visually compare against spec §4.1 + §4.1.1. Fix discrepancies.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/pricing/
git commit -m "feat(billing): add pricing catalog (8 prices × developed currencies + PPP)"
```

---

## Task 2: Shared Stripe HTTP client + idempotency + log sanitization

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/client.go`
- Create: `services/marketplace-api/internal/billing/stripe/logging.go`
- Create: `services/marketplace-api/internal/billing/stripe/client_test.go`
- Create: `services/marketplace-api/internal/billing/stripe/logging_test.go`

**Spec references:** §17.8 (idempotency), §18.1 (PCI-A), §18.6 (log sanitization).

- [ ] **Step 1: Write failing test for sanitized errors**

```go
package stripe_test

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/stripe"
)

func TestSanitizeError_StripsRequestBodyAndResponseBody(t *testing.T) {
    raw := `{"error":{"code":"card_declined","type":"card_error","charge":"ch_x","message":"Your card was declined: 4242..."}}`
    err := stripe.ParseAPIError(402, raw, "req_ABC")

    var apiErr *stripe.APIError
    require.True(t, errors.As(err, &apiErr))
    require.Equal(t, "card_declined", apiErr.Code)
    require.Equal(t, "card_error", apiErr.Type)
    require.Equal(t, "req_ABC", apiErr.RequestID)

    // PII-safe Error() form: no message, no charge ID, no body.
    msg := apiErr.Error()
    require.NotContains(t, msg, "4242")
    require.NotContains(t, msg, "declined:")
    require.NotContains(t, msg, "ch_x")
}

func TestIdempotencyKey_PortalBucket5Min(t *testing.T) {
    // Same store_id in the same 5-min bucket yields same key.
    a := stripe.PortalIdempotencyKey("store-1", 1_712_000_050)
    b := stripe.PortalIdempotencyKey("store-1", 1_712_000_200)
    require.Equal(t, a, b)

    // Different bucket — different key.
    c := stripe.PortalIdempotencyKey("store-1", 1_712_000_500)
    require.NotEqual(t, a, c)
}

func TestIdempotencyKey_CheckoutDayBucket(t *testing.T) {
    a := stripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_000_000)
    b := stripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_086_000) // same UTC day
    require.Equal(t, a, b)

    c := stripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_100_000) // next day
    require.NotEqual(t, a, c)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/billing/stripe/... -v
```

- [ ] **Step 3: Write `client.go`**

```go
package stripe

import (
    "context"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)

const defaultAPIBase = "https://api.stripe.com"

// Client wraps the Stripe REST API with idempotency, sanitized errors,
// and a fixed timeout. No body is ever logged.
type Client struct {
    apiKey  string
    baseURL string
    http    *http.Client
    now     func() time.Time
}

func New(apiKey string) *Client {
    return &Client{
        apiKey:  apiKey,
        baseURL: defaultAPIBase,
        http:    &http.Client{Timeout: 30 * time.Second},
        now:     time.Now,
    }
}

// PostForm performs a POST with application/x-www-form-urlencoded body.
// idempotencyKey is required on every write; callers generate it deterministically.
func (c *Client) PostForm(ctx context.Context, path, idempotencyKey string, values url.Values) ([]byte, error) {
    if idempotencyKey == "" {
        return nil, errors.New("stripe: idempotency key required")
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(values.Encode()))
    if err != nil {
        return nil, fmt.Errorf("stripe: build request: %w", err)
    }
    req.SetBasicAuth(c.apiKey, "")
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    req.Header.Set("Idempotency-Key", idempotencyKey)
    req.Header.Set("Stripe-Version", "2026-01-01")

    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("stripe: do request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("stripe: read response: %w", err)
    }

    if resp.StatusCode >= 400 {
        return nil, ParseAPIError(resp.StatusCode, string(body), resp.Header.Get("Request-Id"))
    }
    return body, nil
}

// Get performs a read-only GET.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
    if err != nil {
        return nil, fmt.Errorf("stripe: build request: %w", err)
    }
    req.SetBasicAuth(c.apiKey, "")
    req.Header.Set("Stripe-Version", "2026-01-01")
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("stripe: do request: %w", err)
    }
    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("stripe: read response: %w", err)
    }
    if resp.StatusCode >= 400 {
        return nil, ParseAPIError(resp.StatusCode, string(body), resp.Header.Get("Request-Id"))
    }
    return body, nil
}

// Idempotency-key generators.
func CustomerIdempotencyKey(storeID string) string {
    return "customer:" + storeID
}

func CheckoutIdempotencyKey(storeID, plan, period string, unixTs int64) string {
    dayBucket := unixTs / 86400
    return fmt.Sprintf("checkout:%s:%s:%s:%d", storeID, plan, period, dayBucket)
}

func SubscriptionIdempotencyKey(storeID, plan, period string) string {
    return fmt.Sprintf("subscription:%s:%s:%s", storeID, plan, period)
}

// PortalIdempotencyKey — §17.8 Council finding #1: 5-min bucket matches Stripe
// portal URL lifetime. hour_bucket returned expired URLs.
func PortalIdempotencyKey(storeID string, unixTs int64) string {
    return fmt.Sprintf("portal:%s:%d", storeID, unixTs/300)
}

func RefundIdempotencyKey(invoiceID string) string {
    return "refund:" + invoiceID
}
```

- [ ] **Step 4: Write `logging.go` (sanitization)**

```go
package stripe

import (
    "encoding/json"
    "fmt"
)

// APIError is the sanitized, log-safe representation of a Stripe API error.
// It deliberately omits error.message, request/response bodies, card data.
type APIError struct {
    HTTPStatus int
    Type       string
    Code       string
    RequestID  string
}

func (e *APIError) Error() string {
    return fmt.Sprintf("stripe: %s/%s (status %d request_id=%s)", e.Type, e.Code, e.HTTPStatus, e.RequestID)
}

// ParseAPIError extracts code/type from a Stripe error body without preserving
// the body. Logs will only contain code + type + request_id.
func ParseAPIError(status int, body, requestID string) error {
    var parsed struct {
        Error struct {
            Code    string `json:"code"`
            Type    string `json:"type"`
            Message string `json:"message"` // intentionally NOT stored on APIError
        } `json:"error"`
    }
    // Best-effort JSON parse; on failure, return generic code to avoid leaking body.
    _ = json.Unmarshal([]byte(body), &parsed)
    return &APIError{
        HTTPStatus: status,
        Type:       parsed.Error.Type,
        Code:       parsed.Error.Code,
        RequestID:  requestID,
    }
}

// SafeWebhookFields extracts only event.id + event.type for structured logging.
// Use this at every webhook log site. Never log the raw event payload.
type SafeWebhookFields struct {
    EventID   string `json:"stripe_event_id"`
    EventType string `json:"stripe_event_type"`
}

func ExtractSafeFields(rawEvent []byte) (SafeWebhookFields, error) {
    var e struct {
        ID   string `json:"id"`
        Type string `json:"type"`
    }
    if err := json.Unmarshal(rawEvent, &e); err != nil {
        return SafeWebhookFields{}, err
    }
    return SafeWebhookFields{EventID: e.ID, EventType: e.Type}, nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./internal/billing/stripe/... -v
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/client.go \
        services/marketplace-api/internal/billing/stripe/client_test.go \
        services/marketplace-api/internal/billing/stripe/logging.go \
        services/marketplace-api/internal/billing/stripe/logging_test.go
git commit -m "feat(billing): shared Stripe HTTP client + sanitized error + idempotency keys"
```

---

## Task 3: Product + Price clients (`currency_options`)

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/product.go`
- Create: `services/marketplace-api/internal/billing/stripe/price.go`
- Tests: `*_test.go` siblings (use HTTP test server mocking the Stripe API shape)

**Spec references:** §4.2.1 (`currency_options`), §19.4 (AU `tax_behavior: exclusive`).

- [ ] **Step 1: Write failing test for Price creation**

```go
func TestPrice_Create_EmitsCurrencyOptions(t *testing.T) {
    seen := make(url.Values)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        b, _ := io.ReadAll(r.Body)
        v, _ := url.ParseQuery(string(b))
        for k, vs := range v { seen[k] = vs }
        w.Header().Set("Request-Id", "req_test")
        _, _ = w.Write([]byte(`{"id":"price_xxx","object":"price","currency":"usd","unit_amount":1900}`))
    }))
    defer srv.Close()

    c := stripe.New("sk_test_x")
    c.SetBaseURLForTesting(srv.URL)

    desc := pricing.MustGetDescriptor(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierDeveloped)
    p, err := stripe.CreatePrice(context.Background(), c, "prod_starter", desc)
    require.NoError(t, err)
    require.Equal(t, "price_xxx", p.ID)

    // Verify currency_options[aud][tax_behavior] = exclusive
    require.Equal(t, "exclusive", seen.Get("currency_options[aud][tax_behavior]"))
    require.Equal(t, "2900", seen.Get("currency_options[aud][unit_amount]"))
    require.NotContains(t, seen.Get("currency_options[usd][tax_behavior]"), "exclusive")
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `product.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "net/url"
)

type Product struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Metadata    map[string]string `json:"metadata"`
    Active      bool              `json:"active"`
}

func CreateProduct(ctx context.Context, c *Client, name, planKey, idempotencyKey string) (*Product, error) {
    v := url.Values{}
    v.Set("name", name)
    v.Set("metadata[plan]", planKey)
    body, err := c.PostForm(ctx, "/v1/products", idempotencyKey, v)
    if err != nil {
        return nil, err
    }
    var p Product
    if err := json.Unmarshal(body, &p); err != nil {
        return nil, err
    }
    return &p, nil
}

// FindProductByLookupKey searches by metadata.plan so bootstrap is idempotent
// even without stored Stripe IDs.
func FindProductByMetadata(ctx context.Context, c *Client, planKey string) (*Product, error) {
    body, err := c.Get(ctx, "/v1/products?active=true&limit=100")
    if err != nil {
        return nil, err
    }
    var page struct {
        Data []Product `json:"data"`
    }
    if err := json.Unmarshal(body, &page); err != nil {
        return nil, err
    }
    for i := range page.Data {
        if page.Data[i].Metadata["plan"] == planKey {
            return &page.Data[i], nil
        }
    }
    return nil, ErrNotFound
}
```

- [ ] **Step 4: Implement `price.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "strconv"
)

type Price struct {
    ID         string            `json:"id"`
    Object     string            `json:"object"`
    Currency   string            `json:"currency"`
    UnitAmount int64             `json:"unit_amount"`
    LookupKey  string            `json:"lookup_key"`
    Metadata   map[string]string `json:"metadata"`
    Active     bool              `json:"active"`
    TaxBehavior string           `json:"tax_behavior"`
}

// CreatePrice issues a Stripe Price with currency_options for every
// currency in desc.Options. The caller is responsible for idempotency: always
// call FindPriceByLookupKey first and only invoke CreatePrice when it returns
// ErrNotFound (see bootstrap.Run in Task 4). Stripe's duplicate-lookup_key
// error codes vary by endpoint and API version; the find-then-create ordering
// avoids any reliance on catching a specific error string.
func CreatePrice(ctx context.Context, c *Client, productID string, desc pricing.PriceDescriptor) (*Price, error) {
    v := url.Values{}
    v.Set("product", productID)
    v.Set("currency", desc.Baseline.Currency)
    v.Set("unit_amount", strconv.FormatInt(desc.Baseline.UnitAmountMinor, 10))
    v.Set("lookup_key", desc.LookupKey)
    v.Set("metadata[plan]", string(desc.Plan))
    v.Set("metadata[period]", string(desc.Period))
    v.Set("metadata[tier]", string(desc.Tier))

    // recurring interval
    interval := "month"
    if desc.Period == pricing.PeriodAnnual {
        interval = "year"
    }
    v.Set("recurring[interval]", interval)

    // Developed tier: emit currency_options. PPP tier: single-currency price, no options.
    if desc.Tier == pricing.TierDeveloped {
        for cur, amt := range desc.Options {
            v.Set(fmt.Sprintf("currency_options[%s][unit_amount]", cur), strconv.FormatInt(amt.UnitAmountMinor, 10))
            if amt.TaxBehavior != "" {
                v.Set(fmt.Sprintf("currency_options[%s][tax_behavior]", cur), amt.TaxBehavior)
            }
        }
    }

    // Idempotency-Key at the HTTP layer catches same-process retries; duplicate
    // lookup_key protection across restarts lives in bootstrap.Run's find-first flow.
    body, err := c.PostForm(ctx, "/v1/prices", "price:"+desc.LookupKey, v)
    if err != nil {
        return nil, err
    }
    var p Price
    if err := json.Unmarshal(body, &p); err != nil {
        return nil, err
    }
    return &p, nil
}

func FindPriceByLookupKey(ctx context.Context, c *Client, lookupKey string) (*Price, error) {
    body, err := c.Get(ctx, "/v1/prices?lookup_keys[]="+url.QueryEscape(lookupKey)+"&active=true")
    if err != nil {
        return nil, err
    }
    var page struct { Data []Price `json:"data"` }
    if err := json.Unmarshal(body, &page); err != nil {
        return nil, err
    }
    if len(page.Data) == 0 {
        return nil, ErrNotFound
    }
    return &page.Data[0], nil
}

var ErrNotFound = errors.New("stripe: resource not found")
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./internal/billing/stripe/... -v
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/product.go \
        services/marketplace-api/internal/billing/stripe/price.go \
        services/marketplace-api/internal/billing/stripe/{product,price}_test.go
git commit -m "feat(billing): Product + Price clients with currency_options support"
```

---

## Task 4: Bootstrap CLI

**Files:**
- Create: `services/marketplace-api/cmd/billing-bootstrap/main.go`
- Create: `services/marketplace-api/cmd/billing-bootstrap/bootstrap.go` (logic, for unit tests)
- Create: `services/marketplace-api/cmd/billing-bootstrap/bootstrap_test.go`

**Spec references:** §2, §4.2.1.

- [ ] **Step 1: Write failing unit test**

```go
func TestBootstrap_IdempotentReRun(t *testing.T) {
    mock := newMockStripe(t)
    defer mock.Close()

    c := stripe.New("sk_test_x")
    c.SetBaseURLForTesting(mock.URL)

    err := bootstrap.Run(context.Background(), c, pricing.AllDescriptors())
    require.NoError(t, err)

    // Second run: must be no-op (no new products/prices created).
    mock.CountersReset()
    err = bootstrap.Run(context.Background(), c, pricing.AllDescriptors())
    require.NoError(t, err)
    require.Zero(t, mock.ProductCreates)
    require.Zero(t, mock.PriceCreates)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `bootstrap.go`**

```go
package bootstrap

import (
    "context"
    "errors"
    "fmt"

    stripec "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/billing/pricing"
)

// Run upserts Products (1 per plan) and Prices (1 per plan × period × tier bucket).
// Safe to re-run: existing Products are reused via metadata.plan lookup;
// existing Prices are reused via lookup_key.
func Run(ctx context.Context, c *stripec.Client, descriptors []pricing.PriceDescriptor) error {
    plans := map[pricing.Plan]string{}
    for _, d := range descriptors { plans[d.Plan] = "" }

    // Upsert one Product per plan.
    for plan := range plans {
        p, err := stripec.FindProductByMetadata(ctx, c, string(plan))
        if errors.Is(err, stripec.ErrNotFound) {
            p, err = stripec.CreateProduct(ctx, c, "Mark8ly "+string(plan), string(plan), "product:"+string(plan))
        }
        if err != nil {
            return fmt.Errorf("bootstrap: upsert product %s: %w", plan, err)
        }
        plans[plan] = p.ID
    }

    // Upsert prices by lookup_key.
    for _, d := range descriptors {
        prodID := plans[d.Plan]
        _, err := stripec.FindPriceByLookupKey(ctx, c, d.LookupKey)
        if errors.Is(err, stripec.ErrNotFound) {
            _, err = stripec.CreatePrice(ctx, c, prodID, d)
        }
        if err != nil {
            return fmt.Errorf("bootstrap: upsert price %s: %w", d.LookupKey, err)
        }
    }
    return nil
}
```

- [ ] **Step 4: Implement `main.go`**

```go
package main

import (
    "context"
    "flag"
    "log/slog"
    "os"
    "time"

    stripec "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/billing/pricing"
    "github.com/tesserix/marketplace-api/cmd/billing-bootstrap/bootstrap"
)

func main() {
    var apiKey string
    flag.StringVar(&apiKey, "api-key", os.Getenv("STRIPE_BILLING_SECRET_KEY"), "Stripe secret key")
    flag.Parse()

    log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    if apiKey == "" {
        log.Error("STRIPE_BILLING_SECRET_KEY required")
        os.Exit(1)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    if err := bootstrap.Run(ctx, stripec.New(apiKey), pricing.AllDescriptors()); err != nil {
        log.Error("bootstrap failed", "err", err)
        os.Exit(1)
    }
    log.Info("bootstrap complete")
}
```

- [ ] **Step 5: Manual integration check — Stripe test mode**

```bash
cd services/marketplace-api
export STRIPE_BILLING_SECRET_KEY=sk_test_...
go run ./cmd/billing-bootstrap
```
Expected: log lines per upsert, no errors. Visit Stripe test dashboard → Products — should see 3 products (starter/studio/pro), each with the expected number of prices.

Re-run:
```bash
go run ./cmd/billing-bootstrap
```
Expected: same exit code 0, no new products created.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/cmd/billing-bootstrap/
git commit -m "feat(billing): idempotent bootstrap CLI for Stripe Products + Prices"
```

---

## Task 5: Customer + Checkout clients

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/customer.go`
- Create: `services/marketplace-api/internal/billing/stripe/checkout.go`
- Tests: `customer_test.go`, `checkout_test.go`

**Spec references:** §4.2.1, §19.4, §17.8.

- [ ] **Step 1: Failing Checkout test**

```go
func TestCreateCheckoutSession_PassesCurrency_AU_TaxExclusive(t *testing.T) {
    seen := url.Values{}
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        v, _ := url.ParseQuery(string(body))
        for k, vs := range v { seen[k] = vs }
        _, _ = w.Write([]byte(`{"id":"cs_x","url":"https://checkout.stripe.com/cs_x"}`))
    }))
    defer srv.Close()

    c := stripe.New("sk_test_x"); c.SetBaseURLForTesting(srv.URL)
    sess, err := stripe.CreateCheckoutSession(context.Background(), c, stripe.CheckoutInput{
        StoreID:      "store-1",
        CustomerID:   "cus_1",
        PriceID:      "price_starter_monthly",
        Currency:     "aud",
        Plan:         "starter",
        Period:       "monthly",
        SuccessURL:   "https://admin.example/success",
        CancelURL:    "https://admin.example/cancel",
        Now:          time.Unix(1_712_000_000, 0),
    })
    require.NoError(t, err)
    require.Equal(t, "cs_x", sess.ID)
    require.Equal(t, "aud", seen.Get("currency"))
    require.Equal(t, "subscription", seen.Get("mode"))
    require.Equal(t, "price_starter_monthly", seen.Get("line_items[0][price]"))
    require.Equal(t, "1", seen.Get("line_items[0][quantity]"))
    // Idempotency key reflects the day bucket.
    require.Contains(t, sess.IdempotencyKey, "checkout:store-1:starter:monthly:")
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `customer.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "net/url"
)

type Customer struct {
    ID       string            `json:"id"`
    Email    string            `json:"email"`
    Metadata map[string]string `json:"metadata"`
}

type CreateCustomerInput struct {
    StoreID  string
    TenantID string
    Email    string
    Name     string
    Country  string // ISO 3166 alpha-2
}

func CreateCustomer(ctx context.Context, c *Client, in CreateCustomerInput) (*Customer, error) {
    v := url.Values{}
    if in.Email != "" { v.Set("email", in.Email) }
    if in.Name != ""  { v.Set("name", in.Name) }
    v.Set("metadata[store_id]", in.StoreID)
    v.Set("metadata[tenant_id]", in.TenantID)
    if in.Country != "" { v.Set("address[country]", in.Country) }

    body, err := c.PostForm(ctx, "/v1/customers", CustomerIdempotencyKey(in.StoreID), v)
    if err != nil {
        return nil, err
    }
    var cu Customer
    if err := json.Unmarshal(body, &cu); err != nil {
        return nil, err
    }
    return &cu, nil
}
```

- [ ] **Step 4: Implement `checkout.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "net/url"
    "strings"
    "time"
)

type CheckoutSession struct {
    ID             string `json:"id"`
    URL            string `json:"url"`
    IdempotencyKey string `json:"-"` // echoed back by the client for logging/observability
}

type CheckoutInput struct {
    StoreID    string
    TenantID   string
    CustomerID string
    PriceID    string
    Currency   string // lowercase ISO; sent to Stripe to pick from currency_options
    Plan       string
    Period     string
    SuccessURL string
    CancelURL  string
    Locale     string
    Now        time.Time // injected for testability
}

func CreateCheckoutSession(ctx context.Context, c *Client, in CheckoutInput) (*CheckoutSession, error) {
    if in.Now.IsZero() {
        in.Now = time.Now()
    }
    v := url.Values{}
    v.Set("mode", "subscription")
    v.Set("customer", in.CustomerID)
    v.Set("line_items[0][price]", in.PriceID)
    v.Set("line_items[0][quantity]", "1")
    v.Set("currency", strings.ToLower(in.Currency))
    v.Set("success_url", in.SuccessURL)
    v.Set("cancel_url", in.CancelURL)
    if in.Locale != "" { v.Set("locale", in.Locale) }
    v.Set("metadata[store_id]", in.StoreID)
    v.Set("metadata[tenant_id]", in.TenantID)
    v.Set("metadata[plan]", in.Plan)
    v.Set("metadata[period]", in.Period)
    v.Set("subscription_data[metadata][store_id]", in.StoreID)
    v.Set("subscription_data[metadata][tenant_id]", in.TenantID)

    key := CheckoutIdempotencyKey(in.StoreID, in.Plan, in.Period, in.Now.Unix())
    body, err := c.PostForm(ctx, "/v1/checkout/sessions", key, v)
    if err != nil {
        return nil, err
    }
    var sess CheckoutSession
    if err := json.Unmarshal(body, &sess); err != nil {
        return nil, err
    }
    sess.IdempotencyKey = key
    return &sess, nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/{customer,checkout}{,_test}.go
git commit -m "feat(billing): Customer + Checkout clients with server-generated idempotency"
```

---

## Task 6: Customer Portal client (5-min bucket)

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/portal.go`
- Create: `services/marketplace-api/internal/billing/stripe/portal_test.go`

**Spec references:** §17.8 Council finding #1 (5-min bucket matches portal URL lifetime).

- [ ] **Step 1: Failing test**

```go
func TestCreatePortalSession_5MinBucketIdempotency(t *testing.T) {
    hits := 0
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        hits++
        require.Contains(t, r.Header.Get("Idempotency-Key"), "portal:store-1:")
        _, _ = w.Write([]byte(`{"id":"ps_x","url":"https://billing.stripe.com/ps_x"}`))
    }))
    defer srv.Close()

    c := stripe.New("sk_test_x"); c.SetBaseURLForTesting(srv.URL)
    in := stripe.PortalInput{StoreID: "store-1", CustomerID: "cus_1", ReturnURL: "https://admin/", Now: time.Unix(1_712_000_060, 0)}

    a, _ := stripe.CreatePortalSession(context.Background(), c, in)
    in.Now = time.Unix(1_712_000_290, 0) // same 5-min bucket
    b, _ := stripe.CreatePortalSession(context.Background(), c, in)
    require.Equal(t, a.IdempotencyKey, b.IdempotencyKey) // Stripe dedupes; our key is stable

    in.Now = time.Unix(1_712_000_600, 0) // next bucket
    c2, _ := stripe.CreatePortalSession(context.Background(), c, in)
    require.NotEqual(t, a.IdempotencyKey, c2.IdempotencyKey)
}
```

- [ ] **Step 2: Implement `portal.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "net/url"
    "time"
)

type PortalSession struct {
    ID             string `json:"id"`
    URL            string `json:"url"`
    IdempotencyKey string `json:"-"`
}

type PortalInput struct {
    StoreID    string
    CustomerID string
    ReturnURL  string
    Now        time.Time
}

func CreatePortalSession(ctx context.Context, c *Client, in PortalInput) (*PortalSession, error) {
    if in.Now.IsZero() { in.Now = time.Now() }
    v := url.Values{}
    v.Set("customer", in.CustomerID)
    v.Set("return_url", in.ReturnURL)

    key := PortalIdempotencyKey(in.StoreID, in.Now.Unix())
    body, err := c.PostForm(ctx, "/v1/billing_portal/sessions", key, v)
    if err != nil {
        return nil, err
    }
    var ps PortalSession
    if err := json.Unmarshal(body, &ps); err != nil {
        return nil, err
    }
    ps.IdempotencyKey = key
    return &ps, nil
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/portal{,_test}.go
git commit -m "feat(billing): Customer Portal client with 5-min bucket idempotency"
```

---

## Task 7: Subscription + Invoice read clients

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/subscription.go`
- Create: `services/marketplace-api/internal/billing/stripe/invoice.go`
- Tests: siblings

These are minimal — only `GetSubscription(id)` and `GetInvoice(id)` are needed by webhook handlers in P2. Cancel + update land later in P3/P6.

- [ ] **Step 1: Failing test**

```go
func TestGetSubscription_ReturnsPlanPeriodCurrency(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/v1/subscriptions/sub_x", r.URL.Path)
        _, _ = w.Write([]byte(`{
            "id":"sub_x","status":"active","currency":"gbp",
            "current_period_start":1710000000,"current_period_end":1712678400,
            "cancel_at_period_end":false,
            "items":{"data":[{"price":{"id":"price_starter_monthly","currency":"gbp","metadata":{"plan":"starter","period":"monthly"}}}]}
        }`))
    }))
    defer srv.Close()

    c := stripe.New("sk_test_x"); c.SetBaseURLForTesting(srv.URL)
    s, err := stripe.GetSubscription(context.Background(), c, "sub_x")
    require.NoError(t, err)
    require.Equal(t, "active", s.Status)
    require.Equal(t, "gbp", s.Currency)
    require.Equal(t, "starter", s.Items.Data[0].Price.Metadata["plan"])
}
```

- [ ] **Step 2: Implement `subscription.go` + `invoice.go`**

Minimal struct shapes mapping only the fields we need. Read-only GET endpoints.

```go
type Subscription struct {
    ID                 string `json:"id"`
    Status             string `json:"status"`
    Currency           string `json:"currency"`
    CurrentPeriodStart int64  `json:"current_period_start"`
    CurrentPeriodEnd   int64  `json:"current_period_end"`
    CancelAtPeriodEnd  bool   `json:"cancel_at_period_end"`
    Customer           string `json:"customer"`
    Items              struct {
        Data []struct {
            Price Price `json:"price"`
        } `json:"data"`
    } `json:"items"`
}

func GetSubscription(ctx context.Context, c *Client, id string) (*Subscription, error) {
    body, err := c.Get(ctx, "/v1/subscriptions/"+url.PathEscape(id))
    if err != nil { return nil, err }
    var s Subscription
    if err := json.Unmarshal(body, &s); err != nil { return nil, err }
    return &s, nil
}
```

Analogous `Invoice` struct with `HostedInvoiceURL`, `PaymentIntent`, `Status`.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/{subscription,invoice}{,_test}.go
git commit -m "feat(billing): Subscription + Invoice read clients"
```

---

## Task 8: Signature verification on raw body

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/signature.go`
- Create: `services/marketplace-api/internal/billing/stripe/signature_test.go`

**Spec references:** §18.5.

- [ ] **Step 1: Failing test with signed payload fixture**

```go
func TestVerifySignature_AcceptsValidSignature(t *testing.T) {
    secret := "whsec_test"
    payload := []byte(`{"id":"evt_1","type":"customer.subscription.updated"}`)
    now := time.Unix(1_712_000_000, 0)

    sig := stripe.BuildSignatureForTesting(payload, secret, now)
    eventID, eventType, err := stripe.VerifySignature(payload, sig, secret, now)
    require.NoError(t, err)
    require.Equal(t, "evt_1", eventID)
    require.Equal(t, "customer.subscription.updated", eventType)
}

func TestVerifySignature_RejectsOldTimestamp(t *testing.T) {
    secret := "whsec_test"
    payload := []byte(`{"id":"evt_1","type":"x"}`)
    nowSigning := time.Unix(1_712_000_000, 0)
    nowVerifying := nowSigning.Add(6 * time.Minute) // outside 5m window

    sig := stripe.BuildSignatureForTesting(payload, secret, nowSigning)
    _, _, err := stripe.VerifySignature(payload, sig, secret, nowVerifying)
    require.ErrorIs(t, err, stripe.ErrStaleSignature)
}

func TestVerifySignature_RejectsTamperedPayload(t *testing.T) {
    secret := "whsec_test"
    payload := []byte(`{"id":"evt_1","type":"x"}`)
    now := time.Unix(1_712_000_000, 0)
    sig := stripe.BuildSignatureForTesting(payload, secret, now)
    _, _, err := stripe.VerifySignature([]byte(`{"id":"evt_2","type":"x"}`), sig, secret, now)
    require.ErrorIs(t, err, stripe.ErrBadSignature)
}
```

- [ ] **Step 2: Implement `signature.go`** (port of existing `internal/payment/stripe.go:268` with a cleaner surface)

```go
package stripe

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "strconv"
    "strings"
    "time"
)

var (
    ErrBadSignature    = errors.New("stripe: signature mismatch")
    ErrStaleSignature  = errors.New("stripe: signature too old")
    ErrMalformedHeader = errors.New("stripe: malformed Stripe-Signature header")
)

const maxSignatureAge = 5 * time.Minute

func VerifySignature(rawBody []byte, header, secret string, now time.Time) (eventID, eventType string, err error) {
    ts, sigs, err := parseStripeSignatureHeader(header)
    if err != nil { return "", "", err }
    if now.Sub(time.Unix(ts, 0)).Abs() > maxSignatureAge {
        return "", "", ErrStaleSignature
    }
    signedPayload := fmt.Sprintf("%d.%s", ts, rawBody)
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(signedPayload))
    expected := hex.EncodeToString(mac.Sum(nil))
    ok := false
    for _, s := range sigs {
        if hmac.Equal([]byte(expected), []byte(s)) { ok = true; break }
    }
    if !ok { return "", "", ErrBadSignature }

    var e struct { ID, Type string }
    if err := json.Unmarshal(rawBody, &e); err != nil {
        return "", "", fmt.Errorf("stripe: parse event body: %w", err)
    }
    return e.ID, e.Type, nil
}

func BuildSignatureForTesting(rawBody []byte, secret string, now time.Time) string {
    ts := now.Unix()
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(fmt.Sprintf("%d.%s", ts, rawBody)))
    return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func parseStripeSignatureHeader(h string) (ts int64, sigs []string, err error) {
    var t int64
    for _, part := range strings.Split(h, ",") {
        kv := strings.SplitN(part, "=", 2)
        if len(kv) != 2 { return 0, nil, ErrMalformedHeader }
        switch kv[0] {
        case "t":
            t, err = strconv.ParseInt(kv[1], 10, 64)
            if err != nil { return 0, nil, ErrMalformedHeader }
        case "v1":
            sigs = append(sigs, kv[1])
        }
    }
    if t == 0 || len(sigs) == 0 { return 0, nil, ErrMalformedHeader }
    return t, sigs, nil
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/stripe/signature{,_test}.go
git commit -m "feat(billing): Stripe webhook signature verification (raw body, 5m window)"
```

---

## Task 9: `webhookevents.Repository`

**Files:**
- Create: `services/marketplace-api/internal/webhookevents/repository.go`
- Create: `services/marketplace-api/internal/webhookevents/repository_test.go`

**Spec references:** §17.7.

- [ ] **Step 1: Failing tests**

```go
func TestInsertIfNew_IdempotentByEventID(t *testing.T) {
    db := testdb.NewDB(t, "stripe_webhook_events")
    repo := webhookevents.NewRepository()

    ok, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_1", EventType: "customer.subscription.updated",
        Payload: []byte(`{}`),
    })
    require.NoError(t, err); require.True(t, ok)

    // Duplicate — InsertIfNew returns (false, nil).
    ok, err = repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_1", EventType: "customer.subscription.updated",
        Payload: []byte(`{}`),
    })
    require.NoError(t, err); require.False(t, ok)
}

func TestGetUnprocessedOrphans_Filters(t *testing.T) {
    db := testdb.NewDB(t, "stripe_webhook_events")
    // Seed: one orphan, one with store_id, one already processed, one flagged manual review
    require.NoError(t, db.Exec(`INSERT INTO stripe_webhook_events (event_id, event_type, payload) VALUES
        ('e1','t','{}'),
        ('e2','t','{}'),
        ('e3','t','{}'),
        ('e4','t','{}')`).Error)
    require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET store_id='...' WHERE event_id='e2'`).Error)
    require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET processed_at=now() WHERE event_id='e3'`).Error)
    require.NoError(t, db.Exec(`UPDATE stripe_webhook_events SET manual_review_required=true WHERE event_id='e4'`).Error)

    repo := webhookevents.NewRepository()
    orphans, err := repo.GetUnprocessedOrphans(context.Background(), db, 10)
    require.NoError(t, err)
    require.Len(t, orphans, 1)
    require.Equal(t, "e1", orphans[0].EventID)
}
```

- [ ] **Step 2: Implement `repository.go`**

```go
package webhookevents

import (
    "context"
    "fmt"

    "gorm.io/gorm"
)

type Repository interface {
    InsertIfNew(ctx context.Context, db *gorm.DB, e StripeWebhookEvent) (bool, error)
    GetUnprocessedOrphans(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error)
    MarkProcessed(ctx context.Context, db *gorm.DB, eventID string) error
    SetStoreID(ctx context.Context, db *gorm.DB, eventID string, storeID string, tenantID string) error
    IncrementRetry(ctx context.Context, db *gorm.DB, eventID string, errMsg string) (newCount int, err error)
    FlagManualReview(ctx context.Context, db *gorm.DB, eventID string, reason string) error
}

type repoImpl struct{}

func NewRepository() Repository { return &repoImpl{} }

func (r *repoImpl) InsertIfNew(ctx context.Context, db *gorm.DB, e StripeWebhookEvent) (bool, error) {
    res := db.WithContext(ctx).Exec(
        `INSERT INTO stripe_webhook_events (event_id, event_type, store_id, tenant_id, payload)
         VALUES (?, ?, ?, ?, ?::jsonb)
         ON CONFLICT (event_id) DO NOTHING`,
        e.EventID, e.EventType, e.StoreID, e.TenantID, string(e.Payload),
    )
    if res.Error != nil {
        return false, fmt.Errorf("webhookevents: InsertIfNew: %w", res.Error)
    }
    return res.RowsAffected > 0, nil
}

func (r *repoImpl) GetUnprocessedOrphans(ctx context.Context, db *gorm.DB, limit int) ([]StripeWebhookEvent, error) {
    var out []StripeWebhookEvent
    err := db.WithContext(ctx).
        Where("store_id IS NULL AND processed_at IS NULL AND manual_review_required = false").
        Order("received_at ASC").
        Limit(limit).
        Find(&out).Error
    return out, err
}

func (r *repoImpl) MarkProcessed(ctx context.Context, db *gorm.DB, eventID string) error {
    return db.WithContext(ctx).Exec(
        `UPDATE stripe_webhook_events SET processed_at = now() WHERE event_id = ?`, eventID,
    ).Error
}

func (r *repoImpl) SetStoreID(ctx context.Context, db *gorm.DB, eventID, storeID, tenantID string) error {
    return db.WithContext(ctx).Exec(
        `UPDATE stripe_webhook_events SET store_id=?, tenant_id=? WHERE event_id=?`,
        storeID, tenantID, eventID,
    ).Error
}

func (r *repoImpl) IncrementRetry(ctx context.Context, db *gorm.DB, eventID, errMsg string) (int, error) {
    var newCount int
    err := db.WithContext(ctx).Raw(
        `UPDATE stripe_webhook_events
         SET retry_count = retry_count + 1, processing_error = ?
         WHERE event_id = ?
         RETURNING retry_count`, errMsg, eventID,
    ).Scan(&newCount).Error
    return newCount, err
}

func (r *repoImpl) FlagManualReview(ctx context.Context, db *gorm.DB, eventID, reason string) error {
    return db.WithContext(ctx).Exec(
        `UPDATE stripe_webhook_events
         SET manual_review_required = true, processing_error = ?
         WHERE event_id = ?`, reason, eventID,
    ).Error
}
```

- [ ] **Step 3: Run tests — PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/webhookevents/repository{,_test}.go
git commit -m "feat(billing): webhookevents repository with idempotent InsertIfNew"
```

---

## Task 10: Webhook HTTP handler

**Files:**
- Create: `services/marketplace-api/internal/handlers/webhooks/stripe.go`
- Create: `services/marketplace-api/internal/handlers/webhooks/stripe_test.go`

**Spec references:** §18.5, §17.6, §17.7.

- [ ] **Step 1: Failing handler test**

```go
func TestStripeWebhook_ValidSignature_PersistsEvent(t *testing.T) {
    db := testdb.NewDB(t, "stripe_webhook_events")
    secret := "whsec_test_x"

    h := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
        DB:     db,
        Secret: secret,
        Repo:   webhookevents.NewRepository(),
        Dispatch: func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
            return nil
        },
        AllowedTypes: map[string]bool{"customer.subscription.updated": true},
        MaxBodyBytes: 512 << 10,
    })

    payload := []byte(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"id":"sub_x","customer":"cus_x","status":"active","currency":"usd"}}}`)
    sig := stripe.BuildSignatureForTesting(payload, secret, time.Now())

    w := httptest.NewRecorder()
    r, _ := http.NewRequest("POST", "/webhooks/stripe-billing", bytes.NewReader(payload))
    r.Header.Set("Stripe-Signature", sig)

    router := gin.New()
    router.POST("/webhooks/stripe-billing", h.Handle)
    router.ServeHTTP(w, r)

    require.Equal(t, 200, w.Code)

    var cnt int64
    require.NoError(t, db.Raw(`SELECT count(*) FROM stripe_webhook_events WHERE event_id='evt_1'`).Scan(&cnt).Error)
    require.EqualValues(t, 1, cnt)
}

func TestStripeWebhook_BadSignature_Rejects401(t *testing.T) {
    // ...returns 401, no row inserted, no panic.
}

func TestStripeWebhook_BodyOver512K_Rejects413(t *testing.T) {
    // Post 700 KB payload — expect 413.
}

func TestStripeWebhook_UnknownType_200ButNotDispatched(t *testing.T) {
    // Allowlist misses — accept + persist (audit trail) but skip dispatcher.
}
```

- [ ] **Step 2: Implement `stripe.go`**

```go
package webhooks

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/datatypes"
    "gorm.io/gorm"
    "log/slog"

    billingstripe "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/webhookevents"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type StripeHandlerConfig struct {
    DB           *gorm.DB
    Secret       string
    Repo         webhookevents.Repository
    Dispatch     func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error
    AllowedTypes map[string]bool
    MaxBodyBytes int64
    Now          func() time.Time
    Logger       *slog.Logger
}

type StripeHandler struct {
    cfg StripeHandlerConfig
}

func NewStripeHandler(cfg StripeHandlerConfig) *StripeHandler {
    if cfg.Now == nil { cfg.Now = time.Now }
    if cfg.MaxBodyBytes == 0 { cfg.MaxBodyBytes = 512 << 10 }
    if cfg.Logger == nil { cfg.Logger = slog.Default() }
    return &StripeHandler{cfg: cfg}
}

func (h *StripeHandler) Handle(c *gin.Context) {
    // 1. Cap body size (§18.5).
    c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.cfg.MaxBodyBytes)
    raw, err := io.ReadAll(c.Request.Body)
    if err != nil {
        c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "body_too_large"})
        return
    }

    // 2. Verify signature on raw bytes BEFORE any parsing.
    sig := c.GetHeader("Stripe-Signature")
    eventID, eventType, err := billingstripe.VerifySignature(raw, sig, h.cfg.Secret, h.cfg.Now())
    if err != nil {
        // Log only metadata; never the body.
        h.cfg.Logger.Warn("stripe: signature verification failed", "err", err.Error())
        c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
        return
    }

    // 3. Event-type allowlist (§18.5).
    allowed := h.cfg.AllowedTypes[eventType]

    // 4. Idempotent insert.
    evt := webhookevents.StripeWebhookEvent{
        EventID: eventID, EventType: eventType,
        Payload: datatypes.JSON(raw),
    }
    inserted, err := h.cfg.Repo.InsertIfNew(c.Request.Context(), h.cfg.DB, evt)
    if err != nil {
        h.cfg.Logger.Error("stripe: InsertIfNew failed", "event_id", eventID, "event_type", eventType, "err", err.Error())
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "persist_failed"})
        return
    }

    // Already-processed idempotency: return 200 without dispatching.
    if !inserted {
        h.cfg.Logger.Info("stripe: duplicate event ignored", "event_id", eventID, "event_type", eventType)
        c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
        return
    }

    // 5. Dispatch only if allowlisted; otherwise persist-only.
    if !allowed {
        h.cfg.Logger.Info("stripe: event_type not in allowlist", "event_id", eventID, "event_type", eventType)
        c.JSON(http.StatusOK, gin.H{"status": "persisted"})
        return
    }

    // 6. Resolve customer → store_id, lock on store_id, dispatch.
    if err := h.dispatchLocked(c.Request.Context(), evt); err != nil {
        // Dispatch errors are tracked in retry_count; return 200 to avoid
        // Stripe retrying independently — our orphan cron owns retries.
        h.cfg.Logger.Error("stripe: dispatch failed (will retry via cron)", "event_id", eventID, "event_type", eventType, "err", err.Error())
        _, _ = h.cfg.Repo.IncrementRetry(c.Request.Context(), h.cfg.DB, eventID, billingstripe.SanitizeForLog(err))
        c.JSON(http.StatusOK, gin.H{"status": "retry_scheduled"})
        return
    }

    _ = h.cfg.Repo.MarkProcessed(c.Request.Context(), h.cfg.DB, eventID)
    c.JSON(http.StatusOK, gin.H{"status": "processed"})
}

func (h *StripeHandler) dispatchLocked(ctx context.Context, evt webhookevents.StripeWebhookEvent) error {
    // Lookup customer → store_id using a helper (implemented in dispatcher.go Task 11).
    storeID, tenantID, ok := lookupStoreByStripeCustomer(ctx, h.cfg.DB, evt.Payload)
    if !ok {
        // Orphan: leave store_id NULL; cron will resolve later.
        return nil
    }
    _ = h.cfg.Repo.SetStoreID(ctx, h.cfg.DB, evt.EventID, storeID, tenantID)

    return subscription.WithAdvisoryLock(ctx, h.cfg.DB, uuid.MustParse(storeID), func(tx *gorm.DB) error {
        return h.cfg.Dispatch(ctx, tx, evt)
    })
}

// helper lives in dispatcher package; inlined here for test-friendly signature.
func lookupStoreByStripeCustomer(ctx context.Context, db *gorm.DB, payload []byte) (string, string, bool) {
    var e struct {
        Data struct { Object struct { Customer string `json:"customer"` } `json:"object"` } `json:"data"`
    }
    _ = json.Unmarshal(payload, &e)
    if e.Data.Object.Customer == "" { return "", "", false }

    var row struct {
        StoreID  string
        TenantID string
    }
    err := db.WithContext(ctx).Raw(
        `SELECT store_id::text, tenant_id::text FROM store_subscriptions WHERE stripe_customer_id = ? LIMIT 1`,
        e.Data.Object.Customer,
    ).Scan(&row).Error
    if err != nil || row.StoreID == "" { return "", "", false }
    return row.StoreID, row.TenantID, true
}
```

Add `SanitizeForLog(err error) string` in `internal/billing/stripe/logging.go`:

```go
func SanitizeForLog(err error) string {
    var apiErr *APIError
    if errors.As(err, &apiErr) { return apiErr.Error() }
    return "internal error"
}
```

- [ ] **Step 3: Run all handler tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/webhooks/{stripe,stripe_test}.go \
        services/marketplace-api/internal/billing/stripe/logging.go
git commit -m "feat(billing): Stripe webhook handler with signature verify + idempotent persist + locked dispatch"
```

---

## Task 11: Event dispatcher + per-event handlers

**Files:**
- Create: `services/marketplace-api/internal/billing/dispatch/dispatcher.go`
- Create: `services/marketplace-api/internal/billing/dispatch/handlers.go`
- Create: `services/marketplace-api/internal/billing/dispatch/dispatcher_test.go`

**Spec references:** §17.6 — 15 events. In P2 we handle **column updates** only; state machine transitions stay in P3.

Events handled here:
- `checkout.session.completed` → bind `billing_currency`, set `stripe_subscription_id`, `current_period_*`, set status to `trialing` if still signup, else `active`.
- `customer.subscription.updated` → refresh `current_period_start/end`, `cancel_at_period_end`.
- `customer.subscription.deleted` → mark `status = expired` (P3 will add the richer state machine).
- `invoice.paid` → no column change; emit audit only.
- `invoice.payment_failed` → set status to `past_due`; emit audit.
- `invoice.payment_action_required` → set status to `payment_action_required`; store hosted_invoice_url in metadata (P6 consumes).
- `customer.updated` → update `stripe_customer` metadata; no column impact.
- `charge.refunded` → emit audit only; P10 owns refund bookkeeping.
- `payment_method.attached` / `detached` → emit audit only.
- `radar.early_fraud_warning` → emit audit with severity=warning.

Events NOT dispatched in P2 (persist-only):
- `invoice.created` / `finalized` — P17 observability may consume.

- [ ] **Step 1: Failing test for checkout.session.completed → billing_currency bound**

```go
func TestDispatch_CheckoutSessionCompleted_BindsBillingCurrency(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", Plan: subscription.PlanStarter, Status: subscription.StatusSignup,
    }).Error)

    payload := []byte(`{
        "id":"evt_1","type":"checkout.session.completed",
        "data":{"object":{
            "id":"cs_x","customer":"cus_x","mode":"subscription",
            "subscription":"sub_x","currency":"gbp",
            "metadata":{"plan":"starter","period":"monthly"}
        }}
    }`)

    d := dispatch.New(/* repo, stripeClient stub */)
    err := d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_1", EventType: "checkout.session.completed", Payload: payload,
    })
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.Equal(t, "GBP", *sub.BillingCurrency)
    require.Equal(t, "sub_x", *sub.StripeSubscriptionID)
}
```

- [ ] **Step 2: Implement `dispatcher.go`**

```go
package dispatch

import (
    "context"
    "encoding/json"
    "fmt"

    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/webhookevents"
)

type Handler func(ctx context.Context, tx *gorm.DB, raw []byte) error

type Dispatcher struct {
    handlers map[string]Handler
}

func New() *Dispatcher {
    d := &Dispatcher{handlers: map[string]Handler{}}
    d.handlers["checkout.session.completed"]   = handleCheckoutSessionCompleted
    d.handlers["customer.subscription.updated"] = handleSubscriptionUpdated
    d.handlers["customer.subscription.deleted"] = handleSubscriptionDeleted
    d.handlers["invoice.paid"]                  = handleInvoicePaid
    d.handlers["invoice.payment_failed"]        = handleInvoicePaymentFailed
    d.handlers["invoice.payment_action_required"] = handleInvoicePaymentActionRequired
    d.handlers["customer.updated"]              = handleCustomerUpdated
    d.handlers["charge.refunded"]               = handleChargeRefunded
    d.handlers["payment_method.attached"]       = handlePaymentMethodAttached
    d.handlers["payment_method.detached"]       = handlePaymentMethodDetached
    d.handlers["radar.early_fraud_warning"]     = handleFraudWarning
    return d
}

func (d *Dispatcher) Dispatch(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
    h, ok := d.handlers[e.EventType]
    if !ok {
        // Allowlist check already happened at HTTP layer; missing here means a bug.
        return fmt.Errorf("dispatch: no handler for %s", e.EventType)
    }
    return h(ctx, tx, e.Payload)
}
```

- [ ] **Step 3: Implement `handlers.go`** — per-event column update logic. Snippet for the critical one:

```go
func handleCheckoutSessionCompleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
    var e struct {
        Data struct {
            Object struct {
                Subscription string `json:"subscription"`
                Customer     string `json:"customer"`
                Currency     string `json:"currency"`
                Metadata     struct { Plan, Period string } `json:"metadata"`
            } `json:"object"`
        } `json:"data"`
    }
    if err := json.Unmarshal(raw, &e); err != nil {
        return fmt.Errorf("dispatch: unmarshal checkout: %w", err)
    }
    obj := e.Data.Object
    if obj.Customer == "" || obj.Currency == "" {
        return errors.New("dispatch: checkout.session.completed missing customer/currency")
    }
    currency := strings.ToUpper(obj.Currency)
    res := tx.Exec(
        `UPDATE store_subscriptions
         SET stripe_subscription_id = ?,
             billing_currency       = COALESCE(billing_currency, ?),  -- §4.2.1 locked; only bind if null
             status                 = CASE status WHEN 'signup' THEN 'trialing' ELSE status END,
             updated_at             = now()
         WHERE stripe_customer_id = ?`,
        obj.Subscription, currency, obj.Customer,
    )
    if res.Error != nil { return fmt.Errorf("dispatch: checkout update: %w", res.Error) }
    if res.RowsAffected == 0 { return errors.New("dispatch: no subscription for customer") }
    return nil
}
```

Other handlers follow the same pattern. `invoice.payment_failed` sets `status='past_due'`; `invoice.payment_action_required` sets `status='payment_action_required'`; `customer.subscription.deleted` sets `status='expired'`. All through plain UPDATEs scoped by `stripe_customer_id` (the lock is already held by the caller on `hashtext(store_id)`).

- [ ] **Step 4: Wire dispatcher into the webhook handler**

In `main.go` (Task 15):

```go
d := dispatch.New()
webhookH := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
    DB: conn, Secret: cfg.StripeBillingWebhookSecret,
    Repo: webhookevents.NewRepository(),
    Dispatch: func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
        return d.Dispatch(ctx, tx, e)
    },
    AllowedTypes: allowedEventTypes(),
})
router.POST("/webhooks/stripe-billing", webhookH.Handle)
```

- [ ] **Step 5: Run all dispatcher tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/
git commit -m "feat(billing): event dispatcher + per-event column handlers"
```

---

## Task 12: `billing_currency` binding

This is already implemented in Task 11 via the `COALESCE(billing_currency, ?)` clause on `checkout.session.completed`. Add a focused regression test to lock it in.

**Files:**
- Modify: `services/marketplace-api/internal/billing/dispatch/handlers_test.go` (add case)

- [ ] **Step 1: Write the test**

```go
func TestHandleCheckoutSessionCompleted_BillingCurrencyLockedAfterFirstBind(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    tenantID, storeID := uuid.New(), uuid.New()

    bc := "GBP"
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", Plan: subscription.PlanStarter,
        Status: subscription.StatusActive, BillingCurrency: &bc,
    }).Error)

    // Attempt second bind with different currency (simulates bug — second checkout).
    raw := []byte(`{"data":{"object":{"customer":"cus_x","subscription":"sub_x","currency":"eur","metadata":{"plan":"starter","period":"monthly"}}}}`)
    require.NoError(t, dispatch.HandleCheckoutSessionCompletedForTesting(context.Background(), db, raw))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.Equal(t, "GBP", *sub.BillingCurrency, "billing_currency must be locked on first bind")
}
```

- [ ] **Step 2: Run — expect PASS (already enforced by COALESCE)**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/handlers_test.go
git commit -m "test(billing): lock-in billing_currency after first bind"
```

---

## Task 13: Orphan resolver

**Files:**
- Create: `services/marketplace-api/internal/billing/dispatch/orphan_resolver.go`
- Create: `services/marketplace-api/internal/billing/dispatch/orphan_resolver_test.go`

**Spec references:** §17.7.

- [ ] **Step 1: Failing test**

```go
func TestOrphanResolver_ResolvesStoreIDAndDispatches(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")
    repo := webhookevents.NewRepository()

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_orphan",
        Plan: subscription.PlanStarter, Status: subscription.StatusSignup,
    }).Error)

    raw := []byte(`{"id":"evt_o1","type":"checkout.session.completed","data":{"object":{"customer":"cus_orphan","subscription":"sub_o1","currency":"usd","metadata":{"plan":"starter","period":"monthly"}}}}`)
    _, err := repo.InsertIfNew(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_o1", EventType: "checkout.session.completed", Payload: raw,
    })
    require.NoError(t, err)

    d := dispatch.New()
    resolver := dispatch.NewOrphanResolver(dispatch.OrphanConfig{
        DB: db, Repo: repo, Dispatcher: d, MaxRetries: 6,
    })
    require.NoError(t, resolver.RunOnce(context.Background()))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.NotNil(t, sub.StripeSubscriptionID)
    require.Equal(t, "sub_o1", *sub.StripeSubscriptionID)

    var e webhookevents.StripeWebhookEvent
    require.NoError(t, db.First(&e, "event_id=?", "evt_o1").Error)
    require.NotNil(t, e.ProcessedAt)
}

func TestOrphanResolver_HitsRetryCap_FlagsManualReview(t *testing.T) {
    // Seed event whose customer has no subscription; RunOnce 6 times; verify manual_review_required flips.
}
```

- [ ] **Step 2: Implement `orphan_resolver.go`**

```go
package dispatch

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/webhookevents"
)

type OrphanConfig struct {
    DB         *gorm.DB
    Repo       webhookevents.Repository
    Dispatcher *Dispatcher
    MaxRetries int
    BatchSize  int
}

type OrphanResolver struct { cfg OrphanConfig }

func NewOrphanResolver(cfg OrphanConfig) *OrphanResolver {
    if cfg.MaxRetries == 0 { cfg.MaxRetries = 6 }
    if cfg.BatchSize == 0 { cfg.BatchSize = 50 }
    return &OrphanResolver{cfg: cfg}
}

func (r *OrphanResolver) RunOnce(ctx context.Context) error {
    orphans, err := r.cfg.Repo.GetUnprocessedOrphans(ctx, r.cfg.DB, r.cfg.BatchSize)
    if err != nil { return err }

    for _, e := range orphans {
        if err := r.resolveOne(ctx, e); err != nil {
            newCount, _ := r.cfg.Repo.IncrementRetry(ctx, r.cfg.DB, e.EventID, err.Error())
            if newCount >= r.cfg.MaxRetries {
                _ = r.cfg.Repo.FlagManualReview(ctx, r.cfg.DB, e.EventID, "retry cap exceeded")
            }
        }
    }
    return nil
}

func (r *OrphanResolver) resolveOne(ctx context.Context, e webhookevents.StripeWebhookEvent) error {
    storeID, tenantID, ok := lookupStoreByStripeCustomer(ctx, r.cfg.DB, e.Payload)
    if !ok {
        return fmt.Errorf("orphan: no subscription for event_id=%s", e.EventID)
    }
    _ = r.cfg.Repo.SetStoreID(ctx, r.cfg.DB, e.EventID, storeID, tenantID)

    sid := uuid.MustParse(storeID)
    err := subscription.WithAdvisoryLock(ctx, r.cfg.DB, sid, func(tx *gorm.DB) error {
        return r.cfg.Dispatcher.Dispatch(ctx, tx, e)
    })
    if err != nil { return err }

    return r.cfg.Repo.MarkProcessed(ctx, r.cfg.DB, e.EventID)
}
```

- [ ] **Step 3: Run tests — PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/orphan_resolver{,_test}.go
git commit -m "feat(billing): orphan resolver with 6-retry cap + manual-review flag"
```

---

## Task 14: Cron + dead-letter + PagerDuty hook

**Files:**
- Create: `services/marketplace-api/internal/billing/dispatch/cron.go`
- Create: `services/marketplace-api/internal/billing/dispatch/pagerduty.go`
- Create: `services/marketplace-api/internal/billing/dispatch/cron_test.go`

**Spec references:** §17.7 (5-min re-attempt, 1h PagerDuty, 24h dead-letter), §21.3 alerts.

- [ ] **Step 1: Failing test for the 1-hour PagerDuty trigger**

```go
func TestOrphanCron_PagerDutyOnUnresolved1h(t *testing.T) {
    db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

    // Seed event received 61 min ago, no subscription match.
    require.NoError(t, db.Exec(
        `INSERT INTO stripe_webhook_events (event_id, event_type, payload, received_at)
         VALUES ('evt_stale','checkout.session.completed','{"data":{"object":{"customer":"cus_missing"}}}'::jsonb, now() - interval '61 minutes')`,
    ).Error)

    var pdCalls int32
    pd := &fakePagerDuty{trigger: func(ctx context.Context, summary string) error {
        atomic.AddInt32(&pdCalls, 1)
        return nil
    }}

    c := dispatch.NewCron(dispatch.CronConfig{
        DB: db, Repo: webhookevents.NewRepository(), Dispatcher: dispatch.New(),
        PagerDuty: pd, StaleThreshold: time.Hour,
    })
    require.NoError(t, c.RunOnce(context.Background()))

    require.EqualValues(t, 1, atomic.LoadInt32(&pdCalls))
}
```

- [ ] **Step 2: Implement `cron.go` + `pagerduty.go`**

```go
package dispatch

import (
    "context"
    "fmt"
    "time"

    "github.com/robfig/cron/v3"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/webhookevents"
)

type PagerDuty interface {
    Trigger(ctx context.Context, summary string) error
}

type CronConfig struct {
    DB             *gorm.DB
    Repo           webhookevents.Repository
    Dispatcher     *Dispatcher
    PagerDuty      PagerDuty
    StaleThreshold time.Duration // §17.7: 1h
    Interval       time.Duration // §17.7: 5m
    MaxRetries     int
}

type Cron struct {
    cfg CronConfig
    sch *cron.Cron
}

func NewCron(cfg CronConfig) *Cron {
    if cfg.StaleThreshold == 0 { cfg.StaleThreshold = time.Hour }
    if cfg.Interval == 0 { cfg.Interval = 5 * time.Minute }
    if cfg.MaxRetries == 0 { cfg.MaxRetries = 6 }
    return &Cron{cfg: cfg}
}

func (c *Cron) Start(ctx context.Context) error {
    c.sch = cron.New()
    _, err := c.sch.AddFunc(
        fmt.Sprintf("@every %s", c.cfg.Interval.String()),
        func() { _ = c.RunOnce(ctx) },
    )
    if err != nil { return err }
    c.sch.Start()
    return nil
}

func (c *Cron) Stop() { if c.sch != nil { c.sch.Stop() } }

func (c *Cron) RunOnce(ctx context.Context) error {
    resolver := NewOrphanResolver(OrphanConfig{
        DB: c.cfg.DB, Repo: c.cfg.Repo, Dispatcher: c.cfg.Dispatcher, MaxRetries: c.cfg.MaxRetries,
    })
    if err := resolver.RunOnce(ctx); err != nil { return err }

    // 1h stale unresolved → page.
    var stale []webhookevents.StripeWebhookEvent
    err := c.cfg.DB.WithContext(ctx).
        Where("store_id IS NULL AND processed_at IS NULL AND manual_review_required = false").
        Where("received_at < now() - make_interval(secs => ?)", int64(c.cfg.StaleThreshold/time.Second)).
        Find(&stale).Error
    if err != nil { return err }

    for _, e := range stale {
        if c.cfg.PagerDuty != nil {
            _ = c.cfg.PagerDuty.Trigger(ctx, fmt.Sprintf("Stripe webhook orphan >1h: event_id=%s type=%s", e.EventID, e.EventType))
        }
    }
    return nil
}
```

`pagerduty.go`:

```go
package dispatch

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
)

type HTTPPagerDuty struct {
    URL    string
    Client *http.Client
}

func (p *HTTPPagerDuty) Trigger(ctx context.Context, summary string) error {
    if p.URL == "" { return nil } // disabled in dev
    body, _ := json.Marshal(map[string]any{
        "summary":  summary,
        "severity": "error",
        "source":   "marketplace-api",
    })
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, strings.NewReader(string(body)))
    req.Header.Set("Content-Type", "application/json")
    client := p.Client
    if client == nil { client = http.DefaultClient }
    resp, err := client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 {
        return fmt.Errorf("pagerduty: status %d", resp.StatusCode)
    }
    return nil
}
```

- [ ] **Step 3: Run tests — PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/{cron,pagerduty}{,_test}.go
git commit -m "feat(billing): orphan cron with PagerDuty on 1h unresolved"
```

---

## Task 15: Wire everything into `main.go`

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go` (remove inline webhook)
- Modify: `services/marketplace-api/internal/handlers/admin/subscription.go` (delegate to new Stripe clients for checkout/portal)
- Modify: `services/marketplace-api/pkg/config/config.go` (add new config knobs)

- [ ] **Step 1: Add config fields**

In `pkg/config/config.go` append to the struct:

```go
StripeAllowedEventTypes     []string      `envconfig:"STRIPE_ALLOWED_EVENT_TYPES" default:"checkout.session.completed,customer.subscription.updated,customer.subscription.deleted,invoice.paid,invoice.payment_failed,invoice.payment_action_required,customer.updated,charge.refunded,payment_method.attached,payment_method.detached,radar.early_fraud_warning"`
WebhookMaxBodyBytes         int64         `envconfig:"WEBHOOK_MAX_BODY_BYTES" default:"524288"`
OrphanRetryMaxCount         int           `envconfig:"ORPHAN_RETRY_MAX_COUNT" default:"6"`
OrphanRetryInterval         time.Duration `envconfig:"ORPHAN_RETRY_INTERVAL" default:"5m"`
OrphanStaleThreshold        time.Duration `envconfig:"ORPHAN_STALE_THRESHOLD" default:"1h"`
PagerDutyWebhookURL         string        `envconfig:"PAGERDUTY_WEBHOOK_URL" default:""`
```

- [ ] **Step 2: Update `main.go`**

Approximate edits (exact line numbers depend on prior merges):

```go
// Replace the existing subscription stub wiring.
stripeCli := billingstripe.New(cfg.StripeBillingSecretKey)
subscriptionSvc := subscription.NewService(subscription.ServiceConfig{
    DB:     conn,
    Repo:   subscriptionRepo,
    Stripe: stripeCli, // concrete client now
    Logger: log,
})

// New webhook handler at /webhooks/stripe-billing.
allowed := map[string]bool{}
for _, t := range cfg.StripeAllowedEventTypes { allowed[t] = true }

webhookHandler := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
    DB:           conn,
    Secret:       cfg.StripeBillingWebhookSecret,
    Repo:         webhookevents.NewRepository(),
    Dispatch:     dispatcher.Dispatch,
    AllowedTypes: allowed,
    MaxBodyBytes: cfg.WebhookMaxBodyBytes,
    Logger:       log,
})
router.POST("/webhooks/stripe-billing", webhookHandler.Handle)

// Orphan cron.
pd := &dispatch.HTTPPagerDuty{URL: cfg.PagerDutyWebhookURL}
orphanCron := dispatch.NewCron(dispatch.CronConfig{
    DB:             conn,
    Repo:           webhookevents.NewRepository(),
    Dispatcher:     dispatcher,
    PagerDuty:      pd,
    StaleThreshold: cfg.OrphanStaleThreshold,
    Interval:       cfg.OrphanRetryInterval,
    MaxRetries:     cfg.OrphanRetryMaxCount,
})
if err := orphanCron.Start(appCtx); err != nil {
    log.Error("orphan cron failed to start", "err", err); os.Exit(1)
}
defer orphanCron.Stop()
```

- [ ] **Step 3: Remove inline webhook route from `routes.go`**

Delete the `POST /webhooks/stripe-billing` registration inside the admin router (pre-P2 stub at `handlers/admin/subscription.go:179`). The new mount is in `main.go` as a router-level route — webhooks are not admin-authed.

- [ ] **Step 4: Update `handlers/admin/subscription.go` — delegate to new clients**

Keep the handler interfaces identical; internally call the new `billingstripe.CreateCheckoutSession` / `CreatePortalSession` instead of the old `StripeClient` interface. Pass tenantID, storeID, selected Price ID (looked up in `pricing` catalog by plan+period+merchant-currency), and the merchant's country-inferred currency.

Currency-selection rule (narrow — full logic moves to P3/P16):
- If `subscription.BillingCurrency` is set → use it.
- Else if `subscription.TaxIDCountry` is set → map country → currency via `pricing.CurrencyForCountry`.
- Else default to `usd`.

- [ ] **Step 5: Build + smoke test**

```bash
cd services/marketplace-api
go build ./...
```

Run a local stack:
```bash
docker-compose up -d postgres
export TEST_DATABASE_URL=... STRIPE_BILLING_SECRET_KEY=sk_test_...
go run ./cmd/billing-bootstrap
go run ./cmd/marketplace-api
# in another terminal:
stripe listen --forward-to localhost:8080/webhooks/stripe-billing --events checkout.session.completed
stripe trigger checkout.session.completed
```

Verify:
- Webhook returns `{"status":"processed"}` on first event, `{"status":"duplicate"}` on retry.
- `stripe_webhook_events` row exists with `processed_at IS NOT NULL`.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go \
        services/marketplace-api/internal/handlers/admin/{routes.go,subscription.go} \
        services/marketplace-api/pkg/config/config.go
git commit -m "feat(billing): wire new Stripe client + webhook dispatcher + orphan cron"
```

---

## Task 16: Integration test suite

**Files:**
- Create: `services/marketplace-api/internal/handlers/webhooks/stripe_integration_test.go`
- Create: `services/marketplace-api/internal/billing/dispatch/e2e_test.go`
- Create: `services/marketplace-api/scripts/webhook-fixtures/` with 11 `.json` fixtures (one per allowlisted event type)

- [ ] **Step 1: Generate fixtures**

Use `stripe fixtures list` or hand-craft — each file contains a valid event envelope. Store under `scripts/webhook-fixtures/{event_type}.json`.

- [ ] **Step 2: Write full-flow integration test**

```go
//go:build integration

func TestFullWebhookFlow_AllAllowlistedEvents(t *testing.T) {
    dir := "../../scripts/webhook-fixtures"
    entries, _ := os.ReadDir(dir)
    for _, e := range entries {
        t.Run(e.Name(), func(t *testing.T) {
            body, _ := os.ReadFile(filepath.Join(dir, e.Name()))
            // Sign, post to handler, verify 200, verify event persisted.
        })
    }
}
```

- [ ] **Step 3: Run**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/handlers/webhooks/... ./internal/billing/dispatch/... -v
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/webhooks/stripe_integration_test.go \
        services/marketplace-api/internal/billing/dispatch/e2e_test.go \
        services/marketplace-api/scripts/webhook-fixtures/
git commit -m "test(billing): integration suite for all 11 allowlisted webhook types"
```

---

## Final verification

- [ ] `go build ./...` green.
- [ ] `go test -tags=integration ./...` green.
- [ ] `stripe listen` end-to-end manual check succeeds for `checkout.session.completed`, `customer.subscription.updated`, `invoice.payment_failed`.
- [ ] Grep: `grep -R "resp.Body\|body, _ := io\.ReadAll" services/marketplace-api/internal/billing/` returns only our sanitized paths (no legacy raw-body log statements).
- [ ] Stripe test dashboard shows 3 Products + 8 developed + N PPP Prices with expected metadata.
- [ ] One signed-payload replay into the webhook endpoint within the 5m window returns 200 (`duplicate`). Two different events return 200 (`processed`).
- [ ] Intentionally-broken dispatch (e.g. missing `stripe_customer_id`) shows `retry_count` incrementing and flipping to `manual_review_required` after 6 tries.

## What's now unlocked

- **P3** plugs state-machine transitions into the per-event handlers (`handleInvoicePaymentFailed` → call state machine instead of direct UPDATE).
- **P5** adds the trial card-add deferred-charge flow to `handleCheckoutSessionCompleted`.
- **P6** consumes `invoice.payment_action_required` persistence to drive the recovery UI.
- **P10** adds refund bookkeeping to `handleChargeRefunded`.
- **P17** observability reads `webhook.processed` / `webhook.failed` / `webhook.orphan_resolved_after_seconds` from the table + cron.

## Execution handoff

Plan complete. Execute with **superpowers:subagent-driven-development** or **superpowers:executing-plans**.
