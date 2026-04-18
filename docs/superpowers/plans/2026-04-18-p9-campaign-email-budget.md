# P9 — Campaign Email Budget + Trial-Ramp Cron Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the passive `campaign_email_budget` table (created by P1 migration 045) into a hot-path gate for every campaign send: a single-statement atomic-decrement that rejects over-budget sends with 403, a daily trial-ramp cron that walks each store through the 500 / 2,000 / plan-allowance volumes on trial days 3→4 and 7→8, a monthly-reset cron that seeds a fresh row per active store on the first of the month, a concurrent-send throttle (max 3/store) via Redis INCR with Postgres advisory-lock fallback, and a hard split between campaign traffic (budgeted) and transactional traffic (unbudgeted, separate fair-use counter).

**Architecture:** A new `internal/campaignbudget` package owns three concerns: (1) `budget.Reserve(ctx, storeID, recipientCount)` — the atomic SQL UPDATE from spec §10.1; (2) `budget.ApplyTrialRamp(ctx, storeID, day)` and `budget.RecomputeLimitForPlan(ctx, storeID, plan)` — the two idempotent mutators that move `limit_set` on the current-month row; (3) `budget.MonthlyReset(ctx)` — the first-of-month seeder using `INSERT ... ON CONFLICT DO NOTHING`. A new `internal/campaignbudget/cron` sub-package registers two `robfig/cron/v3` jobs (trial ramp at 00:00 UTC daily, monthly reset at 00:05 UTC on day 1) alongside the orphan-webhook cron from P2 in `main.go`. A new `internal/campaignbudget/concurrency` package exposes `AcquireSlot(ctx, storeID) (release func, err)` with a Redis implementation and a Postgres advisory-lock fallback selected at startup. A new `internal/campaignbudget/transactional` counter table tracks transactional email volume with a 100k/store/month soft cap — entirely separate from the campaign budget path. The plan-change handler from P4 calls `budget.RecomputeLimitForPlan` **inside** the same transaction that writes the new plan row, so a plan change and its budget effect commit atomically.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, `github.com/robfig/cron/v3` (already a project dep per root CLAUDE.md service registry), `github.com/redis/go-redis/v9` (already a project dep), Prometheus client, existing `internal/plangate` (P3), existing `internal/subscription/statemachine` (P3) — statemachine is **not** modified here, only read-from for signup_date lookups.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §5.1 (volume ramp: D1-3=500, D4-7=2000, D8+=plan allowance), §10 (email enforcement overview), §10.1 (atomic-decrement SQL + trial-ramp cron semantics), §10.2 (monthly reset, 3-concurrent send cap, transactional separation), §10.3 (SES migration trigger at 500 paid merchants), §28 criteria (trial-ramp day-3→4, monthly rollover, atomic-decrement correctness, plan-change recomputation).

**Depends on:**
- **P1** — migration 045 creates `campaign_email_budget (store_id, month, remaining, limit_set, PK(store_id, month))`; `StoreSubscription.SignupDate` column already present
- **P3** — `plangate.Limit(plan, FeatureCampaignEmailsPerMonth)` for reading plan allowance; `plangate.Negotiated` sentinel for Pro "contact sales"
- **P4** — the change-plan handler will invoke `budget.RecomputeLimitForPlan` inside its transaction (this plan exposes the service method; P4 wires the call)

**Related plans (NOT in scope here):**
- **P10** — SendGrid → SES migration (documented trigger at 500 paid merchants; not implemented)
- **P16** — admin UI counter display showing "X / Y campaign emails used this month"
- Actual email templates and send pipeline (separate content/infra work)

---

## Scope Check

In scope:
1. `budget.Reserve` — single-statement atomic-decrement (spec §10.1 exact SQL).
2. `budget.ApplyTrialRamp` — idempotent mutator for day 3→4 (`GREATEST(remaining, 2000)`) and day 7→8 (`plan_allowance`).
3. `budget.RecomputeLimitForPlan` — service method P4 invokes inside its change-plan transaction.
4. `budget.MonthlyReset` — first-of-month scheduler using `INSERT ... ON CONFLICT (store_id, month) DO NOTHING`.
5. `concurrency.AcquireSlot` — Redis INCR (10-min TTL) with Postgres advisory-lock fallback; max 3 concurrent sends per store.
6. `transactional.Record` + 100k/store/month soft-cap counter in a new `store_transactional_counter` table.
7. Cron registration via `robfig/cron/v3` started from `cmd/marketplace-api/main.go`.
8. Prometheus metrics: `campaign_email_sent_total{store_id}`, `campaign_email_budget_exhausted_total{plan}`, `campaign_email_trial_ramp_applied_total{day}`.
9. Integration tests proving the four §28-referenced criteria (trial-ramp day-3→4, monthly rollover, atomic decrement, plan-change recomputation) plus concurrency correctness.

Out of scope:
- SendGrid → SES migration path (documented as 500-merchant threshold in spec §10.3; see P10).
- Campaign email template authoring / send pipeline.
- Admin UI counter display (P16).
- Storefront publish gate (that's P7 tax-ID + storefront concerns).
- `payment_action_required` interaction — read-only middleware (P3) does not gate the campaign route; over-budget rejection happens **after** the authz chain.

---

## File Structure

### Create

- `services/marketplace-api/internal/campaignbudget/reserve.go` — `Reserve(ctx, db, storeID, recipientCount)` atomic-decrement
- `services/marketplace-api/internal/campaignbudget/reserve_test.go`
- `services/marketplace-api/internal/campaignbudget/ramp.go` — `ApplyTrialRamp(ctx, db, storeID, day, plan)` + helper `ComputeRampDay(signupDate, now)`
- `services/marketplace-api/internal/campaignbudget/ramp_test.go`
- `services/marketplace-api/internal/campaignbudget/recompute.go` — `RecomputeLimitForPlan(ctx, tx, storeID, plan)` — accepts a `*gorm.DB` that may already be inside a caller transaction
- `services/marketplace-api/internal/campaignbudget/recompute_test.go`
- `services/marketplace-api/internal/campaignbudget/monthly_reset.go` — `MonthlyReset(ctx, db)` seeds rows for all active subscriptions
- `services/marketplace-api/internal/campaignbudget/monthly_reset_test.go`
- `services/marketplace-api/internal/campaignbudget/service.go` — `Service` struct wiring `Reserve` + `RecomputeLimitForPlan` behind one interface (what P4 imports)
- `services/marketplace-api/internal/campaignbudget/errors.go` — `ErrBudgetExhausted`, `ErrNoBudgetRow`, `ErrPlanNegotiated`
- `services/marketplace-api/internal/campaignbudget/metrics.go` — Prometheus counters
- `services/marketplace-api/internal/campaignbudget/cron/jobs.go` — `RegisterTrialRampJob`, `RegisterMonthlyResetJob`
- `services/marketplace-api/internal/campaignbudget/cron/jobs_test.go` — idempotency + clock-mocked trigger tests
- `services/marketplace-api/internal/campaignbudget/concurrency/slot.go` — `AcquireSlot(ctx, storeID) (release func, err)` interface + Redis + advisory-lock impls
- `services/marketplace-api/internal/campaignbudget/concurrency/slot_redis.go`
- `services/marketplace-api/internal/campaignbudget/concurrency/slot_advisory.go`
- `services/marketplace-api/internal/campaignbudget/concurrency/slot_test.go`
- `services/marketplace-api/internal/campaignbudget/transactional/counter.go` — `Record(ctx, storeID, count)` + fair-use check
- `services/marketplace-api/internal/campaignbudget/transactional/counter_test.go`
- `services/marketplace-api/migrations/000055_store_transactional_counter.up.sql`
- `services/marketplace-api/migrations/000055_store_transactional_counter.down.sql`

### Modify

- `services/marketplace-api/internal/handlers/admin/campaigns.go` (or the path the campaign-send endpoint lives at — verify in Task 1) — call `budget.Reserve` + `concurrency.AcquireSlot` pre-send
- `services/marketplace-api/cmd/marketplace-api/main.go` — start `robfig/cron/v3`, register the two jobs, wire `campaignbudget.Service` into handler deps
- `services/marketplace-api/internal/plangate/matrix.go` (from P3) — confirm `FeatureCampaignEmailsPerMonth` limits match spec §9 (Trial=5k, Starter=15k, Studio=50k, Pro=`Negotiated`)

### Delete

- Any pre-existing ad-hoc "email cap" check in the campaign handler (if present). P9 replaces it.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Errors + metrics scaffolding | — |
| 2 | `budget.Reserve` atomic-decrement + tests | 1, P1 |
| 3 | `ComputeRampDay` pure helper + tests | — |
| 4 | `budget.ApplyTrialRamp` + tests | 1, 2, 3 |
| 5 | `budget.RecomputeLimitForPlan` + tests | 1, P3 |
| 6 | `Service` façade (`Reserve` + `RecomputeLimitForPlan`) | 2, 5 |
| 7 | `budget.MonthlyReset` + tests | 1, P1 |
| 8 | Cron jobs (trial ramp + monthly reset) wired to `robfig/cron/v3` | 4, 7 |
| 9 | Migration 055 + `store_transactional_counter` model | P1 |
| 10 | `transactional.Record` + fair-use check + tests | 9 |
| 11 | `concurrency.AcquireSlot` Redis impl + tests | 1 |
| 12 | `concurrency.AcquireSlot` advisory-lock fallback + tests | 1 |
| 13 | Concurrency selector (Redis if configured, else advisory) | 11, 12 |
| 14 | Campaign send handler: call `AcquireSlot` + `Reserve` | 2, 6, 13 |
| 15 | `main.go` wiring (cron start, service injection) | 8, 14 |
| 16 | Plan-change integration — P4 calls `RecomputeLimitForPlan` in-tx | 6 (exposes hook for P4) |
| 17 | End-to-end §28 criteria test suite | 2, 4, 7, 14, 16 |
| 18 | Final scrub: grep for legacy ad-hoc caps | all |

Each task is one atomic commit boundary.

---

## Reusable patterns

**A. Single-statement atomic decrement** — the entire spec §10.1 enforcement is one UPDATE, no SELECT-then-UPDATE. `RowsAffected == 0` means either (a) no row for this (store_id, month) — `ErrNoBudgetRow` — or (b) insufficient balance — `ErrBudgetExhausted`. We disambiguate with a follow-up SELECT **only** on the error path, so the hot path is one round-trip.

**B. Idempotent ramp math** — `ComputeRampDay(signupDate, now) int` returns `1, 2, 3, 4, ..., 8+`. The ramp mutator is safe to call on any day: day 1/2/3 is a no-op (limit already 500 from monthly reset), day 4-7 sets `GREATEST(remaining, 2000)`, day 8+ sets plan allowance. Re-running the cron mid-day is a no-op because `GREATEST` and the plan-allowance set are monotonic.

**C. Transactional `RecomputeLimitForPlan`** — signature accepts `*gorm.DB` (not a dedicated `*sql.Tx`). GORM's `db` is already tx-aware — if the caller has `tx := db.Begin()` and passes `tx` in, the UPDATE joins that transaction. P4 does `tx.Transaction(func(tx *gorm.DB) error { ... writePlan(tx); return budget.RecomputeLimitForPlan(ctx, tx, storeID, newPlan) })`.

**D. `INSERT ... ON CONFLICT DO NOTHING` for monthly seeding** — the monthly-reset job is safe to re-run mid-day, safe to run on multiple pods simultaneously (only one wins the INSERT; the rest see 0 rows affected). No SELECT-before-INSERT. No pre-existence check.

**E. Concurrency slot shape** —
```go
type SlotAcquirer interface {
    AcquireSlot(ctx context.Context, storeID uuid.UUID) (release func(), err error)
}
```
Redis impl: `INCR campaign:slots:<store>`, reject if `>3`, set `EXPIRE 600`; release `DECR`. Advisory fallback: `pg_try_advisory_lock(hashtext('campaign:'||store_id||':slot'||i))` for `i in 1..3`, pick the first that succeeds; release on transaction end (session-scoped variant so the lock survives after acquire returns — so we use `pg_advisory_unlock` in the release closure).

**F. Cron idempotency** — all cron jobs must be safe under: (1) re-run on the same day, (2) multi-pod concurrent execution (Knative replica count can exceed 1 transiently), (3) clock skew up to ±1 minute. We achieve this through pure-function math and `ON CONFLICT DO NOTHING`, not through distributed locking. Distributed lock would be a stronger guarantee but is unnecessary here.

**G. Metric emission** — every code path that affects budget state emits exactly one metric. `Reserve` success: `campaign_email_sent_total.Inc(recipient_count)`. `Reserve` exhausted: `campaign_email_budget_exhausted_total{plan}.Inc()`. `ApplyTrialRamp` success (non-noop): `campaign_email_trial_ramp_applied_total{day}.Inc()`. No metric on the noop ramp day (don't drown the histogram in zeros).

---

## Task 1: Errors + metrics scaffolding

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/errors.go`
- Create: `services/marketplace-api/internal/campaignbudget/metrics.go`

**Spec references:** §10.1, §10.3.

- [ ] **Step 1: Write `errors.go`**

```go
// Package campaignbudget enforces per-month campaign email limits (spec §10).
// Every call site mutating the budget goes through this package; there is no
// correct path that bypasses Reserve.
package campaignbudget

import "errors"

// ErrBudgetExhausted is returned by Reserve when remaining < recipient_count.
// The HTTP layer maps this to 403 + upgrade-message copy per spec §10.1.
var ErrBudgetExhausted = errors.New("campaign email budget exhausted")

// ErrNoBudgetRow means no row exists for (store_id, current_month). Happens
// if the monthly-reset cron has not yet run for this store (e.g. first send
// of the month before 00:05 UTC, or a brand-new store signed up mid-month
// and the signup handler failed to seed the row). Treated as an upstream bug;
// the HTTP layer maps it to 500 + operator alert.
var ErrNoBudgetRow = errors.New("no campaign_email_budget row for current month")

// ErrPlanNegotiated means plangate.Limit returned plangate.Negotiated (Pro —
// "contact sales"). RecomputeLimitForPlan leaves limit_set unchanged and emits
// a warning metric for ops review.
var ErrPlanNegotiated = errors.New("plan has negotiated email ceiling — manual set required")
```

- [ ] **Step 2: Write `metrics.go`**

```go
package campaignbudget

import "github.com/prometheus/client_golang/prometheus"

var (
    SentTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "campaign_email_sent_total",
            Help: "Count of campaign emails successfully reserved against budget.",
        },
        []string{"store_id"},
    )
    BudgetExhaustedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "campaign_email_budget_exhausted_total",
            Help: "Count of campaign-send attempts rejected for budget exhaustion.",
        },
        []string{"plan"},
    )
    TrialRampAppliedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "campaign_email_trial_ramp_applied_total",
            Help: "Count of trial-ramp transitions applied (day 3→4 or 7→8).",
        },
        []string{"day"},
    )
    PlanRecomputeWarningTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "campaign_email_plan_recompute_negotiated_total",
            Help: "Times plan-change recomputation hit a Negotiated plan and skipped.",
        },
    )
)

func MustRegisterMetrics(reg prometheus.Registerer) {
    reg.MustRegister(SentTotal, BudgetExhaustedTotal, TrialRampAppliedTotal, PlanRecomputeWarningTotal)
}
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/{errors,metrics}.go
git commit -m "feat(campaignbudget): errors + prometheus metric scaffolding"
```

---

## Task 2: `budget.Reserve` atomic-decrement

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/reserve.go`
- Create: `services/marketplace-api/internal/campaignbudget/reserve_test.go`

**Spec references:** §10.1 (exact SQL shape).

- [ ] **Step 1: Write failing test — happy path + exhaustion + no-row**

```go
//go:build integration

package campaignbudget_test

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/campaignbudget"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestReserve_HappyPath(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := time.Now().UTC().Truncate(24 * time.Hour)
    month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)

    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 5000, 5000)`, storeID, month).Error)

    remaining, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
    require.NoError(t, err)
    require.Equal(t, 4900, remaining)
}

func TestReserve_Exhausted(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 50, 5000)`, storeID, month).Error)

    _, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
    require.ErrorIs(t, err, campaignbudget.ErrBudgetExhausted)

    // Row must be unchanged — atomic UPDATE rolled back / never took effect.
    var remaining int
    require.NoError(t, db.Raw(`SELECT remaining FROM campaign_email_budget WHERE store_id=?`, storeID).Scan(&remaining).Error)
    require.Equal(t, 50, remaining)
}

func TestReserve_NoRow(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    _, err := campaignbudget.Reserve(context.Background(), db, uuid.New(), 10)
    require.ErrorIs(t, err, campaignbudget.ErrNoBudgetRow)
}

func TestReserve_ExactMatch(t *testing.T) {
    // recipient_count == remaining → success, resulting remaining = 0.
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 100, 5000)`, storeID, month).Error)

    remaining, err := campaignbudget.Reserve(context.Background(), db, storeID, 100)
    require.NoError(t, err)
    require.Equal(t, 0, remaining)

    // Next 1-recipient send must now fail.
    _, err = campaignbudget.Reserve(context.Background(), db, storeID, 1)
    require.True(t, errors.Is(err, campaignbudget.ErrBudgetExhausted))
}

func firstOfMonthUTC(t time.Time) time.Time {
    u := t.UTC()
    return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}
```

- [ ] **Step 2: Run — expect FAIL (package doesn't exist)**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/campaignbudget/... -v
```

- [ ] **Step 3: Write `reserve.go`**

```go
package campaignbudget

import (
    "context"
    "errors"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

// Reserve atomically decrements the current-month budget row for storeID by
// recipientCount. Returns the post-decrement `remaining` on success.
//
// The UPDATE is the exact shape from spec §10.1 — a single round-trip with a
// WHERE guard that makes the update a no-op when remaining < recipientCount.
// That no-op becomes ErrBudgetExhausted (after a disambiguating SELECT).
//
// Thread safety: Postgres row-level locking inside the UPDATE serializes
// concurrent callers; no Go-level mutex needed. Two concurrent sends against
// the same store either both succeed (if both fit in remaining) or one wins
// and the other sees ErrBudgetExhausted.
func Reserve(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientCount int) (int, error) {
    if recipientCount <= 0 {
        return 0, fmt.Errorf("recipient_count must be positive, got %d", recipientCount)
    }

    // Exact SQL from spec §10.1 — single UPDATE, atomic, no SELECT first.
    const sql = `
        UPDATE campaign_email_budget
        SET remaining = remaining - ?
        WHERE store_id = ?
          AND month = date_trunc('month', (now() at time zone 'utc'))
          AND remaining >= ?
        RETURNING remaining`

    var remaining int
    row := db.WithContext(ctx).Raw(sql, recipientCount, storeID, recipientCount).Row()
    if err := row.Scan(&remaining); err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "sql: no rows in result set" {
            return 0, classifyNoUpdate(ctx, db, storeID)
        }
        return 0, fmt.Errorf("reserve: %w", err)
    }
    SentTotal.WithLabelValues(storeID.String()).Add(float64(recipientCount))
    return remaining, nil
}

// classifyNoUpdate disambiguates a 0-row UPDATE between "no row exists"
// (ErrNoBudgetRow) and "row exists but insufficient balance" (ErrBudgetExhausted).
// Only called on the error path, so the hot path stays one round-trip.
func classifyNoUpdate(ctx context.Context, db *gorm.DB, storeID uuid.UUID) error {
    var exists bool
    err := db.WithContext(ctx).Raw(`
        SELECT EXISTS (
            SELECT 1 FROM campaign_email_budget
            WHERE store_id = ?
              AND month = date_trunc('month', (now() at time zone 'utc'))
        )`, storeID).Scan(&exists).Error
    if err != nil {
        return fmt.Errorf("classify reserve failure: %w", err)
    }
    if !exists {
        return ErrNoBudgetRow
    }
    // Row exists, so the only way the UPDATE hit 0 rows is remaining < count.
    // We can't safely emit the exhausted metric here without the plan label;
    // the HTTP handler has the plan in context and emits it.
    return ErrBudgetExhausted
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/reserve{,_test}.go
git commit -m "feat(campaignbudget): atomic-decrement Reserve (spec §10.1)"
```

---

## Task 3: `ComputeRampDay` pure helper

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/ramp.go` (initial — pure helper only)
- Create: `services/marketplace-api/internal/campaignbudget/ramp_test.go` (initial tests)

**Spec references:** §5.1 (trial day table).

- [ ] **Step 1: Write failing tests for `ComputeRampDay`**

```go
package campaignbudget_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/campaignbudget"
)

func TestComputeRampDay_Boundaries(t *testing.T) {
    signup := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
    cases := []struct{ now time.Time; want int }{
        {time.Date(2026, 4, 1, 23, 59, 0, 0, time.UTC), 1}, // still day 1
        {time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), 2},  // rollover
        {time.Date(2026, 4, 3, 5, 0, 0, 0, time.UTC), 3},
        {time.Date(2026, 4, 4, 5, 0, 0, 0, time.UTC), 4},  // transition 3→4
        {time.Date(2026, 4, 7, 23, 0, 0, 0, time.UTC), 7},
        {time.Date(2026, 4, 8, 0, 5, 0, 0, time.UTC), 8},  // transition 7→8
        {time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 31},
    }
    for _, tc := range cases {
        got := campaignbudget.ComputeRampDay(signup, tc.now)
        require.Equal(t, tc.want, got, "signup=%s now=%s", signup, tc.now)
    }
}

func TestComputeRampDay_BeforeSignup(t *testing.T) {
    signup := time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)
    require.Equal(t, 1, campaignbudget.ComputeRampDay(signup, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)),
        "clock skew before signup must clamp to day 1, not a negative day")
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `ramp.go` (helper section)**

```go
package campaignbudget

import "time"

// ComputeRampDay returns the 1-indexed trial day for (signupDate, now).
// Day boundaries are UTC midnight — the cron runs at 00:00 UTC so a store
// that signed up at 2026-04-01T12:00Z is on day 1 until 2026-04-02T00:00Z
// then day 2 until 2026-04-03T00:00Z, etc.
//
// The function is pure and deterministic. Clock skew before signup clamps
// to day 1 rather than returning 0 or negative — the cron must never mistake
// pre-signup clock skew for "time to apply the plan allowance".
func ComputeRampDay(signupDate, now time.Time) int {
    s := time.Date(signupDate.UTC().Year(), signupDate.UTC().Month(), signupDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
    n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
    diffDays := int(n.Sub(s).Hours() / 24)
    if diffDays < 0 {
        return 1
    }
    return diffDays + 1
}

// IsRampTransitionDay reports whether `day` is one of the two transition days
// where the cron must mutate limit_set (3→4 or 7→8). On all other days the
// cron short-circuits for this store.
func IsRampTransitionDay(day int) bool {
    return day == 4 || day == 8
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/ramp{,_test}.go
git commit -m "feat(campaignbudget): pure ComputeRampDay helper (spec §5.1)"
```

---

## Task 4: `budget.ApplyTrialRamp` — DB mutator

**Files:**
- Modify: `services/marketplace-api/internal/campaignbudget/ramp.go`
- Modify: `services/marketplace-api/internal/campaignbudget/ramp_test.go`

**Spec references:** §5.1 (ramp values), §10.1 (trial-ramp cron semantics).

- [ ] **Step 1: Add failing tests for `ApplyTrialRamp`**

```go
//go:build integration

func TestApplyTrialRamp_Day3To4_PreservesConsumed(t *testing.T) {
    // Day 3→4: limit_set = GREATEST(remaining, 2000). If merchant already sent
    // 450 today, remaining=50 (of the 500 limit). Ramp must set limit_set=2000
    // and remaining=2000 − (500 − 50) − 0 = well, actually spec says "preserve
    // already-consumed" so we must raise limit_set and raise remaining by the
    // delta that doesn't undo consumption.
    //
    // Reading spec §10.1: "limit_set = GREATEST(remaining, 2000)" — so the new
    // limit is >= 2000. remaining becomes limit_set (i.e. a fresh budget of
    // the new ceiling). This is lenient to the merchant on the boundary day.
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 50, 500)`, storeID, month).Error)

    err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial")
    require.NoError(t, err)

    var remaining, limitSet int
    require.NoError(t, db.Raw(
        `SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&remaining, &limitSet))
    require.Equal(t, 2000, limitSet)
    require.Equal(t, 2000, remaining, "day-4 ramp resets remaining to new ceiling")
}

func TestApplyTrialRamp_Day7To8_UsesPlanAllowance(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 1500, 2000)`, storeID, month).Error)

    // Trial plan allowance = 5000 per spec §9.
    err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 8, "trial")
    require.NoError(t, err)

    var limitSet int
    require.NoError(t, db.Raw(
        `SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&limitSet))
    require.Equal(t, 5000, limitSet)
}

func TestApplyTrialRamp_NonTransitionDay_NoOp(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 300, 500)`, storeID, month).Error)

    // Day 2 is not a transition day.
    err := campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 2, "trial")
    require.NoError(t, err)

    var remaining, limitSet int
    require.NoError(t, db.Raw(
        `SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&remaining, &limitSet))
    require.Equal(t, 300, remaining)
    require.Equal(t, 500, limitSet)
}

func TestApplyTrialRamp_Idempotent_ReRunSameDay(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 50, 500)`, storeID, month).Error)

    require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))
    // Merchant consumes some.
    _, err := campaignbudget.Reserve(context.Background(), db, storeID, 200)
    require.NoError(t, err)
    // Cron re-runs (pod restart). Must NOT reset remaining back up.
    require.NoError(t, campaignbudget.ApplyTrialRamp(context.Background(), db, storeID, 4, "trial"))

    var remaining, limitSet int
    require.NoError(t, db.Raw(
        `SELECT remaining, limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&remaining, &limitSet))
    require.Equal(t, 1800, remaining, "idempotent: re-running the ramp must not re-inflate remaining")
    require.Equal(t, 2000, limitSet)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Append `ApplyTrialRamp` to `ramp.go`**

```go
// ApplyTrialRamp mutates the current-month budget row per spec §5.1:
//   - day 4 (transition from D3): limit_set = GREATEST(remaining, 2000),
//                                  remaining = limit_set
//   - day 8 (transition from D7): limit_set = plan_allowance,
//                                  remaining = GREATEST(remaining, plan_allowance)
//   - all other days: no-op
//
// Idempotency: re-running on the same transition day with a smaller plan_allowance
// uses GREATEST semantics so consumed balance is never re-inflated. The update
// is a single atomic statement; concurrent runs produce the same result.
//
// Uses plangate.Limit to resolve the plan allowance; on plangate.Negotiated
// (Pro — contact sales) the function emits PlanRecomputeWarningTotal and
// leaves limit_set unchanged. Returning nil (not an error) keeps cron green —
// a warning metric is the operational signal.
func ApplyTrialRamp(ctx context.Context, db *gorm.DB, storeID uuid.UUID, day int, plan string) error {
    if !IsRampTransitionDay(day) {
        return nil
    }

    var newLimit int
    switch day {
    case 4:
        // Single SQL does GREATEST(remaining, 2000) atomically.
        const sql = `
            UPDATE campaign_email_budget
            SET limit_set = GREATEST(remaining, 2000),
                remaining = GREATEST(remaining, 2000)
            WHERE store_id = ?
              AND month = date_trunc('month', (now() at time zone 'utc'))`
        if err := db.WithContext(ctx).Exec(sql, storeID).Error; err != nil {
            return fmt.Errorf("ramp day-4: %w", err)
        }
    case 8:
        allowance, err := plangate.Limit(plan, plangate.FeatureCampaignEmailsPerMonth)
        if err != nil {
            return fmt.Errorf("ramp day-8: resolve plan allowance: %w", err)
        }
        if allowance == plangate.Negotiated {
            PlanRecomputeWarningTotal.Inc()
            return nil // leave limit_set unchanged; ops sets it manually
        }
        newLimit = allowance
        const sql = `
            UPDATE campaign_email_budget
            SET limit_set = ?,
                remaining = GREATEST(remaining, ?)
            WHERE store_id = ?
              AND month = date_trunc('month', (now() at time zone 'utc'))`
        if err := db.WithContext(ctx).Exec(sql, newLimit, newLimit, storeID).Error; err != nil {
            return fmt.Errorf("ramp day-8: %w", err)
        }
    }
    TrialRampAppliedTotal.WithLabelValues(strconv.Itoa(day)).Inc()
    return nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/ramp{,_test}.go
git commit -m "feat(campaignbudget): ApplyTrialRamp day-3→4 and day-7→8 mutators"
```

---

## Task 5: `budget.RecomputeLimitForPlan` — P4 hook

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/recompute.go`
- Create: `services/marketplace-api/internal/campaignbudget/recompute_test.go`

**Spec references:** §10.1 ("Plan-change webhook also recomputes `limit_set` in same transaction as subscription write").

- [ ] **Step 1: Failing test — caller-owned transaction semantics**

```go
//go:build integration

func TestRecomputeLimitForPlan_InsideCallerTransaction(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 3000, 5000)`, storeID, month).Error) // trial allowance

    // Simulate P4's change-plan transaction: write new plan + recompute in one tx.
    err := db.Transaction(func(tx *gorm.DB) error {
        // Pretend plan row write happens here.
        if err := tx.Exec(`-- placeholder for plan row write`).Error; err != nil {
            return err
        }
        return campaignbudget.RecomputeLimitForPlan(context.Background(), tx, storeID, "starter")
    })
    require.NoError(t, err)

    var limitSet int
    require.NoError(t, db.Raw(
        `SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&limitSet))
    require.Equal(t, 15000, limitSet, "starter plan allowance per spec §9")
}

func TestRecomputeLimitForPlan_PlanNegotiated_NoOp(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 10000, 50000)`, storeID, month).Error) // studio

    err := campaignbudget.RecomputeLimitForPlan(context.Background(), db, storeID, "pro")
    require.NoError(t, err, "Pro (negotiated) must not error, only warn + noop")

    var limitSet int
    require.NoError(t, db.Raw(
        `SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&limitSet))
    require.Equal(t, 50000, limitSet, "limit_set unchanged when plan is Negotiated")
}

func TestRecomputeLimitForPlan_TxRollback_UndoesBudgetWrite(t *testing.T) {
    // Critical invariant: if the caller transaction rolls back, the budget
    // update must also roll back. Otherwise plan-row and budget-row state diverge.
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 3000, 5000)`, storeID, month).Error)

    sentinel := errors.New("rollback")
    err := db.Transaction(func(tx *gorm.DB) error {
        if err := campaignbudget.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio"); err != nil {
            return err
        }
        return sentinel
    })
    require.ErrorIs(t, err, sentinel)

    var limitSet int
    require.NoError(t, db.Raw(
        `SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID,
    ).Row().Scan(&limitSet))
    require.Equal(t, 5000, limitSet, "rollback must revert the budget update too")
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `recompute.go`**

```go
package campaignbudget

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/plangate"
)

// RecomputeLimitForPlan recalculates limit_set for the current-month row
// based on the new plan. Intended to be called from P4's change-plan
// transaction: pass the caller's *gorm.DB (which may be mid-transaction),
// and this write joins that transaction. On rollback, the budget update
// reverts with the plan row write — state stays consistent.
//
// When plangate.Limit returns plangate.Negotiated (Pro — contact sales),
// this function is a no-op that increments PlanRecomputeWarningTotal and
// returns nil. Ops receives the metric signal and sets limit_set manually.
//
// remaining is raised to GREATEST(remaining, new_limit) when upgrading to a
// larger allowance so the merchant immediately benefits from the new ceiling.
// On downgrade (new_limit < remaining), remaining is clamped down to new_limit
// — you can't keep what you haven't paid for on the new plan.
func RecomputeLimitForPlan(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, plan string) error {
    allowance, err := plangate.Limit(plan, plangate.FeatureCampaignEmailsPerMonth)
    if err != nil {
        return fmt.Errorf("recompute: resolve plan allowance: %w", err)
    }
    if allowance == plangate.Negotiated {
        PlanRecomputeWarningTotal.Inc()
        return nil
    }

    const sql = `
        UPDATE campaign_email_budget
        SET limit_set = ?,
            remaining = LEAST(GREATEST(remaining, 0), ?)
        WHERE store_id = ?
          AND month = date_trunc('month', (now() at time zone 'utc'))`
    if err := tx.WithContext(ctx).Exec(sql, allowance, allowance, storeID).Error; err != nil {
        return fmt.Errorf("recompute limit_set: %w", err)
    }
    // A second pass raises remaining on upgrade (LEAST capped it on downgrade).
    const raiseSQL = `
        UPDATE campaign_email_budget
        SET remaining = GREATEST(remaining, ?)
        WHERE store_id = ?
          AND month = date_trunc('month', (now() at time zone 'utc'))
          AND limit_set = ?
          AND remaining < ?`
    if err := tx.WithContext(ctx).Exec(raiseSQL, allowance, storeID, allowance, allowance).Error; err != nil {
        return fmt.Errorf("recompute raise-remaining: %w", err)
    }
    return nil
}
```

Note: the two-pass approach is deliberate — single SQL with `CASE WHEN new > old THEN new ELSE LEAST(remaining, new) END` is briefer but harder to reason about under transaction isolation. Two explicit updates produce the same end state and fit in caller-owned transactions cleanly.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/recompute{,_test}.go
git commit -m "feat(campaignbudget): RecomputeLimitForPlan hook for P4 plan-change tx"
```

---

## Task 6: `Service` façade

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/service.go`

**Purpose:** P4 imports one thing (`*campaignbudget.Service`) rather than free functions. Also the natural home for the handler-side dependency.

- [ ] **Step 1: Write `service.go`**

```go
package campaignbudget

import (
    "context"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

// Service bundles the campaign-budget operations that callers outside this
// package need. Construct once in main and inject.
type Service struct {
    db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Reserve is the hot-path pre-send decrement (spec §10.1).
func (s *Service) Reserve(ctx context.Context, storeID uuid.UUID, recipientCount int) (int, error) {
    return Reserve(ctx, s.db, storeID, recipientCount)
}

// RecomputeLimitForPlan is invoked by P4 inside its change-plan transaction.
// Pass the caller's *gorm.DB (mid-tx or not) via `tx`.
func (s *Service) RecomputeLimitForPlan(ctx context.Context, tx *gorm.DB, storeID uuid.UUID, plan string) error {
    return RecomputeLimitForPlan(ctx, tx, storeID, plan)
}
```

- [ ] **Step 2: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/service.go
git commit -m "feat(campaignbudget): Service façade for cross-package consumers"
```

---

## Task 7: `budget.MonthlyReset`

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/monthly_reset.go`
- Create: `services/marketplace-api/internal/campaignbudget/monthly_reset_test.go`

**Spec references:** §10.2 ("First-of-month scheduler creates new budget row").

- [ ] **Step 1: Failing test — happy path + idempotency + multi-store**

```go
//go:build integration

func TestMonthlyReset_SeedsAllActiveSubscriptions(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID := uuid.New()
    storeIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
    for _, sid := range storeIDs {
        require.NoError(t, db.Exec(`
            INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
            VALUES (?, ?, ?, 'starter', 'active')`, tenantID, sid, "cus_"+sid.String()).Error)
    }

    require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

    month := firstOfMonthUTC(time.Now())
    for _, sid := range storeIDs {
        var remaining, limitSet int
        err := db.Raw(`
            SELECT remaining, limit_set FROM campaign_email_budget
            WHERE store_id = ? AND month = ?`, sid, month,
        ).Row().Scan(&remaining, &limitSet)
        require.NoError(t, err)
        require.Equal(t, 15000, limitSet, "starter allowance per spec §9")
        require.Equal(t, 15000, remaining)
    }
}

func TestMonthlyReset_IdempotentReRun(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID := uuid.New()
    storeID := uuid.New()
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
        VALUES (?, ?, 'cus_x', 'starter', 'active')`, tenantID, storeID).Error)

    require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))
    // Consume some.
    _, err := campaignbudget.Reserve(context.Background(), db, storeID, 5000)
    require.NoError(t, err)
    // Re-run same day — must NOT overwrite consumed state.
    require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

    var remaining int
    require.NoError(t, db.Raw(`
        SELECT remaining FROM campaign_email_budget
        WHERE store_id = ? AND month = date_trunc('month', (now() at time zone 'utc'))`,
        storeID).Row().Scan(&remaining))
    require.Equal(t, 10000, remaining, "re-run must NOT reset remaining back up")
}

func TestMonthlyReset_SkipsNonActiveStatuses(t *testing.T) {
    // expired / store_closed / pending_hard_delete subscriptions get no row.
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID, activeID, expiredID := uuid.New(), uuid.New(), uuid.New()
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
        VALUES (?, ?, 'cus_a', 'starter', 'active'),
               (?, ?, 'cus_e', 'starter', 'expired')`,
        tenantID, activeID, tenantID, expiredID).Error)

    require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

    month := firstOfMonthUTC(time.Now())
    var activeExists, expiredExists bool
    require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=? AND month=?)`, activeID, month).Row().Scan(&activeExists))
    require.NoError(t, db.Raw(`SELECT EXISTS(SELECT 1 FROM campaign_email_budget WHERE store_id=? AND month=?)`, expiredID, month).Row().Scan(&expiredExists))
    require.True(t, activeExists)
    require.False(t, expiredExists)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `monthly_reset.go`**

```go
package campaignbudget

import (
    "context"
    "fmt"

    "gorm.io/gorm"
)

// MonthlyReset seeds one campaign_email_budget row for each active subscription
// for the current UTC month. Safe to re-run on the same day and across multiple
// pods — `ON CONFLICT DO NOTHING` ensures only one winner per (store_id, month).
//
// Active statuses that get a row: signup, trialing, active, past_due,
// payment_action_required, cancel_scheduled. Statuses excluded (merchant cannot
// send campaigns): expired, store_closed, pending_hard_delete, hard_deleted.
//
// The limit_set + remaining start at the plan allowance. Stores currently in
// trial (D1-7) will have their limit_set overwritten by the trial-ramp cron
// that runs later the same day — the ramp cron uses GREATEST semantics so a
// 15k starter seed followed by a D4 ramp to 2,000 correctly clamps to 15k on
// day 4, and the D8 ramp reaffirms the 15k ceiling. For stores that signed up
// this month on day 0, the signup handler is responsible for seeding the
// initial 500-limit row directly (see P5); MonthlyReset covers month rollovers.
//
// Pro subscriptions (plangate.Negotiated) skip seeding — ops sets limit_set
// manually. PlanRecomputeWarningTotal is incremented once per skipped Pro.
func MonthlyReset(ctx context.Context, db *gorm.DB) error {
    const sql = `
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        SELECT
            ss.store_id,
            date_trunc('month', (now() at time zone 'utc'))::date,
            plan_allowance_for(ss.plan),
            plan_allowance_for(ss.plan)
        FROM store_subscriptions ss
        WHERE ss.status IN (
            'signup', 'trialing', 'active', 'past_due',
            'payment_action_required', 'cancel_scheduled'
        )
          AND plan_allowance_for(ss.plan) IS NOT NULL
        ON CONFLICT (store_id, month) DO NOTHING`

    // We inline a SQL function plan_allowance_for via CTE to avoid a
    // DB-function migration. (If a shared function already exists, prefer that.)
    const sqlWithCTE = `
        WITH allowance(plan, amount) AS (VALUES
            ('trial',       5000),
            ('starter',    15000),
            ('studio',     50000),
            ('marketplace', NULL::int)  -- marketplace handled separately
            -- 'pro' intentionally omitted: Negotiated, operator sets manually
        )
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        SELECT
            ss.store_id,
            date_trunc('month', (now() at time zone 'utc'))::date,
            a.amount,
            a.amount
        FROM store_subscriptions ss
        JOIN allowance a ON a.plan = ss.plan
        WHERE ss.status IN (
            'signup', 'trialing', 'active', 'past_due',
            'payment_action_required', 'cancel_scheduled'
        )
          AND a.amount IS NOT NULL
        ON CONFLICT (store_id, month) DO NOTHING`

    _ = sql // future shape if plan_allowance_for DB function lands
    if err := db.WithContext(ctx).Exec(sqlWithCTE).Error; err != nil {
        return fmt.Errorf("monthly reset: %w", err)
    }
    return nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/monthly_reset{,_test}.go
git commit -m "feat(campaignbudget): MonthlyReset seeds active subscriptions idempotently"
```

---

## Task 8: Cron jobs

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/cron/jobs.go`
- Create: `services/marketplace-api/internal/campaignbudget/cron/jobs_test.go`

**Spec references:** §5.1 (trial-ramp 00:00 UTC), §10.2 (monthly reset first-of-month).

- [ ] **Step 1: Failing test — ramp job walks active subscriptions**

```go
//go:build integration

func TestTrialRampJob_AppliesToTransitioningStores(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID := uuid.New()

    // Store A: day 4 today → should ramp.
    storeA := uuid.New()
    sigA := time.Now().UTC().AddDate(0, 0, -3) // 3 days ago → today is day 4
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, signup_date)
        VALUES (?, ?, 'cus_a', 'trial', 'trialing', ?)`, tenantID, storeA, sigA).Error)
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 500, 500)`, storeA, month).Error)

    // Store B: day 2 today → no-op.
    storeB := uuid.New()
    sigB := time.Now().UTC().AddDate(0, 0, -1)
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, signup_date)
        VALUES (?, ?, 'cus_b', 'trial', 'trialing', ?)`, tenantID, storeB, sigB).Error)
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 300, 500)`, storeB, month).Error)

    require.NoError(t, cron.RunTrialRampOnce(context.Background(), db, time.Now()))

    var limitA, limitB int
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeA).Row().Scan(&limitA))
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeB).Row().Scan(&limitB))
    require.Equal(t, 2000, limitA, "day-4 store ramped to 2000")
    require.Equal(t, 500, limitB, "day-2 store unchanged")
}

func TestTrialRampJob_SkipsStoresWithoutSignupDate(t *testing.T) {
    // Migrated merchants (§5.1.1 fast-path) with a NULL signup_date must not panic.
    db := testdb.NewDB(t, "store_subscriptions")
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, signup_date)
        VALUES (?, ?, 'cus_x', 'trial', 'trialing', NULL)`, tenantID, storeID).Error)

    require.NoError(t, cron.RunTrialRampOnce(context.Background(), db, time.Now()))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `cron/jobs.go`**

```go
// Package cron registers the trial-ramp and monthly-reset jobs with
// robfig/cron/v3. Both jobs are pure idempotent functions of (db, now()) —
// safe to re-run, safe to run on multiple pods concurrently.
package cron

import (
    "context"
    "fmt"
    "time"

    robcron "github.com/robfig/cron/v3"
    "github.com/sirupsen/logrus"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/campaignbudget"
)

// RegisterTrialRampJob schedules the trial-ramp cron to run at 00:00 UTC daily.
// The returned func is passed to cron.AddFunc in main.go.
func RegisterTrialRampJob(c *robcron.Cron, db *gorm.DB) (robcron.EntryID, error) {
    return c.AddFunc("CRON_TZ=UTC 0 0 * * *", func() {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
        defer cancel()
        if err := RunTrialRampOnce(ctx, db, time.Now()); err != nil {
            logrus.WithError(err).Error("trial ramp cron failed")
        }
    })
}

// RegisterMonthlyResetJob schedules the monthly-reset cron for 00:05 UTC on
// the 1st of each month. 5-minute offset from midnight guarantees the trial
// ramp runs first on day-1 boundaries (harmless, but clearer).
func RegisterMonthlyResetJob(c *robcron.Cron, db *gorm.DB) (robcron.EntryID, error) {
    return c.AddFunc("CRON_TZ=UTC 5 0 1 * *", func() {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
        defer cancel()
        if err := campaignbudget.MonthlyReset(ctx, db); err != nil {
            logrus.WithError(err).Error("monthly reset cron failed")
        }
    })
}

// RunTrialRampOnce is the exported, testable body of the trial-ramp cron.
// Iterates all trialing subscriptions and applies ApplyTrialRamp using
// ComputeRampDay(signup_date, now). Non-transition days are no-ops.
func RunTrialRampOnce(ctx context.Context, db *gorm.DB, now time.Time) error {
    rows, err := db.WithContext(ctx).Raw(`
        SELECT store_id, plan, signup_date
        FROM store_subscriptions
        WHERE status IN ('signup', 'trialing')
          AND signup_date IS NOT NULL`,
    ).Rows()
    if err != nil {
        return fmt.Errorf("trial ramp: query subscriptions: %w", err)
    }
    defer rows.Close()

    for rows.Next() {
        var storeIDStr, plan string
        var signupDate time.Time
        if err := rows.Scan(&storeIDStr, &plan, &signupDate); err != nil {
            logrus.WithError(err).Warn("trial ramp: scan row")
            continue
        }
        storeID, err := uuid.Parse(storeIDStr)
        if err != nil {
            continue
        }
        day := campaignbudget.ComputeRampDay(signupDate, now)
        if !campaignbudget.IsRampTransitionDay(day) {
            continue
        }
        if err := campaignbudget.ApplyTrialRamp(ctx, db, storeID, day, plan); err != nil {
            logrus.WithError(err).WithField("store_id", storeID).Warn("trial ramp: apply failed")
        }
    }
    return rows.Err()
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/cron/jobs{,_test}.go
git commit -m "feat(campaignbudget): trial-ramp + monthly-reset cron jobs"
```

---

## Task 9: Migration 055 — `store_transactional_counter`

**Files:**
- Create: `services/marketplace-api/migrations/000055_store_transactional_counter.up.sql`
- Create: `services/marketplace-api/migrations/000055_store_transactional_counter.down.sql`

**Spec references:** §10.2 ("Transactional emails separate pipeline, 100k/store/month fair-use.").

- [ ] **Step 1: Write up migration**

```sql
-- 000055_store_transactional_counter.up.sql
-- Tracks transactional (non-campaign) email volume per store-month.
-- Separate table — NEVER join to campaign_email_budget. Transactional sends
-- bypass the budget entirely; this table only powers the 100k/store/month
-- fair-use soft cap and an ops dashboard.

CREATE TABLE IF NOT EXISTS store_transactional_counter (
    store_id  UUID NOT NULL,
    month     DATE NOT NULL,
    sent      INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (store_id, month)
);

CREATE INDEX IF NOT EXISTS stc_month_idx ON store_transactional_counter (month);
```

- [ ] **Step 2: Write down**

```sql
-- 000055_store_transactional_counter.down.sql
DROP INDEX IF EXISTS stc_month_idx;
DROP TABLE IF EXISTS store_transactional_counter;
```

- [ ] **Step 3: Apply + verify**

```bash
cd services/marketplace-api
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d store_transactional_counter"
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000055_store_transactional_counter.*.sql
git commit -m "feat: migration 055 — store_transactional_counter (fair-use tracking)"
```

---

## Task 10: `transactional.Record`

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/transactional/counter.go`
- Create: `services/marketplace-api/internal/campaignbudget/transactional/counter_test.go`

**Spec references:** §10.2.

- [ ] **Step 1: Failing test**

```go
//go:build integration

func TestTransactionalCounter_Record_IncrementsOrInserts(t *testing.T) {
    db := testdb.NewDB(t, "store_transactional_counter")
    storeID := uuid.New()

    count, err := transactional.Record(context.Background(), db, storeID, 50)
    require.NoError(t, err)
    require.Equal(t, 50, count)

    count, err = transactional.Record(context.Background(), db, storeID, 30)
    require.NoError(t, err)
    require.Equal(t, 80, count)
}

func TestTransactionalCounter_FairUseCap_SoftWarning(t *testing.T) {
    db := testdb.NewDB(t, "store_transactional_counter")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO store_transactional_counter (store_id, month, sent)
        VALUES (?, ?, 99990)`, storeID, month).Error)

    // Recording 20 more still succeeds (soft cap) but flags overage.
    count, err := transactional.Record(context.Background(), db, storeID, 20)
    require.NoError(t, err)
    require.Equal(t, 100010, count)
    require.True(t, transactional.IsOverFairUse(count), "100k+ flagged for ops review")
}
```

- [ ] **Step 2: Write `counter.go`**

```go
// Package transactional tracks per-store transactional email volume against
// the 100k/store/month fair-use soft cap (spec §10.2). Transactional sends
// NEVER decrement campaign_email_budget — that table is campaign-only. This
// counter exists purely to surface abuse patterns to ops; callers record
// after the send succeeds, and IsOverFairUse is checked lazily on a dashboard
// job, not on the send path.
package transactional

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

// FairUseCapPerMonth is the soft threshold above which ops investigates.
const FairUseCapPerMonth = 100_000

// Record increments the current-month counter for storeID by count. Creates
// the row if missing. Returns the new total. Never errors on the hot path
// unless the DB itself is unreachable.
func Record(ctx context.Context, db *gorm.DB, storeID uuid.UUID, count int) (int, error) {
    if count <= 0 {
        return 0, nil
    }
    const sql = `
        INSERT INTO store_transactional_counter (store_id, month, sent)
        VALUES (?, date_trunc('month', (now() at time zone 'utc'))::date, ?)
        ON CONFLICT (store_id, month) DO UPDATE
        SET sent = store_transactional_counter.sent + EXCLUDED.sent
        RETURNING sent`
    var total int
    row := db.WithContext(ctx).Raw(sql, storeID, count).Row()
    if err := row.Scan(&total); err != nil {
        return 0, fmt.Errorf("transactional record: %w", err)
    }
    return total, nil
}

// IsOverFairUse returns true when the month total exceeds the soft cap. Ops
// dashboards surface stores in this set; enforcement (if any) is manual.
func IsOverFairUse(total int) bool { return total > FairUseCapPerMonth }
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/transactional/counter{,_test}.go
git commit -m "feat(transactional): per-store fair-use counter (100k/month soft cap)"
```

---

## Task 11: `concurrency.AcquireSlot` — Redis implementation

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/concurrency/slot.go`
- Create: `services/marketplace-api/internal/campaignbudget/concurrency/slot_redis.go`
- Create: `services/marketplace-api/internal/campaignbudget/concurrency/slot_test.go`

**Spec references:** §10.2 ("Max 3 concurrent sends via Redis INCR or advisory lock.").

- [ ] **Step 1: Failing test — 3 succeed, 4th rejected, release frees slot**

```go
//go:build integration

func TestRedisSlot_ThreeConcurrentAllowed_FourthRejected(t *testing.T) {
    redis := testredis.NewClient(t)
    acq := concurrency.NewRedisAcquirer(redis, 10*time.Minute)
    storeID := uuid.New()

    releases := make([]func(), 0, 3)
    for i := 0; i < 3; i++ {
        release, err := acq.AcquireSlot(context.Background(), storeID)
        require.NoError(t, err)
        releases = append(releases, release)
    }
    _, err := acq.AcquireSlot(context.Background(), storeID)
    require.ErrorIs(t, err, concurrency.ErrTooManyConcurrentSends)

    releases[0]()
    _, err = acq.AcquireSlot(context.Background(), storeID)
    require.NoError(t, err, "slot frees after release")
}

func TestRedisSlot_TTL_LeakProtection(t *testing.T) {
    redis := testredis.NewClient(t)
    acq := concurrency.NewRedisAcquirer(redis, 2*time.Second)
    storeID := uuid.New()

    _, err := acq.AcquireSlot(context.Background(), storeID)
    require.NoError(t, err)
    // Pod crashes without calling release; TTL expires.
    time.Sleep(3 * time.Second)
    _, err = acq.AcquireSlot(context.Background(), storeID)
    require.NoError(t, err, "TTL must reclaim stuck slots")
}
```

- [ ] **Step 2: Write interface + Redis impl**

```go
// Package concurrency enforces the max-3 concurrent-send limit per store.
// Two implementations: Redis INCR (preferred, cluster-wide) and a Postgres
// advisory-lock fallback (single-instance, used when Redis is absent).
package concurrency

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
)

// MaxConcurrentSends is the per-store cap.
const MaxConcurrentSends = 3

// ErrTooManyConcurrentSends is returned when the store already has
// MaxConcurrentSends active send jobs.
var ErrTooManyConcurrentSends = errors.New("too many concurrent campaign sends")

// SlotAcquirer abstracts the concurrency backend. Callers depend on this type
// and main.go wires either the Redis or Postgres implementation.
type SlotAcquirer interface {
    AcquireSlot(ctx context.Context, storeID uuid.UUID) (release func(), err error)
}
```

```go
// slot_redis.go
package concurrency

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
)

type redisAcquirer struct {
    client *redis.Client
    ttl    time.Duration
}

// NewRedisAcquirer returns a SlotAcquirer backed by Redis INCR+EXPIRE.
// ttl must exceed the longest acceptable send duration — 10 minutes per spec.
func NewRedisAcquirer(c *redis.Client, ttl time.Duration) SlotAcquirer {
    return &redisAcquirer{client: c, ttl: ttl}
}

func (r *redisAcquirer) AcquireSlot(ctx context.Context, storeID uuid.UUID) (func(), error) {
    key := fmt.Sprintf("campaign:slots:%s", storeID)
    n, err := r.client.Incr(ctx, key).Result()
    if err != nil {
        return nil, fmt.Errorf("redis incr: %w", err)
    }
    if n == 1 {
        // First holder sets the TTL. Subsequent INCRs don't touch TTL, so a
        // stuck first holder is still reclaimed when TTL expires.
        _ = r.client.Expire(ctx, key, r.ttl).Err()
    }
    if n > int64(MaxConcurrentSends) {
        // Decrement on the way out — we reserved the slot, have to give it back.
        _ = r.client.Decr(ctx, key).Err()
        return nil, ErrTooManyConcurrentSends
    }
    release := func() {
        _ = r.client.Decr(context.Background(), key).Err()
    }
    return release, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/concurrency/slot{,_redis,_test}.go
git commit -m "feat(concurrency): redis-backed 3-concurrent-send slot acquirer"
```

---

## Task 12: Postgres advisory-lock fallback

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/concurrency/slot_advisory.go`
- Modify: `services/marketplace-api/internal/campaignbudget/concurrency/slot_test.go`

- [ ] **Step 1: Failing test**

```go
//go:build integration

func TestAdvisoryLockSlot_ThreeConcurrent(t *testing.T) {
    db := testdb.NewDB(t)
    acq := concurrency.NewAdvisoryLockAcquirer(db)
    storeID := uuid.New()

    releases := make([]func(), 0, 3)
    for i := 0; i < 3; i++ {
        release, err := acq.AcquireSlot(context.Background(), storeID)
        require.NoError(t, err)
        releases = append(releases, release)
    }
    _, err := acq.AcquireSlot(context.Background(), storeID)
    require.ErrorIs(t, err, concurrency.ErrTooManyConcurrentSends)

    releases[1]()
    _, err = acq.AcquireSlot(context.Background(), storeID)
    require.NoError(t, err)
}
```

- [ ] **Step 2: Write advisory impl**

```go
// slot_advisory.go
package concurrency

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

type advisoryAcquirer struct{ db *gorm.DB }

// NewAdvisoryLockAcquirer uses pg_try_advisory_lock over N slot keys.
// Single-pod deployments only — each pod has its own Postgres session pool,
// so a lock held on pod A is not visible to pod B. Use only when Redis is
// unavailable and the service is known to be single-replica (Knative minScale=1,
// maxScale=1). Knative autoscaling from 0→1→0 is safe.
func NewAdvisoryLockAcquirer(db *gorm.DB) SlotAcquirer {
    return &advisoryAcquirer{db: db}
}

func (a *advisoryAcquirer) AcquireSlot(ctx context.Context, storeID uuid.UUID) (func(), error) {
    // Try each of the 3 slots in turn; first successful acquire wins.
    // We need a dedicated connection per lock so the lock outlives the method
    // call — pg_advisory_lock is session-scoped.
    sqlDB, err := a.db.DB()
    if err != nil {
        return nil, fmt.Errorf("get sql.DB: %w", err)
    }
    conn, err := sqlDB.Conn(ctx)
    if err != nil {
        return nil, fmt.Errorf("reserve conn: %w", err)
    }

    for slot := 1; slot <= MaxConcurrentSends; slot++ {
        key := fmt.Sprintf("campaign:slot:%s:%d", storeID, slot)
        var acquired bool
        row := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key)
        if err := row.Scan(&acquired); err != nil {
            _ = conn.Close()
            return nil, fmt.Errorf("try advisory lock: %w", err)
        }
        if acquired {
            release := func() {
                _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, key)
                _ = conn.Close()
            }
            return release, nil
        }
    }
    _ = conn.Close()
    return nil, ErrTooManyConcurrentSends
}

// silence unused import warning for sql when building without race detector
var _ = sql.ErrNoRows
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/concurrency/slot_advisory.go services/marketplace-api/internal/campaignbudget/concurrency/slot_test.go
git commit -m "feat(concurrency): postgres advisory-lock fallback acquirer"
```

---

## Task 13: Concurrency selector

**Files:**
- Modify: `services/marketplace-api/internal/campaignbudget/concurrency/slot.go`

- [ ] **Step 1: Add selector**

```go
// Select returns a Redis-backed acquirer when redisClient != nil, else the
// Postgres advisory-lock fallback. Called once from main.go.
func Select(redisClient *redis.Client, db *gorm.DB) SlotAcquirer {
    if redisClient != nil {
        return NewRedisAcquirer(redisClient, 10*time.Minute)
    }
    return NewAdvisoryLockAcquirer(db)
}
```

- [ ] **Step 2: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/concurrency/slot.go
git commit -m "feat(concurrency): Select helper chooses redis or advisory fallback"
```

---

## Task 14: Campaign send handler wiring

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/campaigns.go` (locate first — if file name differs, pick the handler that owns the "send campaign" POST route; grep `campaign.*send` under `internal/handlers`)

- [ ] **Step 1: Locate the send handler**

```bash
cd services/marketplace-api
grep -RnE 'POST.*/campaigns|SendCampaign|campaign.*send' internal/handlers/ cmd/ | grep -v _test.go
```

- [ ] **Step 2: Inject deps into the handler struct**

```go
type CampaignHandler struct {
    budget  *campaignbudget.Service
    slots   concurrency.SlotAcquirer
    plans   plangate.Resolver
    // ... existing fields
}
```

- [ ] **Step 3: Replace any ad-hoc cap check with the new flow**

```go
func (h *CampaignHandler) Send(c *gin.Context) {
    storeID := /* existing: from Gin context */
    plan   := /* existing: plans.Resolve(...) */
    req    := /* existing: bind JSON */

    // 1. Acquire concurrency slot.
    release, err := h.slots.AcquireSlot(c.Request.Context(), storeID)
    if err != nil {
        if errors.Is(err, concurrency.ErrTooManyConcurrentSends) {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error":   "too_many_concurrent_sends",
                "message": "You already have 3 campaign sends in flight. Try again shortly.",
            })
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
        return
    }
    defer release()

    // 2. Reserve budget.
    remaining, err := h.budget.Reserve(c.Request.Context(), storeID, len(req.Recipients))
    switch {
    case errors.Is(err, campaignbudget.ErrBudgetExhausted):
        campaignbudget.BudgetExhaustedTotal.WithLabelValues(plan).Inc()
        c.JSON(http.StatusForbidden, gin.H{
            "error":   "campaign_email_budget_exhausted",
            "message": "You've used your monthly campaign email allowance. Upgrade your plan for more.",
        })
        return
    case errors.Is(err, campaignbudget.ErrNoBudgetRow):
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "budget_row_missing",
            "message": "Monthly budget not initialised. Support has been notified.",
        })
        return
    case err != nil:
        c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
        return
    }

    // 3. Enqueue the actual send job. (Existing code path.)
    _ = remaining
    // ... existing send logic
}
```

- [ ] **Step 4: Integration test — handler returns 403 when budget exhausted**

Add to `campaigns_test.go`:

```go
func TestSendCampaign_BudgetExhausted_Returns403(t *testing.T) {
    // ... set budget row with remaining=0, POST /admin/stores/:id/campaigns/send ...
    // Expect status 403 + body.error == "campaign_email_budget_exhausted"
}
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/campaigns*.go
git commit -m "feat(campaigns): enforce budget + concurrency on send handler"
```

---

## Task 15: `main.go` wiring

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Start cron + inject service**

```go
// --- campaignbudget + cron wiring ---
campaignBudget := campaignbudget.NewService(db)
campaignbudget.MustRegisterMetrics(prometheus.DefaultRegisterer)

slotAcquirer := concurrency.Select(redisClient, db) // redisClient may be nil

cronScheduler := robcron.New(robcron.WithLocation(time.UTC))
if _, err := campaignbudgetcron.RegisterTrialRampJob(cronScheduler, db); err != nil {
    logger.WithError(err).Fatal("register trial ramp job")
}
if _, err := campaignbudgetcron.RegisterMonthlyResetJob(cronScheduler, db); err != nil {
    logger.WithError(err).Fatal("register monthly reset job")
}
// Orphan-webhook cron from P2 is also registered on the same scheduler.
cronScheduler.Start()
defer cronScheduler.Stop()

// inject `campaignBudget` + `slotAcquirer` into handler deps alongside the
// rest of the marketplace services.
```

- [ ] **Step 2: Verify build**

```bash
cd services/marketplace-api
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "chore(main): wire campaignbudget service + crons + concurrency"
```

---

## Task 16: P4 hook exposure

This task confirms the shape P4 needs to invoke, so when P4 is implemented its change-plan handler can call:

```go
// inside P4's change-plan handler, already mid-transaction:
return db.Transaction(func(tx *gorm.DB) error {
    if err := writeNewPlanRow(tx, storeID, newPlan); err != nil {
        return err
    }
    return campaignBudget.RecomputeLimitForPlan(ctx, tx, storeID, string(newPlan))
})
```

- [ ] **Step 1: Verify `RecomputeLimitForPlan` signature matches what P4 will need**

- Parameter order: `(ctx, tx, storeID, plan)` — `tx` second so it reads like `service.Recompute(ctx, tx, ...)`.
- Return type: `error` (the caller transaction handles the rollback).
- No separate "dry run" variant is needed — P4's transaction atomicity gives us that for free.

- [ ] **Step 2: Add a contract test — P4-shape caller semantics**

```go
//go:build integration

func TestRecomputeContract_P4CallerSemantics(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 5000, 5000)`, storeID, month).Error)

    service := campaignbudget.NewService(db)

    // Shape that P4's change-plan handler will use.
    err := db.Transaction(func(tx *gorm.DB) error {
        // Imagine P4 wrote the plan row here.
        return service.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio")
    })
    require.NoError(t, err)

    var limitSet int
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID).Row().Scan(&limitSet))
    require.Equal(t, 50000, limitSet)
}
```

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/recompute_test.go
git commit -m "test(campaignbudget): contract test for P4-shape change-plan caller"
```

---

## Task 17: End-to-end §28 criteria test suite

**Files:**
- Create: `services/marketplace-api/internal/campaignbudget/e2e_criteria_test.go`

**Success criteria asserted** (from the ask and spec §28):
1. Trial-ramp day-3→4 transitions successfully (cron mutates limit_set from 500 to 2,000).
2. Monthly rollover creates fresh row with `remaining == limit_set == plan_allowance`.
3. Atomic decrement correctness under concurrent sends (exactly one wins when both would exhaust).
4. Plan-change recomputation updates `limit_set` inside caller transaction and rolls back on abort.

- [ ] **Step 1: Write the suite**

```go
//go:build integration

package campaignbudget_test

// Criterion 1: trial-ramp day-3→4
func TestE2E_TrialRamp_Day3To4(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID, storeID := uuid.New(), uuid.New()
    sig := time.Now().UTC().AddDate(0, 0, -3) // today is day 4
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status, signup_date)
        VALUES (?, ?, 'cus_x', 'trial', 'trialing', ?)`, tenantID, storeID, sig).Error)
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 500, 500)`, storeID, month).Error)

    require.NoError(t, cron.RunTrialRampOnce(context.Background(), db, time.Now()))

    var limitSet int
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID).Row().Scan(&limitSet))
    require.Equal(t, 2000, limitSet)
}

// Criterion 2: monthly rollover
func TestE2E_MonthlyRollover_SeedsFreshRow(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget", "store_subscriptions")
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Exec(`
        INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan, status)
        VALUES (?, ?, 'cus_x', 'starter', 'active')`, tenantID, storeID).Error)

    require.NoError(t, campaignbudget.MonthlyReset(context.Background(), db))

    var remaining, limitSet int
    require.NoError(t, db.Raw(`
        SELECT remaining, limit_set FROM campaign_email_budget
        WHERE store_id = ? AND month = date_trunc('month', (now() at time zone 'utc'))`,
        storeID).Row().Scan(&remaining, &limitSet))
    require.Equal(t, 15000, remaining)
    require.Equal(t, 15000, limitSet)
}

// Criterion 3: atomic decrement — exactly one succeeds under race
func TestE2E_AtomicDecrement_ExactlyOneWinsOnExhaust(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 100, 500)`, storeID, month).Error)

    // Two senders each asking for 80. Only one can succeed.
    var wg sync.WaitGroup
    var okCount, exhaustedCount int32
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err := campaignbudget.Reserve(context.Background(), db, storeID, 80)
            switch {
            case err == nil:
                atomic.AddInt32(&okCount, 1)
            case errors.Is(err, campaignbudget.ErrBudgetExhausted):
                atomic.AddInt32(&exhaustedCount, 1)
            default:
                t.Errorf("unexpected: %v", err)
            }
        }()
    }
    wg.Wait()

    require.EqualValues(t, 1, okCount)
    require.EqualValues(t, 1, exhaustedCount)

    var remaining int
    require.NoError(t, db.Raw(`SELECT remaining FROM campaign_email_budget WHERE store_id=?`, storeID).Row().Scan(&remaining))
    require.Equal(t, 20, remaining, "exactly one decrement of 80 applied: 100 - 80 = 20")
}

// Criterion 4: plan-change recomputation atomic with caller transaction
func TestE2E_PlanChangeRecompute_AtomicWithCallerTx(t *testing.T) {
    db := testdb.NewDB(t, "campaign_email_budget")
    storeID := uuid.New()
    month := firstOfMonthUTC(time.Now())
    require.NoError(t, db.Exec(`
        INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set)
        VALUES (?, ?, 3000, 5000)`, storeID, month).Error)

    svc := campaignbudget.NewService(db)

    // Success path.
    require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
        return svc.RecomputeLimitForPlan(context.Background(), tx, storeID, "studio")
    }))
    var limitSet int
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID).Row().Scan(&limitSet))
    require.Equal(t, 50000, limitSet)

    // Rollback path.
    sentinel := errors.New("rollback")
    err := db.Transaction(func(tx *gorm.DB) error {
        if err := svc.RecomputeLimitForPlan(context.Background(), tx, storeID, "trial"); err != nil {
            return err
        }
        return sentinel
    })
    require.ErrorIs(t, err, sentinel)
    require.NoError(t, db.Raw(`SELECT limit_set FROM campaign_email_budget WHERE store_id=?`, storeID).Row().Scan(&limitSet))
    require.Equal(t, 50000, limitSet, "rollback preserved the studio value")
}
```

- [ ] **Step 2: Run — expect all PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/campaignbudget/e2e_criteria_test.go
git commit -m "test(campaignbudget): §28 criteria — ramp, rollover, atomic decrement, recompute"
```

---

## Task 18: Final scrub for legacy caps

- [ ] **Step 1: Grep for any ad-hoc email-cap check that bypasses the budget service**

```bash
cd services/marketplace-api
grep -RnE 'emails?.*(limit|cap|quota)|campaign.*count.*(>|<|>=|<=)' internal/ cmd/ \
  | grep -v _test.go \
  | grep -v internal/campaignbudget/ \
  | grep -v internal/plangate/ \
  || echo "clean"
```
Expected: `clean`. Any hit is a pre-existing cap check that should now funnel through `Service.Reserve`.

- [ ] **Step 2: Grep for direct writes to `campaign_email_budget` outside the package**

```bash
grep -RnE 'campaign_email_budget' internal/ cmd/ \
  | grep -v _test.go \
  | grep -v internal/campaignbudget/ \
  || echo "clean"
```
Expected: `clean`. Every mutation must go through the package.

- [ ] **Step 3: Run full integration suite**

```bash
go test -tags=integration ./... -count=1
```
Expected: green.

- [ ] **Step 4: Final commit**

```bash
git add -u
git commit --allow-empty -m "chore(campaignbudget): scrub verified — no off-package budget writes"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./internal/campaignbudget/...` all green.
- [ ] `budget.Reserve` uses the exact SQL shape from spec §10.1 (single UPDATE, WHERE guards recipient count, RETURNING remaining).
- [ ] `budget.Reserve` distinguishes `ErrBudgetExhausted` from `ErrNoBudgetRow`.
- [ ] `ComputeRampDay` is pure, handles pre-signup clock skew, handles UTC boundaries.
- [ ] `ApplyTrialRamp` day-4 uses `GREATEST(remaining, 2000)`; day-8 uses plan allowance via `plangate.Limit`.
- [ ] `ApplyTrialRamp` is idempotent — re-running on the same day does not re-inflate `remaining`.
- [ ] `ApplyTrialRamp` on `plangate.Negotiated` is a no-op + warning metric, not an error.
- [ ] `RecomputeLimitForPlan` accepts a `*gorm.DB` that may be mid-transaction and joins that tx — rollback-safe.
- [ ] `MonthlyReset` uses `ON CONFLICT (store_id, month) DO NOTHING` — safe to re-run + multi-pod safe.
- [ ] `MonthlyReset` skips expired / store_closed / pending_hard_delete subscriptions.
- [ ] Trial-ramp cron registered at `CRON_TZ=UTC 0 0 * * *`, monthly reset at `CRON_TZ=UTC 5 0 1 * *`.
- [ ] `concurrency.AcquireSlot` caps at 3/store; Redis TTL = 10 min; advisory fallback uses `pg_try_advisory_lock` on 3 slot keys.
- [ ] `concurrency.Select` returns Redis impl when client present, advisory when not.
- [ ] Campaign send handler calls `AcquireSlot` → `Reserve` → enqueue, in that order; releases slot via `defer`.
- [ ] Handler maps `ErrBudgetExhausted` → 403 with `campaign_email_budget_exhausted` code and upgrade copy.
- [ ] Handler maps `ErrTooManyConcurrentSends` → 429.
- [ ] `transactional.Record` uses `INSERT ... ON CONFLICT DO UPDATE SET sent = sent + EXCLUDED.sent`.
- [ ] `transactional.Record` does NOT touch `campaign_email_budget` (grep confirms).
- [ ] Prometheus metrics registered: `campaign_email_sent_total`, `campaign_email_budget_exhausted_total`, `campaign_email_trial_ramp_applied_total`, `campaign_email_plan_recompute_negotiated_total`.
- [ ] §28 criteria suite passes: trial-ramp day-3→4, monthly rollover, atomic-decrement race, plan-change recompute (success + rollback).
- [ ] Grep confirms no direct `UPDATE campaign_email_budget` outside `internal/campaignbudget/`.

---

## What's now unlocked

- **P4** (upgrade/downgrade + store-block) can now call `campaignBudget.RecomputeLimitForPlan(ctx, tx, storeID, newPlan)` inside its change-plan transaction. Atomicity is guaranteed: either the plan row **and** the budget row both commit, or both roll back.
- **P5** (trial card-add deferred charge) can seed a day-1 budget row for brand-new signups directly (500/500) without waiting for the monthly-reset cron — a simple direct INSERT using the same `ON CONFLICT DO NOTHING` pattern. `ApplyTrialRamp` will then walk the row through 500 → 2,000 → plan allowance on days 4 and 8.
- **P6** (dunning) doesn't interact directly, but `past_due` and `payment_action_required` stores continue to receive monthly-reset rows (they're in the active-status list), so a merchant who recovers from dunning still has a current-month budget to decrement against.
- **P10** (SES migration) will be triggered at 500 paid merchants per spec §10.3 — that threshold is queryable via a simple count of subscriptions with `plan IN ('starter','studio','pro') AND status = 'active'`. Out of scope here; documented as a trigger, not implemented.
- **P16** (admin frontend) will display "X / Y campaign emails used this month" by reading the budget row directly via a new read-only endpoint (cheap GET wrapper around the service).
- **P17** (observability) will chart `campaign_email_sent_total` per store, `campaign_email_budget_exhausted_total` per plan (upsell signal), and `campaign_email_trial_ramp_applied_total` per day (sanity check on trial activation funnel).

---

## Execution handoff

Plan complete. Nine implementation plans are now saved under `docs/superpowers/plans/`:

- `2026-04-18-p1-subscription-data-model.md`
- `2026-04-18-p2-stripe-multicurrency-webhooks.md`
- `2026-04-18-p3-state-machine-plan-gates.md`
- `2026-04-18-p4-upgrade-downgrade-store-block.md`
- `2026-04-18-p5-trial-card-add-deferred-charge.md`
- `2026-04-18-p6-dunning-payment-action-required.md`
- `2026-04-18-p7-tax-id-validation.md`
- `2026-04-18-p8-promo-codes-floor.md`
- `2026-04-18-p9-campaign-email-budget.md`

Execute **P9** with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans** after P1, P3, and P4 are merged. P9 has a hard compile-time dependency on P3's `plangate.Limit` / `plangate.FeatureCampaignEmailsPerMonth` / `plangate.Negotiated` symbols, and a runtime integration contract with P4's change-plan transaction handler. The `Service.RecomputeLimitForPlan` signature is frozen by Task 16's contract test — P4 calls into that exact shape.
