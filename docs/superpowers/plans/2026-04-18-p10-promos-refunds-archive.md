# P10 — Promo Codes + Refunds + Billing Archive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire three revenue/compliance features on top of P1 (data model) + P2 (Stripe client) + P3 (state machine): (1) a promo-code engine whose backend of record is Stripe Coupon with an absolute per-currency floor, timing-safe validation, and the exact abuse-prevention rules the spec specifies; (2) a 14-day cooling-off refund endpoint with card-fingerprint and device-fingerprint fraud guards logged to a dedicated `refund_audit` table; (3) a billing-archive hook that runs **before** hard-delete to write a 7-year-retained row into the P1-provisioned `billing_archive` table plus a daily expiry sweeper.

**Architecture:** Three self-contained packages — `internal/promo/`, `internal/refund/`, `internal/billing/archive/` — each wired into the admin router and the cron dispatcher. `promo` owns `promo_codes` and `promo_redemptions` (new here); exposes `ApplyPromo(ctx, storeID, code)` which calls the P2 Stripe client to create/fetch a Coupon, runs the §7.4 floor check, updates the subscription discount via `POST /v1/subscriptions/:id`, and emits `subscription.promo_applied`. `refund` owns `refund_audit` (new here); exposes `IssueRefund(ctx, storeID, reason, deviceFP)` which verifies the 14-day gate against Stripe's `charge.created` timestamp, asserts card-fingerprint uniqueness, calls Stripe `/v1/refunds` via P2's `RefundIdempotencyKey`, and emits `subscription.refund_issued` with severity=warning. `archive` exposes `BuildAndPersist(ctx, tx, storeID)` called synchronously from P11's hard-delete path **before** the row is removed, plus an `ExpirySweeper` cron. All three paths emit audit via the P1 emitter; no direct writes to `store_subscriptions.status` happen here (P3's state machine is unaffected — promo/refund don't change state; save-offer reversal `cancel_scheduled → active` is P11's concern).

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `crypto/subtle` for timing-safe equality, existing `internal/ratelimit/` package, P2's `stripe.Client` + `RefundIdempotencyKey`, P1's `audit.Emitter`, P1's migration 044 `billing_archive` table.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §7 (promo rules, shapes, abuse prevention), §7.4 (absolute floor per plan/currency — Council finding #5), §8 (refunds: 14-day cooling-off + fingerprint fraud guards), §15 (cancellation — P11 owns the flow; P10 only exposes the refund/promo service interfaces), §23.1 (audit events), §23.2 (billing archive — 7-year retention, table already exists from P1 migration 044).

**Depends on:**
- **P1** — `billing_archive` table exists; `audit.Emitter` + `subscription.WithAdvisoryLock` + `StoreSubscription` model + FX-rate helper for USD normalization.
- **P2** — `stripe.Client.PostForm`/`.Get` + `RefundIdempotencyKey` + sanitized `APIError`. Add `CouponIdempotencyKey` and `SubscriptionDiscountIdempotencyKey` here.
- **P3** — *unaffected*. Promo and refund are orthogonal to status. Save-offer acceptance (`cancel_scheduled → active`) is invoked by P11 using `statemachine.Transition`; P10 only exposes the promo-application service interface P11 calls.

**Related plans:**
- **P11** (cancellation + save-offer) — invokes `promo.ApplyPromo` during save-offer acceptance and `archive.BuildAndPersist` immediately before hard-delete row removal.
- **P16** (admin frontend) — consumes the promo + refund endpoints.
- **P17** (observability) — reads `subscription.promo_applied`, `subscription.promo_cancelled`, `subscription.refund_issued`, `subscription.billing_archived` for dashboards + alerts.

---

## Scope Check

In scope:
1. `promo_codes` table (migration 050) — code (≥12 chars via CHECK), `stripe_coupon_id`, `discount_type`/`discount_value`, `max_duration_months`, `valid_from/until`, `max_redemptions`, `max_per_email`, `min_effective_price_per_currency` JSONB, `created_by`.
2. `promo_redemptions` table (migration 051) — `promo_code_id`, `store_id`, `subscription_id`, `email`, `redeemed_at`; UNIQUE `(promo_code_id, email)`.
3. `refund_audit` table (migration 052) — subscription/store/tenant, `stripe_refund_id`, `stripe_charge_id`, `amount_minor`, `currency`, `reason`, `card_fingerprint`, `device_fingerprint`, `issued_by`; unique partial index on `card_fingerprint WHERE card_fingerprint IS NOT NULL`.
4. Code generator (§7.3) — min 12 chars, mixed-case alphanumeric, visually-safe charset.
5. Timing-safe validator (§7.3) — `crypto/subtle.ConstantTimeCompare`; uniform `ErrInvalidOrExpired` external error.
6. Floor table + `CheckFloor(plan, currency, effectiveMinor)` (§7.4) — Starter $12/Studio $30/Pro $75 USD; ₹800/₹1,800/₹4,200 INR; unknown currency → `ErrCurrencyNotCovered`.
7. Stripe Coupon client — Create, Get, AttachToSubscription, DetachFromSubscription.
8. `promo.Service` — `ApplyPromo` + `CancelPromo` + `ValidateForSaveOffer`.
9. Rate limits: 5/IP/hour + 10/email/24h via existing `internal/ratelimit/`.
10. Admin endpoint: `POST /admin/stores/:storeId/subscription/apply-promo` + `DELETE /admin/stores/:storeId/subscription/promo`.
11. `refund.Service` — 14-day cooling-off gate (using Stripe's `charge.created`, not local first_charge_at), card-fingerprint uniqueness lookup, device-fingerprint logging, Pro+App setup-fee refuse.
12. Admin endpoint: `POST /admin/stores/:storeId/subscription/refund` (body `{reason}`, header `X-Device-Fingerprint`).
13. `archive.Builder.BuildAndPersist` — fetches all Stripe invoices for the customer, sums `total_revenue_usd` via mid-market FX helper, writes archive row with `archive_expires_at = hard_deleted_at + 7 years`.
14. `archive.Sweeper.RunOnce` — daily cron, batch of 500 with `SKIP LOCKED`, emits audit on each deletion.
15. Audit event helpers (§23.1): `EmitPromoApplied` (incl. reject reason), `EmitPromoCancelled`, `EmitRefundIssued` (severity=warning), `EmitBillingArchived` (op `create`/`expired`).
16. Integration test — spec criterion **#40**: 50% off ₹999 Starter → rejected with `below_absolute_floor` in audit, uniform `promo_invalid_or_expired` in HTTP.
17. Integration test — refund 14-day gate: charge dated 15 days ago → 403 with `outside_cooling_off` + "cancel at period end" message.

Out of scope:
- Cancellation UX, save-offer UI, `cancel_scheduled → active` transition — **P11**.
- Admin UI for promo/refund/archive management — **P16**.
- SOC2 refund-audit exports + compliance reporting — post-launch (§26).
- Grandfathered-price one-shot coupon as a *separate* shape — v1 treats it as a regular Stripe Coupon; §7.2 "grandfathered_price override" is a P11/P16 segment strategy.
- Automated CSM refund approval workflow — v1 is CSM-driven via Stripe Dashboard as escape hatch (§8); this plan's endpoint is for the straightforward 14-day self-serve path.
- Pro+App $2,000 setup-fee refund — never refundable per §8; the endpoint MUST refuse.

---

## File Structure

### Create

- `services/marketplace-api/internal/db/migrations/050_promo_codes.up.sql` / `.down.sql`
- `services/marketplace-api/internal/db/migrations/051_promo_redemptions.up.sql` / `.down.sql`
- `services/marketplace-api/internal/db/migrations/052_refund_audit.up.sql` / `.down.sql`
- `services/marketplace-api/internal/promo/{model,errors,generator,floor,validator,stripe,repository,ratelimit,service}.go` + `*_test.go`
- `services/marketplace-api/internal/refund/{model,errors,repository,service}.go` + `*_test.go`
- `services/marketplace-api/internal/billing/archive/{model,errors,builder,sweeper}.go` + `*_test.go`
- `services/marketplace-api/internal/billing/stripe/refund.go` — Stripe `POST /v1/refunds` + `GetCharge` + `ListInvoicesForCustomer` + `LatestChargeForSubscription`
- `services/marketplace-api/internal/handlers/admin/promo.go` + `promo_test.go` + `promo_integration_test.go` (spec criterion #40)
- `services/marketplace-api/internal/handlers/admin/refund.go` + `refund_test.go` + `refund_integration_test.go` (14-day gate)

### Modify

- `services/marketplace-api/internal/billing/stripe/client.go` — add `CouponIdempotencyKey`, `SubscriptionDiscountIdempotencyKey` helpers.
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire promo + refund + archive deps and register routes; register sweeper cron.
- `services/marketplace-api/internal/audit/events.go` — add `EmitPromoApplied`, `EmitPromoCancelled`, `EmitRefundIssued`, `EmitBillingArchived`.
- `services/marketplace-api/internal/handlers/admin/routes.go` — mount the three new endpoints under `/admin/stores/:storeId/subscription/*` (already on P3's read-only allowlist, so merchants in expired/closed state can still apply save-offer promos or request refunds).
- `services/marketplace-api/internal/cron/registry.go` — register `billing-archive-sweeper` daily at 03:15 UTC.

### Delete

- None. This is additive.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migrations 050 + 051 + 052 | P1 |
| 2 | Promo models + repository | 1 |
| 3 | Code generator (§7.3 charset + length) | — |
| 4 | Absolute floor table + `CheckFloor` (§7.4) | — |
| 5 | Stripe Coupon client + idempotency helpers | P2 |
| 6 | Timing-safe validator (§7.3) | 2, 3 |
| 7 | `promo.Service` — Apply / Cancel / ValidateForSaveOffer | 2, 4, 5, 6 |
| 8 | Rate-limit wrapper (5/IP/hr + 10/email/24h) | — |
| 9 | Promo admin handler + routes | 7, 8 |
| 10 | Spec-criterion #40 integration test (₹999 × 50% reject) | 9 |
| 11 | Refund model + repository | 1 |
| 12 | Stripe refund/charge/invoice client additions | P2 |
| 13 | `refund.Service` — 14-day gate + card-FP uniqueness | 11, 12 |
| 14 | Refund admin handler + routes | 13 |
| 15 | Refund 14-day + fingerprint integration tests | 14 |
| 16 | `archive.Builder` — invoice fetch + FX sum + persist | P1, P2 |
| 17 | `archive.Sweeper` — daily expiry cron | 16 |
| 18 | Audit event helpers | 7, 13, 16 |
| 19 | Wire all in `main.go` + cron registry | all |
| 20 | Final scrub + full suite green | all |

---

## Reusable patterns

**A. Timing-safe uniform reject.** Every reject branch of promo validation — "not found", "expired", "max redemptions reached", "below floor", "already redeemed by email", "max per email", "wrong plan", "annual-only shape on monthly billing" — returns the **same** `ErrInvalidOrExpired` to the caller and the **same** JSON body to the HTTP client: `{"error":"promo_invalid_or_expired"}`. Internally, the service records the true reason in the audit event (never in the response). The validator uses `crypto/subtle.ConstantTimeCompare` when matching the submitted code to candidate rows — even though DB lookup is indexed, constant-time comparison avoids leaking timing signals that could distinguish "close match" from "no match".

**B. Stripe as backend of record.** `promo_codes.stripe_coupon_id` is populated at code creation. `ApplyPromo` uses that coupon ID to call `POST /v1/subscriptions/:subID` with `discounts[0][coupon]=<coupon_id>` and `idempotency_key = SubscriptionDiscountIdempotencyKey(subID, couponID)` so retries are safe. We **do not** model discounts in our DB beyond the redemption record; Stripe is source of truth for what's actually attached. On promo cancel, PUT `discounts[]` back to empty.

**C. Absolute floor as a pure function.** `floor.CheckFloor(plan, currency, effectiveMinor)` is pure data + arithmetic: table lookup for `(plan, currency) -> floor_minor`, one inequality. The caller computes the effective price via `floor.ComputeEffective(baseMinor, percentOffBps, amountOffMinor)` — all minor units, no floats.

**D. Card-fingerprint uniqueness.** `refund_audit` has a unique partial index on `card_fingerprint` where not null. `IssueRefund` calls `refundRepo.ExistsByCardFingerprint(fp)` before touching Stripe — catches "same card, different subscription" fraud. Fingerprint comes from `charge.payment_method_details.card.fingerprint`. When rejecting, handler responds with generic `refund_unavailable` — never leaks fingerprint-match detail.

**E. 14-day gate uses Stripe's `charge.created`, not our `first_charge_at`.** Re-read charge from Stripe at refund time to defeat clock-drift attacks: `windowClosed := now.UTC().After(time.Unix(charge.Created,0).UTC().Add(14*24*time.Hour))`.

**F. Archive builder runs INSIDE the advisory lock of the hard-delete tx.** P11 calls `subscription.WithAdvisoryLock(ctx, db, storeID, func(tx) { archive.BuildAndPersist(tx, ...); tx.Delete(...) })`. Archive failure rolls back the delete. No orphaned deletes.

**G. Sweeper uses `DELETE ... RETURNING id` with `SKIP LOCKED` in batches.** Batch size 500, loop until empty; emit `subscription.billing_archived` with `op:"expired"` for each id. Daily cron; low volume.

**H. Rate-limit keys.** `"promo:ip:" + clientIP` window 1h limit 5; `"promo:email:" + lowerEmail` window 24h limit 10. Email lowered + trimmed before keying.

---

## Task 1: Migrations 050 + 051 + 052

**Files:**
- Create: `services/marketplace-api/internal/db/migrations/050_promo_codes.up.sql` + `.down.sql`
- Create: `services/marketplace-api/internal/db/migrations/051_promo_redemptions.up.sql` + `.down.sql`
- Create: `services/marketplace-api/internal/db/migrations/052_refund_audit.up.sql` + `.down.sql`

**Spec references:** §7.1–§7.4, §8.

- [ ] **Step 1: Write 050 up migration**

```sql
CREATE TYPE promo_discount_type AS ENUM ('percentage', 'amount');

CREATE TABLE promo_codes (
    id                               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                             VARCHAR(64) NOT NULL,
    stripe_coupon_id                 VARCHAR(100) NOT NULL,
    discount_type                    promo_discount_type NOT NULL,
    -- percentage: basis points (5000 = 50.00%, §7.1 max depth).
    -- amount:     minor units in amount_currency.
    discount_value                   INTEGER NOT NULL CHECK (discount_value >= 0),
    amount_currency                  CHAR(3),
    max_duration_months              INTEGER CHECK (max_duration_months IS NULL OR max_duration_months BETWEEN 1 AND 6),
    valid_from                       TIMESTAMPTZ NOT NULL,
    valid_until                      TIMESTAMPTZ NOT NULL,
    max_redemptions                  INTEGER NOT NULL CHECK (max_redemptions > 0),
    max_per_email                    INTEGER NOT NULL DEFAULT 1 CHECK (max_per_email >= 1),
    -- JSONB: {"USD":1200,"INR":80000,...} in MINOR units per §7.4.
    min_effective_price_per_currency JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by                       UUID NOT NULL,
    created_at                       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT promo_code_len CHECK (char_length(code) >= 12),               -- §7.3
    CONSTRAINT promo_code_unique UNIQUE (code),
    CONSTRAINT promo_percentage_max_50 CHECK (                               -- §7.1
        discount_type <> 'percentage' OR discount_value <= 5000
    ),
    CONSTRAINT promo_amount_has_currency CHECK (
        discount_type <> 'amount' OR amount_currency IS NOT NULL
    ),
    CONSTRAINT promo_validity_ordered CHECK (valid_until > valid_from)
);
CREATE INDEX idx_promo_codes_code_active ON promo_codes (code) WHERE valid_until > now();
```

- [ ] **Step 2: Write 051 up migration**

```sql
CREATE TABLE promo_redemptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_code_id   UUID NOT NULL REFERENCES promo_codes(id) ON DELETE RESTRICT,
    store_id        UUID NOT NULL,
    subscription_id UUID NOT NULL,
    email           VARCHAR(320) NOT NULL,
    redeemed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT promo_redemption_email_unique UNIQUE (promo_code_id, email),           -- §7.3
    CONSTRAINT promo_redemption_subscription_unique UNIQUE (subscription_id, promo_code_id) -- §7.1
);
CREATE INDEX idx_promo_redemptions_store ON promo_redemptions (store_id);
CREATE INDEX idx_promo_redemptions_email ON promo_redemptions (lower(email));
```

- [ ] **Step 3: Write 052 up migration**

```sql
CREATE TABLE refund_audit (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id    UUID NOT NULL,
    store_id           UUID NOT NULL,
    tenant_id          UUID NOT NULL,
    stripe_refund_id   VARCHAR(100) NOT NULL,
    stripe_charge_id   VARCHAR(100) NOT NULL,
    stripe_invoice_id  VARCHAR(100),
    amount_minor       BIGINT NOT NULL CHECK (amount_minor > 0),
    currency           CHAR(3) NOT NULL,
    reason             VARCHAR(500) NOT NULL,
    card_fingerprint   VARCHAR(100),
    device_fingerprint VARCHAR(200),
    issued_by          UUID NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT refund_audit_stripe_refund_id_unique UNIQUE (stripe_refund_id)
);
-- §8: one refund per card fingerprint lifetime.
CREATE UNIQUE INDEX idx_refund_audit_card_fp_unique
    ON refund_audit (card_fingerprint) WHERE card_fingerprint IS NOT NULL;
CREATE INDEX idx_refund_audit_store ON refund_audit (store_id);
CREATE INDEX idx_refund_audit_created_at ON refund_audit (created_at);
```

- [ ] **Step 4: Write all three down migrations** — `DROP TABLE IF EXISTS promo_redemptions; DROP TABLE IF EXISTS promo_codes; DROP TYPE IF EXISTS promo_discount_type; DROP TABLE IF EXISTS refund_audit;` split across the three files.

- [ ] **Step 5: Apply + verify + commit**

```bash
cd services/marketplace-api
go run ./cmd/migrate up
psql "$DATABASE_URL" -c '\d promo_codes' -c '\d promo_redemptions' -c '\d refund_audit'
git add services/marketplace-api/internal/db/migrations/050_*.sql \
        services/marketplace-api/internal/db/migrations/051_*.sql \
        services/marketplace-api/internal/db/migrations/052_*.sql
git commit -m "feat(db): promo_codes + promo_redemptions + refund_audit with §7/§8 constraints"
```

---

## Task 2: Promo models + repository

**Files:**
- Create: `services/marketplace-api/internal/promo/model.go`
- Create: `services/marketplace-api/internal/promo/errors.go`
- Create: `services/marketplace-api/internal/promo/repository.go`
- Create: `services/marketplace-api/internal/promo/repository_test.go` (build tag `integration`)

- [ ] **Step 1: Write failing tests**

Tests assert (a) `Create` rejects a code shorter than 12 chars at DB level, (b) `Create` rejects 51%-off percentage via the CHECK constraint, (c) `GetActiveByCode` returns `ErrNotFoundOrExpired` when `valid_until < now()`, (d) `RecordRedemption` twice with the same `(promo_code_id, email)` returns a DB unique-violation error.

```go
//go:build integration

func TestRepository_Create_EnforcesMinCodeLength(t *testing.T) {
    db := testdb.NewDB(t, "promo_codes")
    _, err := promo.NewRepository().Create(ctx, db, promo.PromoCode{Code: "TOO-SHORT", /* …valid rest… */})
    require.Error(t, err, "CHECK char_length(code) >= 12 must fire")
}
```
Four total test funcs of this shape.

- [ ] **Step 2: Write `model.go`** — GORM models `PromoCode` (all fields from migration 050; `MinEffectivePricePerCurrency` as `datatypes.JSON`; `DiscountType` string const enum `percentage`/`amount`) and `Redemption` (fields from migration 051). `TableName()` returns the migration names.

- [ ] **Step 3: Write `errors.go`**

```go
package promo

import "errors"

// External (response-mapped) — uniform across ALL reject paths per §7.3.
var ErrInvalidOrExpired = errors.New("promo: invalid or expired")

// Internal (audit-only). NEVER surfaced in HTTP responses.
var (
    ErrNotFoundOrExpired   = errors.New("promo: not found or expired")
    ErrBelowFloor          = errors.New("promo: below absolute floor")
    ErrMaxRedemptions      = errors.New("promo: max redemptions reached")
    ErrAlreadyRedeemed     = errors.New("promo: already redeemed by this email")
    ErrMaxPerEmail         = errors.New("promo: max per email reached")
    ErrAlreadyHasActive    = errors.New("promo: subscription already has an active promo")
    ErrAnnualOnlyOnMonthly = errors.New("promo: annual-only shape on monthly billing")
    ErrCurrencyNotCovered  = errors.New("promo: no floor configured for billing currency")
    ErrRateLimited         = errors.New("promo: rate limited")
)
```

- [ ] **Step 4: Write `repository.go`** — methods: `Create`, `GetActiveByCode(code, now)` (filters `valid_from <= now AND valid_until > now`, returns `ErrNotFoundOrExpired` on gorm.ErrRecordNotFound), `CountRedemptions(promoCodeID)`, `CountRedemptionsByEmail(promoCodeID, email)` (case-insensitive via `lower(email) = lower(?)`), `ActivePromoForSubscription(subID)`, `RecordRedemption`, `DeleteRedemption(subID)`. All methods take `ctx, db` and use `db.WithContext(ctx)`.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/promo/{model,errors,repository,repository_test}.go
git commit -m "feat(promo): models + repository with per-email and per-subscription uniqueness"
```

---

## Task 3: Code generator (§7.3 charset + length)

**Files:**
- Create: `services/marketplace-api/internal/promo/generator.go`
- Create: `services/marketplace-api/internal/promo/generator_test.go`

- [ ] **Step 1: Write failing tests** — (a) `Generate(n)` with `n < 12` returns an error; (b) output charset is `^[A-HJ-NP-Zabcdefghijkmnopqrstuvwxyz23456789]+$` (no 0/O/I/l/1); (c) for `n=24`, output contains at least one lower and one upper (probabilistic but near-certain with a 44-char alphabet).

- [ ] **Step 2: Write `generator.go`**

```go
package promo

import (
    "crypto/rand"
    "errors"
    "math/big"
)

// charset excludes visually ambiguous chars (§7.3): 0 O o / I l 1 omitted.
const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func Generate(n int) (string, error) {
    if n < 12 { return "", errors.New("promo: minimum length is 12 per §7.3") }
    a := []byte(charset)
    limit := big.NewInt(int64(len(a)))
    out := make([]byte, n)
    for i := range out {
        idx, err := rand.Int(rand.Reader, limit)
        if err != nil { return "", err }
        out[i] = a[idx.Int64()]
    }
    return string(out), nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(promo): code generator with §7.3 safe charset + 12-char floor"
```

---

## Task 4: Absolute floor table + `CheckFloor`

**Files:**
- Create: `services/marketplace-api/internal/promo/floor.go`
- Create: `services/marketplace-api/internal/promo/floor_test.go`

**Spec references:** §7.4 (Council finding #5). **Spec success criterion #40** tests against this directly.

- [ ] **Step 1: Write failing tests**

```go
func TestCheckFloor_Developed_StarterFloor_1200cents(t *testing.T) {
    require.NoError(t, promo.CheckFloor(subscription.PlanStarter, "USD", 1200))
    require.ErrorIs(t, promo.CheckFloor(subscription.PlanStarter, "USD", 1199), promo.ErrBelowFloor)
}

// Success criterion #40 — 50% off ₹999 Starter is rejected.
func TestCheckFloor_50PctOff_999INR_Starter_Rejected(t *testing.T) {
    // ₹999 base = 99900 paise; 50% off = 49950 paise; floor ₹800 = 80000 paise.
    require.ErrorIs(t,
        promo.CheckFloor(subscription.PlanStarter, "INR", int64(99900/2)),
        promo.ErrBelowFloor)
}

func TestCheckFloor_UnknownCurrency_ReturnsCurrencyNotCovered(t *testing.T) {
    require.ErrorIs(t, promo.CheckFloor(subscription.PlanStarter, "ZZZ", 10000), promo.ErrCurrencyNotCovered)
}
```

Also test Studio USD ($3000 floor), Pro USD ($7500 floor), Studio INR (₹1,800), Pro INR (₹4,200), and that `PlanTrial` rejects (free plan has no paid floor).

- [ ] **Step 2: Write `floor.go`**

```go
package promo

import (
    "fmt"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

// floors: (plan, currency) -> absolute floor in MINOR units per §7.4.
// Expand currencies as markets onboard; unknown currency rejects so a new
// market cannot silently bypass the floor.
var floors = map[subscription.SubscriptionPlan]map[string]int64{
    subscription.PlanStarter: {
        "USD": 1200, "EUR": 1100, "GBP": 950, "CAD": 1600, "AUD": 1800,
        "INR": 80_000, "BRL": 6_000, "MXN": 20_000, "PHP": 60_000,
    },
    subscription.PlanStudio: {
        "USD": 3000, "EUR": 2800, "GBP": 2400, "CAD": 4000, "AUD": 4500,
        "INR": 180_000, "BRL": 15_000, "MXN": 50_000,
    },
    subscription.PlanPro: {
        "USD": 7500, "EUR": 7000, "GBP": 6000, "CAD": 10000, "AUD": 11000,
        "INR": 420_000,
    },
    // PlanTrial intentionally absent — free plan has no paid floor.
}

func CheckFloor(plan subscription.SubscriptionPlan, currency string, effectiveMinor int64) error {
    perCurrency, ok := floors[plan]
    if !ok {
        return fmt.Errorf("%w: plan %s has no promo floor (trial?)", ErrBelowFloor, plan)
    }
    floor, ok := perCurrency[currency]
    if !ok {
        return ErrCurrencyNotCovered
    }
    if effectiveMinor < floor {
        return ErrBelowFloor
    }
    return nil
}

// ComputeEffective: baseMinor × (1 − percentOffBps/10000) − amountOffMinor, clamped ≥ 0.
// percentOffBps ∈ [0, 10000]; 5000 = 50.00%. Integer truncation toward zero matches Stripe.
func ComputeEffective(baseMinor int64, percentOffBps int, amountOffMinor int64) int64 {
    if percentOffBps > 0 {
        baseMinor -= baseMinor * int64(percentOffBps) / 10000
    }
    eff := baseMinor - amountOffMinor
    if eff < 0 { eff = 0 }
    return eff
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(promo): absolute floor table + ComputeEffective (§7.4 Council #5)"
```

---

## Task 5: Stripe Coupon client + idempotency helpers

**Files:**
- Create: `services/marketplace-api/internal/promo/stripe.go`
- Create: `services/marketplace-api/internal/promo/stripe_test.go`
- Modify: `services/marketplace-api/internal/billing/stripe/client.go`

**Spec references:** §7 (Stripe Coupon = canonical backend); §17.8 (idempotency).

- [ ] **Step 1: Add helpers to P2 client**

```go
func CouponIdempotencyKey(code string) string { return "coupon:" + code }

func SubscriptionDiscountIdempotencyKey(subID, couponID string) string {
    return fmt.Sprintf("sub-discount:%s:%s", subID, couponID)
}
```

- [ ] **Step 2: Write failing tests (HTTP-mocked)** — verify: (a) `CreateCoupon` posts to `/v1/coupons` with `Idempotency-Key: coupon:<code>`, emits `percent_off=50`, `duration=repeating`, `duration_in_months=6`; (b) `AttachToSubscription` posts to `/v1/subscriptions/:id` with `discounts[0][coupon]=<couponID>` and idempotency `sub-discount:<subID>:<couponID>`; (c) `DetachFromSubscription` posts empty `discounts[]=`.

- [ ] **Step 3: Write `stripe.go`** — thin wrapper around P2's `stripe.Client`:

```go
package promo

type StripeCoupon struct {
    ID         string `json:"id"`
    PercentOff int    `json:"percent_off"`
    AmountOff  int64  `json:"amount_off"`
    Currency   string `json:"currency"`
    Duration   string `json:"duration"`
}

type CreateCouponInput struct {
    Code             string
    // exactly one of PercentOffBps / AmountOffMinor set.
    PercentOffBps    int
    AmountOffMinor   int64
    AmountCurrency   string
    // DurationInMonths: N>0 repeating N; -1 once; 0 forever.
    DurationInMonths int
}

type StripeClient struct{ c *stripec.Client }

func NewStripeClient(c *stripec.Client) *StripeClient { return &StripeClient{c: c} }

func (s *StripeClient) CreateCoupon(ctx context.Context, in CreateCouponInput) (StripeCoupon, error) {
    v := url.Values{}
    v.Set("id", in.Code) // we pin the Stripe coupon ID to our internal code
    switch {
    case in.PercentOffBps > 0:
        v.Set("percent_off", strconv.Itoa(in.PercentOffBps/100))
    case in.AmountOffMinor > 0:
        v.Set("amount_off", strconv.FormatInt(in.AmountOffMinor, 10))
        v.Set("currency", in.AmountCurrency)
    default:
        return StripeCoupon{}, errors.New("promo: must specify percent_off or amount_off")
    }
    switch {
    case in.DurationInMonths > 0:
        v.Set("duration", "repeating"); v.Set("duration_in_months", strconv.Itoa(in.DurationInMonths))
    case in.DurationInMonths == -1:
        v.Set("duration", "once")
    default:
        v.Set("duration", "forever")
    }
    body, err := s.c.PostForm(ctx, "/v1/coupons", stripec.CouponIdempotencyKey(in.Code), v)
    if err != nil { return StripeCoupon{}, err }
    var cp StripeCoupon
    return cp, json.Unmarshal(body, &cp)
}

func (s *StripeClient) AttachToSubscription(ctx context.Context, subID, couponID string) error {
    v := url.Values{}
    v.Set("discounts[0][coupon]", couponID)
    _, err := s.c.PostForm(ctx, "/v1/subscriptions/"+subID,
        stripec.SubscriptionDiscountIdempotencyKey(subID, couponID), v)
    return err
}

func (s *StripeClient) DetachFromSubscription(ctx context.Context, subID string) error {
    v := url.Values{"discounts[]": {""}} // Stripe convention: empty list resets discounts
    _, err := s.c.PostForm(ctx, "/v1/subscriptions/"+subID, "sub-discount-detach:"+subID, v)
    return err
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/promo/{stripe,stripe_test}.go \
        services/marketplace-api/internal/billing/stripe/client.go
git commit -m "feat(promo): Stripe Coupon client + idempotent attach/detach on subscription"
```

---

## Task 6: Timing-safe validator

**Files:**
- Create: `services/marketplace-api/internal/promo/validator.go`
- Create: `services/marketplace-api/internal/promo/validator_test.go`

**Spec references:** §7.3.

- [ ] **Step 1: Write failing tests**

Assert (a) an unknown code and (b) an expired code both return `ErrInvalidOrExpired` (same error value, no info leak); (c) an active code returns the row; (d) a counter exposed via `ConstantTimeCompareCalls()` increments on every lookup so we can prove `subtle.ConstantTimeCompare` was exercised.

- [ ] **Step 2: Write `validator.go`**

```go
package promo

import (
    "context"
    "crypto/subtle"
    "errors"
    "sync/atomic"
    "time"

    "gorm.io/gorm"
)

type Validator struct {
    repo    *Repository
    ctCount int64 // observability hook for §7.3 test
}

func NewValidator(repo *Repository) *Validator { return &Validator{repo: repo} }
func (v *Validator) ConstantTimeCompareCalls() int64 { return atomic.LoadInt64(&v.ctCount) }

// Lookup collapses all failure paths to ErrInvalidOrExpired. Caller is expected
// to record the TRUE reason in the audit event; never in the HTTP response.
func (v *Validator) Lookup(ctx context.Context, db *gorm.DB, submitted string, now time.Time) (PromoCode, error) {
    if len(submitted) < 12 { return PromoCode{}, ErrInvalidOrExpired }

    pc, err := v.repo.GetActiveByCode(ctx, db, submitted, now)
    if err != nil {
        // not found, expired, or DB error -> uniform external error.
        _ = errors.Is(err, ErrNotFoundOrExpired) // retained for clarity
        return PromoCode{}, ErrInvalidOrExpired
    }

    // Defense-in-depth constant-time compare against the canonical row.
    atomic.AddInt64(&v.ctCount, 1)
    if subtle.ConstantTimeCompare([]byte(pc.Code), []byte(submitted)) != 1 {
        return PromoCode{}, ErrInvalidOrExpired
    }
    return pc, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(promo): timing-safe validator with uniform invalid-or-expired response"
```

---

## Task 7: `promo.Service` — Apply / Cancel / ValidateForSaveOffer

**Files:**
- Create: `services/marketplace-api/internal/promo/service.go`
- Create: `services/marketplace-api/internal/promo/service_test.go` (tag `integration`)

**Spec references:** §7.1 (post-trial only, one active promo per subscription), §7.3 (per-email uniqueness), §7.4 (floor), §15.3 (save-offer reuses promo infrastructure).

- [ ] **Step 1: Write failing integration tests**

Covers: (a) Starter USD with 20% off → floor OK → success, audit type `subscription.promo_applied`; (b) Starter INR with 50% off → floor reject → external error `ErrInvalidOrExpired`, audit metadata `reject_reason=below_absolute_floor`; (c) trial plan → reject with `reject_reason=pre_trial_apply`; (d) already-has-active-promo → reject with `already_has_active_promo`; (e) same email on two different subscriptions → second rejects with `max_per_email_reached`; (f) `CancelPromo` calls Stripe detach and deletes the redemption row and emits `subscription.promo_cancelled`; (g) `ValidateForSaveOffer` returns a preview without writing a redemption row.

```go
func TestApply_Starter_INR_50PercentOff_BelowFloor_Rejected(t *testing.T) {
    h := newServiceHarness(t)
    _, sub := h.SeedActiveSubscription(subscription.PlanStarter, "INR", 99900)
    pc := h.SeedPromo("MARK8LY50NOPE42", 5000 /* 50% */, 6)

    _, err := h.Service.ApplyPromo(ctx, promo.ApplyInput{
        SubscriptionID: sub.ID, Email: "buyer@x.com", Code: pc.Code,
    })
    require.ErrorIs(t, err, promo.ErrInvalidOrExpired, "external must be generic per §7.3")

    e := h.Audit.Events()[0]
    require.Equal(t, "subscription.promo_apply_rejected", e.Type)
    require.Equal(t, "below_absolute_floor", e.Metadata["reject_reason"])
}
```

- [ ] **Step 2: Write `service.go`**

Key shape (trimmed):

```go
package promo

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type Service struct {
    db        *gorm.DB
    repo      *Repository
    validator *Validator
    stripe    *StripeClient
    emitter   *audit.Emitter
    subRepo   subscription.Repository
    now       func() time.Time
}

type Deps struct {
    DB *gorm.DB; Repo *Repository; Validator *Validator
    Stripe *StripeClient; Emitter *audit.Emitter
    SubRepo subscription.Repository; Now func() time.Time
}

func NewService(d Deps) *Service {
    if d.Now == nil { d.Now = time.Now }
    return &Service{db: d.DB, repo: d.Repo, validator: d.Validator,
        stripe: d.Stripe, emitter: d.Emitter, subRepo: d.SubRepo, now: d.Now}
}

type ApplyInput struct {
    TenantID, StoreID, SubscriptionID, ActorUserID uuid.UUID
    Email, Code                                    string
}

type ApplyOutput struct {
    PromoCodeID      uuid.UUID
    StripeCouponID   string
    EffectiveMinor   int64
    DurationInMonths int
}

// ApplyPromo runs validation + Stripe attach + redemption write under the
// subscription's advisory lock so concurrent applies serialize. Every reject
// path collapses to ErrInvalidOrExpired externally; the TRUE reason is
// recorded in the audit event (§7.3).
func (s *Service) ApplyPromo(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
    email := strings.ToLower(strings.TrimSpace(in.Email))
    var out ApplyOutput

    err := subscription.WithAdvisoryLock(ctx, s.db, in.SubscriptionID, func(tx *gorm.DB) error {
        sub, err := s.subRepo.GetByID(ctx, tx, in.SubscriptionID)
        if err != nil { return s.rejectAndAudit(in, "subscription_not_found") }

        // §7.1 post-trial only.
        if sub.Plan == subscription.PlanTrial ||
            sub.Status == subscription.StatusTrialing ||
            sub.Status == subscription.StatusSignup {
            return s.rejectAndAudit(in, "pre_trial_apply")
        }

        // §7.1 one active promo per subscription.
        if existing, err := s.repo.ActivePromoForSubscription(ctx, tx, in.SubscriptionID); err == nil && existing.ID != uuid.Nil {
            return s.rejectAndAudit(in, "already_has_active_promo")
        }

        pc, err := s.validator.Lookup(ctx, tx, in.Code, s.now())
        if err != nil { return s.rejectAndAudit(in, "not_found_or_expired") }

        // §7.1 max_redemptions.
        total, err := s.repo.CountRedemptions(ctx, tx, pc.ID)
        if err != nil { return s.rejectAndAudit(in, "count_query_error") }
        if total >= int64(pc.MaxRedemptions) { return s.rejectAndAudit(in, "max_redemptions_reached") }

        // §7.3 per-email uniqueness.
        perEmail, err := s.repo.CountRedemptionsByEmail(ctx, tx, pc.ID, email)
        if err != nil { return s.rejectAndAudit(in, "count_query_error") }
        if perEmail >= int64(pc.MaxPerEmail) { return s.rejectAndAudit(in, "max_per_email_reached") }

        // §7.4 floor check.
        percent, amount := 0, int64(0)
        if pc.DiscountType == DiscountPercentage {
            percent = pc.DiscountValue
        } else {
            amount = int64(pc.DiscountValue)
        }
        effective := ComputeEffective(sub.BaseAmountMinor, percent, amount)
        if err := CheckFloor(sub.Plan, sub.BillingCurrency, effective); err != nil {
            return s.rejectAndAudit(in, "below_absolute_floor")
        }

        if err := s.stripe.AttachToSubscription(ctx, sub.StripeSubscriptionID, pc.StripeCouponID); err != nil {
            return s.rejectAndAudit(in, "stripe_attach_failed")
        }

        rd, err := s.repo.RecordRedemption(ctx, tx, Redemption{
            PromoCodeID: pc.ID, StoreID: in.StoreID, SubscriptionID: in.SubscriptionID,
            Email: email, RedeemedAt: s.now(),
        })
        if err != nil {
            // Compensate Stripe on DB failure.
            _ = s.stripe.DetachFromSubscription(ctx, sub.StripeSubscriptionID)
            return s.rejectAndAudit(in, "redemption_write_failed")
        }

        out = ApplyOutput{
            PromoCodeID: pc.ID, StripeCouponID: pc.StripeCouponID,
            EffectiveMinor: effective, DurationInMonths: ptrOrZero(pc.MaxDurationMonths),
        }
        if s.emitter != nil {
            s.emitter.EmitPromoApplied(audit.PromoApplied{
                TenantID: in.TenantID, StoreID: in.StoreID,
                SubscriptionID: in.SubscriptionID, PromoCodeID: pc.ID,
                StripeCouponID: pc.StripeCouponID, RedemptionID: rd.ID,
                Email: email, EffectiveMinor: effective, ActorUserID: in.ActorUserID,
            })
        }
        return nil
    })
    if err != nil { return ApplyOutput{}, ErrInvalidOrExpired }
    return out, nil
}

func (s *Service) rejectAndAudit(in ApplyInput, reason string) error {
    if s.emitter != nil {
        s.emitter.EmitPromoApplied(audit.PromoApplied{
            TenantID: in.TenantID, StoreID: in.StoreID,
            SubscriptionID: in.SubscriptionID,
            Email: strings.ToLower(in.Email),
            RejectReason: reason, ActorUserID: in.ActorUserID,
        })
    }
    return ErrInvalidOrExpired
}

type CancelInput struct {
    TenantID, StoreID, SubscriptionID, ActorUserID uuid.UUID
    Reason                                         string
}

// CancelPromo — detaches Stripe coupon and removes the redemption row.
// Used by save-offer reversal (P11) and admin override. Idempotent on no-op.
func (s *Service) CancelPromo(ctx context.Context, in CancelInput) error {
    return subscription.WithAdvisoryLock(ctx, s.db, in.SubscriptionID, func(tx *gorm.DB) error {
        sub, err := s.subRepo.GetByID(ctx, tx, in.SubscriptionID)
        if err != nil { return err }
        rd, err := s.repo.ActivePromoForSubscription(ctx, tx, in.SubscriptionID)
        if err != nil {
            if errors.Is(err, ErrNotFoundOrExpired) { return nil } // idempotent
            return err
        }
        if err := s.stripe.DetachFromSubscription(ctx, sub.StripeSubscriptionID); err != nil { return err }
        if err := s.repo.DeleteRedemption(ctx, tx, in.SubscriptionID); err != nil { return err }
        if s.emitter != nil {
            s.emitter.EmitPromoCancelled(audit.PromoCancelled{
                TenantID: in.TenantID, StoreID: in.StoreID,
                SubscriptionID: in.SubscriptionID, PromoCodeID: rd.PromoCodeID,
                Reason: in.Reason, ActorUserID: in.ActorUserID,
            })
        }
        return nil
    })
}

type ValidateInput struct {
    SubscriptionID uuid.UUID
    Email, Code    string
}
type ValidateOutput struct {
    WouldApply       bool
    EffectiveMinor   int64
    DurationInMonths int
}

// ValidateForSaveOffer — preview a save-offer code (§15.3) without redeeming.
// No Stripe attach, no redemption row. Per-email / max_redemptions checks
// deferred to actual apply (P11 calls ApplyPromo on merchant accept).
func (s *Service) ValidateForSaveOffer(ctx context.Context, in ValidateInput) (ValidateOutput, error) {
    sub, err := s.subRepo.GetByID(ctx, s.db, in.SubscriptionID)
    if err != nil { return ValidateOutput{}, ErrInvalidOrExpired }
    pc, err := s.validator.Lookup(ctx, s.db, in.Code, s.now())
    if err != nil { return ValidateOutput{}, ErrInvalidOrExpired }
    percent, amount := 0, int64(0)
    if pc.DiscountType == DiscountPercentage { percent = pc.DiscountValue } else { amount = int64(pc.DiscountValue) }
    eff := ComputeEffective(sub.BaseAmountMinor, percent, amount)
    if err := CheckFloor(sub.Plan, sub.BillingCurrency, eff); err != nil {
        return ValidateOutput{WouldApply: false}, nil
    }
    return ValidateOutput{
        WouldApply: true, EffectiveMinor: eff, DurationInMonths: ptrOrZero(pc.MaxDurationMonths),
    }, nil
}

func ptrOrZero(p *int) int { if p == nil { return 0 }; return *p }
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(promo): Service.ApplyPromo / CancelPromo / ValidateForSaveOffer with floor + abuse checks"
```

---

## Task 8: Rate-limit wrapper

**Files:**
- Create: `services/marketplace-api/internal/promo/ratelimit.go`
- Create: `services/marketplace-api/internal/promo/ratelimit_test.go`

**Spec references:** §7.3 (5/IP/hour + 10/email/24h).

- [ ] **Step 1: Write failing tests** — (a) 6th call from same IP in 1h rejects with `ErrRateLimited`; (b) 11th email call in 24h rejects; (c) different emails are independent; (d) email keying is case-insensitive.

- [ ] **Step 2: Write `ratelimit.go`**

```go
package promo

import (
    "context"
    "strings"
    "time"

    "github.com/tesserix/marketplace-api/internal/ratelimit"
)

type RateLimiter struct{ inner ratelimit.Limiter }

func NewRateLimiter(inner ratelimit.Limiter) *RateLimiter { return &RateLimiter{inner: inner} }

func (r *RateLimiter) CheckIP(ctx context.Context, ip string) error {
    ok, err := r.inner.AllowN(ctx, "promo:ip:"+ip, 5, 1*time.Hour)
    if err != nil { return err }
    if !ok { return ErrRateLimited }
    return nil
}

func (r *RateLimiter) CheckEmail(ctx context.Context, email string) error {
    key := "promo:email:" + strings.ToLower(strings.TrimSpace(email))
    ok, err := r.inner.AllowN(ctx, key, 10, 24*time.Hour)
    if err != nil { return err }
    if !ok { return ErrRateLimited }
    return nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(promo): rate-limit 5/IP/hr + 10/email/24h (§7.3)"
```

---

## Task 9: Promo admin handler + routes

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/promo.go`
- Create: `services/marketplace-api/internal/handlers/admin/promo_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

- [ ] **Step 1: Write failing handler tests** — (a) valid code returns 200 with `stripe_coupon_id` body; (b) unknown code returns 422 with body `{"error":"promo_invalid_or_expired"}`; (c) 6th IP attempt returns 429; (d) `DELETE /subscription/promo` returns 204.

- [ ] **Step 2: Write `promo.go`**

```go
package admin

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/promo"
)

type PromoHandler struct {
    svc *promo.Service
    rl  *promo.RateLimiter
}

func NewPromoHandler(svc *promo.Service, rl *promo.RateLimiter) *PromoHandler {
    return &PromoHandler{svc: svc, rl: rl}
}

type applyPromoBody struct {
    Code string `json:"code" binding:"required,min=12,max=64"`
}

func (h *PromoHandler) Apply(c *gin.Context) {
    var body applyPromoBody
    if err := c.ShouldBindJSON(&body); err != nil {
        // Uniform external error — never leak schema detail.
        c.AbortWithStatusJSON(http.StatusUnprocessableEntity,
            gin.H{"error": "promo_invalid_or_expired"})
        return
    }
    tenantID, _ := c.Get("tenant_id")
    userID, _   := c.Get("user_id")
    email, _    := c.Get("user_email")
    subID, _    := c.Get("subscription_id")
    storeID := uuid.MustParse(c.Param("storeId"))

    if err := h.rl.CheckIP(c.Request.Context(), c.ClientIP()); err != nil {
        c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_requests"}); return
    }
    if err := h.rl.CheckEmail(c.Request.Context(), email.(string)); err != nil {
        c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too_many_requests"}); return
    }

    out, err := h.svc.ApplyPromo(c.Request.Context(), promo.ApplyInput{
        TenantID: tenantID.(uuid.UUID), StoreID: storeID,
        SubscriptionID: subID.(uuid.UUID),
        Email: email.(string), Code: body.Code, ActorUserID: userID.(uuid.UUID),
    })
    if err != nil {
        if errors.Is(err, promo.ErrInvalidOrExpired) {
            c.AbortWithStatusJSON(http.StatusUnprocessableEntity,
                gin.H{"error": "promo_invalid_or_expired"})
            return
        }
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "stripe_coupon_id":   out.StripeCouponID,
        "effective_minor":    out.EffectiveMinor,
        "duration_in_months": out.DurationInMonths,
    })
}

func (h *PromoHandler) Cancel(c *gin.Context) {
    tenantID, _ := c.Get("tenant_id")
    userID, _   := c.Get("user_id")
    subID, _    := c.Get("subscription_id")
    storeID := uuid.MustParse(c.Param("storeId"))

    if err := h.svc.CancelPromo(c.Request.Context(), promo.CancelInput{
        TenantID: tenantID.(uuid.UUID), StoreID: storeID,
        SubscriptionID: subID.(uuid.UUID),
        Reason: c.DefaultQuery("reason", "merchant_requested"),
        ActorUserID: userID.(uuid.UUID),
    }); err != nil {
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"}); return
    }
    c.Status(http.StatusNoContent)
}
```

- [ ] **Step 3: Wire routes** — inside the store-scoped admin group in `routes.go`:

```go
storeRoute.POST("/subscription/apply-promo", promoHandler.Apply)
storeRoute.DELETE("/subscription/promo",     promoHandler.Cancel)
```

These routes land under `/admin/stores/:storeId/subscription/*`, which P3's `DefaultAllowlist` already allows for read-only merchants — so a merchant in `expired`/`store_closed` state can still apply a save-offer promo to reactivate.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/{promo,promo_test,routes}.go
git commit -m "feat(admin): POST /subscription/apply-promo + DELETE /subscription/promo endpoints"
```

---

## Task 10: Spec-criterion #40 integration test

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/promo_integration_test.go`

**Spec references:** §28 criterion **#40** — "Promo code 50% off ₹999 Starter: rejected with 'below absolute floor' error".

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestIntegration_SpecCriterion40_INR_50PercentOff_999Starter_Rejected(t *testing.T) {
    h := newAdminHarness(t)
    sub := h.SeedActiveSub(subscription.PlanStarter, "INR", 99900 /* ₹999 */)
    h.SeedPromo("MARK8LY50NOPE42", 5000 /* 50% */, 6)

    resp := h.POST(sub.StoreID, "/subscription/apply-promo",
        map[string]string{"code": "MARK8LY50NOPE42"})

    // External: uniform 422.
    require.Equal(t, 422, resp.Code)
    require.JSONEq(t, `{"error":"promo_invalid_or_expired"}`, resp.Body.String())

    // Internal: audit records TRUE reason.
    events := h.Audit.Events()
    var found bool
    for _, e := range events {
        if e.Type == "subscription.promo_apply_rejected" &&
            e.Metadata["reject_reason"] == "below_absolute_floor" {
            found = true; break
        }
    }
    require.True(t, found, "audit must record below_absolute_floor reject reason")

    // Stripe was NEVER called — floor runs before Stripe attach.
    require.Empty(t, h.StripeStub.CallLog)
    // No redemption row:
    count, _ := h.Repo.CountRedemptions(h.Ctx, h.DB, sub.ID)
    require.EqualValues(t, 0, count)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/promo_integration_test.go
git commit -m "test(promo): spec criterion #40 — 50% off ₹999 Starter below-floor reject"
```

---

## Task 11: Refund model + repository

**Files:**
- Create: `services/marketplace-api/internal/refund/model.go`
- Create: `services/marketplace-api/internal/refund/errors.go`
- Create: `services/marketplace-api/internal/refund/repository.go`
- Create: `services/marketplace-api/internal/refund/repository_test.go` (`integration`)

- [ ] **Step 1: Write failing tests** — (a) `Create` + `ExistsByCardFingerprint` round-trip; (b) second `Create` with the same `card_fingerprint` returns a DB unique-violation error.

- [ ] **Step 2: Write `model.go`** — GORM `Refund` struct mirrors migration 052. All FK-ish fields typed as `uuid.UUID`; `CardFingerprint`/`DeviceFingerprint`/`StripeInvoiceID` are `*string` (nullable).

- [ ] **Step 3: Write `errors.go`**

```go
package refund

import "errors"

var (
    ErrOutsideCoolingOff      = errors.New("refund: outside 14-day cooling-off window")
    ErrDuplicateFingerprint   = errors.New("refund: card fingerprint already refunded")
    ErrChargeNotFound         = errors.New("refund: charge not found on Stripe")
    ErrSetupFeeNonRefundable  = errors.New("refund: setup fee is never refundable")
    ErrNoChargeOnSubscription = errors.New("refund: subscription has no successful charges")
)
```

- [ ] **Step 4: Write `repository.go`** — methods: `Create(ctx, db, Refund) (Refund, error)` and `ExistsByCardFingerprint(ctx, db, fp string) (bool, error)`.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/refund/
git commit -m "feat(refund): model + repository with card-fingerprint uniqueness lookup"
```

---

## Task 12: Stripe refund/charge/invoice client

**Files:**
- Create: `services/marketplace-api/internal/billing/stripe/refund.go`
- Create: `services/marketplace-api/internal/billing/stripe/refund_test.go`

- [ ] **Step 1: Write failing tests** — HTTP-mocked: (a) `CreateRefund` posts to `/v1/refunds` with `Idempotency-Key: refund:<invoiceID>`, sends `charge=`, `amount=`, `reason=requested_by_customer`, and `metadata[merchant_reason]=<reason>`; (b) `GetCharge` returns parsed `Charge` including `payment_method_details.card.fingerprint`; (c) `LatestChargeForSubscription` walks invoices list + picks newest paid invoice's charge.

- [ ] **Step 2: Write `refund.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "errors"
    "net/url"
    "strconv"
)

var ErrNotFound = errors.New("stripe: resource not found")

type Refund struct {
    ID       string `json:"id"`
    Charge   string `json:"charge"`
    Amount   int64  `json:"amount"`
    Currency string `json:"currency"`
    Status   string `json:"status"`
}

func (c *Client) CreateRefund(ctx context.Context, chargeID, invoiceID string, amountMinor int64, reason string) (Refund, error) {
    v := url.Values{}
    v.Set("charge", chargeID)
    if amountMinor > 0 { v.Set("amount", strconv.FormatInt(amountMinor, 10)) }
    if reason != "" {
        v.Set("reason", "requested_by_customer")
        v.Set("metadata[merchant_reason]", reason)
    }
    body, err := c.PostForm(ctx, "/v1/refunds", RefundIdempotencyKey(invoiceID), v)
    if err != nil { return Refund{}, err }
    var r Refund
    return r, json.Unmarshal(body, &r)
}

type Charge struct {
    ID                   string `json:"id"`
    Amount               int64  `json:"amount"`
    Currency             string `json:"currency"`
    Created              int64  `json:"created"`
    Invoice              string `json:"invoice"`
    Paid                 bool   `json:"paid"`
    PaymentMethodDetails struct {
        Card struct {
            Fingerprint string `json:"fingerprint"`
        } `json:"card"`
    } `json:"payment_method_details"`
}

func (c *Client) GetCharge(ctx context.Context, id string) (Charge, error) {
    body, err := c.Get(ctx, "/v1/charges/"+id)
    if err != nil { return Charge{}, err }
    var ch Charge
    return ch, json.Unmarshal(body, &ch)
}

// LatestChargeForSubscription — lists invoices for the customer (filtered by
// subscription), picks the newest paid invoice, GETs its charge. Returns
// ErrNotFound when no paid invoice exists.
func (c *Client) LatestChargeForSubscription(ctx context.Context, customerID, subID string) (Charge, error) {
    body, err := c.Get(ctx, "/v1/invoices?customer="+customerID+"&subscription="+subID+"&status=paid&limit=5")
    if err != nil { return Charge{}, err }
    var resp struct {
        Data []struct {
            ID, Charge string `json:"charge"`
            Created    int64  `json:"created"`
        } `json:"data"`
    }
    if err := json.Unmarshal(body, &resp); err != nil { return Charge{}, err }
    if len(resp.Data) == 0 || resp.Data[0].Charge == "" { return Charge{}, ErrNotFound }
    return c.GetCharge(ctx, resp.Data[0].Charge)
}

// ListInvoicesForCustomer — ALL invoices (paginated). Used by archive.Builder.
type InvoiceSummary struct {
    ID         string `json:"id"`
    AmountPaid int64  `json:"amount_paid"`
    Currency   string `json:"currency"`
    Created    int64  `json:"created"`
    Status     string `json:"status"`
}

func (c *Client) ListInvoicesForCustomer(ctx context.Context, customerID string) ([]InvoiceSummary, error) {
    // Walk pages of /v1/invoices?customer=&limit=100&starting_after=...
    // (Implementation detail: loop until response.has_more == false.)
    panic("implement walker")
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(billing): Stripe refund + charge + invoice-walker endpoints"
```

---

## Task 13: `refund.Service` — 14-day gate + card-FP uniqueness

**Files:**
- Create: `services/marketplace-api/internal/refund/service.go`
- Create: `services/marketplace-api/internal/refund/service_test.go` (`integration`)

**Spec references:** §8.

- [ ] **Step 1: Write failing tests**

(a) charge 3 days old → refund succeeds, audit event `subscription.refund_issued` severity=warning; (b) charge 15 days old → `ErrOutsideCoolingOff`; (c) second refund on same card fingerprint → `ErrDuplicateFingerprint`; (d) no successful charge on subscription → `ErrNoChargeOnSubscription`; (e) `DeviceFingerprint` is persisted into `refund_audit`; (f) idempotency key passed to Stripe is `refund:<invoiceID>`; (g) Pro+App merchant with `reason` containing "setup" → `ErrSetupFeeNonRefundable`.

- [ ] **Step 2: Write `service.go`**

```go
package refund

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    stripec "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

const coolingOffWindow = 14 * 24 * time.Hour

type Service struct {
    db      *gorm.DB
    repo    *Repository
    subRepo subscription.Repository
    stripe  *stripec.Client
    emitter *audit.Emitter
    now     func() time.Time
}

type Deps struct {
    DB *gorm.DB; Repo *Repository; SubRepo subscription.Repository
    Stripe *stripec.Client; Emitter *audit.Emitter; Now func() time.Time
}

func NewService(d Deps) *Service {
    if d.Now == nil { d.Now = time.Now }
    return &Service{db: d.DB, repo: d.Repo, subRepo: d.SubRepo,
        stripe: d.Stripe, emitter: d.Emitter, now: d.Now}
}

type IssueInput struct {
    TenantID, StoreID, SubscriptionID, IssuedBy uuid.UUID
    Reason, DeviceFingerprint                   string
}
type IssueOutput struct {
    StripeRefundID string
    AmountMinor    int64
    Currency       string
}

// IssueRefund runs: subscription lookup → Pro+App setup-fee refuse → latest
// charge from Stripe → 14-day cooling-off gate (Stripe's charge.created, not
// local) → card-fingerprint uniqueness lookup → Stripe /v1/refunds (idempotent
// on invoice ID) → refund_audit row → audit emit severity=warning.
func (s *Service) IssueRefund(ctx context.Context, in IssueInput) (IssueOutput, error) {
    sub, err := s.subRepo.GetByID(ctx, s.db, in.SubscriptionID)
    if err != nil { return IssueOutput{}, err }

    // §8: Pro+App setup fee NEVER refundable.
    if sub.HasWhiteLabelAppAddOn && strings.Contains(strings.ToLower(in.Reason), "setup") {
        return IssueOutput{}, ErrSetupFeeNonRefundable
    }

    charge, err := s.stripe.LatestChargeForSubscription(ctx, sub.StripeCustomerID, sub.StripeSubscriptionID)
    if err != nil {
        if errors.Is(err, stripec.ErrNotFound) { return IssueOutput{}, ErrNoChargeOnSubscription }
        return IssueOutput{}, err
    }
    if !charge.Paid { return IssueOutput{}, ErrNoChargeOnSubscription }

    // §8: 14-day gate using Stripe's charge.created.
    created := time.Unix(charge.Created, 0).UTC()
    if s.now().UTC().After(created.Add(coolingOffWindow)) {
        return IssueOutput{}, ErrOutsideCoolingOff
    }

    // §8: card-fingerprint uniqueness.
    fp := charge.PaymentMethodDetails.Card.Fingerprint
    if fp != "" {
        exists, err := s.repo.ExistsByCardFingerprint(ctx, s.db, fp)
        if err != nil { return IssueOutput{}, err }
        if exists { return IssueOutput{}, ErrDuplicateFingerprint }
    }

    sr, err := s.stripe.CreateRefund(ctx, charge.ID, charge.Invoice, charge.Amount, in.Reason)
    if err != nil { return IssueOutput{}, err }

    var fpPtr, devPtr *string
    if fp != "" { fpPtr = &fp }
    if in.DeviceFingerprint != "" { d := in.DeviceFingerprint; devPtr = &d }
    inv := charge.Invoice

    if _, err := s.repo.Create(ctx, s.db, Refund{
        SubscriptionID: in.SubscriptionID, StoreID: in.StoreID, TenantID: in.TenantID,
        StripeRefundID: sr.ID, StripeChargeID: charge.ID, StripeInvoiceID: &inv,
        AmountMinor: sr.Amount, Currency: sr.Currency,
        Reason: in.Reason, CardFingerprint: fpPtr, DeviceFingerprint: devPtr,
        IssuedBy: in.IssuedBy,
    }); err != nil {
        // Stripe refunded but audit write failed — emit error-severity event for paging.
        if s.emitter != nil {
            s.emitter.EmitRefundIssued(audit.RefundIssued{
                Severity: audit.SeverityError,
                TenantID: in.TenantID, StoreID: in.StoreID, SubscriptionID: in.SubscriptionID,
                StripeRefundID: sr.ID, StripeChargeID: charge.ID,
                AmountMinor: sr.Amount, Currency: sr.Currency,
                AuditWriteError: err.Error(), IssuedBy: in.IssuedBy,
            })
        }
        return IssueOutput{}, err
    }

    if s.emitter != nil {
        s.emitter.EmitRefundIssued(audit.RefundIssued{
            Severity: audit.SeverityWarning,
            TenantID: in.TenantID, StoreID: in.StoreID, SubscriptionID: in.SubscriptionID,
            StripeRefundID: sr.ID, StripeChargeID: charge.ID,
            AmountMinor: sr.Amount, Currency: sr.Currency,
            Reason: in.Reason, CardFingerprint: fpPtr, DeviceFingerprint: devPtr,
            IssuedBy: in.IssuedBy,
        })
    }
    return IssueOutput{StripeRefundID: sr.ID, AmountMinor: sr.Amount, Currency: sr.Currency}, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(refund): IssueRefund with 14-day + card-fingerprint gates (§8)"
```

---

## Task 14: Refund admin handler + routes

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/refund.go`
- Create: `services/marketplace-api/internal/handlers/admin/refund_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

- [ ] **Step 1: Write failing handler tests** — (a) valid inside-window → 200 + `stripe_refund_id`; (b) outside window → 403 body includes `outside_cooling_off` **and** "cancel_at_period_end" message; (c) duplicate card FP → 403 `refund_unavailable` (no fingerprint leak); (d) `X-Device-Fingerprint` header propagates to `refund_audit`.

- [ ] **Step 2: Write `refund.go`**

```go
package admin

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/refund"
)

type RefundHandler struct{ svc *refund.Service }

func NewRefundHandler(svc *refund.Service) *RefundHandler { return &RefundHandler{svc: svc} }

type refundBody struct {
    Reason string `json:"reason" binding:"required,min=5,max=500"`
}

func (h *RefundHandler) Issue(c *gin.Context) {
    var body refundBody
    if err := c.ShouldBindJSON(&body); err != nil {
        c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_reason"}); return
    }
    tenantID, _ := c.Get("tenant_id")
    userID, _   := c.Get("user_id")
    subID, _    := c.Get("subscription_id")
    storeID := uuid.MustParse(c.Param("storeId"))

    out, err := h.svc.IssueRefund(c.Request.Context(), refund.IssueInput{
        TenantID: tenantID.(uuid.UUID), StoreID: storeID,
        SubscriptionID: subID.(uuid.UUID), Reason: body.Reason,
        DeviceFingerprint: c.GetHeader("X-Device-Fingerprint"),
        IssuedBy: userID.(uuid.UUID),
    })
    if err != nil {
        switch {
        case errors.Is(err, refund.ErrOutsideCoolingOff):
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error":   "outside_cooling_off",
                "message": "Refund window is 14 days from first charge. Use cancel_at_period_end instead.",
            })
        case errors.Is(err, refund.ErrDuplicateFingerprint):
            // Do NOT leak fingerprint-match detail (§8 fraud prevention).
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "refund_unavailable",
                "message": "Contact support to request a refund on this card.",
            })
        case errors.Is(err, refund.ErrSetupFeeNonRefundable):
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "setup_fee_nonrefundable"})
        case errors.Is(err, refund.ErrNoChargeOnSubscription):
            c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "no_charge_to_refund"})
        default:
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
        }
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "stripe_refund_id": out.StripeRefundID,
        "amount_minor":     out.AmountMinor,
        "currency":         out.Currency,
    })
}
```

- [ ] **Step 3: Wire route**

```go
storeRoute.POST("/subscription/refund", refundHandler.Issue)
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -am "feat(admin): POST /subscription/refund with 14-day gate + fingerprint guard"
```

---

## Task 15: Refund 14-day + fingerprint integration tests

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/refund_integration_test.go`

- [ ] **Step 1: Write the tests**

```go
//go:build integration

func TestIntegration_Refund_13Days_HappyPath(t *testing.T) {
    h := newAdminHarness(t)
    sub := h.SeedActiveSub(subscription.PlanStarter, "USD", 1900)
    h.MockCharge(sub.StripeCustomerID, 13*24*time.Hour, "fp_OK", 1900)

    resp := h.POSTHeaders(sub.StoreID, "/subscription/refund",
        map[string]string{"reason": "buyer declined platform"},
        map[string]string{"X-Device-Fingerprint": "dev-xyz"})
    require.Equal(t, 200, resp.Code)

    // Device fingerprint reached refund_audit:
    var row refund.Refund
    require.NoError(t, h.DB.Where("subscription_id = ?", sub.ID).First(&row).Error)
    require.NotNil(t, row.DeviceFingerprint)
    require.Equal(t, "dev-xyz", *row.DeviceFingerprint)
}

func TestIntegration_Refund_15Days_Rejected(t *testing.T) {
    h := newAdminHarness(t)
    sub := h.SeedActiveSub(subscription.PlanStarter, "USD", 1900)
    h.MockCharge(sub.StripeCustomerID, 15*24*time.Hour, "fp_LATE", 1900)

    resp := h.POST(sub.StoreID, "/subscription/refund",
        map[string]string{"reason": "too late"})
    require.Equal(t, 403, resp.Code)
    require.Contains(t, resp.Body.String(), "outside_cooling_off")
    require.Contains(t, resp.Body.String(), "cancel_at_period_end")
}

func TestIntegration_Refund_SameCard_SecondRefund_Rejected(t *testing.T) {
    h := newAdminHarness(t)
    sub1 := h.SeedActiveSub(subscription.PlanStarter, "USD", 1900)
    h.MockCharge(sub1.StripeCustomerID, 5*24*time.Hour, "fp_shared", 1900)
    require.Equal(t, 200, h.POST(sub1.StoreID, "/subscription/refund",
        map[string]string{"reason": "first"}).Code)

    // Second subscription, SAME fingerprint.
    sub2 := h.SeedActiveSub(subscription.PlanStudio, "USD", 4900)
    h.MockCharge(sub2.StripeCustomerID, 3*24*time.Hour, "fp_shared", 4900)

    resp := h.POST(sub2.StoreID, "/subscription/refund",
        map[string]string{"reason": "second"})
    require.Equal(t, 403, resp.Code)
    require.Contains(t, resp.Body.String(), "refund_unavailable")
    require.NotContains(t, resp.Body.String(), "fingerprint",
        "do NOT leak fraud-prevention detail to caller")
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/refund_integration_test.go
git commit -m "test(refund): 14-day gate + card-fingerprint reuse integration tests"
```

---

## Task 16: `archive.Builder` — invoice fetch + FX sum + persist

**Files:**
- Create: `services/marketplace-api/internal/billing/archive/model.go`
- Create: `services/marketplace-api/internal/billing/archive/errors.go`
- Create: `services/marketplace-api/internal/billing/archive/builder.go`
- Create: `services/marketplace-api/internal/billing/archive/builder_test.go` (`integration`)

**Spec references:** §23.2 (table exists via P1 migration 044).

- [ ] **Step 1: Write `model.go`** — GORM `BillingArchive` mirroring the P1 migration 044 schema. All invoice data in `AllInvoices datatypes.JSON`. Nullable tax/country fields as `*string`.

- [ ] **Step 2: Write `errors.go`**

```go
package archive
import "errors"
var (
    ErrSubscriptionNotFound = errors.New("archive: subscription not found")
    ErrStripeReadFailed     = errors.New("archive: stripe invoice read failed")
)
```

- [ ] **Step 3: Write failing tests**

(a) `BuildAndPersist` writes a row with `archive_expires_at == hard_deleted_at + 7 years`; (b) FX-normalized `total_revenue_usd` matches expected (₹99,900 paise = ₹999 × mocked $0.012/₹ = $11.99); (c) audit event `subscription.billing_archived` with `op:"create"` emitted.

- [ ] **Step 4: Write `builder.go`**

```go
package archive

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/datatypes"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    stripec "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/fx"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

const RetentionYears = 7

type Builder struct {
    stripe  *stripec.Client
    fx      fx.Rates // P1 mid-market FX helper
    subRepo subscription.Repository
    emitter *audit.Emitter
    now     func() time.Time
}

type Deps struct {
    Stripe *stripec.Client; FX fx.Rates
    SubRepo subscription.Repository; Emitter *audit.Emitter; Now func() time.Time
}

func NewBuilder(d Deps) *Builder {
    if d.Now == nil { d.Now = time.Now }
    return &Builder{stripe: d.Stripe, fx: d.FX, subRepo: d.SubRepo, emitter: d.Emitter, now: d.Now}
}

type Input struct {
    TenantID, StoreID uuid.UUID
    HardDeletedAt     time.Time
}

type Output struct{ ArchiveID uuid.UUID }

// BuildAndPersist MUST run INSIDE P11's hard-delete advisory-locked tx
// (pattern F in the plan preamble). Caller provides the transactional *gorm.DB.
func (b *Builder) BuildAndPersist(ctx context.Context, tx *gorm.DB, in Input) (Output, error) {
    sub, err := b.subRepo.GetByStoreID(ctx, tx, in.TenantID, in.StoreID)
    if err != nil { return Output{}, fmt.Errorf("%w: %v", ErrSubscriptionNotFound, err) }

    invoices, err := b.stripe.ListInvoicesForCustomer(ctx, sub.StripeCustomerID)
    if err != nil { return Output{}, fmt.Errorf("%w: %v", ErrStripeReadFailed, err) }

    var totalUSD float64
    for _, inv := range invoices {
        usd, err := b.fx.ConvertMinorToUSDFloat(ctx, inv.AmountPaid, inv.Currency)
        if err != nil { return Output{}, fmt.Errorf("fx convert %s→USD: %w", inv.Currency, err) }
        totalUSD += usd
    }
    raw, err := json.Marshal(invoices)
    if err != nil { return Output{}, err }

    row := BillingArchive{
        OriginalStoreID: in.StoreID, OriginalTenantID: in.TenantID,
        BusinessName: sub.BusinessName, TaxID: sub.TaxID, TaxIDCountry: sub.TaxIDCountry,
        BillingCountry: sub.BillingCountry, BillingCurrency: &sub.BillingCurrency,
        StripeCustomerID: sub.StripeCustomerID, AllInvoices: datatypes.JSON(raw),
        TotalRevenueUSD: round2(totalUSD),
        HardDeletedAt: in.HardDeletedAt,
        ArchiveExpiresAt: in.HardDeletedAt.AddDate(RetentionYears, 0, 0),
    }
    if err := tx.WithContext(ctx).Create(&row).Error; err != nil { return Output{}, err }

    if b.emitter != nil {
        b.emitter.EmitBillingArchived(audit.BillingArchived{
            TenantID: in.TenantID, StoreID: in.StoreID,
            ArchiveID: row.ID, Op: "create", TotalRevenueUSD: row.TotalRevenueUSD,
        })
    }
    return Output{ArchiveID: row.ID}, nil
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/archive/
git commit -m "feat(archive): BuildAndPersist with FX normalization + 7-year expiry"
```

---

## Task 17: `archive.Sweeper` — daily expiry cron

**Files:**
- Create: `services/marketplace-api/internal/billing/archive/sweeper.go`
- Create: `services/marketplace-api/internal/billing/archive/sweeper_test.go` (`integration`)

**Spec references:** §23.2 retention expiry.

- [ ] **Step 1: Write failing tests** — (a) seed one expired + one retained row; `RunOnce` returns `deleted==1`; expired row gone; audit event with `op:"expired"` emitted; (b) seed 501 expired rows; `RunOnce` returns `deleted==501` (tests batching).

- [ ] **Step 2: Write `sweeper.go`**

```go
package archive

import (
    "context"
    "time"

    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
)

const sweeperBatchSize = 500

type Sweeper struct {
    emitter *audit.Emitter
    now     func() time.Time
}

type SweeperDeps struct {
    Emitter *audit.Emitter
    Now     func() time.Time
}

func NewSweeper(d SweeperDeps) *Sweeper {
    if d.Now == nil { d.Now = time.Now }
    return &Sweeper{emitter: d.Emitter, now: d.Now}
}

// RunOnce deletes every billing_archive row whose retention has elapsed.
// Uses SKIP LOCKED to avoid blocking concurrent readers. Batch size 500.
func (s *Sweeper) RunOnce(ctx context.Context, db *gorm.DB) (int64, error) {
    var total int64
    for {
        var ids []string
        err := db.WithContext(ctx).Raw(`
            DELETE FROM billing_archive
            WHERE id IN (
                SELECT id FROM billing_archive
                WHERE archive_expires_at < ?
                LIMIT ?
                FOR UPDATE SKIP LOCKED
            )
            RETURNING id::text`,
            s.now().UTC(), sweeperBatchSize).Scan(&ids).Error
        if err != nil { return total, err }
        n := int64(len(ids))
        total += n
        if s.emitter != nil {
            for _, id := range ids {
                s.emitter.EmitBillingArchived(audit.BillingArchived{ArchiveIDRaw: id, Op: "expired"})
            }
        }
        if n < sweeperBatchSize { break }
    }
    return total, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(archive): daily expiry sweeper with SKIP LOCKED batch delete"
```

---

## Task 18: Audit event helpers

**Files:**
- Modify: `services/marketplace-api/internal/audit/events.go`
- Modify (or create): `services/marketplace-api/internal/audit/events_test.go`

**Spec references:** §23.1.

- [ ] **Step 1: Write failing tests**

(a) `EmitPromoApplied` with non-empty `RejectReason` → event type `subscription.promo_apply_rejected`; empty reject → `subscription.promo_applied`. Metadata includes `stripe_coupon_id` and `effective_minor`. (b) `EmitRefundIssued` severity defaults to `warning` when unset. (c) `EmitBillingArchived` with `Op:"create"` vs `Op:"expired"` yields the same event type but different `op` metadata.

- [ ] **Step 2: Extend `events.go`** — follow the existing `EmitStateTransition` pattern from P1. Shape:

```go
type PromoApplied struct {
    TenantID, StoreID, SubscriptionID, PromoCodeID, RedemptionID, ActorUserID uuid.UUID
    StripeCouponID, Email, RejectReason                                       string
    EffectiveMinor                                                            int64
}

func (e *Emitter) EmitPromoApplied(p PromoApplied) {
    t := "subscription.promo_applied"
    if p.RejectReason != "" { t = "subscription.promo_apply_rejected" }
    e.record(Event{
        Type: t, Severity: SeverityInfo,
        TenantID: p.TenantID, StoreID: p.StoreID, ActorUserID: p.ActorUserID,
        Metadata: map[string]any{
            "subscription_id":  p.SubscriptionID.String(),
            "promo_code_id":    p.PromoCodeID.String(),
            "stripe_coupon_id": p.StripeCouponID,
            "redemption_id":    p.RedemptionID.String(),
            "email":            p.Email,
            "effective_minor":  p.EffectiveMinor,
            "reject_reason":    p.RejectReason,
        },
    })
}

type PromoCancelled struct {
    TenantID, StoreID, SubscriptionID, PromoCodeID, ActorUserID uuid.UUID
    Reason                                                      string
}

func (e *Emitter) EmitPromoCancelled(p PromoCancelled) {
    e.record(Event{
        Type: "subscription.promo_cancelled", Severity: SeverityInfo,
        TenantID: p.TenantID, StoreID: p.StoreID, ActorUserID: p.ActorUserID,
        Metadata: map[string]any{
            "subscription_id": p.SubscriptionID.String(),
            "promo_code_id":   p.PromoCodeID.String(),
            "reason":          p.Reason,
        },
    })
}

type RefundIssued struct {
    Severity                                           Severity
    TenantID, StoreID, SubscriptionID, IssuedBy        uuid.UUID
    StripeRefundID, StripeChargeID, Currency, Reason   string
    AmountMinor                                        int64
    CardFingerprint, DeviceFingerprint                 *string
    AuditWriteError                                    string
}

func (e *Emitter) EmitRefundIssued(r RefundIssued) {
    sev := r.Severity
    if sev == "" { sev = SeverityWarning }
    e.record(Event{
        Type: "subscription.refund_issued", Severity: sev,
        TenantID: r.TenantID, StoreID: r.StoreID, ActorUserID: r.IssuedBy,
        Metadata: map[string]any{
            "subscription_id":    r.SubscriptionID.String(),
            "stripe_refund_id":   r.StripeRefundID,
            "stripe_charge_id":   r.StripeChargeID,
            "amount_minor":       r.AmountMinor,
            "currency":           r.Currency,
            "reason":             r.Reason,
            "card_fingerprint":   ptrOrEmpty(r.CardFingerprint),
            "device_fingerprint": ptrOrEmpty(r.DeviceFingerprint),
            "audit_write_error":  r.AuditWriteError,
        },
    })
}

type BillingArchived struct {
    TenantID, StoreID uuid.UUID
    ArchiveID         uuid.UUID
    ArchiveIDRaw      string
    Op                string // "create" | "expired"
    TotalRevenueUSD   float64
}

func (e *Emitter) EmitBillingArchived(b BillingArchived) {
    id := b.ArchiveID.String()
    if b.ArchiveIDRaw != "" { id = b.ArchiveIDRaw }
    e.record(Event{
        Type: "subscription.billing_archived", Severity: SeverityInfo,
        TenantID: b.TenantID, StoreID: b.StoreID,
        Metadata: map[string]any{
            "archive_id":        id,
            "op":                b.Op,
            "total_revenue_usd": b.TotalRevenueUSD,
        },
    })
}

func ptrOrEmpty(p *string) string { if p == nil { return "" }; return *p }
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(audit): Emit{PromoApplied,PromoCancelled,RefundIssued,BillingArchived}"
```

---

## Task 19: Wire all in `main.go` + cron registry

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/internal/cron/registry.go`

- [ ] **Step 1: Instantiate deps in `main.go`**

```go
promoRepo   := promo.NewRepository()
promoVal    := promo.NewValidator(promoRepo)
promoStripe := promo.NewStripeClient(stripeClient)
promoRL     := promo.NewRateLimiter(rateLimitClient)
promoSvc    := promo.NewService(promo.Deps{
    DB: db, Repo: promoRepo, Validator: promoVal,
    Stripe: promoStripe, Emitter: auditEmitter, SubRepo: subRepo,
})

refundRepo := refund.NewRepository()
refundSvc  := refund.NewService(refund.Deps{
    DB: db, Repo: refundRepo, SubRepo: subRepo,
    Stripe: stripeClient, Emitter: auditEmitter,
})

archiveBuilder := archive.NewBuilder(archive.Deps{
    Stripe: stripeClient, FX: fxRates, SubRepo: subRepo, Emitter: auditEmitter,
})
archiveSweeper := archive.NewSweeper(archive.SweeperDeps{Emitter: auditEmitter})

promoHandler  := admin.NewPromoHandler(promoSvc, promoRL)
refundHandler := admin.NewRefundHandler(refundSvc)
```

- [ ] **Step 2: Register routes**

```go
// In routes.go, inside the /admin/stores/:storeId group (P3 allowlist):
storeRoute.POST("/subscription/apply-promo", promoHandler.Apply)
storeRoute.DELETE("/subscription/promo",     promoHandler.Cancel)
storeRoute.POST("/subscription/refund",      refundHandler.Issue)
```

- [ ] **Step 3: Register sweeper cron**

```go
// In cron/registry.go:
cronRegistry.Register(cron.Entry{
    Name:     "billing-archive-sweeper",
    Schedule: "15 3 * * *", // daily at 03:15 UTC
    Run: func(ctx context.Context) error {
        _, err := archiveSweeper.RunOnce(ctx, db)
        return err
    },
})
```

- [ ] **Step 4: Expose `archive.Builder` for P11**

Add `ArchiveBuilder *archive.Builder` to the app's `Deps` / DI struct that P11 consumes. P11 invokes `archiveBuilder.BuildAndPersist(ctx, tx, archive.Input{…})` inside its hard-delete advisory-locked transaction (pattern F).

- [ ] **Step 5: Build + integration smoke**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./... -count=1
```

- [ ] **Step 6: Commit**

```bash
git commit -am "feat(marketplace-api): wire promo + refund + archive services and daily sweeper"
```

---

## Task 20: Final scrub + full suite green

- [ ] **Step 1: Grep for accidental leakage of §7.3 internal errors to HTTP handlers**

```bash
cd services/marketplace-api
grep -RnE 'below_absolute_floor|max_redemptions_reached|max_per_email_reached|already_has_active_promo|pre_trial_apply' \
    internal/handlers/ | grep -v "_test.go" | grep -v audit_ || echo "clean"
```
Expected: `clean`. Handlers only ever surface `promo_invalid_or_expired` on promo reject. Internal reasons live in audit metadata only (§7.3 timing-safe generic response).

- [ ] **Step 2: Grep for raw Stripe key logging**

```bash
grep -RnE 'sk_(test|live)_[A-Za-z0-9]' internal/ cmd/ | grep -v "_test.go" || echo "clean"
```
Expected: `clean`.

- [ ] **Step 3: Grep to ensure no raw status UPDATEs got introduced by P10**

```bash
grep -RnE 'UPDATE\s+store_subscriptions\s+SET\s+status' \
    internal/promo/ internal/refund/ internal/billing/archive/ || echo "clean"
```
Expected: `clean`. P10 does not touch subscription status — P3's state machine owns that.

- [ ] **Step 4: Full suite**

```bash
go test -tags=integration ./... -count=1
```
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add -u
git commit --allow-empty -m "chore: scrub verified — P10 no info leaks, no secret logs, no raw status UPDATEs"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] `promo_codes` has `CHECK char_length(code) >= 12` and `CHECK discount_value <= 5000` for percentage.
- [ ] `promo_redemptions` has `UNIQUE (promo_code_id, email)` and `UNIQUE (subscription_id, promo_code_id)`.
- [ ] `refund_audit` has unique partial index on `card_fingerprint WHERE card_fingerprint IS NOT NULL`.
- [ ] `promo.CheckFloor(PlanStarter, "INR", 49950) == ErrBelowFloor` (₹999 × 50%, floor ₹800).
- [ ] Handler test: 50% off ₹999 Starter returns **422** with body `{"error":"promo_invalid_or_expired"}` (not 200, not specific). **Spec criterion #40 met.**
- [ ] Handler test: unknown code, expired code, below-floor code all return the **same** 422 body (§7.3).
- [ ] Rate-limit: 6th apply from same IP in 1h returns **429**.
- [ ] Refund happy path: charge 3 days old → **200** + `stripe_refund_id` + audit event severity=warning + device_fingerprint recorded in `refund_audit`.
- [ ] Refund 14-day gate: charge 15 days old → **403** `outside_cooling_off` with "cancel_at_period_end" message.
- [ ] Refund fingerprint reuse: second refund on same card fingerprint → **403** `refund_unavailable` (no leak of fingerprint-match detail in response body).
- [ ] Refund idempotency: Stripe call uses `Idempotency-Key: refund:<invoiceID>` (P2's `RefundIdempotencyKey`).
- [ ] `archive.Builder.BuildAndPersist` writes a row with `archive_expires_at == hard_deleted_at + 7 years` and `total_revenue_usd` summed via FX.
- [ ] `archive.Sweeper.RunOnce` deletes expired rows in batches of 500 using `SKIP LOCKED` and emits one `subscription.billing_archived{op:"expired"}` per deletion.
- [ ] Audit events emitted on all paths: `subscription.promo_applied`, `subscription.promo_apply_rejected`, `subscription.promo_cancelled`, `subscription.refund_issued` (severity=warning), `subscription.billing_archived`.
- [ ] Handlers never surface `below_absolute_floor` / `max_redemptions_reached` / `already_has_active_promo` — only the generic response.
- [ ] `statemachine.Transition` was NOT called from promo or refund code paths (grep confirms).

## What's now unlocked

- **P11** (cancellation + save-offer) — calls `promo.ValidateForSaveOffer` during save-offer preview, `promo.ApplyPromo` on merchant accept, and `archive.BuildAndPersist` inside the advisory-locked hard-delete transaction. P11 also uses `statemachine.Transition` for `cancel_scheduled → active` on save-offer acceptance.
- **P16** (admin frontend) — consumes `POST /subscription/apply-promo`, `DELETE /subscription/promo`, `POST /subscription/refund`. The promo response includes `effective_minor` + `duration_in_months` for UI price display; the refund response includes the Stripe refund ID for toast links.
- **P17** (observability) — the four new audit types are first-class inputs to the refund-count / promo-redemption / archive-size gauges. Suggested alerts:
  - `subscription.promo_apply_rejected{reason="below_absolute_floor"}` spike → probable upstream code misconfiguration.
  - `subscription.refund_issued{severity="error"}` → page on-call (divergence between Stripe refund and `refund_audit` row).
  - `subscription.billing_archived{op="expired"}` → weekly digest of retention-sweep volume.

## Execution handoff

Plan complete. P10 saved to `docs/superpowers/plans/2026-04-18-p10-promos-refunds-archive.md`. Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Ordering: P10 may run in parallel with P4/P5/P6 once P1 + P2 + P3 are merged. P11 depends on P10 for the service interfaces (`promo.Service`, `archive.Builder`).
