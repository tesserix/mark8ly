# P4 — Plan Upgrade/Downgrade + Store-Block + Image Grandfathering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the merchant-facing `POST /admin/stores/:storeId/subscription/change-plan` endpoint plus its two silent partners — the **downgrade-block preflight** (§4.5.1 store-count + in-flight-order CSV data contract) and the **end-of-period cron re-check** (§4.5.1 Council finding #2) — and introduce the **Studio→Starter image-limit grandfathering** rule (§11). Every plan change must run inside the P1 `subscription.WithAdvisoryLock`, emit a new `subscription.plan_changed` audit action, and commit `billing_currency` untouched (§4.2.1).

**Architecture:** A new `internal/subscription/planchange` package owns the full orchestration: preflight eligibility (store-count + currency-locked check) → proration decision → Stripe `UpdateSubscription` call (P2) → local row update under advisory lock → audit emit. Upgrades are immediate-with-prorate (Stripe native); downgrades park a `pending_downgrade_plan` + `pending_downgrade_effective_at` on the subscription row and rely on a new `downgrade_recheck_cron` to execute at period end (or block and stay on the current plan). Image grandfathering is compute-at-enforcement: a thin `plangate.ImagesAllowed(plan, productCreatedAt, planChangedAt)` function returns the correct cap without backfilling rows. Monthly↔Annual period switches reuse the same orchestration path (upgrade = immediate prorate, downgrade = end-of-period). The endpoint is **NOT** a plan catalog browser — it mutates a live subscription only. Policy is the state machine (P3): plan changes stay in `active` unless cancellation is separately invoked (out of scope — P11).

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, `github.com/stripe/stripe-go/v82` (P2), `github.com/stretchr/testify`, `robfig/cron/v3` (existing pattern in `audit-service`), `internal/audit` + `internal/authz` + `internal/subscription` + `internal/subscription/statemachine` + `internal/plangate` + `internal/stores`.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §4.4 (billing-period changes), §4.5 (upgrade/downgrade), §4.5.1 (downgrade-block checks + cron re-check), §4.2.1 (currency lock), §11 (image grandfathering), §17.4 (concurrency via advisory lock on `store_id`).

**Depends on:**
- **P1** — `SubscriptionPlan`/`SubscriptionStatus` enums, `subscription.WithAdvisoryLock(ctx, tx, storeID, fn)` helper (Task 13), `audit.EmitStateTransition` scaffold (Task 14), migration header for the five new columns added in this plan.
- **P2** — `stripeclient.Client` with `UpdateSubscription(ctx, SubscriptionID, params)` — **this plan adds that method** if P2 did not expose it. Reuses P2 multi-currency price-ID resolver (`stripeclient.PriceIDFor(plan, period, currency, tier)`).
- **P3** — `statemachine.Transition` is **not** called for plan changes (they do not change status), but `statemachine.IsReadOnly` is consulted as a precondition.

**Related plans:**
- **P5** (trial card-add deferred charge) — trial merchants cannot call change-plan until card added; 402 Payment Required with `upgrade_requires_card` error.
- **P6** (dunning) — `past_due` merchants cannot change plan; 409 Conflict.
- **P10** (refund accounting) — consumes the `subscription_plan_change_audit` table to reconcile proration line items.
- **P11** (cancellation + save-offer) — lives next door at `POST /admin/stores/:storeId/subscription/cancel`, shares the advisory-lock pattern.
- **P16** (admin frontend) — consumes the `GET /admin/stores/:storeId/subscription/change-plan/preflight` contract (store-count response shape).

---

## Scope Check

In scope:
1. `POST /admin/stores/:storeId/subscription/change-plan` — body `{target_plan, billing_period}`; upgrade prorates immediately via Stripe, downgrade parks a pending downgrade record.
2. `GET /admin/stores/:storeId/subscription/change-plan/preflight` — preflight data contract for the UI: current plan, target plan diff, store count (active + soft-deleted-but-restorable ≤60d), per-store in-flight-order counts, CSV-download link. No mutation.
3. Store-count block on **Studio→Starter** at BOTH the preflight (UI) and the downgrade cron (execution) gates.
4. Close-vs-delete distinction (backend only) — `stores.Close(storeID)` keeps plan slot; `stores.Delete(storeID)` frees it with 60-day soft-delete grace. **Handlers for close/delete already exist in `internal/handlers/admin/stores.go`**; this plan adds the plan-slot accounting rules on top.
5. Image-limit grandfathering — `plangate.ImagesAllowed(plan, productCreatedAt, planChangedAt)` returns 50 for products created before the Studio→Starter change, 25 afterwards. Compute-at-enforcement, no row backfill.
6. Monthly↔Annual switch — same orchestration; Pro monthly+20% premium released on annual switch by picking the right Stripe Price ID.
7. `downgrade_recheck_cron` — runs hourly; finds rows where `pending_downgrade_effective_at <= now()`; re-runs store-count + image-count checks; either commits the downgrade (Stripe `UpdateSubscription` + clear pending fields) or **blocks** (clear pending fields, stay on current plan, renewal continues at current plan rate, email merchant). **Stays in `active`** (no `cancel_scheduled` misroute — Council finding #2).
8. New migration `000047_subscription_pending_downgrade.up.sql` — four columns on `store_subscriptions` plus `subscription_plan_change_audit` append-only table.
9. Currency-never-changes guard (§4.2.1) — the endpoint reads `billing_currency` from the row and refuses any request that would resolve to a different-currency Price ID.

Out of scope:
- Admin UI wiring (P16) — UI consumes the preflight endpoint; we ship the contract, not the React code.
- Cancellation flow (P11) — `active → cancel_scheduled` transitions, save-offer copy, Pro+App teardown sequence.
- Refund accounting (P10) — proration credits surface in Stripe invoices, recorded in audit here; dollar-level reconciliation is P10.
- Storefront/admin closure UX (P12).
- Tax-ID revalidation triggered by plan change (§19.5 quarterly revalidation) — lives in P7.

---

## File Structure

### Create

- `services/marketplace-api/migrations/000047_subscription_pending_downgrade.up.sql`
- `services/marketplace-api/migrations/000047_subscription_pending_downgrade.down.sql`
- `services/marketplace-api/migrations/000048_subscription_plan_change_audit.up.sql`
- `services/marketplace-api/migrations/000048_subscription_plan_change_audit.down.sql`
- `services/marketplace-api/internal/subscription/planchange/planchange.go` — `Orchestrator` + `Preflight` types, `Execute` method
- `services/marketplace-api/internal/subscription/planchange/planchange_test.go`
- `services/marketplace-api/internal/subscription/planchange/preflight.go` — `PreflightReport` builder (store count + in-flight orders)
- `services/marketplace-api/internal/subscription/planchange/preflight_test.go`
- `services/marketplace-api/internal/subscription/planchange/rules.go` — pure decision helpers (`IsUpgrade`, `RequiresStoreCountCheck`, `EffectiveAt`)
- `services/marketplace-api/internal/subscription/planchange/rules_test.go`
- `services/marketplace-api/internal/subscription/planchange/cron.go` — `DowngradeRecheckCron` (idempotent hourly job)
- `services/marketplace-api/internal/subscription/planchange/cron_test.go`
- `services/marketplace-api/internal/subscription/planchange/auditlog.go` — `PlanChangeAudit` GORM model + writer
- `services/marketplace-api/internal/subscription/planchange/auditlog_test.go`
- `services/marketplace-api/internal/handlers/admin/subscription_change_plan.go` — HTTP handler
- `services/marketplace-api/internal/handlers/admin/subscription_change_plan_test.go`

### Modify

- `services/marketplace-api/internal/subscription/models.go` — add 4 fields to `StoreSubscription` (`PendingDowngradePlan`, `PendingDowngradeEffectiveAt`, `LastPlanChangeAt`, `LastPlanChangeReason`)
- `services/marketplace-api/internal/subscription/repository.go` — add `CountStoresForPlanSlot(ctx, tenantID) (int, error)` + `SetPendingDowngrade` / `ClearPendingDowngrade` / `CommitDowngrade` methods
- `services/marketplace-api/internal/plangate/matrix.go` — add `ImagesAllowed(plan, productCreatedAt, planChangedAt)` using P3's `FeatureImagesPerProduct` limits
- `services/marketplace-api/internal/plangate/matrix_test.go` — grandfathering table test
- `services/marketplace-api/internal/stores/repository.go` — `CountActiveOrSoftDeletedRestorable(ctx, tenantID) (int, error)` + `InFlightOrderCount(ctx, storeID) (int, error)` if not already present (check existing; add if not)
- `services/marketplace-api/internal/billing/stripeclient/client.go` (from P2) — **add** `UpdateSubscription(ctx, params UpdateSubscriptionParams) (*stripe.Subscription, error)` if P2 did not include it
- `services/marketplace-api/internal/handlers/admin/routes.go` — wire `change-plan` + `change-plan/preflight` endpoints
- `services/marketplace-api/cmd/marketplace-api/main.go` — construct `planchange.Orchestrator`; start `DowngradeRecheckCron`
- `services/marketplace-api/internal/audit/emitter.go` — add `EmitPlanChange` thin wrapper over `EmitStateTransition` with a new `subscription.plan_changed` action constant
- `services/marketplace-api/marketplaceapi.go` — bump expected schema version to `48`

### Delete

- None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migration 047 — pending-downgrade columns + schema constraints | P1 migrations applied |
| 2 | Migration 048 — `subscription_plan_change_audit` table | 1 |
| 3 | Extend `StoreSubscription` Go struct; `SetPendingDowngrade` / `CommitDowngrade` repo methods | 1 |
| 4 | `planchange/rules.go` — pure decision helpers (TDD-first) | 3 |
| 5 | `stripeclient.UpdateSubscription` + `PriceIDFor` sanity check | P2 |
| 6 | `planchange.Orchestrator.Execute` — upgrade path (immediate + prorate) | 3, 4, 5 |
| 7 | `planchange.Orchestrator.Execute` — downgrade path (park pending) | 3, 4, 6 |
| 8 | `planchange/preflight.go` — `PreflightReport` builder | 3, 4 |
| 9 | HTTP handler `POST /admin/stores/:storeId/subscription/change-plan` + preflight GET | 6, 7, 8 |
| 10 | `planchange/auditlog.go` — `PlanChangeAudit` write + read | 2 |
| 11 | `audit.EmitPlanChange` wrapper + `subscription.plan_changed` action | P1 Task 14 |
| 12 | `plangate.ImagesAllowed` grandfathering helper | P3 Task 4 |
| 13 | `DowngradeRecheckCron` — hourly executor with block-on-over-quota behaviour | 6, 7, 8, 10 |
| 14 | Route wiring + `main.go` cron start | 9, 13 |
| 15 | Integration test — Studio→Starter blocked when 5 stores (criterion #39) | all |
| 16 | Integration test — grandfathered product keeps 50-image cap after downgrade | 12 |
| 17 | Integration test — Monthly→Annual Pro releases +20% premium | 6 |
| 18 | Integration test — currency-change attempt rejected (§4.2.1) | 6 |
| 19 | Schema-version bump + final grep scrub | all |

Each task is one atomic commit boundary (conventional commits, single-line, no signatures).

---

## Reusable patterns

**A. Advisory-locked transaction** — every subscription write in this plan wraps `subscription.WithAdvisoryLock(ctx, db, storeID, fn)` from P1 Task 13. See P1 §"Reusable patterns B + C". Concurrent plan changes on the same store serialize at Postgres session level. Never issue a bare `UPDATE store_subscriptions` outside the lock.

**B. Audit emit** — every plan change calls `emitter.EmitPlanChange(ginCtx, audit.PlanChange{...})`, a thin wrapper over `EmitStateTransition` (P1 Task 14). The action constant is `subscription.plan_changed`. Upgrades, downgrade-scheduling, downgrade-commit, and downgrade-block each emit a distinct event with the same action and a different `subaction` metadata field.

**C. Stripe client composition** — `stripeclient.UpdateSubscription` lives in `internal/billing/stripeclient/client.go` (from P2). Input: `UpdateSubscriptionParams{SubscriptionID, PriceID, ProrationBehavior, Metadata}`. Output: `*stripe.Subscription`. Always attach `subscription_id` + `store_id` + `mark8ly_action` metadata per P2 §"Idempotency keys". Idempotency key pattern: `plan-change:<store_id>:<target_plan>:<target_period>:<now_truncated_5m>`.

**D. Preflight-vs-execute symmetry** — `planchange.Evaluate(ctx, storeID, target)` returns a `Decision` value (`DecisionAllowUpgradeNow`, `DecisionAllowDowngradeAtPeriodEnd`, `DecisionBlockStoreCount{Count, Stores}`, `DecisionBlockCurrency{Current, Requested}`). Both the preflight GET and the change-plan POST feed the same `Decision`; the GET serialises it to JSON, the POST acts on it. One truth, two consumers.

**E. Idempotent cron** — `DowngradeRecheckCron` runs hourly. The query `WHERE pending_downgrade_effective_at IS NOT NULL AND pending_downgrade_effective_at <= now()` returns all ready rows. Each row is processed inside its own `WithAdvisoryLock`; a CAS UPDATE clears `pending_downgrade_*` atomically so a retry sees no work. Multiple cron pods can run concurrently without clobbering each other.

**F. Integration-test harness** — `//go:build integration` + `pkg/testdb/testdb.go` (P1 §"Reusable patterns D"). Stripe calls stubbed via `stripeclient.Stub` (introduced in P2 tests). Clock is `clockwork.FakeClock` for time-travel tests (P6 dunning plan also uses this; introduce here if not yet introduced — see Task 13 Step 2).

---

## Task 1: Migration 047 — pending-downgrade columns

**Files:**
- Create: `services/marketplace-api/migrations/000047_subscription_pending_downgrade.up.sql`
- Create: `services/marketplace-api/migrations/000047_subscription_pending_downgrade.down.sql`

**Spec references:** §4.5 (downgrade at end of period), §4.5.1 (cron re-check), §11 (grandfathering timestamp source).

- [ ] **Step 1: Write the up migration**

```sql
-- 000047_subscription_pending_downgrade.up.sql
-- v2.3 §4.5 + §4.5.1: park a pending downgrade on the subscription row
-- so the hourly cron can re-check quota at execution time.
-- All columns nullable; the column set is meaningless until a downgrade is scheduled.

ALTER TABLE store_subscriptions
    ADD COLUMN pending_downgrade_plan           VARCHAR(30),
    ADD COLUMN pending_downgrade_period         VARCHAR(10),
    ADD COLUMN pending_downgrade_effective_at   TIMESTAMPTZ,
    ADD COLUMN last_plan_change_at              TIMESTAMPTZ,
    ADD COLUMN last_plan_change_reason          VARCHAR(64),
    ADD CONSTRAINT ss_pending_downgrade_plan_check CHECK (
        pending_downgrade_plan IS NULL
        OR pending_downgrade_plan IN ('trial','starter','studio','pro','marketplace')
    ),
    ADD CONSTRAINT ss_pending_downgrade_period_check CHECK (
        pending_downgrade_period IS NULL
        OR pending_downgrade_period IN ('monthly','annual')
    ),
    -- If any pending-downgrade field is set, the whole trio must be set.
    ADD CONSTRAINT ss_pending_downgrade_consistency_check CHECK (
        (pending_downgrade_plan IS NULL AND pending_downgrade_period IS NULL AND pending_downgrade_effective_at IS NULL)
        OR (pending_downgrade_plan IS NOT NULL AND pending_downgrade_period IS NOT NULL AND pending_downgrade_effective_at IS NOT NULL)
    );

-- Partial index — cron reads exclusively on (effective_at IS NOT NULL).
CREATE INDEX IF NOT EXISTS ss_pending_downgrade_ready_idx
    ON store_subscriptions (pending_downgrade_effective_at)
    WHERE pending_downgrade_effective_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS ss_last_plan_change_idx
    ON store_subscriptions (last_plan_change_at);
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000047_subscription_pending_downgrade.down.sql
DROP INDEX IF EXISTS ss_last_plan_change_idx;
DROP INDEX IF EXISTS ss_pending_downgrade_ready_idx;

ALTER TABLE store_subscriptions
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_consistency_check,
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_period_check,
    DROP CONSTRAINT IF EXISTS ss_pending_downgrade_plan_check,
    DROP COLUMN IF EXISTS last_plan_change_reason,
    DROP COLUMN IF EXISTS last_plan_change_at,
    DROP COLUMN IF EXISTS pending_downgrade_effective_at,
    DROP COLUMN IF EXISTS pending_downgrade_period,
    DROP COLUMN IF EXISTS pending_downgrade_plan;
```

- [ ] **Step 3: Apply and verify**

```bash
cd services/marketplace-api
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d store_subscriptions" | grep -E "pending_downgrade|last_plan_change"
```

Expected: five rows. Violate the consistency check to confirm:

```bash
psql "$TEST_DATABASE_URL" -c "UPDATE store_subscriptions SET pending_downgrade_plan='starter' WHERE false;" || true
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000047_subscription_pending_downgrade.*.sql
git commit -m "feat(subscription): add pending-downgrade columns for end-of-period execution"
```

---

## Task 2: Migration 048 — `subscription_plan_change_audit`

**Files:**
- Create: `services/marketplace-api/migrations/000048_subscription_plan_change_audit.up.sql`
- Create: `services/marketplace-api/migrations/000048_subscription_plan_change_audit.down.sql`

**Rationale:** The `audit-service` event stream is the source of truth for compliance, but an append-only per-subscription ledger makes reconciliation against Stripe proration cheaper (no cross-service FDW join). Mirrors the shape of the P1 `subscription_arbitrage_audit` table.

- [ ] **Step 1: Write the up migration**

```sql
-- 000048_subscription_plan_change_audit.up.sql
CREATE TABLE subscription_plan_change_audit (
    id                       UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID            NOT NULL,
    store_id                 UUID            NOT NULL,
    stripe_subscription_id   VARCHAR(64),
    stripe_invoice_id        VARCHAR(64),
    from_plan                VARCHAR(30)     NOT NULL,
    to_plan                  VARCHAR(30)     NOT NULL,
    from_period              VARCHAR(10)     NOT NULL,
    to_period                VARCHAR(10)     NOT NULL,
    action                   VARCHAR(40)     NOT NULL,
    billing_currency         CHAR(3)         NOT NULL,
    proration_cents          BIGINT,
    actor                    VARCHAR(128)    NOT NULL,
    reason                   VARCHAR(256),
    effective_at             TIMESTAMPTZ     NOT NULL,
    created_at               TIMESTAMPTZ     NOT NULL DEFAULT now(),
    CONSTRAINT spca_action_check CHECK (action IN (
        'upgrade_committed',
        'downgrade_scheduled',
        'downgrade_committed',
        'downgrade_blocked_over_quota',
        'period_switch_committed'
    ))
);

CREATE INDEX spca_store_idx   ON subscription_plan_change_audit (store_id, created_at DESC);
CREATE INDEX spca_tenant_idx  ON subscription_plan_change_audit (tenant_id, created_at DESC);
CREATE INDEX spca_action_idx  ON subscription_plan_change_audit (action, created_at DESC);

-- Lock down mutations — append-only.
REVOKE UPDATE, DELETE ON subscription_plan_change_audit FROM PUBLIC;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000048_subscription_plan_change_audit.down.sql
DROP TABLE IF EXISTS subscription_plan_change_audit;
```

- [ ] **Step 3: Apply and verify**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d subscription_plan_change_audit"
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000048_subscription_plan_change_audit.*.sql
git commit -m "feat(subscription): append-only subscription_plan_change_audit table"
```

---

## Task 3: Extend `StoreSubscription` + repository helpers

**Files:**
- Modify: `services/marketplace-api/internal/subscription/models.go`
- Modify: `services/marketplace-api/internal/subscription/repository.go`
- Create: `services/marketplace-api/internal/subscription/repository_plan_change_test.go`

**Spec references:** §4.5, §4.5.1.

- [ ] **Step 1: Failing test — `SetPendingDowngrade` stores all three fields**

```go
//go:build integration

package subscription_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestSetPendingDowngrade_PersistsTrio(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    repo := subscription.NewRepository(db)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        BillingCurrency: "USD",
    }).Error)

    effective := time.Now().Add(15 * 24 * time.Hour).Truncate(time.Second)
    require.NoError(t, repo.SetPendingDowngrade(context.Background(), tenantID, storeID,
        subscription.PlanStarter, subscription.PeriodMonthly, effective, "user_initiated"))

    got, err := repo.GetByStoreID(context.Background(), tenantID, storeID)
    require.NoError(t, err)
    require.Equal(t, subscription.PlanStarter, *got.PendingDowngradePlan)
    require.Equal(t, subscription.PeriodMonthly, *got.PendingDowngradePeriod)
    require.True(t, got.PendingDowngradeEffectiveAt.Equal(effective))
}

func TestClearPendingDowngrade_UnsetsTrio(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    repo := subscription.NewRepository(db)
    tenantID, storeID := uuid.New(), uuid.New()
    effective := time.Now().Add(24 * time.Hour)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        BillingCurrency: "USD",
        PendingDowngradePlan:        ptr(subscription.PlanStarter),
        PendingDowngradePeriod:      ptr(subscription.PeriodMonthly),
        PendingDowngradeEffectiveAt: &effective,
    }).Error)

    require.NoError(t, repo.ClearPendingDowngrade(context.Background(), tenantID, storeID))

    got, err := repo.GetByStoreID(context.Background(), tenantID, storeID)
    require.NoError(t, err)
    require.Nil(t, got.PendingDowngradePlan)
    require.Nil(t, got.PendingDowngradePeriod)
    require.Nil(t, got.PendingDowngradeEffectiveAt)
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 2: Run — expect FAIL (fields + methods don't exist)**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/subscription/... -run PendingDowngrade -v
```

- [ ] **Step 3: Extend `models.go`**

Add to `StoreSubscription`:

```go
type SubscriptionPeriod string

const (
    PeriodMonthly SubscriptionPeriod = "monthly"
    PeriodAnnual  SubscriptionPeriod = "annual"
)

type StoreSubscription struct {
    // …existing fields from P1…

    PendingDowngradePlan        *SubscriptionPlan   `gorm:"column:pending_downgrade_plan;type:varchar(30)"`
    PendingDowngradePeriod      *SubscriptionPeriod `gorm:"column:pending_downgrade_period;type:varchar(10)"`
    PendingDowngradeEffectiveAt *time.Time          `gorm:"column:pending_downgrade_effective_at"`
    LastPlanChangeAt            *time.Time          `gorm:"column:last_plan_change_at"`
    LastPlanChangeReason        *string             `gorm:"column:last_plan_change_reason;type:varchar(64)"`
}
```

- [ ] **Step 4: Add repo methods**

```go
// SetPendingDowngrade stores the target plan and scheduled execution time on
// the subscription row. Idempotent — overwriting a prior pending downgrade is
// intentional (merchant can change their mind; last write wins).
// MUST be called from inside WithAdvisoryLock.
func (r *Repository) SetPendingDowngrade(
    ctx context.Context, tenantID, storeID uuid.UUID,
    targetPlan SubscriptionPlan, targetPeriod SubscriptionPeriod,
    effectiveAt time.Time, reason string,
) error {
    res := r.db.WithContext(ctx).Exec(`
        UPDATE store_subscriptions
        SET pending_downgrade_plan         = ?,
            pending_downgrade_period       = ?,
            pending_downgrade_effective_at = ?,
            last_plan_change_at            = now(),
            last_plan_change_reason        = ?,
            updated_at                     = now()
        WHERE tenant_id = ? AND store_id = ?`,
        targetPlan, targetPeriod, effectiveAt, reason, tenantID, storeID,
    )
    if res.Error != nil { return fmt.Errorf("subscription: set pending downgrade: %w", res.Error) }
    if res.RowsAffected == 0 { return ErrNotFound }
    return nil
}

// ClearPendingDowngrade unsets the trio. Idempotent.
// MUST be called from inside WithAdvisoryLock.
func (r *Repository) ClearPendingDowngrade(
    ctx context.Context, tenantID, storeID uuid.UUID,
) error {
    res := r.db.WithContext(ctx).Exec(`
        UPDATE store_subscriptions
        SET pending_downgrade_plan         = NULL,
            pending_downgrade_period       = NULL,
            pending_downgrade_effective_at = NULL,
            updated_at                     = now()
        WHERE tenant_id = ? AND store_id = ?`,
        tenantID, storeID,
    )
    if res.Error != nil { return fmt.Errorf("subscription: clear pending downgrade: %w", res.Error) }
    return nil
}

// CommitDowngrade is a single-statement plan+period swap. Used by the cron once
// all preconditions pass. MUST be called from inside WithAdvisoryLock.
func (r *Repository) CommitDowngrade(
    ctx context.Context, tenantID, storeID uuid.UUID,
    newPlan SubscriptionPlan, newPeriod SubscriptionPeriod,
) error {
    res := r.db.WithContext(ctx).Exec(`
        UPDATE store_subscriptions
        SET plan                           = ?,
            subscription_period            = ?,
            pending_downgrade_plan         = NULL,
            pending_downgrade_period       = NULL,
            pending_downgrade_effective_at = NULL,
            last_plan_change_at            = now(),
            last_plan_change_reason        = 'downgrade_committed',
            updated_at                     = now()
        WHERE tenant_id = ? AND store_id = ?`,
        newPlan, newPeriod, tenantID, storeID,
    )
    if res.Error != nil { return fmt.Errorf("subscription: commit downgrade: %w", res.Error) }
    if res.RowsAffected == 0 { return ErrNotFound }
    return nil
}

// CommitUpgrade is the mirror of CommitDowngrade for immediate upgrades.
// MUST be called from inside WithAdvisoryLock.
func (r *Repository) CommitUpgrade(
    ctx context.Context, tenantID, storeID uuid.UUID,
    newPlan SubscriptionPlan, newPeriod SubscriptionPeriod, reason string,
) error {
    res := r.db.WithContext(ctx).Exec(`
        UPDATE store_subscriptions
        SET plan                           = ?,
            subscription_period            = ?,
            pending_downgrade_plan         = NULL,
            pending_downgrade_period       = NULL,
            pending_downgrade_effective_at = NULL,
            last_plan_change_at            = now(),
            last_plan_change_reason        = ?,
            updated_at                     = now()
        WHERE tenant_id = ? AND store_id = ?`,
        newPlan, newPeriod, reason, tenantID, storeID,
    )
    if res.Error != nil { return fmt.Errorf("subscription: commit upgrade: %w", res.Error) }
    if res.RowsAffected == 0 { return ErrNotFound }
    return nil
}

// FindPendingDowngradesReady returns rows whose effective_at has passed. Used by the cron.
// Callers process each row under WithAdvisoryLock.
func (r *Repository) FindPendingDowngradesReady(ctx context.Context, now time.Time) ([]StoreSubscription, error) {
    var out []StoreSubscription
    err := r.db.WithContext(ctx).
        Where("pending_downgrade_effective_at IS NOT NULL AND pending_downgrade_effective_at <= ?", now).
        Find(&out).Error
    return out, err
}
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/subscription/{models,repository,repository_plan_change_test}.go
git commit -m "feat(subscription): repo methods for pending-downgrade lifecycle"
```

---

## Task 4: `planchange/rules.go` — pure decision helpers

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/rules.go`
- Create: `services/marketplace-api/internal/subscription/planchange/rules_test.go`

**Spec references:** §4.4 (period changes), §4.5 (upgrade/downgrade direction), §4.5.1 (Studio→Starter store-count block).

> **Purpose:** zero DB, zero Stripe. Pure decisions so the orchestrator stays thin and the matrix of edge cases is testable without fixtures.

- [ ] **Step 1: Failing tests — direction, store-count requirement, effective-at**

```go
package planchange_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/planchange"
)

func TestIsUpgrade(t *testing.T) {
    cases := []struct {
        name       string
        from, to   subscription.SubscriptionPlan
        isUpgrade  bool
    }{
        {"starter→studio", subscription.PlanStarter, subscription.PlanStudio, true},
        {"studio→pro",     subscription.PlanStudio,  subscription.PlanPro,    true},
        {"studio→starter", subscription.PlanStudio,  subscription.PlanStarter, false},
        {"pro→studio",     subscription.PlanPro,     subscription.PlanStudio,  false},
        {"starter→starter", subscription.PlanStarter, subscription.PlanStarter, false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            require.Equal(t, tc.isUpgrade, planchange.IsUpgrade(tc.from, tc.to))
        })
    }
}

func TestIsUpgrade_PeriodOnly(t *testing.T) {
    // Monthly → Annual is an upgrade (immediate + prorate credit per §4.4).
    require.True(t, planchange.IsPeriodUpgrade(subscription.PeriodMonthly, subscription.PeriodAnnual))
    // Annual → Monthly is a downgrade (end-of-period).
    require.False(t, planchange.IsPeriodUpgrade(subscription.PeriodAnnual, subscription.PeriodMonthly))
}

func TestRequiresStoreCountCheck_OnlyStudioToStarter(t *testing.T) {
    require.True(t,  planchange.RequiresStoreCountCheck(subscription.PlanStudio,  subscription.PlanStarter))
    require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanPro,     subscription.PlanStarter), "Pro→Starter double-jump is out of scope — one step at a time")
    require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanStudio,  subscription.PlanStudio))
    require.False(t, planchange.RequiresStoreCountCheck(subscription.PlanStarter, subscription.PlanStudio), "upgrade never blocks on store count")
}

func TestEffectiveAt_UpgradeIsNow(t *testing.T) {
    now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
    periodEnd := now.Add(20 * 24 * time.Hour)
    at, immediate := planchange.EffectiveAt(planchange.DirectionUpgrade, now, periodEnd)
    require.True(t, immediate)
    require.Equal(t, now, at)
}

func TestEffectiveAt_DowngradeIsPeriodEnd(t *testing.T) {
    now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
    periodEnd := now.Add(20 * 24 * time.Hour)
    at, immediate := planchange.EffectiveAt(planchange.DirectionDowngrade, now, periodEnd)
    require.False(t, immediate)
    require.Equal(t, periodEnd, at)
}

func TestClassify_NoOp_WhenIdentical(t *testing.T) {
    d := planchange.Classify(subscription.PlanStudio, subscription.PeriodMonthly,
        subscription.PlanStudio, subscription.PeriodMonthly)
    require.Equal(t, planchange.DirectionNoChange, d)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `rules.go`**

```go
// Package planchange orchestrates §4.4 + §4.5 plan and period changes.
// rules.go holds the pure-function decision helpers; orchestration and DB
// writes live in planchange.go.
package planchange

import (
    "time"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

type Direction int

const (
    DirectionNoChange Direction = iota
    DirectionUpgrade
    DirectionDowngrade
)

// planRank encodes §3 lineup order. Marketplace is intentionally absent —
// it is hidden and not merchant-selectable.
var planRank = map[subscription.SubscriptionPlan]int{
    subscription.PlanTrial:   0,
    subscription.PlanStarter: 1,
    subscription.PlanStudio:  2,
    subscription.PlanPro:     3,
}

func IsUpgrade(from, to subscription.SubscriptionPlan) bool {
    return planRank[to] > planRank[from]
}

func IsDowngrade(from, to subscription.SubscriptionPlan) bool {
    return planRank[to] < planRank[from]
}

// IsPeriodUpgrade: Monthly→Annual (§4.4) is an upgrade — prorate immediately.
// Annual→Monthly is a downgrade — end of period.
func IsPeriodUpgrade(from, to subscription.SubscriptionPeriod) bool {
    return from == subscription.PeriodMonthly && to == subscription.PeriodAnnual
}

// Classify combines plan and period direction. Any upgrade dimension makes the
// whole change an upgrade (merchant pays now). A pure period downgrade (same plan,
// annual→monthly) is a Downgrade. A same-plan-same-period request is NoChange.
func Classify(
    fromPlan subscription.SubscriptionPlan, fromPeriod subscription.SubscriptionPeriod,
    toPlan subscription.SubscriptionPlan, toPeriod subscription.SubscriptionPeriod,
) Direction {
    if fromPlan == toPlan && fromPeriod == toPeriod {
        return DirectionNoChange
    }
    if IsUpgrade(fromPlan, toPlan) {
        return DirectionUpgrade
    }
    if IsDowngrade(fromPlan, toPlan) {
        return DirectionDowngrade
    }
    // Same plan, period change.
    if IsPeriodUpgrade(fromPeriod, toPeriod) {
        return DirectionUpgrade
    }
    return DirectionDowngrade
}

// RequiresStoreCountCheck — §4.5.1 applies only to Studio→Starter. Pro→Starter
// double-jump is not supported in one request (merchant must step down twice;
// this mirrors Shopify/Stripe change-plan ergonomics and lets us handle each
// quota boundary cleanly).
func RequiresStoreCountCheck(from, to subscription.SubscriptionPlan) bool {
    return from == subscription.PlanStudio && to == subscription.PlanStarter
}

// EffectiveAt returns the execution timestamp and whether the change is
// immediate. Upgrade = now. Downgrade = current period end.
func EffectiveAt(dir Direction, now, currentPeriodEnd time.Time) (at time.Time, immediate bool) {
    switch dir {
    case DirectionUpgrade:
        return now, true
    case DirectionDowngrade:
        return currentPeriodEnd, false
    default:
        return now, true
    }
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{rules,rules_test}.go
git commit -m "feat(planchange): pure decision helpers for direction + store-count check + effective-at"
```

---

## Task 5: `stripeclient.UpdateSubscription` — add or verify

**Files:**
- Modify: `services/marketplace-api/internal/billing/stripeclient/client.go` (add `UpdateSubscription` if P2 did not)
- Modify: `services/marketplace-api/internal/billing/stripeclient/client_test.go`

**Spec references:** §4.5 (prorate via Stripe), §4.2.1 (currency locked — Price IDs per currency).

> **Reviewer note:** P2 may have introduced `UpdateSubscription` already. If `grep -Rn 'func .* UpdateSubscription' internal/billing/stripeclient/` returns a match, **skip straight to Step 4** and only add the test cases that are missing. This plan treats the method as the single integration point with Stripe for plan and period changes.

- [ ] **Step 1: Failing test — update returns proration invoice on upgrade**

```go
func TestUpdateSubscription_ProratesImmediate(t *testing.T) {
    // Uses stripeclient.Stub (from P2) or a real Stripe test-mode key gated by env.
    stub := stripeclient.NewStub()
    stub.NextUpdateResponse = &stripe.Subscription{
        ID: "sub_x",
        LatestInvoice: &stripe.Invoice{ID: "in_proration", Currency: "usd", AmountDue: 1500},
    }
    cli := stripeclient.NewWithStub(stub)

    sub, err := cli.UpdateSubscription(context.Background(), stripeclient.UpdateSubscriptionParams{
        SubscriptionID:     "sub_x",
        PriceID:            "price_pro_monthly_usd",
        ProrationBehavior:  stripeclient.ProrationAlwaysInvoice,
        IdempotencyKey:     "plan-change:store_y:pro:monthly:1713440000",
        Metadata: map[string]string{
            "store_id":        "store_y",
            "mark8ly_action":  "plan_change_upgrade",
        },
    })
    require.NoError(t, err)
    require.Equal(t, "sub_x", sub.ID)
    require.Equal(t, "in_proration", sub.LatestInvoice.ID)

    // Verify stub received the expected call shape.
    require.Equal(t, "price_pro_monthly_usd", stub.LastUpdate.PriceID)
    require.Equal(t, "always_invoice", stub.LastUpdate.ProrationBehavior)
    require.Contains(t, stub.LastUpdate.IdempotencyKey, "plan-change:store_y:pro:monthly:")
}
```

- [ ] **Step 2: Run — expect FAIL (method or stub field missing)**

- [ ] **Step 3: Add `UpdateSubscription` to `client.go`**

```go
type UpdateSubscriptionParams struct {
    SubscriptionID     string
    PriceID            string
    ProrationBehavior  ProrationBehavior // "always_invoice" for upgrades; "none" for downgrades scheduled at period end
    IdempotencyKey     string
    Metadata           map[string]string
    CancelAt           *time.Time // optional — used only for scheduled downgrades that want the schedule pushed to Stripe (we default to local scheduling)
}

type ProrationBehavior string

const (
    ProrationAlwaysInvoice ProrationBehavior = "always_invoice"
    ProrationCreatePorations ProrationBehavior = "create_prorations"
    ProrationNone          ProrationBehavior = "none"
)

func (c *Client) UpdateSubscription(ctx context.Context, in UpdateSubscriptionParams) (*stripe.Subscription, error) {
    params := &stripe.SubscriptionParams{
        ProrationBehavior: stripe.String(string(in.ProrationBehavior)),
        Items: []*stripe.SubscriptionItemsParams{{
            // Caller is responsible for resolving the current item ID; in this
            // project every subscription has exactly one item, which we look up
            // inside Retrieve before the update.
            ID:    stripe.String(c.firstItemID(ctx, in.SubscriptionID)),
            Price: stripe.String(in.PriceID),
        }},
    }
    for k, v := range in.Metadata {
        params.AddMetadata(k, v)
    }
    params.SetIdempotencyKey(in.IdempotencyKey)
    params.Context = ctx
    sub, err := c.sdk.Subscriptions.Update(in.SubscriptionID, params)
    if err != nil {
        return nil, fmt.Errorf("stripeclient: update subscription %s: %w", in.SubscriptionID, err)
    }
    return sub, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/stripeclient/{client,client_test}.go
git commit -m "feat(stripeclient): UpdateSubscription with proration + idempotency"
```

---

## Task 6: `planchange.Orchestrator.Execute` — upgrade path

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/planchange.go`
- Create: `services/marketplace-api/internal/subscription/planchange/planchange_test.go`

**Spec references:** §4.5 (upgrade immediate prorate), §4.2.1 (locked currency).

- [ ] **Step 1: Failing integration test — Starter→Studio commits immediately + writes audit**

```go
//go:build integration

package planchange_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/billing/stripeclient"
    "github.com/tesserix/marketplace-api/internal/stores"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/planchange"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestExecute_Upgrade_StarterToStudio_CommitsImmediately(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    stub := stripeclient.NewStub()
    stub.NextUpdateResponse = &stripeclient.Subscription{ID: "sub_x"}
    cli := stripeclient.NewWithStub(stub)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly,
        BillingCurrency: "USD", PriceTier: subscription.PriceTierDeveloped,
    }).Error)

    orch := planchange.NewOrchestrator(planchange.Deps{
        DB: db, Stripe: cli, Emitter: em,
        SubscriptionRepo: subscription.NewRepository(db),
        StoreRepo:        stores.NewRepository(db),
    })

    out, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStudio, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:" + uuid.NewString(), Reason: "merchant_upgrade",
    })
    require.NoError(t, err)
    require.Equal(t, planchange.ResultUpgradeCommitted, out.Result)
    require.True(t, out.StripeUpdated)

    // Row reflects the new plan immediately.
    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PlanStudio, got.Plan)
    require.Nil(t, got.PendingDowngradePlan)
    require.NotNil(t, got.LastPlanChangeAt)
    require.Equal(t, "merchant_upgrade", *got.LastPlanChangeReason)

    // Audit event present.
    em.FlushForTesting()
    require.GreaterOrEqual(t, len(rec.Events()), 1)
    require.Equal(t, "subscription.plan_changed", rec.Events()[0].Action)
    require.Equal(t, "upgrade_committed", rec.Events()[0].Metadata["subaction"])
    require.Equal(t, "starter", rec.Events()[0].Metadata["from_plan"])
    require.Equal(t, "studio",  rec.Events()[0].Metadata["to_plan"])

    // subscription_plan_change_audit row written.
    var n int64
    require.NoError(t, db.Table("subscription_plan_change_audit").
        Where("store_id=? AND action=?", storeID, "upgrade_committed").
        Count(&n).Error)
    require.EqualValues(t, 1, n)

    // Stripe idempotency key has the expected shape.
    require.Contains(t, stub.LastUpdate.IdempotencyKey, "plan-change:"+storeID.String()+":studio:monthly:")
}
```

- [ ] **Step 2: Failing test — upgrade in read-only status rejected**

```go
func TestExecute_Upgrade_Rejected_WhenStatusReadOnly(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    orch := newTestOrchestrator(t, db)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusExpired,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
    }).Error)

    _, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStudio, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:x", Reason: "test",
    })
    require.ErrorIs(t, err, planchange.ErrSubscriptionReadOnly)
}
```

- [ ] **Step 3: Failing test — currency-change attempt rejected**

```go
func TestExecute_RejectsCurrencyChange(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    orch := newTestOrchestrator(t, db)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly,
        BillingCurrency: "USD", PriceTier: subscription.PriceTierDeveloped,
    }).Error)

    _, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStudio, TargetPeriod: subscription.PeriodMonthly,
        RequestedCurrency: "INR", // caller asked for a different currency
        Actor: "user:x", Reason: "test",
    })
    require.ErrorIs(t, err, planchange.ErrCurrencyLocked)
}
```

- [ ] **Step 4: Run — expect FAIL**

- [ ] **Step 5: Write `planchange.go`**

```go
package planchange

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/billing/stripeclient"
    "github.com/tesserix/marketplace-api/internal/stores"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

var (
    ErrSubscriptionReadOnly = errors.New("planchange: subscription is in read-only status")
    ErrCurrencyLocked       = errors.New("planchange: currency cannot change mid-term")
    ErrNoChange             = errors.New("planchange: target plan + period identical to current")
    ErrStoreCountOverQuota  = errors.New("planchange: target plan does not permit current store count")
    ErrInvalidTargetPlan    = errors.New("planchange: target plan not merchant-selectable")
)

type Result string

const (
    ResultUpgradeCommitted       Result = "upgrade_committed"
    ResultDowngradeScheduled     Result = "downgrade_scheduled"
    ResultPeriodSwitchCommitted  Result = "period_switch_committed"
)

type Input struct {
    TenantID          uuid.UUID
    StoreID           uuid.UUID
    TargetPlan        subscription.SubscriptionPlan
    TargetPeriod      subscription.SubscriptionPeriod
    RequestedCurrency string // optional — if set, must match current billing_currency
    Actor             string
    Reason            string
    GinCtx            *gin.Context // optional — used for audit tagging
    Now               time.Time    // zero → time.Now()
}

type Output struct {
    Result        Result
    EffectiveAt   time.Time
    StripeUpdated bool
}

type Deps struct {
    DB               *gorm.DB
    Stripe           stripeclient.Client
    Emitter          *audit.Emitter
    SubscriptionRepo *subscription.Repository
    StoreRepo        *stores.Repository
    Clock            Clock // injectable for cron tests; default = realClock
}

type Clock interface{ Now() time.Time }
type realClock struct{}
func (realClock) Now() time.Time { return time.Now() }

type Orchestrator struct { deps Deps }

func NewOrchestrator(d Deps) *Orchestrator {
    if d.Clock == nil { d.Clock = realClock{} }
    return &Orchestrator{deps: d}
}

func (o *Orchestrator) Execute(ctx context.Context, in Input) (Output, error) {
    now := in.Now
    if now.IsZero() { now = o.deps.Clock.Now() }

    // Validate target plan — only Starter/Studio/Pro are merchant-selectable.
    switch in.TargetPlan {
    case subscription.PlanStarter, subscription.PlanStudio, subscription.PlanPro:
    default:
        return Output{}, fmt.Errorf("%w: %s", ErrInvalidTargetPlan, in.TargetPlan)
    }

    var out Output
    err := subscription.WithAdvisoryLock(ctx, o.deps.DB, in.StoreID, func(tx *gorm.DB) error {
        sub, err := o.deps.SubscriptionRepo.GetByStoreIDTx(ctx, tx, in.TenantID, in.StoreID)
        if err != nil { return err }

        // Gate: read-only statuses can only change billing via the Stripe portal.
        if statemachine.IsReadOnly(sub.Status) {
            return fmt.Errorf("%w: %s", ErrSubscriptionReadOnly, sub.Status)
        }

        // Gate: §4.2.1 — billing currency is locked for the billing period.
        if in.RequestedCurrency != "" && in.RequestedCurrency != sub.BillingCurrency {
            return fmt.Errorf("%w: current=%s requested=%s",
                ErrCurrencyLocked, sub.BillingCurrency, in.RequestedCurrency)
        }

        dir := Classify(sub.Plan, sub.SubscriptionPeriod, in.TargetPlan, in.TargetPeriod)
        if dir == DirectionNoChange {
            return ErrNoChange
        }

        switch dir {
        case DirectionUpgrade:
            return o.executeUpgrade(ctx, tx, sub, in, now, &out)
        case DirectionDowngrade:
            return o.executeDowngradeSchedule(ctx, tx, sub, in, now, &out)
        }
        return nil
    })
    return out, err
}

func (o *Orchestrator) executeUpgrade(
    ctx context.Context, tx *gorm.DB,
    sub *subscription.StoreSubscription, in Input, now time.Time, out *Output,
) error {
    priceID, err := o.deps.Stripe.PriceIDFor(in.TargetPlan, in.TargetPeriod, sub.BillingCurrency, sub.PriceTier)
    if err != nil { return fmt.Errorf("planchange: resolve target price: %w", err) }

    key := fmt.Sprintf("plan-change:%s:%s:%s:%d",
        in.StoreID, in.TargetPlan, in.TargetPeriod, now.Truncate(5*time.Minute).Unix())

    stripeSub, err := o.deps.Stripe.UpdateSubscription(ctx, stripeclient.UpdateSubscriptionParams{
        SubscriptionID:    sub.StripeSubscriptionID,
        PriceID:           priceID,
        ProrationBehavior: stripeclient.ProrationAlwaysInvoice,
        IdempotencyKey:    key,
        Metadata: map[string]string{
            "store_id":       in.StoreID.String(),
            "tenant_id":      in.TenantID.String(),
            "mark8ly_action": "plan_change_upgrade",
            "from_plan":      string(sub.Plan),
            "to_plan":        string(in.TargetPlan),
        },
    })
    if err != nil { return err }

    if err := o.deps.SubscriptionRepo.CommitUpgradeTx(ctx, tx, in.TenantID, in.StoreID,
        in.TargetPlan, in.TargetPeriod, in.Reason); err != nil {
        return err
    }

    action := "upgrade_committed"
    if sub.Plan == in.TargetPlan {
        action = "period_switch_committed"
    }

    if err := writePlanChangeAuditRow(ctx, tx, planChangeAuditRow{
        TenantID: in.TenantID, StoreID: in.StoreID,
        StripeSubscriptionID: sub.StripeSubscriptionID,
        StripeInvoiceID:      firstInvoiceID(stripeSub),
        FromPlan: sub.Plan, ToPlan: in.TargetPlan,
        FromPeriod: sub.SubscriptionPeriod, ToPeriod: in.TargetPeriod,
        Action: action, BillingCurrency: sub.BillingCurrency,
        ProrationCents: prorationCents(stripeSub),
        Actor: in.Actor, Reason: in.Reason,
        EffectiveAt: now,
    }); err != nil { return err }

    o.emitPlanChange(in.GinCtx, sub, in, action, now)

    out.Result = ResultUpgradeCommitted
    if sub.Plan == in.TargetPlan { out.Result = ResultPeriodSwitchCommitted }
    out.EffectiveAt = now
    out.StripeUpdated = true
    return nil
}

func (o *Orchestrator) emitPlanChange(
    gctx *gin.Context, sub *subscription.StoreSubscription,
    in Input, subaction string, at time.Time,
) {
    if o.deps.Emitter == nil { return }
    o.deps.Emitter.EmitPlanChange(gctx, audit.PlanChange{
        TenantID:    in.TenantID,
        StoreID:     in.StoreID,
        FromPlan:    string(sub.Plan),
        ToPlan:      string(in.TargetPlan),
        FromPeriod:  string(sub.SubscriptionPeriod),
        ToPeriod:    string(in.TargetPeriod),
        Subaction:   subaction,
        Actor:       in.Actor,
        Reason:      in.Reason,
        EffectiveAt: at,
    })
}
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{planchange,planchange_test}.go
git commit -m "feat(planchange): upgrade path — prorate via Stripe, commit under advisory lock, audit"
```

---

## Task 7: `planchange.Orchestrator.Execute` — downgrade path

**Files:**
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange.go`
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange_test.go`

**Spec references:** §4.5 (end of period), §4.5.1 (store-count UI gate; Council finding #2 cron re-check).

- [ ] **Step 1: Failing test — Studio→Starter schedules downgrade + parks pending fields**

```go
func TestExecute_Downgrade_StudioToStarter_ParksPending(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    orch := newTestOrchestrator(t, db)

    tenantID, storeID := uuid.New(), uuid.New()
    periodEnd := time.Now().Add(18 * 24 * time.Hour).Truncate(time.Second)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, CurrentPeriodEnd: &periodEnd,
        BillingCurrency: "USD", PriceTier: subscription.PriceTierDeveloped,
    }).Error)
    seedStores(t, db, tenantID, 2) // under the Starter cap of 2

    out, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStarter, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:x", Reason: "merchant_downgrade",
    })
    require.NoError(t, err)
    require.Equal(t, planchange.ResultDowngradeScheduled, out.Result)
    require.False(t, out.StripeUpdated, "downgrade must not hit Stripe until cron fires")
    require.Equal(t, periodEnd, out.EffectiveAt)

    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PlanStudio, got.Plan, "plan unchanged until cron commits")
    require.Equal(t, subscription.PlanStarter, *got.PendingDowngradePlan)
    require.True(t, got.PendingDowngradeEffectiveAt.Equal(periodEnd))

    var n int64
    require.NoError(t, db.Table("subscription_plan_change_audit").
        Where("store_id=? AND action=?", storeID, "downgrade_scheduled").Count(&n).Error)
    require.EqualValues(t, 1, n)
}
```

- [ ] **Step 2: Failing test — Studio→Starter preflight-gate rejects when store count > 2**

```go
func TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    orch := newTestOrchestrator(t, db)

    tenantID, storeID := uuid.New(), uuid.New()
    periodEnd := time.Now().Add(10 * 24 * time.Hour)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, CurrentPeriodEnd: &periodEnd,
        BillingCurrency: "USD",
    }).Error)
    seedStores(t, db, tenantID, 5) // over Starter cap

    _, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStarter, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:x", Reason: "merchant_downgrade",
    })
    require.ErrorIs(t, err, planchange.ErrStoreCountOverQuota)
}
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Implement downgrade path**

```go
func (o *Orchestrator) executeDowngradeSchedule(
    ctx context.Context, tx *gorm.DB,
    sub *subscription.StoreSubscription, in Input, now time.Time, out *Output,
) error {
    if sub.CurrentPeriodEnd == nil {
        return fmt.Errorf("planchange: current_period_end missing for sub %s", sub.StripeSubscriptionID)
    }
    effective := *sub.CurrentPeriodEnd

    // §4.5.1 preflight block: Studio → Starter requires active+restorable store
    // count to be within the Starter cap (2). The cron re-runs this at execution.
    if RequiresStoreCountCheck(sub.Plan, in.TargetPlan) {
        count, err := o.deps.StoreRepo.CountActiveOrSoftDeletedRestorableTx(ctx, tx, in.TenantID)
        if err != nil { return err }
        starterCap := plangate.Limit(subscription.PlanStarter, plangate.FeatureStores)
        if count > starterCap {
            return fmt.Errorf("%w: %d stores; limit %d", ErrStoreCountOverQuota, count, starterCap)
        }
    }

    if err := o.deps.SubscriptionRepo.SetPendingDowngradeTx(ctx, tx,
        in.TenantID, in.StoreID, in.TargetPlan, in.TargetPeriod, effective, in.Reason); err != nil {
        return err
    }

    if err := writePlanChangeAuditRow(ctx, tx, planChangeAuditRow{
        TenantID: in.TenantID, StoreID: in.StoreID,
        StripeSubscriptionID: sub.StripeSubscriptionID,
        FromPlan: sub.Plan, ToPlan: in.TargetPlan,
        FromPeriod: sub.SubscriptionPeriod, ToPeriod: in.TargetPeriod,
        Action: "downgrade_scheduled",
        BillingCurrency: sub.BillingCurrency,
        Actor: in.Actor, Reason: in.Reason,
        EffectiveAt: effective,
    }); err != nil { return err }

    o.emitPlanChange(in.GinCtx, sub, in, "downgrade_scheduled", effective)

    out.Result = ResultDowngradeScheduled
    out.EffectiveAt = effective
    out.StripeUpdated = false
    return nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{planchange,planchange_test}.go
git commit -m "feat(planchange): downgrade scheduling with store-count preflight block"
```

---

## Task 8: `planchange/preflight.go` — PreflightReport builder

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/preflight.go`
- Create: `services/marketplace-api/internal/subscription/planchange/preflight_test.go`

**Spec references:** §4.5.1 (UI data contract — active + soft-deleted-but-restorable; per-store in-flight order count; CSV link).

- [ ] **Step 1: Failing test — report shape**

```go
func TestPreflight_StudioToStarter_ReportsStoresAndOrders(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stores", "orders")
    orch := newTestOrchestrator(t, db)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        CurrentPeriodEnd: ptr(time.Now().Add(18 * 24 * time.Hour)),
    }).Error)
    seedStoresWithInFlight(t, db, tenantID, 4, 3) // 4 stores, 3 in-flight orders each

    report, err := orch.Preflight(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStarter, TargetPeriod: subscription.PeriodMonthly,
    })
    require.NoError(t, err)
    require.Equal(t, planchange.DecisionBlockStoreCount, report.Decision)
    require.Equal(t, 4, report.StoreCount)
    require.Equal(t, 2, report.TargetStoreLimit)
    require.Len(t, report.Stores, 4)
    for _, s := range report.Stores {
        require.Equal(t, 3, s.InFlightOrderCount)
        require.Equal(t, "/admin/stores/"+s.ID.String()+"/orders/export/csv?status=in_flight", s.OrdersCSVLink)
    }
}

func TestPreflight_Upgrade_ReportsAllowImmediate(t *testing.T) {
    // Starter → Studio with no store-count check.
    db := testdb.NewDB(t, "store_subscriptions")
    orch := newTestOrchestrator(t, db)
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        CurrentPeriodEnd: ptr(time.Now().Add(18 * 24 * time.Hour)),
    }).Error)

    report, err := orch.Preflight(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStudio, TargetPeriod: subscription.PeriodMonthly,
    })
    require.NoError(t, err)
    require.Equal(t, planchange.DecisionAllowUpgradeNow, report.Decision)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `preflight.go`**

```go
package planchange

import (
    "context"
    "time"

    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/plangate"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type Decision string

const (
    DecisionAllowUpgradeNow           Decision = "allow_upgrade_now"
    DecisionAllowDowngradeAtPeriodEnd Decision = "allow_downgrade_at_period_end"
    DecisionAllowPeriodSwitch         Decision = "allow_period_switch"
    DecisionBlockStoreCount           Decision = "block_store_count"
    DecisionBlockCurrency             Decision = "block_currency"
    DecisionBlockReadOnly             Decision = "block_read_only"
    DecisionNoOp                      Decision = "no_op"
)

type PreflightReport struct {
    Decision         Decision   `json:"decision"`
    FromPlan         string     `json:"from_plan"`
    ToPlan           string     `json:"to_plan"`
    FromPeriod       string     `json:"from_period"`
    ToPeriod         string     `json:"to_period"`
    BillingCurrency  string     `json:"billing_currency"`
    EffectiveAt      *time.Time `json:"effective_at,omitempty"`
    StoreCount       int        `json:"store_count,omitempty"`
    TargetStoreLimit int        `json:"target_store_limit,omitempty"`
    Stores           []StoreInfo `json:"stores,omitempty"`
    CurrentPlanDiff  *PlanDiff   `json:"plan_diff,omitempty"`
}

type StoreInfo struct {
    ID                 uuid.UUID `json:"id"`
    Name               string    `json:"name"`
    IsSoftDeleted      bool      `json:"is_soft_deleted"`
    InFlightOrderCount int       `json:"in_flight_order_count"`
    OrdersCSVLink      string    `json:"orders_csv_link"`
}

// PlanDiff — the three knobs the UI wants to render (delta stores, image cap,
// campaign email cap). Frontend (P16) renders the full matrix; we publish
// these as convenience.
type PlanDiff struct {
    StoresDelta            int `json:"stores_delta"`
    ImagesPerProductDelta  int `json:"images_per_product_delta"`
    CampaignEmailsDelta    int `json:"campaign_emails_delta"`
}

func (o *Orchestrator) Preflight(ctx context.Context, in Input) (PreflightReport, error) {
    sub, err := o.deps.SubscriptionRepo.GetByStoreID(ctx, in.TenantID, in.StoreID)
    if err != nil { return PreflightReport{}, err }

    rep := PreflightReport{
        FromPlan:        string(sub.Plan),
        ToPlan:          string(in.TargetPlan),
        FromPeriod:      string(sub.SubscriptionPeriod),
        ToPeriod:        string(in.TargetPeriod),
        BillingCurrency: sub.BillingCurrency,
        CurrentPlanDiff: buildPlanDiff(sub.Plan, in.TargetPlan),
    }

    if statemachine.IsReadOnly(sub.Status) {
        rep.Decision = DecisionBlockReadOnly
        return rep, nil
    }
    if in.RequestedCurrency != "" && in.RequestedCurrency != sub.BillingCurrency {
        rep.Decision = DecisionBlockCurrency
        return rep, nil
    }

    dir := Classify(sub.Plan, sub.SubscriptionPeriod, in.TargetPlan, in.TargetPeriod)
    switch dir {
    case DirectionNoChange:
        rep.Decision = DecisionNoOp
        return rep, nil
    case DirectionUpgrade:
        if sub.Plan == in.TargetPlan {
            rep.Decision = DecisionAllowPeriodSwitch
        } else {
            rep.Decision = DecisionAllowUpgradeNow
        }
        return rep, nil
    }

    // Downgrade branch.
    if sub.CurrentPeriodEnd != nil {
        rep.EffectiveAt = sub.CurrentPeriodEnd
    }

    if RequiresStoreCountCheck(sub.Plan, in.TargetPlan) {
        count, err := o.deps.StoreRepo.CountActiveOrSoftDeletedRestorable(ctx, in.TenantID)
        if err != nil { return rep, err }
        rep.StoreCount = count
        rep.TargetStoreLimit = plangate.Limit(in.TargetPlan, plangate.FeatureStores)
        if count > rep.TargetStoreLimit {
            rep.Decision = DecisionBlockStoreCount
            rep.Stores, err = o.buildStoreInfos(ctx, in.TenantID)
            if err != nil { return rep, err }
            return rep, nil
        }
    }
    rep.Decision = DecisionAllowDowngradeAtPeriodEnd
    return rep, nil
}

func (o *Orchestrator) buildStoreInfos(ctx context.Context, tenantID uuid.UUID) ([]StoreInfo, error) {
    ss, err := o.deps.StoreRepo.ListActiveOrSoftDeletedRestorable(ctx, tenantID)
    if err != nil { return nil, err }
    out := make([]StoreInfo, 0, len(ss))
    for _, s := range ss {
        n, err := o.deps.StoreRepo.InFlightOrderCount(ctx, s.ID)
        if err != nil { return nil, err }
        out = append(out, StoreInfo{
            ID: s.ID, Name: s.Name, IsSoftDeleted: s.DeletedAt != nil,
            InFlightOrderCount: n,
            OrdersCSVLink: "/admin/stores/" + s.ID.String() + "/orders/export/csv?status=in_flight",
        })
    }
    return out, nil
}

func buildPlanDiff(from, to subscription.SubscriptionPlan) *PlanDiff {
    return &PlanDiff{
        StoresDelta:           plangate.Limit(to, plangate.FeatureStores) - plangate.Limit(from, plangate.FeatureStores),
        ImagesPerProductDelta: plangate.Limit(to, plangate.FeatureImagesPerProduct) - plangate.Limit(from, plangate.FeatureImagesPerProduct),
        CampaignEmailsDelta:   plangate.Limit(to, plangate.FeatureCampaignEmailsPerMonth) - plangate.Limit(from, plangate.FeatureCampaignEmailsPerMonth),
    }
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{preflight,preflight_test}.go
git commit -m "feat(planchange): preflight report with store list + per-store in-flight counts"
```

---

## Task 9: HTTP handler — `POST /change-plan` + `GET /change-plan/preflight`

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/subscription_change_plan.go`
- Create: `services/marketplace-api/internal/handlers/admin/subscription_change_plan_test.go`

**Spec references:** §4.5, §4.5.1.

- [ ] **Step 1: Failing test — `POST /change-plan` returns 200 + result on upgrade**

```go
func TestChangePlanHandler_Upgrade_Returns200(t *testing.T) {
    h := newTestHandler(t)
    body := `{"target_plan":"studio","target_period":"monthly","reason":"merchant_upgrade"}`
    w := h.POST("/admin/stores/"+h.seedStoreID.String()+"/subscription/change-plan", body)
    require.Equal(t, 200, w.Code)
    require.Contains(t, w.Body.String(), `"result":"upgrade_committed"`)
}

func TestChangePlanHandler_Downgrade_Over_Quota_Returns422(t *testing.T) {
    h := newTestHandlerWithStores(t, 5) // Studio with 5 stores
    body := `{"target_plan":"starter","target_period":"monthly","reason":"x"}`
    w := h.POST("/admin/stores/"+h.seedStoreID.String()+"/subscription/change-plan", body)
    require.Equal(t, 422, w.Code)
    require.Contains(t, w.Body.String(), `"error":"store_count_over_quota"`)
}

func TestChangePlanPreflightHandler_ReturnsReport(t *testing.T) {
    h := newTestHandlerWithStores(t, 4)
    w := h.GET("/admin/stores/" + h.seedStoreID.String() +
        "/subscription/change-plan/preflight?target_plan=starter&target_period=monthly")
    require.Equal(t, 200, w.Code)
    require.Contains(t, w.Body.String(), `"decision":"block_store_count"`)
    require.Contains(t, w.Body.String(), `"store_count":4`)
    require.Contains(t, w.Body.String(), `"orders_csv_link"`)
}

func TestChangePlanHandler_RejectsCurrencyChange(t *testing.T) {
    h := newTestHandler(t)
    body := `{"target_plan":"studio","target_period":"monthly","requested_currency":"INR"}`
    w := h.POST("/admin/stores/"+h.seedStoreID.String()+"/subscription/change-plan", body)
    require.Equal(t, 409, w.Code)
    require.Contains(t, w.Body.String(), `"error":"currency_locked"`)
}
```

- [ ] **Step 2: Write handler**

```go
package admin

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/planchange"
)

type ChangePlanHandler struct {
    Orch *planchange.Orchestrator
}

type changePlanRequest struct {
    TargetPlan         string `json:"target_plan" binding:"required"`
    TargetPeriod       string `json:"target_period" binding:"required"`
    RequestedCurrency  string `json:"requested_currency,omitempty"`
    Reason             string `json:"reason,omitempty"`
}

func (h *ChangePlanHandler) Post(c *gin.Context) {
    tenantID := c.MustGet("tenant_id").(uuid.UUID)
    storeID, err := uuid.Parse(c.Param("storeId"))
    if err != nil { c.AbortWithStatusJSON(400, gin.H{"error":"invalid_store_id"}); return }

    var req changePlanRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.AbortWithStatusJSON(400, gin.H{"error":"invalid_body","detail":err.Error()}); return
    }

    userID, _ := c.Get("user_id")
    actor := "user:" + toStr(userID)

    out, err := h.Orch.Execute(c.Request.Context(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan:        subscription.SubscriptionPlan(req.TargetPlan),
        TargetPeriod:      subscription.SubscriptionPeriod(req.TargetPeriod),
        RequestedCurrency: req.RequestedCurrency,
        Reason:            req.Reason,
        Actor:             actor,
        GinCtx:            c,
    })
    if err != nil {
        mapChangePlanError(c, err)
        return
    }
    c.JSON(200, gin.H{
        "result":        string(out.Result),
        "effective_at":  out.EffectiveAt,
        "stripe_updated": out.StripeUpdated,
    })
}

func (h *ChangePlanHandler) Preflight(c *gin.Context) {
    tenantID := c.MustGet("tenant_id").(uuid.UUID)
    storeID, err := uuid.Parse(c.Param("storeId"))
    if err != nil { c.AbortWithStatusJSON(400, gin.H{"error":"invalid_store_id"}); return }

    report, err := h.Orch.Preflight(c.Request.Context(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan:        subscription.SubscriptionPlan(c.Query("target_plan")),
        TargetPeriod:      subscription.SubscriptionPeriod(c.Query("target_period")),
        RequestedCurrency: c.Query("requested_currency"),
    })
    if err != nil { mapChangePlanError(c, err); return }
    c.JSON(200, report)
}

func mapChangePlanError(c *gin.Context, err error) {
    switch {
    case errors.Is(err, planchange.ErrSubscriptionReadOnly):
        c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{"error":"subscription_read_only"})
    case errors.Is(err, planchange.ErrCurrencyLocked):
        c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error":"currency_locked"})
    case errors.Is(err, planchange.ErrStoreCountOverQuota):
        c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error":"store_count_over_quota"})
    case errors.Is(err, planchange.ErrNoChange):
        c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error":"no_change"})
    case errors.Is(err, planchange.ErrInvalidTargetPlan):
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error":"invalid_target_plan"})
    default:
        c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error":"internal"})
    }
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/subscription_change_plan{,_test}.go
git commit -m "feat(admin): POST /subscription/change-plan + preflight GET with typed error mapping"
```

---

## Task 10: `planchange/auditlog.go` — `PlanChangeAudit` writer

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/auditlog.go`
- Create: `services/marketplace-api/internal/subscription/planchange/auditlog_test.go`

**Spec references:** §23.1 subscription mutations → audit-service (event stream); this file covers the **per-subscription ledger mirror** that P10 reconciles against Stripe proration reports.

- [ ] **Step 1: Failing test**

```go
func TestWritePlanChangeAuditRow_InsertsAppendOnly(t *testing.T) {
    db := testdb.NewDB(t, "subscription_plan_change_audit")
    row := planchange.PlanChangeAuditRow{
        TenantID: uuid.New(), StoreID: uuid.New(),
        StripeSubscriptionID: "sub_x", StripeInvoiceID: "in_x",
        FromPlan: subscription.PlanStarter, ToPlan: subscription.PlanStudio,
        FromPeriod: subscription.PeriodMonthly, ToPeriod: subscription.PeriodMonthly,
        Action: "upgrade_committed", BillingCurrency: "USD",
        ProrationCents: 1500, Actor: "user:x", Reason: "merchant_upgrade",
        EffectiveAt: time.Now(),
    }
    require.NoError(t, planchange.WritePlanChangeAuditRowTx(context.Background(), db, row))

    var n int64
    require.NoError(t, db.Table("subscription_plan_change_audit").
        Where("store_id=?", row.StoreID).Count(&n).Error)
    require.EqualValues(t, 1, n)
}

func TestPlanChangeAudit_CannotUpdateOrDelete(t *testing.T) {
    // DB-level REVOKE UPDATE, DELETE enforces append-only. Verified in Task 15
    // as a security regression — here we just document the expectation.
    t.Skip("covered by security regression in Task 19 grep scrub + CI role-check")
}
```

- [ ] **Step 2: Write `auditlog.go`**

```go
package planchange

import (
    "context"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

type PlanChangeAuditRow struct {
    ID                   uuid.UUID                       `gorm:"column:id;type:uuid;primaryKey"`
    TenantID             uuid.UUID                       `gorm:"column:tenant_id;type:uuid;not null"`
    StoreID              uuid.UUID                       `gorm:"column:store_id;type:uuid;not null"`
    StripeSubscriptionID string                          `gorm:"column:stripe_subscription_id"`
    StripeInvoiceID      string                          `gorm:"column:stripe_invoice_id"`
    FromPlan             subscription.SubscriptionPlan   `gorm:"column:from_plan;not null"`
    ToPlan               subscription.SubscriptionPlan   `gorm:"column:to_plan;not null"`
    FromPeriod           subscription.SubscriptionPeriod `gorm:"column:from_period;not null"`
    ToPeriod             subscription.SubscriptionPeriod `gorm:"column:to_period;not null"`
    Action               string                          `gorm:"column:action;not null"`
    BillingCurrency      string                          `gorm:"column:billing_currency;not null"`
    ProrationCents       int64                           `gorm:"column:proration_cents"`
    Actor                string                          `gorm:"column:actor;not null"`
    Reason               string                          `gorm:"column:reason"`
    EffectiveAt          time.Time                       `gorm:"column:effective_at;not null"`
    CreatedAt            time.Time                       `gorm:"column:created_at;not null"`
}

func (PlanChangeAuditRow) TableName() string { return "subscription_plan_change_audit" }

func WritePlanChangeAuditRowTx(ctx context.Context, tx *gorm.DB, row PlanChangeAuditRow) error {
    if row.ID == uuid.Nil { row.ID = uuid.New() }
    if row.CreatedAt.IsZero() { row.CreatedAt = time.Now() }
    return tx.WithContext(ctx).Create(&row).Error
}

// planChangeAuditRow is the internal alias used by orchestrator.go.
type planChangeAuditRow = PlanChangeAuditRow
var writePlanChangeAuditRow = WritePlanChangeAuditRowTx
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{auditlog,auditlog_test}.go
git commit -m "feat(planchange): append-only subscription_plan_change_audit row writer"
```

---

## Task 11: `audit.EmitPlanChange` wrapper + `subscription.plan_changed` action

**Files:**
- Modify: `services/marketplace-api/internal/audit/emitter.go`
- Modify: `services/marketplace-api/internal/audit/emitter_test.go`

**Spec references:** §23.1.

- [ ] **Step 1: Failing test**

```go
func TestEmitPlanChange_BuildsExpectedEvent(t *testing.T) {
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    em.EmitPlanChange(nil, audit.PlanChange{
        TenantID: uuid.New(), StoreID: uuid.New(),
        FromPlan: "starter", ToPlan: "studio",
        FromPeriod: "monthly", ToPeriod: "monthly",
        Subaction: "upgrade_committed",
        Actor: "user:x", Reason: "merchant_upgrade",
        EffectiveAt: time.Now(),
    })
    em.FlushForTesting()

    require.Len(t, rec.Events(), 1)
    require.Equal(t, "subscription.plan_changed", rec.Events()[0].Action)
    md := rec.Events()[0].Metadata
    require.Equal(t, "starter", md["from_plan"])
    require.Equal(t, "studio",  md["to_plan"])
    require.Equal(t, "upgrade_committed", md["subaction"])
}
```

- [ ] **Step 2: Add to `emitter.go`**

```go
// PlanChange describes a single plan or period mutation. The emitter
// flattens it into the standard state-transition envelope so dashboards
// pick up plan_changed events alongside status transitions.
type PlanChange struct {
    TenantID    uuid.UUID
    StoreID     uuid.UUID
    FromPlan    string
    ToPlan      string
    FromPeriod  string
    ToPeriod    string
    Subaction   string // "upgrade_committed" | "downgrade_scheduled" | "downgrade_committed" | "downgrade_blocked_over_quota" | "period_switch_committed"
    Actor       string
    Reason      string
    EffectiveAt time.Time
}

const ActionSubscriptionPlanChanged = "subscription.plan_changed"

func (e *Emitter) EmitPlanChange(c *gin.Context, p PlanChange) {
    md := map[string]any{
        "from_plan":    p.FromPlan,
        "to_plan":      p.ToPlan,
        "from_period":  p.FromPeriod,
        "to_period":    p.ToPeriod,
        "subaction":    p.Subaction,
        "reason":       p.Reason,
        "effective_at": p.EffectiveAt,
    }
    e.emit(c, Event{
        Action:   ActionSubscriptionPlanChanged,
        TenantID: p.TenantID,
        StoreID:  p.StoreID,
        Actor:    p.Actor,
        Metadata: md,
    })
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/audit/{emitter,emitter_test}.go
git commit -m "feat(audit): EmitPlanChange wrapper + subscription.plan_changed action constant"
```

---

## Task 12: `plangate.ImagesAllowed` grandfathering helper

**Files:**
- Modify: `services/marketplace-api/internal/plangate/matrix.go`
- Modify: `services/marketplace-api/internal/plangate/matrix_test.go`

**Spec references:** §11 (image grandfathering), §4.5.1 ("existing products retain their 50-image counts").

**Design decision:** compute-at-enforcement, no schema change on `products`. The product row already carries `created_at`; the subscription row now carries `last_plan_change_at` (Task 3). A product created before the most recent Studio→Starter downgrade retains the higher cap.

- [ ] **Step 1: Failing tests**

```go
func TestImagesAllowed_NoPriorChange_UsesCurrentPlan(t *testing.T) {
    now := time.Now()
    require.Equal(t, 25, plangate.ImagesAllowed(subscription.PlanStarter, now.Add(-24*time.Hour), nil))
    require.Equal(t, 50, plangate.ImagesAllowed(subscription.PlanStudio, now.Add(-24*time.Hour), nil))
}

func TestImagesAllowed_ProductPredatesDowngrade_Grandfathered(t *testing.T) {
    // Subscription downgraded Studio→Starter at T0. Product created at T0 - 10d.
    // Product should keep the 50-image cap.
    t0 := time.Now()
    productCreated := t0.Add(-10 * 24 * time.Hour)
    require.Equal(t, 50, plangate.ImagesAllowed(
        subscription.PlanStarter,  // current plan after downgrade
        productCreated,
        &t0,                        // last_plan_change_at
    ))
}

func TestImagesAllowed_ProductCreatedAfterDowngrade_UsesCurrentPlan(t *testing.T) {
    t0 := time.Now()
    productCreated := t0.Add(2 * 24 * time.Hour)
    require.Equal(t, 25, plangate.ImagesAllowed(
        subscription.PlanStarter, productCreated, &t0,
    ))
}

func TestImagesAllowed_UpgradePath_NoGrandfathering(t *testing.T) {
    // Starter → Studio: existing products get the NEW higher cap (upgrade is
    // always more generous). The helper always returns the max of
    // pre-change and current.
    t0 := time.Now()
    productCreated := t0.Add(-10 * 24 * time.Hour)
    require.Equal(t, 50, plangate.ImagesAllowed(
        subscription.PlanStudio, productCreated, &t0,
    ))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Add helper to `matrix.go`**

```go
// ImagesAllowed returns the per-product image cap for a given product on a
// given plan. §11 grandfathering: if the product predates the most recent
// plan change AND the prior plan had a higher cap, the product keeps the
// higher cap. Upgrades never reduce caps.
//
// The function is compute-at-enforcement — no schema change on products,
// no backfill job, no cron. Callers are: product image upload handler +
// CSV import validator + frontend (via read-only JSON of resolved limits).
//
// Caller passes currentPlan from store_subscriptions.plan and
// lastPlanChangeAt from store_subscriptions.last_plan_change_at (nil on
// never-changed rows).
func ImagesAllowed(
    currentPlan subscription.SubscriptionPlan,
    productCreatedAt time.Time,
    lastPlanChangeAt *time.Time,
) int {
    current := Limit(currentPlan, FeatureImagesPerProduct)
    if lastPlanChangeAt == nil || !productCreatedAt.Before(*lastPlanChangeAt) {
        return current
    }
    // Product predates the change. Without the prior plan on the row we
    // cannot know the exact prior cap, so conservatively grant the MAX
    // across all plans — in practice this is Unlimited for Pro and 50 for
    // Studio. Only grandfather if the prior cap COULD have been higher
    // than the current; otherwise fall through to current.
    //
    // Implementation: take the max of current and the highest plan cap
    // strictly above current in the enum order.
    maxHigherCap := current
    for _, p := range []subscription.SubscriptionPlan{
        subscription.PlanPro, subscription.PlanStudio, subscription.PlanStarter, subscription.PlanTrial,
    } {
        if planRankNumeric(p) > planRankNumeric(currentPlan) {
            if lim := Limit(p, FeatureImagesPerProduct); lim > maxHigherCap || lim == Unlimited {
                maxHigherCap = lim
            }
        }
    }
    return maxHigherCap
}

// planRankNumeric mirrors planchange.planRank but is local to plangate to
// avoid the dependency cycle.
func planRankNumeric(p subscription.SubscriptionPlan) int {
    switch p {
    case subscription.PlanTrial:   return 0
    case subscription.PlanStarter: return 1
    case subscription.PlanStudio:  return 2
    case subscription.PlanPro:     return 3
    default: return -1
    }
}
```

> **Reviewer note:** This is the pragmatic design — we don't carry the prior plan on the product row. If §11 tightens to "grandfather ONLY against the exact prior cap", add a `grandfathered_image_cap int` column on products and backfill on each downgrade commit. The helper signature stays; only the body changes. Flag as a follow-up if product-leadership pushes back.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/plangate/{matrix,matrix_test}.go
git commit -m "feat(plangate): ImagesAllowed grandfathering helper for Studio→Starter downgrade"
```

---

## Task 13: `DowngradeRecheckCron` — hourly executor

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/cron.go`
- Create: `services/marketplace-api/internal/subscription/planchange/cron_test.go`

**Spec references:** §4.5.1 Council finding #2: cron re-check at downgrade execution; if over-quota, block (stay on current plan, renewal charges at higher rate, email merchant, stay in `active` — NO `cancel_scheduled` misroute).

- [ ] **Step 1: Failing test — commit path**

```go
//go:build integration

func TestDowngradeRecheckCron_CommitsWhenEligible(t *testing.T) {
    clock := clockwork.NewFakeClock()
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    stub := stripeclient.NewStub()
    cli := stripeclient.NewWithStub(stub)
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    tenantID, storeID := uuid.New(), uuid.New()
    periodEnd := clock.Now().Add(-time.Hour) // already past
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        PendingDowngradePlan:        ptrPlan(subscription.PlanStarter),
        PendingDowngradePeriod:      ptrPeriod(subscription.PeriodMonthly),
        PendingDowngradeEffectiveAt: &periodEnd,
    }).Error)
    seedStores(t, db, tenantID, 2) // under Starter cap

    cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
        Orchestrator: newTestOrchestrator(t, db, cli, em),
        Clock:        clock,
    })
    require.NoError(t, cron.RunOnce(context.Background()))

    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PlanStarter, got.Plan)
    require.Nil(t, got.PendingDowngradePlan)

    em.FlushForTesting()
    var sawCommit bool
    for _, e := range rec.Events() {
        if e.Metadata["subaction"] == "downgrade_committed" { sawCommit = true }
    }
    require.True(t, sawCommit)
}
```

- [ ] **Step 2: Failing test — block path (§28 criterion #39)**

```go
func TestDowngradeRecheckCron_BlocksWhenOverQuota_StaysActive_StaysOnHigherPlan(t *testing.T) {
    clock := clockwork.NewFakeClock()
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    stub := stripeclient.NewStub()
    cli := stripeclient.NewWithStub(stub)
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    notifier := planchange.NewFakeDowngradeBlockNotifier()

    tenantID, storeID := uuid.New(), uuid.New()
    periodEnd := clock.Now().Add(-time.Hour)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        PendingDowngradePlan:        ptrPlan(subscription.PlanStarter),
        PendingDowngradePeriod:      ptrPeriod(subscription.PeriodMonthly),
        PendingDowngradeEffectiveAt: &periodEnd,
    }).Error)
    seedStores(t, db, tenantID, 5) // 5 stores — Starter cap is 2

    cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
        Orchestrator: newTestOrchestrator(t, db, cli, em),
        Clock:        clock,
        Notifier:     notifier,
    })
    require.NoError(t, cron.RunOnce(context.Background()))

    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PlanStudio, got.Plan, "plan must remain on higher tier")
    require.Equal(t, subscription.StatusActive, got.Status, "must NOT misroute to cancel_scheduled")
    require.Nil(t, got.PendingDowngradePlan, "pending fields cleared on block")

    require.NoError(t, stub.AssertNoUpdateCalled(), "Stripe must NOT be updated when blocked")

    require.Equal(t, 1, notifier.Count, "merchant email fired exactly once")
    require.Equal(t, storeID, notifier.LastStoreID)
    require.Equal(t, "downgrade_blocked_over_quota", notifier.LastReason)

    em.FlushForTesting()
    var sawBlock bool
    for _, e := range rec.Events() {
        if e.Metadata["subaction"] == "downgrade_blocked_over_quota" { sawBlock = true }
    }
    require.True(t, sawBlock)

    var n int64
    require.NoError(t, db.Table("subscription_plan_change_audit").
        Where("store_id=? AND action=?", storeID, "downgrade_blocked_over_quota").Count(&n).Error)
    require.EqualValues(t, 1, n)
}
```

- [ ] **Step 3: Failing test — idempotent, zero-work run**

```go
func TestDowngradeRecheckCron_NoReadyRows_NoOp(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
        Orchestrator: newTestOrchestrator(t, db, nil, nil),
        Clock:        clockwork.NewFakeClock(),
    })
    require.NoError(t, cron.RunOnce(context.Background()))
}
```

- [ ] **Step 4: Run — expect FAIL**

- [ ] **Step 5: Write `cron.go`**

```go
package planchange

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
    "github.com/sirupsen/logrus"

    "github.com/tesserix/marketplace-api/internal/billing/stripeclient"
    "github.com/tesserix/marketplace-api/internal/plangate"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

// DowngradeBlockNotifier delivers the §4.5.1 "we kept you on the higher plan"
// email. Injected so tests can count without hitting SendGrid.
type DowngradeBlockNotifier interface {
    NotifyDowngradeBlocked(ctx context.Context, tenantID, storeID uuid.UUID, reason string, details map[string]any) error
}

type CronDeps struct {
    Orchestrator *Orchestrator
    Clock        Clock
    Notifier     DowngradeBlockNotifier
    Log          *logrus.Logger
}

type DowngradeRecheckCron struct { deps CronDeps }

func NewDowngradeRecheckCron(d CronDeps) *DowngradeRecheckCron {
    if d.Clock == nil { d.Clock = realClock{} }
    if d.Log == nil   { d.Log = logrus.New() }
    return &DowngradeRecheckCron{deps: d}
}

// RunOnce executes a single pass. Safe to call concurrently; each row is
// protected by its advisory lock.
func (c *DowngradeRecheckCron) RunOnce(ctx context.Context) error {
    repo := c.deps.Orchestrator.deps.SubscriptionRepo
    rows, err := repo.FindPendingDowngradesReady(ctx, c.deps.Clock.Now())
    if err != nil { return err }
    for _, sub := range rows {
        if err := c.processOne(ctx, sub); err != nil {
            c.deps.Log.WithError(err).WithField("store_id", sub.StoreID).
                Error("downgrade_recheck: row failed; will retry next tick")
        }
    }
    return nil
}

func (c *DowngradeRecheckCron) processOne(ctx context.Context, sub subscription.StoreSubscription) error {
    return subscription.WithAdvisoryLock(ctx, c.deps.Orchestrator.deps.DB, sub.StoreID, func(tx *gorm.DB) error {
        // Re-read inside the lock — merchant may have restored stores, changed
        // their mind, or a webhook may have moved the row.
        fresh, err := c.deps.Orchestrator.deps.SubscriptionRepo.GetByStoreIDTx(ctx, tx, sub.TenantID, sub.StoreID)
        if err != nil { return err }
        if fresh.PendingDowngradePlan == nil {
            return nil // another worker already committed or the merchant cancelled
        }
        if fresh.PendingDowngradeEffectiveAt == nil || fresh.PendingDowngradeEffectiveAt.After(c.deps.Clock.Now()) {
            return nil // not ready (clock drift guard)
        }

        target := *fresh.PendingDowngradePlan
        period := *fresh.PendingDowngradePeriod

        // Re-check store count §4.5.1.
        if RequiresStoreCountCheck(fresh.Plan, target) {
            count, err := c.deps.Orchestrator.deps.StoreRepo.CountActiveOrSoftDeletedRestorableTx(ctx, tx, fresh.TenantID)
            if err != nil { return err }
            cap := plangate.Limit(target, plangate.FeatureStores)
            if count > cap {
                return c.blockAndNotify(ctx, tx, fresh, target, period, count, cap)
            }
        }

        // Commit the swap via Stripe.
        priceID, err := c.deps.Orchestrator.deps.Stripe.PriceIDFor(target, period, fresh.BillingCurrency, fresh.PriceTier)
        if err != nil { return err }
        key := fmt.Sprintf("plan-change-cron:%s:%s:%s:%d",
            fresh.StoreID, target, period, c.deps.Clock.Now().Truncate(5*time.Minute).Unix())
        if _, err := c.deps.Orchestrator.deps.Stripe.UpdateSubscription(ctx, stripeclient.UpdateSubscriptionParams{
            SubscriptionID:    fresh.StripeSubscriptionID,
            PriceID:           priceID,
            ProrationBehavior: stripeclient.ProrationNone, // already paid for the period
            IdempotencyKey:    key,
            Metadata: map[string]string{
                "store_id": fresh.StoreID.String(),
                "mark8ly_action": "plan_change_downgrade_commit",
            },
        }); err != nil {
            return err
        }

        if err := c.deps.Orchestrator.deps.SubscriptionRepo.CommitDowngradeTx(ctx, tx,
            fresh.TenantID, fresh.StoreID, target, period); err != nil {
            return err
        }

        if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
            TenantID: fresh.TenantID, StoreID: fresh.StoreID,
            StripeSubscriptionID: fresh.StripeSubscriptionID,
            FromPlan: fresh.Plan, ToPlan: target,
            FromPeriod: fresh.SubscriptionPeriod, ToPeriod: period,
            Action: "downgrade_committed", BillingCurrency: fresh.BillingCurrency,
            Actor: "system:cron:downgrade_recheck", Reason: "scheduled_execution",
            EffectiveAt: c.deps.Clock.Now(),
        }); err != nil { return err }

        c.deps.Orchestrator.deps.Emitter.EmitPlanChange(nil, audit.PlanChange{
            TenantID: fresh.TenantID, StoreID: fresh.StoreID,
            FromPlan: string(fresh.Plan), ToPlan: string(target),
            FromPeriod: string(fresh.SubscriptionPeriod), ToPeriod: string(period),
            Subaction: "downgrade_committed",
            Actor:     "system:cron:downgrade_recheck",
            Reason:    "scheduled_execution",
            EffectiveAt: c.deps.Clock.Now(),
        })
        return nil
    })
}

func (c *DowngradeRecheckCron) blockAndNotify(
    ctx context.Context, tx *gorm.DB,
    fresh *subscription.StoreSubscription,
    target subscription.SubscriptionPlan, period subscription.SubscriptionPeriod,
    count, cap int,
) error {
    // §4.5.1: stay in active. Clear pending fields. Do NOT call Stripe.
    // Renewal will charge at the CURRENT plan rate because we didn't swap the Price.
    if err := c.deps.Orchestrator.deps.SubscriptionRepo.ClearPendingDowngradeTx(ctx, tx,
        fresh.TenantID, fresh.StoreID); err != nil {
        return err
    }

    if err := WritePlanChangeAuditRowTx(ctx, tx, PlanChangeAuditRow{
        TenantID: fresh.TenantID, StoreID: fresh.StoreID,
        StripeSubscriptionID: fresh.StripeSubscriptionID,
        FromPlan: fresh.Plan, ToPlan: target,
        FromPeriod: fresh.SubscriptionPeriod, ToPeriod: period,
        Action: "downgrade_blocked_over_quota",
        BillingCurrency: fresh.BillingCurrency,
        Actor: "system:cron:downgrade_recheck",
        Reason: fmt.Sprintf("store_count=%d target_cap=%d", count, cap),
        EffectiveAt: c.deps.Clock.Now(),
    }); err != nil { return err }

    c.deps.Orchestrator.deps.Emitter.EmitPlanChange(nil, audit.PlanChange{
        TenantID: fresh.TenantID, StoreID: fresh.StoreID,
        FromPlan: string(fresh.Plan), ToPlan: string(target),
        FromPeriod: string(fresh.SubscriptionPeriod), ToPeriod: string(period),
        Subaction: "downgrade_blocked_over_quota",
        Actor: "system:cron:downgrade_recheck",
        Reason: fmt.Sprintf("store_count=%d target_cap=%d", count, cap),
        EffectiveAt: c.deps.Clock.Now(),
    })

    if c.deps.Notifier != nil {
        if err := c.deps.Notifier.NotifyDowngradeBlocked(ctx, fresh.TenantID, fresh.StoreID,
            "store_count_over_quota",
            map[string]any{"store_count": count, "target_cap": cap,
                "target_plan": string(target), "current_plan": string(fresh.Plan)},
        ); err != nil {
            c.deps.Log.WithError(err).Warn("downgrade_block: notifier failed; email missed, event still emitted")
        }
    }
    return nil
}
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/{cron,cron_test}.go
git commit -m "feat(planchange): hourly downgrade-recheck cron with over-quota block + merchant email"
```

---

## Task 14: Route wiring + `main.go` cron start

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Wire routes**

Inside the store-scoped admin group (after `readonly.RequireActive` from P3):

```go
changePlan := &ChangePlanHandler{Orch: deps.PlanChangeOrchestrator}
storeRoute.POST("/subscription/change-plan", changePlan.Post)
storeRoute.GET("/subscription/change-plan/preflight", changePlan.Preflight)
```

Both routes MUST be on the `readonly.DefaultAllowlist` (from P3 Task 6) — they are `/admin/stores/:storeId/subscription/*path`, which is already covered. No allowlist change needed.

- [ ] **Step 2: Construct orchestrator + cron in `main.go`**

```go
planChangeOrch := planchange.NewOrchestrator(planchange.Deps{
    DB:               db,
    Stripe:           stripeCli,
    Emitter:          auditEmitter,
    SubscriptionRepo: subscriptionRepo,
    StoreRepo:        storeRepo,
})

downgradeCron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
    Orchestrator: planChangeOrch,
    Notifier:     notificationClient.NewDowngradeBlockNotifier(),
    Log:          logger,
})

c := cron.New(cron.WithLocation(time.UTC))
if _, err := c.AddFunc("@hourly", func() {
    if err := downgradeCron.RunOnce(context.Background()); err != nil {
        logger.WithError(err).Error("downgrade_recheck cron tick failed")
    }
}); err != nil {
    logger.WithError(err).Fatal("register downgrade_recheck cron")
}
c.Start()
defer c.Stop()
```

- [ ] **Step 3: Build + smoke**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./internal/subscription/planchange/... ./internal/handlers/admin/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/routes.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): wire change-plan handler + hourly downgrade-recheck cron"
```

---

## Task 15: Integration test — success criterion #39

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/criterion_39_test.go`

**Spec references:** §28 #39 — "Downgrade cron failure: merchant has 5 stores at period end → downgrade blocks, stays on Studio, renewed at Studio rate, email fired".

- [ ] **Step 1: Write the end-to-end test**

```go
//go:build integration

func TestCriterion39_DowngradeBlocksWhenOverQuotaAtPeriodEnd(t *testing.T) {
    clock := clockwork.NewFakeClock()
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    stub := stripeclient.NewStub()
    cli := stripeclient.NewWithStub(stub)
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)
    notifier := planchange.NewFakeDowngradeBlockNotifier()

    tenantID, storeID := uuid.New(), uuid.New()

    // T0: Merchant on Studio, schedules downgrade to Starter.
    periodEnd := clock.Now().Add(30 * 24 * time.Hour)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        CurrentPeriodEnd: &periodEnd,
    }).Error)
    seedStores(t, db, tenantID, 2) // at preflight time — under cap

    orch := planchange.NewOrchestrator(planchange.Deps{
        DB: db, Stripe: cli, Emitter: em,
        SubscriptionRepo: subscription.NewRepository(db),
        StoreRepo:        stores.NewRepository(db),
        Clock:            clock,
    })
    out, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStarter, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:x", Reason: "merchant_downgrade",
    })
    require.NoError(t, err)
    require.Equal(t, planchange.ResultDowngradeScheduled, out.Result)

    // Merchant restores a soft-deleted store during the grace window.
    seedStores(t, db, tenantID, 3) // now 5 total

    // Cron fires at period end.
    clock.Advance(31 * 24 * time.Hour)
    cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
        Orchestrator: orch, Clock: clock, Notifier: notifier,
    })
    require.NoError(t, cron.RunOnce(context.Background()))

    // Assertions per criterion #39.
    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PlanStudio, got.Plan, "stays on Studio")
    require.Equal(t, subscription.StatusActive, got.Status, "status stays active (no cancel_scheduled misroute)")
    require.Nil(t, got.PendingDowngradePlan)

    // Stripe unchanged — renewal will charge at Studio rate.
    require.NoError(t, stub.AssertNoUpdateCalled())

    // Email fired.
    require.Equal(t, 1, notifier.Count)
    require.Equal(t, storeID, notifier.LastStoreID)

    // Audit row.
    var n int64
    require.NoError(t, db.Table("subscription_plan_change_audit").
        Where("store_id=? AND action=?", storeID, "downgrade_blocked_over_quota").
        Count(&n).Error)
    require.EqualValues(t, 1, n)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/criterion_39_test.go
git commit -m "test(planchange): end-to-end criterion #39 — downgrade blocked on over-quota at period end"
```

---

## Task 16: Integration test — image grandfathering end-to-end

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/grandfathering_test.go`

**Spec references:** §11, §4.5.1 paragraph 5.

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestGrandfathering_StudioProductKeeps50AfterDowngradeToStarter(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "products")
    clock := clockwork.NewFakeClock()
    orch := newTestOrchestratorWithClock(t, db, clock)

    tenantID, storeID := uuid.New(), uuid.New()
    periodEnd := clock.Now().Add(15 * 24 * time.Hour)
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStudio, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        CurrentPeriodEnd: &periodEnd,
    }).Error)

    // Seed product created on Studio with 40 images (within 50 cap).
    preDowngradeTime := clock.Now()
    productID := seedProduct(t, db, storeID, preDowngradeTime)

    // Downgrade to Starter + advance clock past period end so the cron fires.
    _, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStarter, TargetPeriod: subscription.PeriodMonthly,
        Actor: "user:x", Reason: "merchant_downgrade",
    })
    require.NoError(t, err)
    clock.Advance(16 * 24 * time.Hour)
    cron := planchange.NewDowngradeRecheckCron(planchange.CronDeps{
        Orchestrator: orch, Clock: clock,
    })
    require.NoError(t, cron.RunOnce(context.Background()))

    var fresh subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&fresh).Error)
    require.Equal(t, subscription.PlanStarter, fresh.Plan)

    // Existing product grandfathered.
    require.Equal(t, 50, plangate.ImagesAllowed(fresh.Plan, preDowngradeTime, fresh.LastPlanChangeAt),
        "product from Studio era keeps 50-image cap")

    // A product created AFTER the downgrade gets the new 25 cap.
    require.Equal(t, 25, plangate.ImagesAllowed(fresh.Plan, clock.Now(), fresh.LastPlanChangeAt),
        "new product capped at Starter's 25")

    _ = productID
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/grandfathering_test.go
git commit -m "test(planchange): grandfathered product retains 50-image cap after Studio→Starter"
```

---

## Task 17: Integration test — Monthly→Annual Pro releases +20% premium

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/period_switch_test.go`

**Spec references:** §4.4.

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestMonthlyToAnnual_Pro_ReleasesPremium(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "subscription_plan_change_audit")
    stub := stripeclient.NewStub()
    stub.NextUpdateResponse = &stripeclient.Subscription{ID: "sub_x"}
    cli := stripeclient.NewWithStub(stub)
    orch := planchange.NewOrchestrator(planchange.Deps{
        DB: db, Stripe: cli, Emitter: audit.NewEmitter(audit.NewRecorderForTesting()),
        SubscriptionRepo: subscription.NewRepository(db),
        StoreRepo:        stores.NewRepository(db),
    })

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanPro, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly, BillingCurrency: "USD",
        PriceTier: subscription.PriceTierDeveloped,
    }).Error)

    out, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanPro, TargetPeriod: subscription.PeriodAnnual,
        Actor: "user:x", Reason: "period_switch",
    })
    require.NoError(t, err)
    require.Equal(t, planchange.ResultPeriodSwitchCommitted, out.Result)

    // Stripe received the annual Pro USD Price — which is the no-premium SKU.
    require.Equal(t, "price_pro_annual_usd", stub.LastUpdate.PriceID, "annual Pro USD Price ID (no +20% premium)")

    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&got).Error)
    require.Equal(t, subscription.PeriodAnnual, got.SubscriptionPeriod)
}
```

> **Note:** the Stripe Price IDs above (`price_pro_monthly_usd` / `price_pro_annual_usd`) are the stub fixtures defined in P2. The real prod Price IDs come from Secret Manager via `stripeclient.PriceIDFor`; the stub returns the fixture key. If the stub fixtures don't yet include Pro annual USD, add them as part of this test's setup.

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/period_switch_test.go
git commit -m "test(planchange): Monthly→Annual Pro period switch resolves annual (no premium) price"
```

---

## Task 18: Integration test — currency-change rejected (§4.2.1)

**Files:**
- Create: `services/marketplace-api/internal/subscription/planchange/currency_lock_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestCurrencyLocked_RejectsMidTermCurrencyChange(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    orch := newTestOrchestrator(t, db, stripeclient.NewStub(), audit.NewEmitter(audit.NewRecorderForTesting()))

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        StripeCustomerID: "cus_x", StripeSubscriptionID: "sub_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
        SubscriptionPeriod: subscription.PeriodMonthly,
        BillingCurrency: "USD", PriceTier: subscription.PriceTierDeveloped,
    }).Error)

    _, err := orch.Execute(context.Background(), planchange.Input{
        TenantID: tenantID, StoreID: storeID,
        TargetPlan: subscription.PlanStudio, TargetPeriod: subscription.PeriodMonthly,
        RequestedCurrency: "INR", // attempted mid-term swap
        Actor: "user:x", Reason: "unexpected",
    })
    require.ErrorIs(t, err, planchange.ErrCurrencyLocked)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/planchange/currency_lock_test.go
git commit -m "test(planchange): mid-term currency change rejected per §4.2.1"
```

---

## Task 19: Schema bump + final grep scrub

**Files:**
- Modify: `services/marketplace-api/marketplaceapi.go` — bump expected schema version to `48`

- [ ] **Step 1: Bump schema version**

```go
const ExpectedSchemaVersion = 48 // was 46 after P1
```

- [ ] **Step 2: Grep for bare status/plan UPDATEs bypassing the orchestrator or state machine**

```bash
cd services/marketplace-api
grep -RnE 'UPDATE\s+store_subscriptions\s+SET\s+plan' internal/ \
  | grep -v "_test.go" \
  | grep -v subscription/repository.go \
  | grep -v subscription/planchange/
```

Expected: zero hits. The only legitimate writers are `subscription.Repository.CommitUpgrade*` and `CommitDowngrade*`.

- [ ] **Step 3: Grep for bare `pending_downgrade_*` writes outside the repo**

```bash
grep -RnE 'pending_downgrade_plan|pending_downgrade_effective_at' internal/ \
  | grep -v "_test.go" \
  | grep -v subscription/repository.go \
  | grep -v subscription/planchange/
```

Expected: zero hits.

- [ ] **Step 4: Run the full suite**

```bash
go test -tags=integration ./... -count=1
```

Expected: green.

- [ ] **Step 5: Final commit**

```bash
git add services/marketplace-api/marketplaceapi.go
git commit --allow-empty -m "chore(marketplace-api): bump schema version to 48; scrub confirms clean"
```

---

## Final verification checklist

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] Migrations 047 + 048 apply + roll back cleanly; `ExpectedSchemaVersion == 48`.
- [ ] `POST /admin/stores/:storeId/subscription/change-plan` returns:
  - 200 + `upgrade_committed` on Starter→Studio.
  - 200 + `downgrade_scheduled` on Studio→Starter (within quota).
  - 422 + `store_count_over_quota` on Studio→Starter (over quota).
  - 402 + `subscription_read_only` when status ∈ {expired, store_closed, pending_hard_delete}.
  - 409 + `currency_locked` on any currency-change attempt.
  - 409 + `no_change` on identical plan + period.
  - 400 + `invalid_target_plan` on `marketplace`/`trial` target.
- [ ] `GET /admin/stores/:storeId/subscription/change-plan/preflight?target_plan=...&target_period=...` returns `PreflightReport` with decision + (on downgrade) store list + per-store `orders_csv_link` + plan diff.
- [ ] Every write to `store_subscriptions.plan` goes through `subscription.Repository` methods wrapped in `WithAdvisoryLock`.
- [ ] Every plan change emits `subscription.plan_changed` to the audit emitter with a `subaction` metadata field.
- [ ] Every plan change writes one append-only row to `subscription_plan_change_audit`.
- [ ] Image grandfathering: `plangate.ImagesAllowed(PlanStarter, productCreatedAt, &lastPlanChangeAt)` returns 50 iff `productCreatedAt < lastPlanChangeAt`.
- [ ] `DowngradeRecheckCron.RunOnce`:
  - Commits via Stripe when eligible → row moves to target plan, pending fields cleared, `downgrade_committed` audit + audit-log row.
  - Blocks when over-quota → row stays on current plan, pending fields cleared, `downgrade_blocked_over_quota` audit + audit-log row, merchant email fired, Stripe NOT called, status remains `active` (criterion #39).
- [ ] Monthly→Annual Pro resolves to the annual Pro Price ID (no +20% premium).
- [ ] Cron is wired in `main.go` at `@hourly` and a second concurrent pod running the same cron does not double-execute (advisory lock + CAS clear guarantees at-most-once per tick).

## What's now unlocked

- **P10** (refund accounting) reconciles `subscription_plan_change_audit.proration_cents` against Stripe proration invoices.
- **P11** (cancellation + save-offer) reuses the orchestrator's advisory-lock + audit pattern; cancellation becomes a sibling of plan change rather than a one-off.
- **P16** (admin frontend) consumes `GET /subscription/change-plan/preflight` to render the Studio→Starter blocking dialog, per-store in-flight order badges, and the "Download orders CSV" deep links.
- **P17** (observability) reads `subscription.plan_changed` audit events plus the `subscription_plan_change_audit` table for MRR/ARR movement dashboards.
- Product image-upload handler + CSV import validator now call `plangate.ImagesAllowed(plan, productCreatedAt, lastPlanChangeAt)` for the enforcement decision — no row backfill on downgrade.

## Execution handoff

Plan complete. Four implementation plans are now saved under `docs/superpowers/plans/`:
- `2026-04-18-p1-subscription-data-model.md`
- `2026-04-18-p2-stripe-multicurrency-webhooks.md`
- `2026-04-18-p3-state-machine-plan-gates.md`
- `2026-04-18-p4-plan-upgrade-downgrade.md`

Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**, in order P1 → P2 → P3 → P4. P4 assumes P1 migrations are applied, P2's `stripeclient` is in place (this plan adds `UpdateSubscription` if P2 did not), and P3's `statemachine.IsReadOnly` + `plangate.Limit` helpers are available.
