# P1 — Subscription Data Model + Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lay the complete database + Go-type foundation for the v2.3 subscription model (migrations, struct/enum extensions, the `GetByStoreID` tenant_id fix, an advisory-lock helper, and a reusable audit-emit scaffold for state transitions) so that P2 (Stripe multi-currency + webhooks) and P3 (state machine + gates) can be built on stable primitives.

**Architecture:** Eight additive migrations extend `store_subscriptions` with v2.3 columns and add six new tables (`stripe_webhook_events`, `subscription_arbitrage_audit`, `business_entity_attestations`, `billing_archive`, `campaign_email_budget`, `white_label_app_lifecycle`). Plan-enum values are renamed in place (no new column). `SubscriptionStatus` gains the v2.3 states. Repository methods grow a mandatory `tenantID` parameter and are audited at every call site. A `subscription.WithAdvisoryLock(ctx, tx, storeID, fn)` helper wraps `pg_advisory_xact_lock(hashtext(store_id))` for reuse across P3/P4. `audit.EmitStateTransition` gives state-machine code one uniform call site. None of this changes runtime behaviour on its own — it only unblocks P2/P3.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15 (CNPG), golang-migrate v4, `github.com/stretchr/testify`, `pgcrypto` (already enabled by existing migrations), existing `internal/audit` + `internal/authz` packages.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §2 (infra audit), §17.1–17.4 (states + concurrency), §17.7 (webhook events), §18.2 + §18.8 + §19.3.1 (security), §23.2 (billing archive), §10.1 (email budget), §13.5 (app lifecycle).

**Related plans (NOT in scope here):**
- **P2** — Stripe multi-currency + webhook core (consumes `stripe_webhook_events`, multi-currency columns)
- **P3** — State machine + plan gates + read-only middleware (consumes expanded enum + advisory-lock helper + audit scaffold)
- Tax ID fields already present on the subscription row are **populated** by P7; here we only add the columns.

---

## Scope Check

- Every migration in this plan is **additive** (ADD COLUMN / CREATE TABLE). No existing column is dropped or re-typed. Existing production rows keep working with NULL/default values.
- **Plan-enum rename** is the only destructive-ish step (values `free`/`enterprise` → `trial`/`pro`). A `CASE` UPDATE handles in-place. Marketplace stays hidden.
- **Status-enum expansion** only adds values; never removes.
- No Stripe work, no webhook dispatcher, no cron jobs, no UI. Those are P2/P3/later.

---

## File Structure

### Migrations (create new)

- `services/marketplace-api/migrations/000038_subscription_v2_columns.up.sql` — add 10 columns to `store_subscriptions` + backfill + indexes
- `services/marketplace-api/migrations/000038_subscription_v2_columns.down.sql`
- `services/marketplace-api/migrations/000039_subscription_plan_v2_rename.up.sql` — rename plan values + update CHECK constraint
- `services/marketplace-api/migrations/000039_subscription_plan_v2_rename.down.sql`
- `services/marketplace-api/migrations/000040_subscription_status_v2_expand.up.sql` — extend status CHECK with v2.3 values
- `services/marketplace-api/migrations/000040_subscription_status_v2_expand.down.sql`
- `services/marketplace-api/migrations/000041_stripe_webhook_events.up.sql`
- `services/marketplace-api/migrations/000041_stripe_webhook_events.down.sql`
- `services/marketplace-api/migrations/000042_subscription_arbitrage_audit.up.sql`
- `services/marketplace-api/migrations/000042_subscription_arbitrage_audit.down.sql`
- `services/marketplace-api/migrations/000043_business_entity_attestations.up.sql` — with UPDATE-blocking trigger AND `REVOKE DELETE`
- `services/marketplace-api/migrations/000043_business_entity_attestations.down.sql`
- `services/marketplace-api/migrations/000044_billing_archive.up.sql`
- `services/marketplace-api/migrations/000044_billing_archive.down.sql`
- `services/marketplace-api/migrations/000045_campaign_email_budget.up.sql`
- `services/marketplace-api/migrations/000045_campaign_email_budget.down.sql`
- `services/marketplace-api/migrations/000046_white_label_app_lifecycle.up.sql`
- `services/marketplace-api/migrations/000046_white_label_app_lifecycle.down.sql`

### Go types & packages (create)

- `services/marketplace-api/internal/subscription/app_lifecycle.go` — `WhiteLabelAppStatus` enum + struct
- `services/marketplace-api/internal/subscription/advisory_lock.go` — `WithAdvisoryLock` helper
- `services/marketplace-api/internal/subscription/advisory_lock_test.go`
- `services/marketplace-api/internal/billingarchive/models.go` — `BillingArchive` GORM model
- `services/marketplace-api/internal/campaignbudget/models.go` — `CampaignEmailBudget` GORM model
- `services/marketplace-api/internal/arbitrage/models.go` — `SubscriptionArbitrageAudit` GORM model
- `services/marketplace-api/internal/attestation/models.go` — `BusinessEntityAttestation` GORM model
- `services/marketplace-api/internal/webhookevents/models.go` — `StripeWebhookEvent` GORM model

### Go types & packages (modify)

- `services/marketplace-api/internal/subscription/models.go` — add 10 fields to `StoreSubscription`, expand `SubscriptionPlan` + `SubscriptionStatus` enums, add `TaxIDNameMatch` type
- `services/marketplace-api/internal/subscription/repository.go` — add `tenantID uuid.UUID` parameter to `GetByStoreID`, `Create`, `Update`; add new `GetByStripeCustomerID` stays tenant-agnostic but is locked down via Stripe binding (documented)
- `services/marketplace-api/internal/subscription/service.go` — thread tenant ID through every repo call, update `GetSubscription` signature to require tenant context
- `services/marketplace-api/internal/handlers/admin/subscription.go` — pass tenant_id into service calls (already extracted from Gin context; just thread it through)
- `services/marketplace-api/internal/plangate/gate.go` — update `PlanResolver.Resolve` to pass tenant ID (read from Gin context before calling)
- `services/marketplace-api/internal/audit/emitter.go` — add `EmitStateTransition` convenience helper (doesn't change existing API)

### Tests (create)

- `services/marketplace-api/internal/subscription/repository_test.go` — integration, verifies `GetByStoreID` refuses to return another tenant's subscription
- `services/marketplace-api/internal/subscription/advisory_lock_test.go` — integration, verifies two concurrent locks serialize
- `services/marketplace-api/internal/attestation/models_test.go` — security: verifies DELETE + UPDATE rejected (Success criterion 50)
- `services/marketplace-api/internal/arbitrage/models_test.go` — verifies `ip_country` + `ip_hash` columns present; no raw IP column
- `services/marketplace-api/internal/webhookevents/models_test.go` — verifies PK on `event_id` + `ON CONFLICT DO NOTHING` INSERT pattern

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migration 038 — subscription v2 columns | — |
| 2 | Migration 039 — plan enum rename | 1 |
| 3 | Migration 040 — status enum expand | 1 |
| 4 | Expand Go `SubscriptionPlan` + `SubscriptionStatus` + `StoreSubscription` | 1–3 |
| 5 | Migration 041 — `stripe_webhook_events` | 4 |
| 6 | Migration 042 — `subscription_arbitrage_audit` | 4 |
| 7 | Migration 043 — `business_entity_attestations` (trigger + REVOKE DELETE) | 4 |
| 8 | Migration 044 — `billing_archive` | 4 |
| 9 | Migration 045 — `campaign_email_budget` | 4 |
| 10 | Migration 046 — `white_label_app_lifecycle` | 4 |
| 11 | Go models for all 6 new tables | 5–10 |
| 12 | Fix `GetByStoreID` tenant_id hole + call-site audit | 4 |
| 13 | Advisory-lock helper `subscription.WithAdvisoryLock` | 12 |
| 14 | Audit-emit `EmitStateTransition` scaffold | 4 |
| 15 | Security regression test — attestation DELETE rejected | 7, 11 |
| 16 | Schema-version bump + CI verification | all |

Each task is one atomic commit boundary.

---

## Reusable patterns referenced in this plan

**A. Migration authoring** — Every migration is a pair (`.up.sql` + `.down.sql`). Register via `iofs.New()` already in `pkg/migrate/migrate.go`. Expected schema version is a const in `services/marketplace-api/marketplaceapi.go` — bump to `46` in Task 16.

**B. Advisory lock pattern (existing in codebase)** — `internal/ticket/repository.go:191-197` shows the canonical form:
```go
if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, storeID.String()).Error; err != nil {
    return fmt.Errorf("advisory lock: %w", err)
}
```
Task 13 generalises this into `subscription.WithAdvisoryLock`.

**C. Audit emit pattern (existing)** — `internal/audit/emitter.go:94` — `emitter.Emit(c *gin.Context, Event{...})` is fire-and-forget async. Task 14 adds a typed wrapper.

**D. Integration test pattern (existing)** — `//go:build integration` + `pkg/testdb/testdb.go` — use `testdb.NewDB(t, "store_subscriptions")` for tests that commit, `testdb.NewTx(t)` for rollback-safe tests. Requires `TEST_DATABASE_URL` env var.

**E. GORM model pattern** — column tags `gorm:"column:foo;type:varchar(30);not null"`, `TableName() string` method on the struct, `uuid.UUID` from `github.com/google/uuid`.

---

## Task 1: Migration 038 — subscription v2 columns

**Files:**
- Create: `services/marketplace-api/migrations/000038_subscription_v2_columns.up.sql`
- Create: `services/marketplace-api/migrations/000038_subscription_v2_columns.down.sql`

**Spec references:** §2 (infra audit table), §4.2.1 (`billing_currency` binding), §4.1.1 (`price_tier`), §3.4 (`has_white_label_app_add_on`), §18.8 (`arbitrage_flag`), §13.5 (`app_lifecycle_status`), §19 (tax ID columns), §19.3 name match.

- [ ] **Step 1: Write the up migration**

```sql
-- 000038_subscription_v2_columns.up.sql
-- v2.3 subscription model: add tax ID, multi-currency, PPP tier, arbitrage, app lifecycle columns.
-- All columns nullable with sensible defaults so existing rows remain valid.

ALTER TABLE store_subscriptions
    ADD COLUMN reverse_charge_tax_id     VARCHAR(50),
    ADD COLUMN tax_id_country            CHAR(2),
    ADD COLUMN tax_id_validated          BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN tax_id_validated_at       TIMESTAMPTZ,
    ADD COLUMN tax_id_name_match         VARCHAR(20)  NOT NULL DEFAULT 'not_checked'
        CHECK (tax_id_name_match IN ('matched', 'unmatched', 'not_checked')),
    ADD COLUMN billing_currency          CHAR(3),
    ADD COLUMN price_tier                VARCHAR(20)  NOT NULL DEFAULT 'developed'
        CHECK (price_tier IN ('developed', 'ppp')),
    ADD COLUMN has_white_label_app_add_on BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN arbitrage_flag            BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN app_lifecycle_status      VARCHAR(30);

-- tenant_id was NOT indexed before (per exploration); add it now for safe per-tenant scans.
CREATE INDEX IF NOT EXISTS ss_tenant_idx        ON store_subscriptions (tenant_id);
CREATE INDEX IF NOT EXISTS ss_billing_currency_idx ON store_subscriptions (billing_currency) WHERE billing_currency IS NOT NULL;
CREATE INDEX IF NOT EXISTS ss_arbitrage_idx     ON store_subscriptions (arbitrage_flag) WHERE arbitrage_flag = true;
CREATE INDEX IF NOT EXISTS ss_tax_validated_idx ON store_subscriptions (tax_id_validated, tax_id_country);
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000038_subscription_v2_columns.down.sql
DROP INDEX IF EXISTS ss_tax_validated_idx;
DROP INDEX IF EXISTS ss_arbitrage_idx;
DROP INDEX IF EXISTS ss_billing_currency_idx;
DROP INDEX IF EXISTS ss_tenant_idx;

ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS app_lifecycle_status,
    DROP COLUMN IF EXISTS arbitrage_flag,
    DROP COLUMN IF EXISTS has_white_label_app_add_on,
    DROP COLUMN IF EXISTS price_tier,
    DROP COLUMN IF EXISTS billing_currency,
    DROP COLUMN IF EXISTS tax_id_name_match,
    DROP COLUMN IF EXISTS tax_id_validated_at,
    DROP COLUMN IF EXISTS tax_id_validated,
    DROP COLUMN IF EXISTS tax_id_country,
    DROP COLUMN IF EXISTS reverse_charge_tax_id;
```

- [ ] **Step 3: Apply migration against a throwaway DB and verify**

Assuming `TEST_DATABASE_URL` points at the same CNPG test database used for integration tests:

Run:
```bash
cd services/marketplace-api
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
```
Expected: `migrate 038: store_subscriptions columns added`

Verify column list:
```bash
psql "$TEST_DATABASE_URL" -c "\d store_subscriptions" | grep -E "(reverse_charge|billing_currency|price_tier|arbitrage_flag)"
```
Expected: all four names appear.

- [ ] **Step 4: Verify down migration restores original shape**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" down 1
psql "$TEST_DATABASE_URL" -c "\d store_subscriptions" | grep -c billing_currency
```
Expected output: `0`

Re-apply up:
```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/migrations/000038_subscription_v2_columns.*.sql
git commit -m "feat: add v2.3 subscription columns (tax, currency, arbitrage, lifecycle)"
```

---

## Task 2: Migration 039 — plan enum rename

**Files:**
- Create: `services/marketplace-api/migrations/000039_subscription_plan_v2_rename.up.sql`
- Create: `services/marketplace-api/migrations/000039_subscription_plan_v2_rename.down.sql`

**Spec references:** §3 (plan lineup) — Trial, Starter, Studio, Pro, Marketplace (hidden). Old enum was free/starter/pro/enterprise/marketplace.

**Decision:** Rename in place rather than add new column — the column is `VARCHAR(30)`, no native enum type. `free → trial`, `enterprise → pro` (old pro merges UP into new pro since new Pro is the single higher tier; starter stays). Marketplace stays. New value `studio` added.

> **Reviewer note:** This mapping is a one-time migration of existing rows. There are no production rows today (per session state), so the mapping is effectively cosmetic — but we still write it correctly because staging may carry data.

- [ ] **Step 1: Write up migration**

```sql
-- 000039_subscription_plan_v2_rename.up.sql
-- Remap plan values to v2.3 lineup: trial | starter | studio | pro | marketplace.
-- Old: free | starter | pro | enterprise | marketplace.

UPDATE store_subscriptions
SET plan = CASE plan
    WHEN 'free'       THEN 'trial'
    WHEN 'enterprise' THEN 'pro'
    WHEN 'pro'        THEN 'pro'        -- legacy single-tier pro merges into new pro
    WHEN 'starter'    THEN 'starter'
    WHEN 'marketplace' THEN 'marketplace'
    ELSE plan
END
WHERE plan IN ('free', 'enterprise', 'pro', 'starter', 'marketplace');

-- Change default from 'free' to 'trial' for newly-inserted rows.
ALTER TABLE store_subscriptions
    ALTER COLUMN plan SET DEFAULT 'trial';

-- Add CHECK constraint enumerating the v2.3 values (there was none before).
ALTER TABLE store_subscriptions
    ADD CONSTRAINT store_subscriptions_plan_check
        CHECK (plan IN ('trial', 'starter', 'studio', 'pro', 'marketplace'));
```

- [ ] **Step 2: Write down migration**

```sql
-- 000039_subscription_plan_v2_rename.down.sql
ALTER TABLE store_subscriptions DROP CONSTRAINT IF EXISTS store_subscriptions_plan_check;

UPDATE store_subscriptions
SET plan = CASE plan
    WHEN 'trial'  THEN 'free'
    WHEN 'studio' THEN 'pro'  -- studio has no pre-v2.3 equivalent; collapse to pro
    ELSE plan
END
WHERE plan IN ('trial', 'studio');

ALTER TABLE store_subscriptions ALTER COLUMN plan SET DEFAULT 'free';
```

- [ ] **Step 3: Apply + verify**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "SELECT DISTINCT plan FROM store_subscriptions;"
```
Expected: only values from `{trial, starter, studio, pro, marketplace}`.

Try to insert invalid plan:
```bash
psql "$TEST_DATABASE_URL" -c "INSERT INTO store_subscriptions (tenant_id, store_id, stripe_customer_id, plan) VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000002','cus_x','free');"
```
Expected: ERROR mentioning `store_subscriptions_plan_check`.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000039_subscription_plan_v2_rename.*.sql
git commit -m "feat: rename subscription plan enum to v2.3 (trial/starter/studio/pro/marketplace)"
```

---

## Task 3: Migration 040 — status enum expansion

**Files:**
- Create: `services/marketplace-api/migrations/000040_subscription_status_v2_expand.up.sql`
- Create: `services/marketplace-api/migrations/000040_subscription_status_v2_expand.down.sql`

**Spec references:** §17.1 — new states: `signup`, `payment_action_required`, `cancel_scheduled`, `expired`, `store_closed`, `pending_hard_delete`, `hard_deleted`. Old values: `active`, `trialing`, `past_due`, `cancelled`, `incomplete`.

**Decision:** Remap `cancelled → expired`, drop `incomplete` (unused; no current rows with it — verify before migrating); keep `active`, `trialing`, `past_due`.

- [ ] **Step 1: Verify no rows use `incomplete` in current DB**

```bash
psql "$TEST_DATABASE_URL" -c "SELECT status, count(*) FROM store_subscriptions GROUP BY status;"
```
If any row has `incomplete`, halt and escalate — mapping needs product decision. (At v1 scale per project memory, there are no prod rows.)

- [ ] **Step 2: Write up migration**

```sql
-- 000040_subscription_status_v2_expand.up.sql
-- Remap legacy statuses + add v2.3 states.
UPDATE store_subscriptions
SET status = CASE status
    WHEN 'cancelled'  THEN 'expired'
    WHEN 'incomplete' THEN 'signup'
    ELSE status
END
WHERE status IN ('cancelled', 'incomplete');

ALTER TABLE store_subscriptions
    ADD CONSTRAINT store_subscriptions_status_check
        CHECK (status IN (
            'signup',
            'trialing',
            'active',
            'past_due',
            'payment_action_required',
            'cancel_scheduled',
            'expired',
            'store_closed',
            'pending_hard_delete',
            'hard_deleted'
        ));
```

- [ ] **Step 3: Write down migration**

```sql
-- 000040_subscription_status_v2_expand.down.sql
ALTER TABLE store_subscriptions DROP CONSTRAINT IF EXISTS store_subscriptions_status_check;

-- Collapse v2.3-only statuses back onto the legacy set.
UPDATE store_subscriptions
SET status = CASE status
    WHEN 'signup'                  THEN 'incomplete'
    WHEN 'payment_action_required' THEN 'past_due'
    WHEN 'cancel_scheduled'        THEN 'active'
    WHEN 'store_closed'            THEN 'cancelled'
    WHEN 'pending_hard_delete'     THEN 'cancelled'
    WHEN 'hard_deleted'            THEN 'cancelled'
    WHEN 'expired'                 THEN 'cancelled'
    ELSE status
END;
```

- [ ] **Step 4: Apply + verify CHECK rejects invalid value**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "UPDATE store_subscriptions SET status='nonsense' WHERE false;" || true
```
Expected: constraint is present; insert of bad value would fail.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/migrations/000040_subscription_status_v2_expand.*.sql
git commit -m "feat: expand subscription status enum with v2.3 states"
```

---

## Task 4: Expand Go `SubscriptionPlan` + `SubscriptionStatus` + `StoreSubscription`

**Files:**
- Modify: `services/marketplace-api/internal/subscription/models.go`

**Spec references:** §3 (plans), §17.1 (statuses), §2 (columns).

- [ ] **Step 1: Write the failing test first**

Create `services/marketplace-api/internal/subscription/models_test.go`:

```go
//go:build integration

package subscription_test

import (
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestStoreSubscription_V23ColumnsRoundTrip(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")

    tenantID := uuid.New()
    storeID := uuid.New()

    validated := uuid.New().String()[:8] // placeholder timestamp-ish not used; we set via GORM
    _ = validated

    sub := subscription.StoreSubscription{
        TenantID:             tenantID,
        StoreID:              storeID,
        StripeCustomerID:     "cus_test",
        Plan:                 subscription.PlanStudio,
        Status:               subscription.StatusSignup,
        ReverseChargeTaxID:   strPtr("GB123456789"),
        TaxIDCountry:         strPtr("GB"),
        TaxIDValidated:       true,
        BillingCurrency:      strPtr("GBP"),
        PriceTier:            subscription.PriceTierDeveloped,
        HasWhiteLabelAppAddOn: false,
        ArbitrageFlag:        false,
        TaxIDNameMatch:       subscription.TaxIDNameMatchMatched,
    }
    require.NoError(t, db.Create(&sub).Error)

    var got subscription.StoreSubscription
    require.NoError(t, db.Where("store_id = ?", storeID).First(&got).Error)

    require.Equal(t, subscription.PlanStudio, got.Plan)
    require.Equal(t, subscription.StatusSignup, got.Status)
    require.Equal(t, "GBP", *got.BillingCurrency)
    require.Equal(t, subscription.PriceTierDeveloped, got.PriceTier)
    require.True(t, got.TaxIDValidated)
    require.Equal(t, subscription.TaxIDNameMatchMatched, got.TaxIDNameMatch)
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Run the test — expect FAIL**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/subscription/... -run TestStoreSubscription_V23ColumnsRoundTrip -v
```
Expected: build errors on `PlanStudio`, `StatusSignup`, `ReverseChargeTaxID`, etc. — undefined.

- [ ] **Step 3: Update `models.go` — plan constants**

In `services/marketplace-api/internal/subscription/models.go`, replace the existing `SubscriptionPlan` constant block with:

```go
type SubscriptionPlan string

const (
    PlanTrial       SubscriptionPlan = "trial"
    PlanStarter     SubscriptionPlan = "starter"
    PlanStudio      SubscriptionPlan = "studio"
    PlanPro         SubscriptionPlan = "pro"
    PlanMarketplace SubscriptionPlan = "marketplace" // hidden from UI
)

// AllPublicPlans returns the plans displayable on the pricing page (in order).
func AllPublicPlans() []SubscriptionPlan {
    return []SubscriptionPlan{PlanTrial, PlanStarter, PlanStudio, PlanPro}
}
```

- [ ] **Step 4: Update `models.go` — status constants**

```go
type SubscriptionStatus string

const (
    StatusSignup                SubscriptionStatus = "signup"
    StatusTrialing              SubscriptionStatus = "trialing"
    StatusActive                SubscriptionStatus = "active"
    StatusPastDue               SubscriptionStatus = "past_due"
    StatusPaymentActionRequired SubscriptionStatus = "payment_action_required"
    StatusCancelScheduled       SubscriptionStatus = "cancel_scheduled"
    StatusExpired               SubscriptionStatus = "expired"
    StatusStoreClosed           SubscriptionStatus = "store_closed"
    StatusPendingHardDelete     SubscriptionStatus = "pending_hard_delete"
    StatusHardDeleted           SubscriptionStatus = "hard_deleted"
)
```

Remove the old `StatusIncomplete` and `StatusCancelled` constants. (Grep for them; there should be zero references after this; if any remain, fix them — most are likely in `status_test.go` which will be removed by this edit.)

- [ ] **Step 5: Update `models.go` — new typed columns**

Add just below the status block:

```go
type PriceTier string

const (
    PriceTierDeveloped PriceTier = "developed"
    PriceTierPPP       PriceTier = "ppp"
)

type TaxIDNameMatch string

const (
    TaxIDNameMatchMatched    TaxIDNameMatch = "matched"
    TaxIDNameMatchUnmatched  TaxIDNameMatch = "unmatched"
    TaxIDNameMatchNotChecked TaxIDNameMatch = "not_checked"
)
```

- [ ] **Step 6: Define `WhiteLabelAppStatus` stub to keep this task's commit compileable**

Add a temporary stub at the top of `internal/subscription/models.go` so the struct field in Step 7 resolves. Task 11 will move this to its own file and replace the stub with the full enum:

```go
// WhiteLabelAppStatus: temporary stub so Task 4 compiles independently.
// Task 11 moves this type (and its constants) into internal/subscription/app_lifecycle.go
// and expands it with the 6-value enum from §13.5. Do NOT add constants here.
type WhiteLabelAppStatus string
```

This stub is mandatory, not optional — without it the commit at Step 10 will not build. Task 11 Step 5 deletes this stub when the full file replaces it.

- [ ] **Step 7: Update `StoreSubscription` struct**

Replace the existing struct with:

```go
type StoreSubscription struct {
    ID                    uuid.UUID          `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
    TenantID              uuid.UUID          `gorm:"column:tenant_id;type:uuid;not null;index:ss_tenant_idx"`
    StoreID               uuid.UUID          `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
    StripeCustomerID      string             `gorm:"column:stripe_customer_id;type:varchar(100);not null"`
    StripeSubscriptionID  *string            `gorm:"column:stripe_subscription_id;type:varchar(100)"`
    Plan                  SubscriptionPlan   `gorm:"column:plan;type:varchar(30);not null;default:trial"`
    Status                SubscriptionStatus `gorm:"column:status;type:varchar(30);not null;default:signup"`
    CurrentPeriodStart    *time.Time         `gorm:"column:current_period_start"`
    CurrentPeriodEnd      *time.Time         `gorm:"column:current_period_end"`
    CancelAtPeriodEnd     bool               `gorm:"column:cancel_at_period_end;not null;default:false"`

    // v2.3 — tax ID
    ReverseChargeTaxID    *string            `gorm:"column:reverse_charge_tax_id;type:varchar(50)"`
    TaxIDCountry          *string            `gorm:"column:tax_id_country;type:char(2)"`
    TaxIDValidated        bool               `gorm:"column:tax_id_validated;not null;default:false"`
    TaxIDValidatedAt      *time.Time         `gorm:"column:tax_id_validated_at"`
    TaxIDNameMatch        TaxIDNameMatch     `gorm:"column:tax_id_name_match;type:varchar(20);not null;default:not_checked"`

    // v2.3 — multi-currency
    BillingCurrency       *string            `gorm:"column:billing_currency;type:char(3)"`
    PriceTier             PriceTier          `gorm:"column:price_tier;type:varchar(20);not null;default:developed"`

    // v2.3 — add-on + flags
    HasWhiteLabelAppAddOn bool                    `gorm:"column:has_white_label_app_add_on;not null;default:false"`
    ArbitrageFlag         bool                    `gorm:"column:arbitrage_flag;not null;default:false"`
    AppLifecycleStatus    *WhiteLabelAppStatus    `gorm:"column:app_lifecycle_status;type:varchar(30)"`

    CreatedAt             time.Time          `gorm:"column:created_at;not null;default:now()"`
    UpdatedAt             time.Time          `gorm:"column:updated_at;not null;default:now()"`
}

func (StoreSubscription) TableName() string { return "store_subscriptions" }
```

- [ ] **Step 8: Run the test — expect PASS**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/subscription/... -run TestStoreSubscription_V23ColumnsRoundTrip -v
```
Expected: PASS.

- [ ] **Step 9: Fix any compile errors elsewhere**

```bash
cd services/marketplace-api
go build ./...
```

Three specific breakage sites to fix (enumerated so nothing's missed):

**a. `internal/plangate/gate.go`** references `PlanFree`, `PlanEnterprise`. Map `PlanFree → PlanTrial`, `PlanEnterprise → PlanPro`. Do **not** change plangate's feature matrix yet — that's P3's job. For P1 we only want the service to compile.

Minimal plangate change — update the `planOrder` map and `Plan` alias:

```go
// In internal/plangate/gate.go, replace the Plan enum with:
type Plan = subscription.SubscriptionPlan

// Keep a planOrder map matching the new 4 tiers + marketplace:
var planOrder = map[Plan]int{
    subscription.PlanTrial:       0,
    subscription.PlanStarter:     1,
    subscription.PlanStudio:      2,
    subscription.PlanPro:         3,
    subscription.PlanMarketplace: 4,
}
```

Note to reviewer: making `plangate.Plan` an alias of `subscription.SubscriptionPlan` is deliberate — there's only one plan universe. P3 will rewrite `featureMatrix`, so this is the minimum to keep main green.

If `featureMatrix` still references `PlanFree` / `PlanEnterprise`, substitute with the new constants (even if the numbers become placeholders — the real matrix rewrite is P3 Task 2).

**b. `internal/subscription/service.go:78` — `isValidPlan`** contains a literal error string `"plan must be free, starter, pro, or enterprise"` and a body that whitelists those legacy values. Update the accepted set to `{trial, starter, studio, pro}` and rewrite the error message to `"plan must be trial, starter, studio, or pro"`. A simple grep won't catch this because the literal is hidden inside the error string rather than importing the old constants — search explicitly:

```bash
grep -Rn '"plan must be\|PlanFree\|PlanEnterprise' services/marketplace-api/internal/
```

**c. Any other test fixtures or helpers** that instantiate `StoreSubscription{Plan: "free"|"enterprise", Status: "cancelled"|"incomplete"}` — fix to the new values.

- [ ] **Step 10: Commit**

```bash
git add services/marketplace-api/internal/subscription/models.go \
        services/marketplace-api/internal/subscription/models_test.go \
        services/marketplace-api/internal/subscription/service.go \
        services/marketplace-api/internal/plangate/gate.go
git commit -m "feat: expand SubscriptionPlan + SubscriptionStatus enums + v2.3 columns"
```

---

## Task 5: Migration 041 — `stripe_webhook_events`

**Files:**
- Create: `services/marketplace-api/migrations/000041_stripe_webhook_events.up.sql`
- Create: `services/marketplace-api/migrations/000041_stripe_webhook_events.down.sql`

**Spec references:** §17.7 — webhook idempotency + orphan handling. Table is consumed by P2's webhook dispatcher; here we only create it.

- [ ] **Step 1: Write up migration**

```sql
-- 000041_stripe_webhook_events.up.sql
-- §17.7: idempotency table for Stripe webhook dispatch.
-- event_id is the Stripe event.id — used as PK so INSERT ON CONFLICT is the idempotency guard.

CREATE TABLE stripe_webhook_events (
    event_id         VARCHAR(100) PRIMARY KEY,
    event_type       VARCHAR(100) NOT NULL,
    store_id         UUID,
    tenant_id        UUID,
    payload          JSONB        NOT NULL,
    received_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    processed_at     TIMESTAMPTZ,
    processing_error TEXT,
    retry_count      INT          NOT NULL DEFAULT 0,
    manual_review_required BOOLEAN NOT NULL DEFAULT false
);

-- Orphan-resolver cron (P2) scans for unprocessed events; index for that path.
CREATE INDEX swe_orphan_idx
    ON stripe_webhook_events (received_at)
    WHERE processed_at IS NULL AND store_id IS NULL AND manual_review_required = false;

-- Observability: per-event-type + per-time-window queries.
CREATE INDEX swe_type_received_idx
    ON stripe_webhook_events (event_type, received_at DESC);

-- Manual-review queue dashboard.
CREATE INDEX swe_manual_review_idx
    ON stripe_webhook_events (received_at DESC)
    WHERE manual_review_required = true;

COMMENT ON COLUMN stripe_webhook_events.event_id IS 'Stripe event.id; serves as idempotency key.';
COMMENT ON COLUMN stripe_webhook_events.store_id IS 'Resolved from stripe_customer_id -> store_subscriptions; NULL until resolved.';
COMMENT ON COLUMN stripe_webhook_events.manual_review_required IS '§17.7 — set after 6 retries (30 min); cron stops retrying.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000041_stripe_webhook_events.down.sql
DROP TABLE IF EXISTS stripe_webhook_events;
```

- [ ] **Step 3: Apply + verify**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d stripe_webhook_events" | grep -E "(event_id|retry_count|manual_review_required)"
```
Expected: all three columns present.

Verify PK uniqueness:
```bash
psql "$TEST_DATABASE_URL" <<'SQL'
INSERT INTO stripe_webhook_events (event_id, event_type, payload) VALUES ('evt_1','test','{}');
INSERT INTO stripe_webhook_events (event_id, event_type, payload) VALUES ('evt_1','test','{}') ON CONFLICT (event_id) DO NOTHING;
SELECT count(*) FROM stripe_webhook_events WHERE event_id='evt_1';
DELETE FROM stripe_webhook_events WHERE event_id='evt_1';
SQL
```
Expected: `count = 1`.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000041_stripe_webhook_events.*.sql
git commit -m "feat: add stripe_webhook_events table for idempotent dispatch"
```

---

## Task 6: Migration 042 — `subscription_arbitrage_audit`

**Files:**
- Create: `services/marketplace-api/migrations/000042_subscription_arbitrage_audit.up.sql`
- Create: `services/marketplace-api/migrations/000042_subscription_arbitrage_audit.down.sql`

**Spec references:** §18.8 — geo-pricing anti-arbitrage, HMAC-SHA256 IP hashing, no raw IP stored.

- [ ] **Step 1: Write up migration**

```sql
-- 000042_subscription_arbitrage_audit.up.sql
-- §18.8 — geo-pricing arbitrage audit. HMAC-SHA256 of IP only (raw IP never persisted).

CREATE TABLE subscription_arbitrage_audit (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id      UUID        NOT NULL REFERENCES store_subscriptions(id) ON DELETE CASCADE,
    tenant_id            UUID        NOT NULL,
    store_id             UUID        NOT NULL,
    card_country         CHAR(2),
    billing_country      CHAR(2),
    ip_country           CHAR(2),
    ip_hash              VARCHAR(64),
    resolved_price_tier  VARCHAR(20) NOT NULL,
    mismatch_reason      VARCHAR(100),
    flagged_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by          UUID,
    reviewed_at          TIMESTAMPTZ,
    resolution           VARCHAR(30) NOT NULL DEFAULT 'ongoing'
        CHECK (resolution IN ('ongoing', 'false_positive_cleared', 'reprice_developed', 'terminated')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX saa_ongoing_idx ON subscription_arbitrage_audit (flagged_at)
    WHERE resolution = 'ongoing';
CREATE INDEX saa_tenant_idx ON subscription_arbitrage_audit (tenant_id);
CREATE INDEX saa_subscription_idx ON subscription_arbitrage_audit (subscription_id);

COMMENT ON COLUMN subscription_arbitrage_audit.ip_hash IS 'HMAC-SHA256(key from Secret Manager arbitrage-ip-hmac-key, data=raw_ip); 30d rotation.';
COMMENT ON COLUMN subscription_arbitrage_audit.ip_country IS 'Derived from IP geolookup; durable join field beyond HMAC rotation window.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000042_subscription_arbitrage_audit.down.sql
DROP TABLE IF EXISTS subscription_arbitrage_audit;
```

- [ ] **Step 3: Apply + verify partial index exists**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d subscription_arbitrage_audit"
psql "$TEST_DATABASE_URL" -c "SELECT indexname, indexdef FROM pg_indexes WHERE tablename='subscription_arbitrage_audit';"
```
Expected: `saa_ongoing_idx` has `WHERE (resolution = 'ongoing')` in the indexdef.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000042_subscription_arbitrage_audit.*.sql
git commit -m "feat: add subscription_arbitrage_audit table (HMAC IP, no raw IP)"
```

---

## Task 7: Migration 043 — `business_entity_attestations` (immutable)

**Files:**
- Create: `services/marketplace-api/migrations/000043_business_entity_attestations.up.sql`
- Create: `services/marketplace-api/migrations/000043_business_entity_attestations.down.sql`

**Spec references:** §19.3.1 — US/CA B2B attestation. Both the UPDATE-blocking trigger AND the `REVOKE DELETE` at role level are required (Security finding; trigger alone insufficient — `DROP TRIGGER` bypass).

**Key decision:** the role that the marketplace-api service connects as. Per exploration the DB user is likely `marketplace_user` (deployment convention from CLAUDE.md: `{service_prefix}_user`). If the actual production role differs, substitute it; the migration can be run as a superuser/migration role so the `REVOKE FROM` will succeed.

- [ ] **Step 1: Write up migration**

```sql
-- 000043_business_entity_attestations.up.sql
-- §19.3.1 — US/CA B2B entity attestation. Append-only: trigger blocks UPDATE; role revoke blocks DELETE.
-- Both required (Security finding): a DROP TRIGGER alone would bypass the UPDATE block,
-- but the DELETE revoke is at role level and closes the path.

CREATE TABLE business_entity_attestations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id         UUID        NOT NULL,
    tenant_id        UUID        NOT NULL,
    country          CHAR(2)     NOT NULL,
    checkbox_text    TEXT        NOT NULL,
    checkbox_version VARCHAR(20) NOT NULL,
    user_agent       TEXT,
    ip_hash          VARCHAR(64),
    signed_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX bea_store_idx ON business_entity_attestations (store_id);
CREATE INDEX bea_tenant_idx ON business_entity_attestations (tenant_id);

-- Trigger: reject UPDATE.
CREATE OR REPLACE FUNCTION raise_immutable_exception() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'business_entity_attestations is append-only; UPDATE rejected';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER business_entity_no_update
    BEFORE UPDATE ON business_entity_attestations
    FOR EACH ROW EXECUTE FUNCTION raise_immutable_exception();

-- Role-level DELETE revoke. The app role must NOT be able to DELETE these rows.
-- Use a DO block to handle the case where the role doesn't yet exist in a dev DB.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'marketplace_user') THEN
        REVOKE DELETE ON business_entity_attestations FROM marketplace_user;
    END IF;
    -- Also ensure PUBLIC has no DELETE grant (belt + braces).
    REVOKE DELETE ON business_entity_attestations FROM PUBLIC;
END$$;

COMMENT ON TABLE business_entity_attestations IS 'Append-only per §19.3.1. Trigger blocks UPDATE, role-level revoke blocks DELETE.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000043_business_entity_attestations.down.sql
DROP TRIGGER IF EXISTS business_entity_no_update ON business_entity_attestations;
DROP FUNCTION IF EXISTS raise_immutable_exception();
DROP TABLE IF EXISTS business_entity_attestations;
```

- [ ] **Step 3: Apply + verify trigger and revoke**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up

# Insert a row as superuser (works)
psql "$TEST_DATABASE_URL" <<'SQL'
INSERT INTO business_entity_attestations (store_id, tenant_id, country, checkbox_text, checkbox_version)
VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000002','US','I attest','v1');
SQL

# Verify UPDATE is blocked
psql "$TEST_DATABASE_URL" -c "UPDATE business_entity_attestations SET country='CA';" 2>&1 | grep -q "append-only" && echo "UPDATE blocked ✓"

# Verify DELETE grant is absent (check via information_schema)
psql "$TEST_DATABASE_URL" -c "SELECT has_table_privilege('marketplace_user','business_entity_attestations','DELETE');" 2>/dev/null || true
```
Expected: UPDATE fails with the `append-only` message. (If `marketplace_user` doesn't exist locally, skip the DELETE privilege check; CI will verify.)

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000043_business_entity_attestations.*.sql
git commit -m "feat: add business_entity_attestations with trigger + role-level DELETE revoke"
```

---

## Task 8: Migration 044 — `billing_archive`

**Files:**
- Create: `services/marketplace-api/migrations/000044_billing_archive.up.sql`
- Create: `services/marketplace-api/migrations/000044_billing_archive.down.sql`

**Spec references:** §23.2 — 7-year retention of billing records, retained post-hard-delete under legal-obligation basis.

- [ ] **Step 1: Write up migration**

```sql
-- 000044_billing_archive.up.sql
-- §23.2 — 7-year billing retention for GDPR legal-obligation basis. No PII beyond what tax/audit law requires.

CREATE TABLE billing_archive (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    original_store_id    UUID         NOT NULL,
    original_tenant_id   UUID         NOT NULL,
    business_name        VARCHAR(500) NOT NULL,
    tax_id               VARCHAR(50),
    tax_id_country       CHAR(2),
    billing_country      CHAR(2),
    billing_currency     CHAR(3),
    stripe_customer_id   VARCHAR(100) NOT NULL,
    all_invoices         JSONB        NOT NULL,
    total_revenue_usd    NUMERIC(12,2),
    hard_deleted_at      TIMESTAMPTZ  NOT NULL,
    archive_expires_at   TIMESTAMPTZ  NOT NULL
);

CREATE INDEX ba_expires_idx      ON billing_archive (archive_expires_at);
CREATE INDEX ba_stripe_cust_idx  ON billing_archive (stripe_customer_id);
CREATE INDEX ba_tenant_idx       ON billing_archive (original_tenant_id);

COMMENT ON TABLE billing_archive IS '§23.2 — retained 7 years after hard-delete under legal-obligation basis.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000044_billing_archive.down.sql
DROP TABLE IF EXISTS billing_archive;
```

- [ ] **Step 3: Apply + verify**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "\d billing_archive"
```
Expected: table + 3 indexes present.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000044_billing_archive.*.sql
git commit -m "feat: add billing_archive (7-year retention)"
```

---

## Task 9: Migration 045 — `campaign_email_budget`

**Files:**
- Create: `services/marketplace-api/migrations/000045_campaign_email_budget.up.sql`
- Create: `services/marketplace-api/migrations/000045_campaign_email_budget.down.sql`

**Spec references:** §10.1 — atomic-decrement campaign budget with trial ramp (first 7 days).

- [ ] **Step 1: Write up migration**

```sql
-- 000045_campaign_email_budget.up.sql
-- §10.1 — per-store monthly campaign email budget. Consumed atomically by pre-send enforcement (P9).

CREATE TABLE campaign_email_budget (
    store_id    UUID NOT NULL,
    month       DATE NOT NULL,
    remaining   INT  NOT NULL,
    limit_set   INT  NOT NULL,  -- mutated by trial-ramp cron D3→D4 and D7→D8
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, month)
);

-- Sanity check: can never go negative.
ALTER TABLE campaign_email_budget
    ADD CONSTRAINT campaign_email_budget_remaining_nonneg CHECK (remaining >= 0);

COMMENT ON COLUMN campaign_email_budget.limit_set IS '§5.1 — trial ramp: D1-3=500, D4-7=2000, D8+=plan allowance.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000045_campaign_email_budget.down.sql
DROP TABLE IF EXISTS campaign_email_budget;
```

- [ ] **Step 3: Apply + verify CHECK constraint**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "INSERT INTO campaign_email_budget (store_id, month, remaining, limit_set) VALUES ('00000000-0000-0000-0000-000000000001','2026-04-01',-1,500);" 2>&1 | grep -q "remaining_nonneg" && echo "CHECK works ✓"
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000045_campaign_email_budget.*.sql
git commit -m "feat: add campaign_email_budget with atomic-decrement support"
```

---

## Task 10: Migration 046 — `white_label_app_lifecycle`

**Files:**
- Create: `services/marketplace-api/migrations/000046_white_label_app_lifecycle.up.sql`
- Create: `services/marketplace-api/migrations/000046_white_label_app_lifecycle.down.sql`

**Spec references:** §13.5 — separate lifecycle enum orthogonal to subscription status.

- [ ] **Step 1: Write up migration**

```sql
-- 000046_white_label_app_lifecycle.up.sql
-- §13.5 — app lifecycle runs independently of subscription state during Pro+App teardown.

CREATE TABLE white_label_app_lifecycle (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id        UUID        NOT NULL,
    tenant_id       UUID        NOT NULL,
    status          VARCHAR(30) NOT NULL
        CHECK (status IN (
            'active',
            'sunset_scheduled',
            'downloads_blocked',
            'pulled',
            'firebase_archived',
            'credentials_purged'
        )),
    scheduled_at    TIMESTAMPTZ,
    actor           VARCHAR(100) NOT NULL,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX wlal_store_idx ON white_label_app_lifecycle (store_id, created_at DESC);
CREATE INDEX wlal_scheduled_idx ON white_label_app_lifecycle (scheduled_at)
    WHERE scheduled_at IS NOT NULL;

COMMENT ON TABLE white_label_app_lifecycle IS '§13.5 — append-only transition log for the white-label app teardown sequence.';
```

- [ ] **Step 2: Write down migration**

```sql
-- 000046_white_label_app_lifecycle.down.sql
DROP TABLE IF EXISTS white_label_app_lifecycle;
```

- [ ] **Step 3: Apply + verify CHECK**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
psql "$TEST_DATABASE_URL" -c "INSERT INTO white_label_app_lifecycle (store_id, tenant_id, status, actor) VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000002','bogus','test');" 2>&1 | grep -q "white_label_app_lifecycle_status_check\|invalid input" && echo "CHECK works ✓"
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000046_white_label_app_lifecycle.*.sql
git commit -m "feat: add white_label_app_lifecycle table"
```

---

## Task 11: Go models for the 6 new tables

**Files (create):**
- `services/marketplace-api/internal/subscription/app_lifecycle.go`
- `services/marketplace-api/internal/webhookevents/models.go`
- `services/marketplace-api/internal/arbitrage/models.go`
- `services/marketplace-api/internal/attestation/models.go`
- `services/marketplace-api/internal/billingarchive/models.go`
- `services/marketplace-api/internal/campaignbudget/models.go`

**Files (create tests):**
- `services/marketplace-api/internal/webhookevents/models_test.go`
- `services/marketplace-api/internal/arbitrage/models_test.go`
- `services/marketplace-api/internal/attestation/models_test.go`

These are thin GORM models only — no repository logic yet. Repositories land in P2 / P3 / P6 / P7 / P9 / P10 etc.

- [ ] **Step 1: Write the failing webhook-events test**

Create `services/marketplace-api/internal/webhookevents/models_test.go`:

```go
//go:build integration

package webhookevents_test

import (
    "encoding/json"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/webhookevents"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestStripeWebhookEvent_InsertOnConflictNoop(t *testing.T) {
    db := testdb.NewDB(t, "stripe_webhook_events")

    payload, _ := json.Marshal(map[string]any{"foo": "bar"})
    evt := webhookevents.StripeWebhookEvent{
        EventID:   "evt_idempotent",
        EventType: "customer.subscription.updated",
        Payload:   payload,
    }
    require.NoError(t, db.Create(&evt).Error)

    // ON CONFLICT DO NOTHING — second insert must not error and row count must stay 1.
    err := db.Exec(`INSERT INTO stripe_webhook_events (event_id, event_type, payload)
                    VALUES (?, ?, ?::jsonb) ON CONFLICT (event_id) DO NOTHING`,
        evt.EventID, evt.EventType, string(payload)).Error
    require.NoError(t, err)

    var count int64
    require.NoError(t, db.Table("stripe_webhook_events").Where("event_id=?", evt.EventID).Count(&count).Error)
    require.EqualValues(t, 1, count)
}
```

- [ ] **Step 2: Run the test — expect FAIL (package doesn't exist)**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/webhookevents/... -v
```
Expected: build error — package missing.

- [ ] **Step 3: Create `webhookevents/models.go`**

```go
package webhookevents

import (
    "time"

    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type StripeWebhookEvent struct {
    EventID              string         `gorm:"column:event_id;type:varchar(100);primaryKey"`
    EventType            string         `gorm:"column:event_type;type:varchar(100);not null"`
    StoreID              *uuid.UUID     `gorm:"column:store_id;type:uuid"`
    TenantID             *uuid.UUID     `gorm:"column:tenant_id;type:uuid"`
    Payload              datatypes.JSON `gorm:"column:payload;type:jsonb;not null"`
    ReceivedAt           time.Time      `gorm:"column:received_at;not null;default:now()"`
    ProcessedAt          *time.Time     `gorm:"column:processed_at"`
    ProcessingError      *string        `gorm:"column:processing_error"`
    RetryCount           int            `gorm:"column:retry_count;not null;default:0"`
    ManualReviewRequired bool           `gorm:"column:manual_review_required;not null;default:false"`
}

func (StripeWebhookEvent) TableName() string { return "stripe_webhook_events" }
```

- [ ] **Step 4: Run the webhook-events test — expect PASS**

```bash
go test -tags=integration ./internal/webhookevents/... -v
```
Expected: PASS.

- [ ] **Step 5: Create `subscription/app_lifecycle.go` — and delete the stub from Task 4**

First, remove the stub `type WhiteLabelAppStatus string` from `internal/subscription/models.go` (the stub added in Task 4 Step 6). Then create the new file as the single source of truth:

```go
package subscription

import (
    "time"

    "github.com/google/uuid"
)

type WhiteLabelAppStatus string

const (
    AppStatusActive            WhiteLabelAppStatus = "active"
    AppStatusSunsetScheduled   WhiteLabelAppStatus = "sunset_scheduled"
    AppStatusDownloadsBlocked  WhiteLabelAppStatus = "downloads_blocked"
    AppStatusPulled            WhiteLabelAppStatus = "pulled"
    AppStatusFirebaseArchived  WhiteLabelAppStatus = "firebase_archived"
    AppStatusCredentialsPurged WhiteLabelAppStatus = "credentials_purged"
)

type WhiteLabelAppLifecycleEntry struct {
    ID          uuid.UUID            `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
    StoreID     uuid.UUID            `gorm:"column:store_id;type:uuid;not null"`
    TenantID    uuid.UUID            `gorm:"column:tenant_id;type:uuid;not null"`
    Status      WhiteLabelAppStatus  `gorm:"column:status;type:varchar(30);not null"`
    ScheduledAt *time.Time           `gorm:"column:scheduled_at"`
    Actor       string               `gorm:"column:actor;type:varchar(100);not null"`
    Reason      *string              `gorm:"column:reason"`
    CreatedAt   time.Time            `gorm:"column:created_at;not null;default:now()"`
}

func (WhiteLabelAppLifecycleEntry) TableName() string { return "white_label_app_lifecycle" }
```

- [ ] **Step 6: Create `arbitrage/models.go`**

```go
package arbitrage

import (
    "time"

    "github.com/google/uuid"
)

type Resolution string

const (
    ResolutionOngoing              Resolution = "ongoing"
    ResolutionFalsePositiveCleared Resolution = "false_positive_cleared"
    ResolutionRepriceDeveloped     Resolution = "reprice_developed"
    ResolutionTerminated           Resolution = "terminated"
)

type SubscriptionArbitrageAudit struct {
    ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
    SubscriptionID    uuid.UUID  `gorm:"column:subscription_id;type:uuid;not null"`
    TenantID          uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
    StoreID           uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
    CardCountry       *string    `gorm:"column:card_country;type:char(2)"`
    BillingCountry    *string    `gorm:"column:billing_country;type:char(2)"`
    IPCountry         *string    `gorm:"column:ip_country;type:char(2)"`
    IPHash            *string    `gorm:"column:ip_hash;type:varchar(64)"`
    ResolvedPriceTier string     `gorm:"column:resolved_price_tier;type:varchar(20);not null"`
    MismatchReason    *string    `gorm:"column:mismatch_reason;type:varchar(100)"`
    FlaggedAt         time.Time  `gorm:"column:flagged_at;not null;default:now()"`
    ReviewedBy        *uuid.UUID `gorm:"column:reviewed_by;type:uuid"`
    ReviewedAt        *time.Time `gorm:"column:reviewed_at"`
    Resolution        Resolution `gorm:"column:resolution;type:varchar(30);not null;default:ongoing"`
    CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()"`
}

func (SubscriptionArbitrageAudit) TableName() string { return "subscription_arbitrage_audit" }
```

Write test `internal/arbitrage/models_test.go` that inserts + reads back a row; verifies no `raw_ip` column exists by querying `information_schema`:

```go
//go:build integration

package arbitrage_test

import (
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/arbitrage"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestSubscriptionArbitrageAudit_NoRawIPColumn(t *testing.T) {
    db := testdb.NewDB(t, "subscription_arbitrage_audit")

    var cnt int64
    require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
                               WHERE table_name='subscription_arbitrage_audit'
                                 AND column_name IN ('raw_ip','ip','client_ip')`).Scan(&cnt).Error)
    require.EqualValues(t, 0, cnt, "raw IP columns must not exist")
}

func TestSubscriptionArbitrageAudit_RoundTrip(t *testing.T) {
    db := testdb.NewDB(t, "subscription_arbitrage_audit", "store_subscriptions")

    sub := subscription.StoreSubscription{
        TenantID: uuid.New(), StoreID: uuid.New(),
        StripeCustomerID: "cus_x", Plan: subscription.PlanStudio, Status: subscription.StatusActive,
    }
    require.NoError(t, db.Create(&sub).Error)

    ipCountry, ipHash := "IN", "abc123"
    row := arbitrage.SubscriptionArbitrageAudit{
        SubscriptionID: sub.ID, TenantID: sub.TenantID, StoreID: sub.StoreID,
        IPCountry: &ipCountry, IPHash: &ipHash,
        ResolvedPriceTier: "ppp",
    }
    require.NoError(t, db.Create(&row).Error)

    var got arbitrage.SubscriptionArbitrageAudit
    require.NoError(t, db.First(&got, "id=?", row.ID).Error)
    require.Equal(t, arbitrage.ResolutionOngoing, got.Resolution)
}
```

- [ ] **Step 7: Run arbitrage tests — expect PASS**

```bash
go test -tags=integration ./internal/arbitrage/... -v
```

- [ ] **Step 8: Create `attestation/models.go`**

```go
package attestation

import (
    "time"

    "github.com/google/uuid"
)

type BusinessEntityAttestation struct {
    ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
    StoreID         uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
    TenantID        uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
    Country         string    `gorm:"column:country;type:char(2);not null"`
    CheckboxText    string    `gorm:"column:checkbox_text;not null"`
    CheckboxVersion string    `gorm:"column:checkbox_version;type:varchar(20);not null"`
    UserAgent       *string   `gorm:"column:user_agent"`
    IPHash          *string   `gorm:"column:ip_hash;type:varchar(64)"`
    SignedAt        time.Time `gorm:"column:signed_at;not null;default:now()"`
}

func (BusinessEntityAttestation) TableName() string { return "business_entity_attestations" }
```

Test file `attestation/models_test.go` is Task 15 (security regression); create a stub here:

```go
//go:build integration

package attestation_test

import (
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/attestation"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestBusinessEntityAttestation_RoundTrip(t *testing.T) {
    db := testdb.NewDB(t, "business_entity_attestations")

    row := attestation.BusinessEntityAttestation{
        StoreID: uuid.New(), TenantID: uuid.New(),
        Country: "US", CheckboxText: "I attest", CheckboxVersion: "v1",
    }
    require.NoError(t, db.Create(&row).Error)

    var got attestation.BusinessEntityAttestation
    require.NoError(t, db.First(&got, "id=?", row.ID).Error)
    require.Equal(t, "US", got.Country)
}
```

(Task 15 adds the DELETE/UPDATE rejection test, which needs a non-superuser DB role.)

- [ ] **Step 9: Create `billingarchive/models.go`**

```go
package billingarchive

import (
    "time"

    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type BillingArchive struct {
    ID                 uuid.UUID      `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
    OriginalStoreID    uuid.UUID      `gorm:"column:original_store_id;type:uuid;not null"`
    OriginalTenantID   uuid.UUID      `gorm:"column:original_tenant_id;type:uuid;not null"`
    BusinessName       string         `gorm:"column:business_name;type:varchar(500);not null"`
    TaxID              *string        `gorm:"column:tax_id;type:varchar(50)"`
    TaxIDCountry       *string        `gorm:"column:tax_id_country;type:char(2)"`
    BillingCountry     *string        `gorm:"column:billing_country;type:char(2)"`
    BillingCurrency    *string        `gorm:"column:billing_currency;type:char(3)"`
    StripeCustomerID   string         `gorm:"column:stripe_customer_id;type:varchar(100);not null"`
    AllInvoices        datatypes.JSON `gorm:"column:all_invoices;type:jsonb;not null"`
    TotalRevenueUSD    *float64       `gorm:"column:total_revenue_usd;type:numeric(12,2)"`
    HardDeletedAt      time.Time      `gorm:"column:hard_deleted_at;not null"`
    ArchiveExpiresAt   time.Time      `gorm:"column:archive_expires_at;not null"`
}

func (BillingArchive) TableName() string { return "billing_archive" }
```

- [ ] **Step 10: Create `campaignbudget/models.go`**

```go
package campaignbudget

import (
    "time"

    "github.com/google/uuid"
)

type CampaignEmailBudget struct {
    StoreID   uuid.UUID `gorm:"column:store_id;type:uuid;primaryKey"`
    Month     time.Time `gorm:"column:month;type:date;primaryKey"`
    Remaining int       `gorm:"column:remaining;not null"`
    LimitSet  int       `gorm:"column:limit_set;not null"`
    UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (CampaignEmailBudget) TableName() string { return "campaign_email_budget" }
```

- [ ] **Step 11: Build + run all new tests**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./internal/webhookevents/... ./internal/arbitrage/... ./internal/attestation/... -v
```
Expected: all PASS.

- [ ] **Step 12: Commit**

```bash
git add services/marketplace-api/internal/subscription/app_lifecycle.go \
        services/marketplace-api/internal/webhookevents/ \
        services/marketplace-api/internal/arbitrage/ \
        services/marketplace-api/internal/attestation/ \
        services/marketplace-api/internal/billingarchive/ \
        services/marketplace-api/internal/campaignbudget/
git commit -m "feat: add GORM models for v2.3 subscription tables"
```

---

## Task 12: Fix `GetByStoreID` tenant_id hole + call-site audit

**Files:**
- Modify: `services/marketplace-api/internal/subscription/repository.go`
- Modify: `services/marketplace-api/internal/subscription/service.go`
- Modify: `services/marketplace-api/internal/handlers/admin/subscription.go`
- Modify: `services/marketplace-api/internal/plangate/gate.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (wiring only)
- Create: `services/marketplace-api/internal/subscription/repository_test.go`

**Spec references:** §18.2 — "SECURITY FIX: require tenant_id parameter; audit all call sites."

**Threat model:** Today, `GetByStoreID(ctx, db, storeID)` returns a subscription filtered only by `store_id`. If another tenant's code path (bug, IDOR, middleware gap) passes a foreign store ID, the repository returns data it shouldn't. We fix this by making `tenantID` mandatory in the signature and filtering `WHERE tenant_id = ? AND store_id = ?`.

- [ ] **Step 1: Write the failing security test**

Create `services/marketplace-api/internal/subscription/repository_test.go`:

```go
//go:build integration

package subscription_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestRepository_GetByStoreID_RequiresTenantMatch(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    repo := subscription.NewRepository()

    tenantA := uuid.New()
    tenantB := uuid.New()
    store := uuid.New()

    // Subscription belongs to tenant A.
    sub := subscription.StoreSubscription{
        TenantID:         tenantA,
        StoreID:          store,
        StripeCustomerID: "cus_A",
        Plan:             subscription.PlanStarter,
        Status:           subscription.StatusActive,
    }
    require.NoError(t, db.Create(&sub).Error)

    // Tenant A can read it.
    gotA, err := repo.GetByStoreID(context.Background(), db, tenantA, store)
    require.NoError(t, err)
    require.NotNil(t, gotA)
    require.Equal(t, tenantA, gotA.TenantID)

    // Tenant B gets ErrNotFound, NOT the subscription.
    gotB, err := repo.GetByStoreID(context.Background(), db, tenantB, store)
    require.ErrorIs(t, err, subscription.ErrNotFound)
    require.Nil(t, gotB)
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/subscription/... -run TestRepository_GetByStoreID_RequiresTenantMatch -v
```
Expected: compile error — `GetByStoreID` signature mismatch.

- [ ] **Step 3: Update repository interface + implementation**

In `services/marketplace-api/internal/subscription/repository.go`:

```go
// ErrNotFound is returned when a subscription does not exist or does not belong to the caller's tenant.
var ErrNotFound = errors.New("subscription: not found")

type Repository interface {
    // GetByStoreID returns the subscription for (tenantID, storeID).
    // Returns ErrNotFound when no matching row exists under the caller's tenant —
    // critically, never returns another tenant's subscription.
    GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error)

    // Create inserts a subscription. Caller must set TenantID on the model.
    Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

    // Update saves all fields; tenant_id is used as an extra filter guard.
    Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error

    // GetByStripeCustomerID is used by webhook dispatch where the tenant is
    // unknown until resolution; returns ErrNotFound if customer has no subscription.
    // Consumers MUST NOT expose this through tenant-facing APIs; it is
    // webhook/audit only. Callers must not return the raw row to HTTP clients.
    GetByStripeCustomerID(ctx context.Context, db *gorm.DB, customerID string) (*StoreSubscription, error)
}

type repoImpl struct{}

func NewRepository() Repository { return &repoImpl{} }

func (r *repoImpl) GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error) {
    var s StoreSubscription
    err := db.WithContext(ctx).
        Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
        First(&s).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("subscription: GetByStoreID: %w", err)
    }
    return &s, nil
}

func (r *repoImpl) Update(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
    res := db.WithContext(ctx).
        Model(s).
        Where("tenant_id = ? AND store_id = ?", s.TenantID, s.StoreID).
        Updates(s)
    if res.Error != nil {
        return fmt.Errorf("subscription: Update: %w", res.Error)
    }
    if res.RowsAffected == 0 {
        return ErrNotFound
    }
    return nil
}

func (r *repoImpl) Create(ctx context.Context, db *gorm.DB, s *StoreSubscription) error {
    if s.TenantID == uuid.Nil {
        return errors.New("subscription: Create: TenantID required")
    }
    if err := db.WithContext(ctx).Create(s).Error; err != nil {
        return fmt.Errorf("subscription: Create: %w", err)
    }
    return nil
}

func (r *repoImpl) GetByStripeCustomerID(ctx context.Context, db *gorm.DB, customerID string) (*StoreSubscription, error) {
    var s StoreSubscription
    err := db.WithContext(ctx).Where("stripe_customer_id = ?", customerID).First(&s).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("subscription: GetByStripeCustomerID: %w", err)
    }
    return &s, nil
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test -tags=integration ./internal/subscription/... -run TestRepository_GetByStoreID_RequiresTenantMatch -v
```
Expected: PASS.

- [ ] **Step 5: Audit + fix every call site**

```bash
cd services/marketplace-api
grep -Rn "repo\.GetByStoreID\|GetByStoreID(\|\.svc\.GetSubscription\|\.svc\.CreateCheckoutSession\|\.svc\.CreatePortalSession" internal/ cmd/
```

**Known call sites from exploration — fix EACH explicitly:**

| Location | What to change |
|---|---|
| `internal/plangate/gate.go:162` | `r.repo.GetByStoreID(ctx, r.db, storeID)` → add `tenantID` arg; `PlanResolver.Resolve` grows a `tenantID` parameter |
| `internal/subscription/service.go` — `GetSubscription` | grow a `tenantID` parameter; pass to repo |
| `internal/subscription/service.go:~85` — `CreateCheckoutSession` | already takes a `CheckoutInput` struct that **already carries `TenantID`** per exploration — verify; its internal `repo.GetByStoreID` call needs the tenant arg |
| `internal/subscription/service.go:~108` — `CreatePortalSession(ctx, storeID, returnURL)` | signature change: `CreatePortalSession(ctx context.Context, tenantID, storeID uuid.UUID, returnURL string)`; its internal `repo.GetByStoreID` call takes the new tenant |
| `internal/handlers/admin/subscription.go:~79` — `GetSubscription` handler | already parses `tenantID` from Gin — thread into `h.svc.GetSubscription(ctx, tenantID, storeID)` |
| `internal/handlers/admin/subscription.go:~122` — `CreateCheckout` handler | already parses `tenantID` (confirmed at exploration line 110) — ensure it populates `CheckoutInput.TenantID` (or wherever the new service consumes it) |
| `internal/handlers/admin/subscription.go:~164` — `CreatePortal` handler | currently passes only `storeID`; parse `tenantID` from Gin and add it to the new `CreatePortalSession` signature |

For **`plangate/gate.go`**: `PlanResolver.Resolve` is called from middleware. Middleware has Gin context with `tenant_id` already set. Change the signature to take the tenant:

```go
// Old: func (r *PlanResolver) Resolve(ctx context.Context, storeID uuid.UUID) Plan
// New:
func (r *PlanResolver) Resolve(ctx context.Context, tenantID, storeID uuid.UUID) subscription.SubscriptionPlan {
    if r.repo == nil || r.db == nil {
        return subscription.PlanTrial
    }
    sub, err := r.repo.GetByStoreID(ctx, r.db, tenantID, storeID)
    if err != nil {
        return subscription.PlanTrial
    }
    return sub.Plan
}
```

And update the `RequireFeature` / `RequirePlan` middleware (same file) to parse `tenant_id` from Gin context before calling Resolve:

```go
tenantID, err := uuid.Parse(c.GetString("tenant_id"))
if err != nil {
    c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
    return
}
storeID := ... // existing parse
plan := resolver.Resolve(c.Request.Context(), tenantID, storeID)
```

For **`subscription/service.go`**: update **all three** service methods that touch the repo:

```go
func (s *Service) GetSubscription(ctx context.Context, tenantID, storeID uuid.UUID) (*StoreSubscription, error) {
    return s.repo.GetByStoreID(ctx, s.db, tenantID, storeID)
}

// CreateCheckoutSession already receives CheckoutInput{TenantID, StoreID, ...}; ensure
// the internal repo.GetByStoreID lookup uses in.TenantID.

// CreatePortalSession signature grows tenantID:
func (s *Service) CreatePortalSession(ctx context.Context, tenantID, storeID uuid.UUID, returnURL string) (string, error) {
    sub, err := s.repo.GetByStoreID(ctx, s.db, tenantID, storeID)
    // ... existing logic
}
```

For **`handlers/admin/subscription.go`**: each of the three subscription handlers already pulls `tenantID` from `c.GetString("tenant_id")` (confirmed at handler lines ~110 / ~164). Thread it into every service call:

```go
tenantID, err := uuid.Parse(c.GetString("tenant_id"))
if err != nil { /* existing error response */ return }
storeID := /* existing parse */

// GetSubscription handler:
sub, err := h.svc.GetSubscription(c.Request.Context(), tenantID, storeID)

// CreateCheckout handler: populate CheckoutInput.TenantID (already a field).

// CreatePortal handler:
url, err := h.svc.CreatePortalSession(c.Request.Context(), tenantID, storeID, req.ReturnURL)
```

- [ ] **Step 6: Build — expect it to compile**

```bash
go build ./...
```
If build breaks, grep for further call sites — there may be helper functions or tests that call `GetByStoreID` directly. Fix each one.

- [ ] **Step 7: Run the existing test suite — expect no regressions**

```bash
go test -tags=integration ./internal/subscription/... ./internal/plangate/... ./internal/handlers/admin/... -v
```

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/subscription/repository.go \
        services/marketplace-api/internal/subscription/repository_test.go \
        services/marketplace-api/internal/subscription/service.go \
        services/marketplace-api/internal/plangate/gate.go \
        services/marketplace-api/internal/handlers/admin/subscription.go
git commit -m "fix(security): GetByStoreID now requires tenant_id (closes IDOR hole)"
```

---

## Task 13: Advisory-lock helper `subscription.WithAdvisoryLock`

**Files:**
- Create: `services/marketplace-api/internal/subscription/advisory_lock.go`
- Create: `services/marketplace-api/internal/subscription/advisory_lock_test.go`

**Spec references:** §17.4 — "Every subscription write: `pg_advisory_xact_lock(hashtext(store_id::text))` + re-read + CAS guard". §4.5.1 — downgrade-block uses same lock.

Reused pattern: `internal/ticket/repository.go:191-197`. We generalise so P3's state-machine writes can call one helper.

- [ ] **Step 1: Write the failing test**

```go
//go:build integration

package subscription_test

import (
    "context"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

// TestWithAdvisoryLock_Serializes spawns two goroutines that both try to lock
// the same store_id. The second one must wait for the first to release —
// observable via a counter + tracked interleaving.
func TestWithAdvisoryLock_Serializes(t *testing.T) {
    db := testdb.NewRawDB(t) // shared DB, NOT a transaction (advisory locks are tx-scoped)
    storeID := uuid.New()

    var inside int32
    var peakInside int32
    var wg sync.WaitGroup

    run := func() {
        defer wg.Done()
        err := subscription.WithAdvisoryLock(context.Background(), db, storeID, func(tx *testdb.Tx) error {
            now := atomic.AddInt32(&inside, 1)
            for {
                p := atomic.LoadInt32(&peakInside)
                if now > p {
                    if atomic.CompareAndSwapInt32(&peakInside, p, now) {
                        break
                    }
                    continue
                }
                break
            }
            time.Sleep(50 * time.Millisecond)
            atomic.AddInt32(&inside, -1)
            return nil
        })
        require.NoError(t, err)
    }

    wg.Add(2)
    go run()
    go run()
    wg.Wait()

    require.EqualValues(t, 1, peakInside, "locks must serialize — peak concurrency inside critical section must be 1")
}
```

Note: `testdb.NewRawDB` may not exist. If only `NewDB(t, tables...)` exists, use that — the test just needs a real connection pool that can open two transactions. Adjust the test helper call to match the actual package API (discovered in exploration). The helper signature below presumes `*gorm.DB`:

- [ ] **Step 2: Run test — expect FAIL (compile error)**

```bash
go test -tags=integration ./internal/subscription/... -run TestWithAdvisoryLock_Serializes -v
```
Expected: `WithAdvisoryLock` undefined.

- [ ] **Step 3: Write `advisory_lock.go`**

```go
package subscription

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

// WithAdvisoryLock runs fn inside a transaction that holds a pg_advisory_xact_lock
// on hashtext(store_id). The lock is automatically released on commit/rollback.
// Use for every subscription-mutating code path (state transitions, plan changes,
// downgrade-block re-checks) per §17.4.
//
// The function is generic over the return type so callers can chain reads into the
// locked section without extra round-trips.
func WithAdvisoryLock(ctx context.Context, db *gorm.DB, storeID uuid.UUID, fn func(tx *gorm.DB) error) error {
    return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?))`, storeID.String()).Error; err != nil {
            return fmt.Errorf("subscription: advisory lock for store %s: %w", storeID, err)
        }
        return fn(tx)
    })
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test -tags=integration ./internal/subscription/... -run TestWithAdvisoryLock_Serializes -v
```

- [ ] **Step 5: Document reuse in package doc**

Add a top-of-file comment on `advisory_lock.go`:

```go
// Package subscription provides the canonical WithAdvisoryLock helper that
// MUST wrap every subscription-mutating database operation. Consumers in
// plangate, state-machine, downgrade-block, and webhook dispatch rely on it
// to serialize concurrent state transitions for a single store.
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/subscription/advisory_lock.go \
        services/marketplace-api/internal/subscription/advisory_lock_test.go
git commit -m "feat: add subscription.WithAdvisoryLock helper for serialized writes"
```

---

## Task 14: Audit-emit state-transition scaffold

**Files:**
- Modify: `services/marketplace-api/internal/audit/emitter.go` (add method; do not change existing API)
- Create: `services/marketplace-api/internal/audit/state_transition_test.go`

**Spec references:** §23.1 — "Every write emits structured event: created, plan change, status, card events, refund, promo, hard-delete, SSO config, add-on purchased/cancelled, app lifecycle transitions." P3 will call this from the state machine; P1 only provides the hook.

- [ ] **Step 1: Write failing test**

```go
//go:build integration

package audit_test

import (
    "context"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/audit"
)

// Stub recording emitter so we can verify the Event payload without depending on
// audit-service. If the production emitter needs a concrete dependency (Pub/Sub,
// HTTP client), use its test constructor (likely audit.NewForTesting()).
func TestEmitStateTransition_FillsActionAndMetadata(t *testing.T) {
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    c, _ := gin.CreateTestContext(nil)
    c.Request, _ = http.NewRequestWithContext(context.Background(), "POST", "/", nil)
    c.Set("tenant_id", uuid.New().String())
    c.Set("user_id", uuid.New().String())

    em.EmitStateTransition(c, audit.StateTransition{
        StoreID:    uuid.New(),
        From:       "trialing",
        To:         "active",
        Actor:      "system:webhook",
        Reason:     "invoice.paid",
    })

    // Drain any async queue.
    em.FlushForTesting()

    events := rec.Events()
    require.Len(t, events, 1)
    require.Equal(t, "subscription.state_transition", events[0].Action)
    require.Equal(t, "subscription", events[0].ResourceType)
    require.Equal(t, "trialing", events[0].Metadata["from_status"])
    require.Equal(t, "active", events[0].Metadata["to_status"])
    require.Equal(t, "invoice.paid", events[0].Metadata["reason"])
}
```

> Reviewer note: the exact recorder API depends on what's already exported by `internal/audit`. If `NewRecorderForTesting` and `FlushForTesting` aren't already exposed, open them (lightweight additions). If the emitter uses a channel-backed async queue, `FlushForTesting` should drain the channel deterministically.

- [ ] **Step 2: Run test — expect FAIL**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/audit/... -run TestEmitStateTransition -v
```

- [ ] **Step 3: Add `StateTransition` + `EmitStateTransition` to `emitter.go`**

Append to `internal/audit/emitter.go`:

```go
// StateTransition records a subscription state move for the audit trail.
// Populate Actor with either a user UUID (for merchant-driven transitions) or
// a sentinel like "system:webhook:stripe" / "system:cron:trial_expiry"
// (for automated transitions).
type StateTransition struct {
    StoreID     uuid.UUID
    TenantID    uuid.UUID // optional; if zero, Emit pulls from gin context
    From        string
    To          string
    Plan        string // current plan; useful for cross-state dashboards
    Actor       string // e.g. "system:webhook:stripe" or "user:<uuid>"
    Reason      string // e.g. "invoice.paid" or "merchant_cancelled"
    StripeEventID string
}

// EmitStateTransition is the canonical hook for every subscription state
// change (§23.1). It builds a standard Event and delegates to Emit.
func (e *Emitter) EmitStateTransition(c *gin.Context, t StateTransition) {
    md := map[string]any{
        "from_status": t.From,
        "to_status":   t.To,
        "actor":       t.Actor,
    }
    if t.Plan != "" {
        md["plan"] = t.Plan
    }
    if t.Reason != "" {
        md["reason"] = t.Reason
    }
    if t.StripeEventID != "" {
        md["stripe_event_id"] = t.StripeEventID
    }

    severity := SeverityInfo
    switch t.To {
    case "expired", "store_closed", "pending_hard_delete", "hard_deleted", "payment_action_required":
        severity = SeverityWarning
    }

    e.Emit(c, Event{
        Action:       "subscription.state_transition",
        ResourceType: "subscription",
        ResourceID:   t.StoreID.String(),
        Severity:     severity,
        Metadata:     md,
        TenantID:     t.TenantID,
        StoreID:      t.StoreID,
        ForceActorType: classifyActor(t.Actor),
    })
}

func classifyActor(actor string) ActorType {
    if strings.HasPrefix(actor, "system:") {
        return ActorSystem
    }
    return ActorUser
}
```

Confirm the existing `ActorSystem`/`ActorUser` constants exist; if not, substitute the right values from `audit.ActorType`.

- [ ] **Step 4: Run test — expect PASS**

```bash
go test -tags=integration ./internal/audit/... -run TestEmitStateTransition -v
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/audit/emitter.go \
        services/marketplace-api/internal/audit/state_transition_test.go
git commit -m "feat: add audit.EmitStateTransition scaffold for subscription lifecycle"
```

---

## Task 15: Security regression test — attestation DELETE + UPDATE rejected

**Files:**
- Create: `services/marketplace-api/internal/attestation/immutability_test.go`

**Spec references:** §19.3.1 + Success criterion 50 — "DELETE attempt by app DB user: rejected by role-level revoke (even if trigger dropped)."

This test needs to connect as a non-superuser role. We configure a dedicated test role in the test DB.

- [ ] **Step 1: Confirm a limited-role connection is available**

Check if `pkg/testdb` exposes a second DSN for the app-role user. If not, add `TEST_DATABASE_APP_URL` — a DSN connecting as `marketplace_user` (or whatever the production role is named). Local dev setup script `scripts/testdb-setup.sh` should grant that role `INSERT, SELECT, UPDATE` on all tables but NOT `DELETE` on `business_entity_attestations`.

If this infra isn't present, the test file should still compile and SKIP with a clear message:

```go
if os.Getenv("TEST_DATABASE_APP_URL") == "" {
    t.Skip("TEST_DATABASE_APP_URL not set — role-level revoke test requires limited-privilege user")
}
```

- [ ] **Step 2: Write the test**

```go
//go:build integration

package attestation_test

import (
    "os"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/attestation"
    "github.com/tesserix/marketplace-api/pkg/testdb"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func TestBusinessEntityAttestation_UpdateRejectedByTrigger(t *testing.T) {
    db := testdb.NewDB(t, "business_entity_attestations")
    row := attestation.BusinessEntityAttestation{
        StoreID: uuid.New(), TenantID: uuid.New(),
        Country: "US", CheckboxText: "attest", CheckboxVersion: "v1",
    }
    require.NoError(t, db.Create(&row).Error)

    err := db.Exec("UPDATE business_entity_attestations SET country='CA' WHERE id=?", row.ID).Error
    require.Error(t, err, "UPDATE must be rejected by trigger")
    require.Contains(t, err.Error(), "append-only")
}

func TestBusinessEntityAttestation_DeleteRejectedByRoleRevoke(t *testing.T) {
    appDSN := os.Getenv("TEST_DATABASE_APP_URL")
    if appDSN == "" {
        t.Skip("TEST_DATABASE_APP_URL not set — skipping role-level revoke test")
    }

    // Seed a row via superuser connection.
    admin := testdb.NewDB(t, "business_entity_attestations")
    row := attestation.BusinessEntityAttestation{
        StoreID: uuid.New(), TenantID: uuid.New(),
        Country: "US", CheckboxText: "attest", CheckboxVersion: "v1",
    }
    require.NoError(t, admin.Create(&row).Error)

    // Open a connection as the app role.
    appDB, err := gorm.Open(postgres.Open(appDSN), &gorm.Config{})
    require.NoError(t, err)

    // DELETE must be denied at the role level, NOT the trigger.
    err = appDB.Exec("DELETE FROM business_entity_attestations WHERE id=?", row.ID).Error
    require.Error(t, err)
    require.Contains(t, err.Error(), "permission denied")
}
```

- [ ] **Step 3: Run — expect PASS (UPDATE) + PASS or SKIP (DELETE)**

```bash
go test -tags=integration ./internal/attestation/... -v
```

- [ ] **Step 4: Document the role setup**

Add a README note to `services/marketplace-api/pkg/testdb/README.md`:

```markdown
### Role-level revoke tests

Some security tests (e.g. `business_entity_attestations` DELETE rejection)
need a non-superuser connection. Set `TEST_DATABASE_APP_URL` to a DSN whose
role has been granted only `SELECT, INSERT, UPDATE` on the table — NOT
`DELETE`. Migration 000043 applies the `REVOKE DELETE` during migration.
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/attestation/immutability_test.go \
        services/marketplace-api/pkg/testdb/README.md
git commit -m "test(security): attestation UPDATE + DELETE rejected by trigger/revoke"
```

---

## Task 16: Schema-version bump + CI verification

**Files:**
- Modify: `services/marketplace-api/marketplaceapi.go` (const `ExpectedSchemaVersion`)

Per exploration step 9, the service boot asserts the schema version. We must bump the constant after all migrations land.

- [ ] **Step 1: Update the constant**

In `services/marketplace-api/marketplaceapi.go`:

```go
// Before:
// const ExpectedSchemaVersion = 37

// After:
const ExpectedSchemaVersion = 46
```

- [ ] **Step 2: Local smoke test — `main.go` boots with fresh DB**

```bash
cd services/marketplace-api
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
go build ./...
```
Expected: no version-mismatch panic if you start the service pointing at the test DB.

- [ ] **Step 3: Run the full test suite**

```bash
go test -tags=integration ./... -count=1
```
Expected: all tests pass. If any module-level test broke on plan-enum rename, fix it now (most likely in `internal/plangate/gate_test.go`). Keep fixes minimal — P3 will rewrite that file.

- [ ] **Step 4: Verify CI runs the role-level revoke test (Task 15) — not silently skipped**

Task 15 guards `TestBusinessEntityAttestation_DeleteRejectedByRoleRevoke` with `t.Skip` when `TEST_DATABASE_APP_URL` is unset. On main, this test MUST run — otherwise the security regression from §19.3.1 can regress unnoticed.

Inspect `.github/workflows/ci.yml` (or the marketplace-api workflow that runs integration tests). Confirm `TEST_DATABASE_APP_URL` is set in the env block for the integration-test step; if not, add it pointing at the same CNPG instance but with a role that has `SELECT, INSERT, UPDATE` on `business_entity_attestations` and NOT `DELETE`. Set the role up in the same `scripts/testdb-setup.sh` referenced in Task 15.

If the CI workflow change needs a separate PR, capture a TODO here and open a follow-up issue — do not merge this plan's PR without that follow-up tracked.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/marketplaceapi.go \
        .github/workflows/ci.yml scripts/testdb-setup.sh  # only if touched
git commit -m "chore: bump ExpectedSchemaVersion to 46 (v2.3 subscription foundation)"
```

---

## Final verification

After all 16 tasks:

- [ ] All 8 migrations apply cleanly from a fresh DB; `down` reverses cleanly.
- [ ] Full `go test -tags=integration ./...` green.
- [ ] `go build ./...` clean.
- [ ] `git log --oneline | head -20` shows a linear, clearly-labelled history.
- [ ] No lingering references to `PlanFree`, `PlanEnterprise`, `StatusCancelled`, `StatusIncomplete` in the codebase:
  ```bash
  grep -R "PlanFree\|PlanEnterprise\|StatusCancelled\|StatusIncomplete" services/marketplace-api/ || echo "clean"
  ```

## What's now unlocked

- **P2** can implement `currency_options` Price objects and webhook dispatch against the new `stripe_webhook_events` table and the extended `StoreSubscription` struct.
- **P3** can rewrite `plangate.featureMatrix` against the new 4-plan set, use `WithAdvisoryLock` for state-machine writes, and call `EmitStateTransition` on every move.
- **P6/P7** can land geo-arbitrage and tax-ID validation on the tables prepared here.
- Every downstream plan benefits from the `GetByStoreID` tenant fix.

## Execution handoff

Plan complete. Two execution options once you're ready to run it:

**1. Subagent-Driven (recommended)** — fresh subagent per task with a review checkpoint between each
**2. Inline Execution** — run tasks in the current session with checkpoints

**REQUIRED SUB-SKILL:** Use `superpowers:subagent-driven-development` OR `superpowers:executing-plans`.
