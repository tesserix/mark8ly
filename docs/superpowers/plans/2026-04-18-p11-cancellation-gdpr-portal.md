# P11 — Cancellation Flow + Save-Offer + Post-Cancellation Lifecycle + Win-Back + GDPR Customer Portal

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the full merchant cancellation + post-cancellation lifecycle as a deterministic pipeline of state-machine transitions driven by four idempotent crons, plus the GDPR customer-subject data portal (order history + erasure request) that satisfies Art. 17 / Art. 20 rights on closed or deleted stores. The save-offer is strictly prospective: the current period is never credited — the promo applies to the *next* invoice only (Success Criterion #54).

**Architecture:** A new `internal/subscription/cancel` package owns the merchant-facing endpoint (`POST /admin/stores/:storeId/subscription/cancel`). It accepts an optional survey reason and an optional `accept_save_offer` bool. Save-offer acceptance funnels through P10's promo service with `prospective_only=true` and drives `statemachine.Transition(cancel_scheduled → active)` as a reversal, or on rejection drives `statemachine.Transition(active → cancel_scheduled)` setting `cancels_at = current_period_end`. A new `internal/subscription/lifecycle` package registers four idempotent daily crons against the existing cron runner (audit-service pattern): 01:00 UTC finalize cancellation (`cancel_scheduled → expired`), 01:30 UTC 14-day closure (`expired → store_closed`), 02:00 UTC 90-day hard-delete queue (`store_closed → pending_hard_delete`), 03:00 UTC 150-day hard delete (`pending_hard_delete → hard_deleted`) — every one of them is a pure function of `(time, subscription row)`, so double-runs are no-ops under P3's CAS. A new `internal/subscription/harddelete` package owns the destructive step: calls P10's `billing_archive.Builder.Build(ctx, storeID)` to freeze the billing ledger into the archive table from P1, calls Stripe `Customers.Del(customerID)`, then runs the tenant-scoped DELETE sweep across `orders, customers, products, stores, media` (five tables; audit emit per sweep), emits `subscription.hard_deleted`, and finally transitions `pending_hard_delete → hard_deleted`. A fifth cron at 10:00 UTC drives win-back: subscriptions where `status=expired` and `cancels_at` was exactly 30 days ago receive a 20%-off-6-months promo email via P10's promo service + the notification service. The GDPR portal lives in a new `internal/customerportal` package: a DB migration adds a per-store HMAC secret `storefront_customer_portal_secret`, `GET /my-orders/:email/:order_token` verifies `HMAC-SHA256(secret, email||order_id)` and returns order history (unauth; the token *is* the auth), and `POST /customer-erasure` appends to a new `customer_erasure_requests` table that routes to Mark8ly's support queue — merchants cannot touch it. Pro+App hook: on the finalize transition (`cancel_scheduled → expired`), if `has_white_label_app_add_on=true`, emit `subscription.pro_app_cancelled`; P15 listens, P11 only publishes.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `github.com/robfig/cron/v3` (already used by audit-service), `github.com/stripe/stripe-go/v76` (P2), existing P1 `billing_archive` table + P1 `subscription.WithAdvisoryLock`, P3 `statemachine.Transition`, P3 `audit.EmitStateTransition`, P10 promo service + billing-archive builder.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §15 (cancellation), §15.1 (save-offer prospective-only), §15.2 (post-cancellation lifecycle), §15.3 (win-back), §15.4 (GDPR customer-subject data portal), §15.5 (Pro+App cancellation), §17.2 (transitions).

**Depends on:**
- **P1** — state enum, `billing_archive` table, `WithAdvisoryLock`
- **P2** — Stripe client (customer-delete), webhook audit
- **P3** — `statemachine.Transition`, `audit.EmitStateTransition`, `ReadOnlyStatuses`
- **P10** — promo service (apply + validate floor), `billing_archive.Builder`

**Related plans:**
- **P12** (Cloudflare Worker closed.html) — consumes the `store_closed` status flipped by this plan's 01:30 cron
- **P15** (white-label app teardown) — consumes `subscription.pro_app_cancelled` event emitted here
- **P16** (admin frontend) — builds the cancel dialog, save-offer UI, and billing-archive view
- **P17** (observability) — alerts on cron failure + hard-delete failure

---

## Scope Check

In scope:
1. `POST /admin/stores/:storeId/subscription/cancel` — handler + save-offer branch + state transitions
2. Save-offer prospective-only contract: discount attaches to Stripe subscription effective *next* invoice period, never a credit on the current period (Success Criterion #54).
3. Four lifecycle crons — finalize, 14-day, 90-day, 150-day — all idempotent, all routed through `statemachine.Transition`.
4. Hard-delete pipeline: billing archive build → Stripe customer delete → tenant-scoped DELETE sweep → audit emit → transition.
5. Win-back cron at day-30 post-expiry — 20%-off-6-months promo email.
6. DB migration adding `storefront_customer_portal_secret CHAR(64)` to `stores`.
7. `GET /my-orders/:email/:order_token` storefront endpoint — no auth; HMAC token is the auth; 90-day window.
8. `POST /customer-erasure` — appends to `customer_erasure_requests`; routes to Mark8ly support queue.
9. Pro+App `subscription.pro_app_cancelled` event emit on finalize transition.

Out of scope:
- Admin UI / merchant-facing cancel dialog + survey UI → **P16**
- Storefront closed.html / 410 Gone page rendering → **P12**
- Refund flow (separate from cancel; always dominates in 14-day window) → **P10** owns
- Tax-revalidation unpublish path → **P7** owns
- White-label app teardown *internals* (Apple/Google credential rotation + secret delete) → **P15** owns (this plan only emits the event)
- GDPR portal HTML/email template design → **P16** (this plan returns JSON; HTML render is UI concern)
- Actual Mark8ly support queue tooling — this plan only writes rows to `customer_erasure_requests`; triage flow is operational

---

## File Structure

### Create

- `services/marketplace-api/internal/subscription/cancel/handler.go`
- `services/marketplace-api/internal/subscription/cancel/handler_test.go`
- `services/marketplace-api/internal/subscription/cancel/service.go`
- `services/marketplace-api/internal/subscription/cancel/service_test.go`
- `services/marketplace-api/internal/subscription/lifecycle/crons.go` — registers 4 crons + win-back
- `services/marketplace-api/internal/subscription/lifecycle/crons_test.go`
- `services/marketplace-api/internal/subscription/lifecycle/finalize.go` — cancel_scheduled → expired
- `services/marketplace-api/internal/subscription/lifecycle/close.go` — expired → store_closed (14d)
- `services/marketplace-api/internal/subscription/lifecycle/queue_hard_delete.go` — store_closed → pending_hard_delete (90d)
- `services/marketplace-api/internal/subscription/lifecycle/winback.go` — day-30 email
- `services/marketplace-api/internal/subscription/harddelete/runner.go` — pending_hard_delete → hard_deleted
- `services/marketplace-api/internal/subscription/harddelete/runner_test.go`
- `services/marketplace-api/internal/subscription/harddelete/sweeper.go` — tenant-scoped DELETE across tables
- `services/marketplace-api/internal/customerportal/handler.go`
- `services/marketplace-api/internal/customerportal/handler_test.go`
- `services/marketplace-api/internal/customerportal/token.go` — HMAC construct + verify
- `services/marketplace-api/internal/customerportal/erasure.go`
- `services/marketplace-api/db/migrations/20260418_customer_portal.sql`

### Modify

- `services/marketplace-api/cmd/marketplace-api/main.go` — wire cancel handler, crons, customer portal router group
- `services/marketplace-api/internal/handlers/admin/routes.go` — mount `POST /admin/stores/:storeId/subscription/cancel`
- `services/marketplace-api/internal/handlers/storefront/routes.go` — mount `/my-orders/:email/:order_token` + `/customer-erasure`
- `services/marketplace-api/internal/subscription/statemachine/transitions.go` — add `Metadata` carry-through field if not already present (for `pro_app_cancelled` hook); P3 already defines the transition table, this is a pure additive field on `TransitionInput`

### Delete

- None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Customer-portal DB migration (secret + erasure table) | — |
| 2 | Cancel service + handler (reject + survey + save-offer) | P3, P10 |
| 3 | Save-offer prospective-only — Success Criterion #54 test | 2, P10 |
| 4 | Finalize cron: `cancel_scheduled → expired` + Pro+App event emit | 2, P3 |
| 5 | 14-day cron: `expired → store_closed` | 4, P3 |
| 6 | 90-day cron: `store_closed → pending_hard_delete` | 5, P3 |
| 7 | Hard-delete runner: billing archive + Stripe delete + sweep | 6, P1, P2, P10 |
| 8 | 150-day cron: invokes hard-delete runner | 7, P3 |
| 9 | Win-back cron: day-30 email with promo | 4, P10 |
| 10 | Customer portal: HMAC token + order-history endpoint | 1 |
| 11 | Customer erasure endpoint: append-only request table | 1 |
| 12 | End-to-end lifecycle integration test + final verification | all |

---

## Reusable patterns

**A. Every transition goes through P3's `statemachine.Transition`.** No direct `UPDATE store_subscriptions SET status=...` anywhere in this plan. Crons call `Transition`; the handler calls `Transition`; the hard-delete runner calls `Transition`. Idempotent retries are safe because `Transition` returns `ErrCASConflict` on a racing writer, which the caller treats as "already transitioned — skip."

**B. Every destructive sweep and every transition wraps in `subscription.WithAdvisoryLock(ctx, db, storeID, fn)`.** This is the P1 Postgres session-level lock keyed by `storeID`. Two crons racing on the same store serialize; the loser sees CAS conflict and skips.

**C. Cron shape — pure function of `(now, row)`.** Every lifecycle cron is:
```go
// Pseudocode
rows := repo.FindEligible(ctx, db, now)   // pure SQL: time + status filter
for each row:
    _ = statemachine.Transition(ctx, TransitionInput{...})  // CAS-guarded
```
No in-memory state. Restarting the cron mid-run or running it twice at 01:00:00 and 01:00:03 produces the same end state.

**D. Save-offer prospective contract.** The handler NEVER issues a Stripe refund, NEVER calls `InvoiceItem.Create` with a negative amount, and NEVER touches `subscription.current_period_end`. It calls P10's `promoService.Apply(ctx, ApplyInput{SubscriptionID: stripeSubID, ProspectiveOnly: true, DiscountPct: 50, Cycles: 3})` which in turn creates a Stripe `Coupon` + `SubscriptionSchedule` phase that attaches from the next billing period. The current invoice is untouched.

**E. Audit envelope.** Every cron run + hard-delete sweep emits via P3's `audit.EmitStateTransition` with `actor="system:cron"` and `reason` set to the cron's name (e.g. `"lifecycle:finalize_cancel"`). Hard-delete sweep additionally emits per-table `audit.Emit` with counts for `orders|customers|products|stores|media`.

**F. HMAC token construction (customer portal).** Token is `hex(hmac_sha256(store.storefront_customer_portal_secret, email||"|"||order_id))`. Secret is per-store (32 random bytes at store creation), stored server-side only; email + order_id are included in URL path params so the token is self-contained. Verification is constant-time (`hmac.Equal`). No expiry in the token itself — expiry is enforced by the 90-day-post-closure window check on the subscription state.

**G. Append-only erasure table.** `customer_erasure_requests` has no UPDATE path — handler only INSERTs. No admin endpoint exposes delete/modify. Mark8ly support reads via read-only DB role.

---

## Task 1: Customer portal DB migration

**Files:**
- Create: `services/marketplace-api/db/migrations/20260418_customer_portal.sql`

**Spec references:** §15.4.

- [ ] **Step 1: Write failing test — columns exist after migration**

```go
// services/marketplace-api/db/migrations/customer_portal_test.go
//go:build integration

func TestMigration_CustomerPortal_AddsSecretAndErasureTable(t *testing.T) {
    db := testdb.NewDB(t)

    var col struct{ Count int }
    require.NoError(t, db.Raw(`
      SELECT count(*) FROM information_schema.columns
      WHERE table_name='stores' AND column_name='storefront_customer_portal_secret'
    `).Scan(&col).Error)
    require.Equal(t, 1, col.Count)

    // Secret has CHAR(64) length + NOT NULL
    var info struct{ DataType string; Nullable string; MaxLen int }
    require.NoError(t, db.Raw(`
      SELECT data_type as data_type, is_nullable as nullable, character_maximum_length as max_len
      FROM information_schema.columns
      WHERE table_name='stores' AND column_name='storefront_customer_portal_secret'
    `).Scan(&info).Error)
    require.Equal(t, "character", info.DataType)
    require.Equal(t, "NO", info.Nullable)
    require.Equal(t, 64, info.MaxLen)

    // Erasure table exists + append-only (no UPDATE trigger allowed on existing rows)
    require.NoError(t, db.Raw(`SELECT 1 FROM customer_erasure_requests LIMIT 0`).Error)
}

func TestMigration_ExistingStores_BackfilledWithRandomSecret(t *testing.T) {
    db := testdb.NewDB(t)
    // insert a store, run migration-up, verify secret populated
    // ... (project migration harness)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd services/marketplace-api
go test -tags=integration ./db/migrations/... -run CustomerPortal -v
```

- [ ] **Step 3: Write `20260418_customer_portal.sql`**

```sql
-- up
ALTER TABLE stores
  ADD COLUMN storefront_customer_portal_secret CHAR(64) NOT NULL
    DEFAULT encode(gen_random_bytes(32), 'hex');

-- Backfill already handled by DEFAULT for existing rows.
-- Drop the DEFAULT so every future insert must explicitly compute a secret
-- (keeps creation path auditable — §15.4).
ALTER TABLE stores ALTER COLUMN storefront_customer_portal_secret DROP DEFAULT;

CREATE TABLE customer_erasure_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL,
    store_id         UUID NOT NULL,
    customer_email   TEXT NOT NULL,
    reason           TEXT,
    requester_ip     TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    support_ticket_id TEXT,  -- nullable; set by Mark8ly support on triage

    CONSTRAINT customer_erasure_requests_email_nonempty CHECK (length(customer_erasure_requests.customer_email) > 0)
);

CREATE INDEX idx_customer_erasure_store_email
    ON customer_erasure_requests (store_id, customer_email);

-- Append-only enforcement: revoke UPDATE/DELETE from the app role.
-- Support-team role retains SELECT/UPDATE for triage.
REVOKE UPDATE, DELETE ON customer_erasure_requests FROM marketplace_api_app;

-- down
DROP TABLE customer_erasure_requests;
ALTER TABLE stores DROP COLUMN storefront_customer_portal_secret;
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/db/migrations/20260418_customer_portal.sql \
        services/marketplace-api/db/migrations/customer_portal_test.go
git commit -m "feat(db): customer-portal secret on stores + append-only erasure table (§15.4)"
```

---

## Task 2: Cancel service + handler

**Files:**
- Create: `services/marketplace-api/internal/subscription/cancel/service.go`
- Create: `services/marketplace-api/internal/subscription/cancel/service_test.go`
- Create: `services/marketplace-api/internal/subscription/cancel/handler.go`
- Create: `services/marketplace-api/internal/subscription/cancel/handler_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Spec references:** §15.1.

- [ ] **Step 1: Write failing tests — reject save-offer vs accept save-offer**

```go
func TestCancel_RejectSaveOffer_TransitionsActiveToCancelScheduled(t *testing.T) {
    svc, h := newCancelTestRig(t, subscription.StatusActive)
    req := cancel.Request{SurveyReason: "too_expensive", AcceptSaveOffer: false}

    resp, err := svc.Cancel(ctx, h.tenantID, h.storeID, "user:merchant@x.com", req)

    require.NoError(t, err)
    require.Equal(t, "cancel_scheduled", resp.Status)
    require.NotZero(t, resp.CancelsAt)
    require.Equal(t, h.currentPeriodEnd, resp.CancelsAt,
        "cancels_at must equal current_period_end (§15.1)")

    // No Stripe coupon attached.
    require.Empty(t, h.stripeMock.CouponsCreated)
}

func TestCancel_AcceptSaveOffer_TransitionsCancelScheduledBackToActive_ProspectiveOnly(t *testing.T) {
    // Start from cancel_scheduled (merchant previously hit cancel, now accepts save offer).
    svc, h := newCancelTestRig(t, subscription.StatusCancelScheduled)
    req := cancel.Request{AcceptSaveOffer: true}

    resp, err := svc.Cancel(ctx, h.tenantID, h.storeID, "user:m@x.com", req)

    require.NoError(t, err)
    require.Equal(t, "active", resp.Status)

    // P10 promo service called with prospective_only=true
    require.Len(t, h.promoMock.ApplyCalls, 1)
    require.True(t, h.promoMock.ApplyCalls[0].ProspectiveOnly)
    require.Equal(t, 50, h.promoMock.ApplyCalls[0].DiscountPct)
    require.Equal(t, 3,  h.promoMock.ApplyCalls[0].Cycles)

    // Current period is NOT credited — no invoice item, no refund.
    require.Empty(t, h.stripeMock.InvoiceItemsCreated,
        "save-offer must not credit current period (Success Criterion #54)")
    require.Empty(t, h.stripeMock.RefundsCreated)
}

func TestCancel_AcceptSaveOffer_FromActive_IsIdempotent(t *testing.T) {
    // Accepting save offer from active (never scheduled cancel) — no-op; return active.
    svc, h := newCancelTestRig(t, subscription.StatusActive)
    req := cancel.Request{AcceptSaveOffer: true}

    resp, err := svc.Cancel(ctx, h.tenantID, h.storeID, "user:m@x.com", req)

    require.NoError(t, err)
    require.Equal(t, "active", resp.Status)
    require.Len(t, h.promoMock.ApplyCalls, 1, "promo still applies prospectively")
}

func TestCancel_FromExpired_Rejected(t *testing.T) {
    svc, h := newCancelTestRig(t, subscription.StatusExpired)
    _, err := svc.Cancel(ctx, h.tenantID, h.storeID, "user:m", cancel.Request{})
    require.ErrorIs(t, err, statemachine.ErrInvalidTransition)
}
```

- [ ] **Step 2: Run — expect FAIL (package doesn't exist)**

- [ ] **Step 3: Write `service.go`**

```go
package cancel

import (
    "context"
    "time"

    "github.com/tesserix/marketplace-api/internal/promo"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

// Request is the merchant-facing cancel payload. All fields optional.
type Request struct {
    SurveyReason    string `json:"survey_reason,omitempty"`
    AcceptSaveOffer bool   `json:"accept_save_offer,omitempty"`
}

// Response echoes the resulting state + the billing cliff (if scheduled).
type Response struct {
    Status    string    `json:"status"`
    CancelsAt time.Time `json:"cancels_at,omitempty"`
}

type Service struct {
    DB       GormDB
    Repo     subscription.Repository
    Promo    promo.Service          // from P10
    Emitter  statemachine.Emitter   // P3 audit
}

// Cancel is the single entry point for merchant-driven cancellation.
// Routes:
//   (active | trialing, accept=false) → cancel_scheduled (set cancels_at)
//   (active | trialing, accept=true)  → active  (+ prospective promo)
//   (cancel_scheduled,  accept=false) → no-op (already scheduled)
//   (cancel_scheduled,  accept=true)  → active  (+ prospective promo, reversal)
//   anything else                     → ErrInvalidTransition
func (s *Service) Cancel(ctx context.Context, tenantID, storeID uuid.UUID, actor string, req Request) (Response, error) {
    var out Response
    err := subscription.WithAdvisoryLock(ctx, s.DB, storeID, func(tx GormDB) error {
        sub, err := s.Repo.GetByStoreID(ctx, tx, tenantID, storeID)
        if err != nil { return err }

        if req.AcceptSaveOffer {
            // Prospective promo — attaches starting next invoice; current period untouched.
            if _, err := s.Promo.Apply(ctx, promo.ApplyInput{
                SubscriptionID:  sub.StripeSubscriptionID,
                ProspectiveOnly: true,
                DiscountPct:     50,
                Cycles:          3,
                Reason:          "save_offer",
            }); err != nil {
                return err
            }
            // Reversal (cancel_scheduled → active) — or stays active.
            if sub.Status == subscription.StatusCancelScheduled {
                if err := statemachine.Transition(ctx, statemachine.TransitionInput{
                    DB: tx, Emitter: s.Emitter, TenantID: tenantID, StoreID: storeID,
                    From: subscription.StatusCancelScheduled,
                    To:   subscription.StatusActive,
                    Actor: actor, Reason: "save_offer_accepted",
                    Metadata: map[string]any{"survey_reason": req.SurveyReason},
                }); err != nil {
                    return err
                }
            }
            out = Response{Status: string(subscription.StatusActive)}
            return nil
        }

        // Reject path — schedule cancellation if currently active/trialing.
        if sub.Status == subscription.StatusCancelScheduled {
            out = Response{Status: string(subscription.StatusCancelScheduled), CancelsAt: sub.CancelsAt}
            return nil // idempotent no-op
        }
        if err := statemachine.Transition(ctx, statemachine.TransitionInput{
            DB: tx, Emitter: s.Emitter, TenantID: tenantID, StoreID: storeID,
            From: sub.Status,
            To:   subscription.StatusCancelScheduled,
            Actor: actor, Reason: "merchant_cancel",
            Metadata: map[string]any{"survey_reason": req.SurveyReason},
        }); err != nil {
            return err
        }
        if err := tx.Model(&subscription.StoreSubscription{}).
            Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
            Update("cancels_at", sub.CurrentPeriodEnd).Error; err != nil {
            return err
        }
        out = Response{Status: string(subscription.StatusCancelScheduled), CancelsAt: sub.CurrentPeriodEnd}
        return nil
    })
    return out, err
}
```

- [ ] **Step 4: Write `handler.go`**

```go
package cancel

import "github.com/gin-gonic/gin"

func Handler(svc *Service) gin.HandlerFunc {
    return func(c *gin.Context) {
        tenantID := ctxutil.MustTenantID(c)
        storeID  := ctxutil.MustStoreID(c)
        actor    := "user:" + ctxutil.MustUserEmail(c)

        var req Request
        if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
            c.AbortWithStatusJSON(400, gin.H{"error":"bad_request","message":err.Error()})
            return
        }
        resp, err := svc.Cancel(c.Request.Context(), tenantID, storeID, actor, req)
        switch {
        case errors.Is(err, statemachine.ErrInvalidTransition):
            c.JSON(409, gin.H{"error":"invalid_state", "message":"cancel not allowed from current state"})
        case err != nil:
            log.WithError(err).Error("cancel_failed")
            c.JSON(500, gin.H{"error":"cancel_failed"})
        default:
            c.JSON(200, resp)
        }
    }
}
```

- [ ] **Step 5: Wire route in `routes.go`**

```go
// In admin store-scoped group, AFTER RequireActive (which allowlists subscription/*)
storeRoute.POST("/subscription/cancel", cancel.Handler(deps.CancelService))
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/subscription/cancel/ \
        services/marketplace-api/internal/handlers/admin/routes.go
git commit -m "feat(subscription): merchant cancel + save-offer handler (§15.1)"
```

---

## Task 3: Save-offer prospective-only — Success Criterion #54 integration test

**Files:**
- Create: `services/marketplace-api/internal/subscription/cancel/save_offer_prospective_test.go`

**Purpose:** Success Criterion #54 — "Save-offer acceptance mid-cycle: current period NOT credited; next invoice applies discount."

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestSaveOffer_MidCycle_CurrentPeriodNotCredited_NextInvoiceDiscounted(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanStarter)

    // Seed a Stripe subscription whose current_period_start is 10 days ago
    // and current_period_end is 20 days from now — i.e. mid-cycle.
    sub := suite.SeedStripeSub(storeID, "cus_test", "sub_test",
        /*periodStart*/ time.Now().Add(-10*24*time.Hour),
        /*periodEnd*/   time.Now().Add( 20*24*time.Hour))

    // Step 1: merchant cancels (reject save offer first).
    resp := suite.AdminPOST(tenantID, storeID, "/subscription/cancel",
        cancel.Request{AcceptSaveOffer: false, SurveyReason: "too_expensive"})
    require.Equal(t, 200, resp.Code)
    require.Contains(t, resp.Body.String(), "cancel_scheduled")

    // Step 2: merchant changes mind, accepts save offer.
    resp = suite.AdminPOST(tenantID, storeID, "/subscription/cancel",
        cancel.Request{AcceptSaveOffer: true})
    require.Equal(t, 200, resp.Code)
    require.Contains(t, resp.Body.String(), "active")

    // ASSERT 1: No Stripe invoice-item was created on current period.
    require.Empty(t, suite.StripeMock.InvoiceItemsCreated(sub.StripeSubscriptionID),
        "no invoice item — current period must not be credited")
    require.Empty(t, suite.StripeMock.RefundsCreated(sub.StripeCustomerID),
        "no refund — current period charge stands")

    // ASSERT 2: A SubscriptionSchedule phase was created effective at period_end.
    schedules := suite.StripeMock.SchedulesCreated(sub.StripeSubscriptionID)
    require.Len(t, schedules, 1)
    require.Equal(t, sub.CurrentPeriodEnd.Unix(), schedules[0].Phases[0].StartDate,
        "discount phase starts at current period end (next invoice)")
    require.Equal(t, 50, schedules[0].Phases[0].DiscountPct)
    require.Equal(t, 3,  schedules[0].Phases[0].Iterations)

    // ASSERT 3: Final state is active (reversal succeeded).
    updated, _ := suite.Repo.GetByStoreID(ctx, suite.DB, tenantID, storeID)
    require.Equal(t, subscription.StatusActive, updated.Status)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/cancel/save_offer_prospective_test.go
git commit -m "test(cancel): save-offer prospective-only — Success Criterion #54"
```

---

## Task 4: Finalize cron — `cancel_scheduled → expired` + Pro+App event

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/finalize.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/finalize_test.go`

**Spec references:** §15.2, §15.5.

- [ ] **Step 1: Write failing test**

```go
func TestFinalize_TransitionsCancelScheduledToExpired_OnceCancelsAtPassed(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    eventPub := eventpub.NewRecordingPublisher()

    now := time.Date(2026, 5, 1, 1, 0, 0, 0, time.UTC)
    tenantID, storeID := uuid.New(), uuid.New()

    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        Plan: subscription.PlanStarter, Status: subscription.StatusCancelScheduled,
        CancelsAt: now.Add(-1 * time.Hour), // past
        HasWhiteLabelAppAddOn: false,
    }).Error)

    j := lifecycle.NewFinalizeJob(db, em, eventPub, clockAt(now))
    require.NoError(t, j.Run(context.Background()))

    var sub subscription.StoreSubscription
    _ = db.Where("store_id=?", storeID).First(&sub).Error
    require.Equal(t, subscription.StatusExpired, sub.Status)

    // No Pro+App event — add-on not set.
    require.Empty(t, eventPub.Of("subscription.pro_app_cancelled"))
}

func TestFinalize_EmitsProAppEvent_WhenAddOnPresent(t *testing.T) {
    db, em, eventPub := setupFinalizeRig(t)
    tenantID, storeID := uuid.New(), uuid.New()
    _ = db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID,
        Plan: subscription.PlanPro, Status: subscription.StatusCancelScheduled,
        CancelsAt: time.Now().Add(-time.Hour),
        HasWhiteLabelAppAddOn: true,
    }).Error

    require.NoError(t, lifecycle.NewFinalizeJob(db, em, eventPub, clockNow()).Run(ctx))

    events := eventPub.Of("subscription.pro_app_cancelled")
    require.Len(t, events, 1)
    require.Equal(t, storeID.String(), events[0].Attrs["store_id"])
}

func TestFinalize_SkipsFutureCancelsAt(t *testing.T) {
    // cancels_at still in the future — cron must be a no-op.
    // ...
}

func TestFinalize_Idempotent_DoubleRun_NoExtraTransitions(t *testing.T) {
    // Running the cron twice only emits one state-transition audit event per row.
    // ...
}
```

- [ ] **Step 2: Write `finalize.go`**

```go
package lifecycle

import (
    "context"
    "time"
)

type FinalizeJob struct {
    DB       GormDB
    Emitter  statemachine.Emitter
    Events   eventpub.Publisher
    Clock    func() time.Time
}

const finalizeReason = "lifecycle:finalize_cancel"

func (j *FinalizeJob) Run(ctx context.Context) error {
    now := j.Clock()
    var rows []subscription.StoreSubscription
    if err := j.DB.WithContext(ctx).
        Where("status = ? AND cancels_at IS NOT NULL AND cancels_at < ?",
              subscription.StatusCancelScheduled, now).
        Limit(500).Find(&rows).Error; err != nil {
        return err
    }

    for _, r := range rows {
        err := subscription.WithAdvisoryLock(ctx, j.DB, r.StoreID, func(tx GormDB) error {
            if err := statemachine.Transition(ctx, statemachine.TransitionInput{
                DB: tx, Emitter: j.Emitter,
                TenantID: r.TenantID, StoreID: r.StoreID,
                From: subscription.StatusCancelScheduled, To: subscription.StatusExpired,
                Actor: "system:cron", Reason: finalizeReason,
            }); err != nil {
                if errors.Is(err, statemachine.ErrCASConflict) { return nil } // already done
                return err
            }
            if r.HasWhiteLabelAppAddOn {
                return j.Events.Publish(ctx, messaging.Event{
                    Type: "subscription.pro_app_cancelled",
                    Attrs: map[string]string{
                        "tenant_id": r.TenantID.String(),
                        "store_id":  r.StoreID.String(),
                    },
                })
            }
            return nil
        })
        if err != nil {
            log.WithError(err).WithField("store_id", r.StoreID).Error("finalize_failed")
            // Continue to next row; per-row isolation.
        }
    }
    return nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/finalize.go \
        services/marketplace-api/internal/subscription/lifecycle/finalize_test.go
git commit -m "feat(lifecycle): daily finalize cron — cancel_scheduled→expired + Pro+App event (§15.2/§15.5)"
```

---

## Task 5: 14-day cron — `expired → store_closed`

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/close.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/close_test.go`

**Spec references:** §15.2, §17.2 (transition codified in P3).

- [ ] **Step 1: Failing test**

```go
func TestClose_TransitionsExpiredToStoreClosed_After14Days(t *testing.T) {
    db, em := setupCloseRig(t)
    tenantID, storeID := uuid.New(), uuid.New()

    expiredAt := time.Now().Add(-15 * 24 * time.Hour) // 15 days ago
    _ = db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, Status: subscription.StatusExpired,
        StatusUpdatedAt: expiredAt,
    }).Error

    require.NoError(t, lifecycle.NewCloseJob(db, em, clockNow()).Run(ctx))

    var sub subscription.StoreSubscription
    _ = db.Where("store_id=?", storeID).First(&sub).Error
    require.Equal(t, subscription.StatusStoreClosed, sub.Status)
}

func TestClose_DoesNotFireBefore14Days(t *testing.T) {
    // expired 13 days ago → remains expired
    // ...
}
```

- [ ] **Step 2: Write `close.go`** — identical shape to `finalize.go`; SQL predicate differs:

```go
where := "status = ? AND status_updated_at < ?"
args  := []any{subscription.StatusExpired, now.Add(-14 * 24 * time.Hour)}
// transition: expired → store_closed
// reason: "lifecycle:close_14d"
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/close.go \
        services/marketplace-api/internal/subscription/lifecycle/close_test.go
git commit -m "feat(lifecycle): daily 14-day cron — expired→store_closed (§15.2)"
```

---

## Task 6: 90-day cron — `store_closed → pending_hard_delete`

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/queue_hard_delete.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/queue_hard_delete_test.go`

**Spec references:** §15.2.

- [ ] **Step 1: Failing test — 90 days post-expiry = 76 days in `store_closed`**

```go
func TestQueueHardDelete_Fires_76DaysAfterClose(t *testing.T) {
    // closed 76 days ago → pending_hard_delete
    // ...
}

func TestQueueHardDelete_NoopWhenRecent(t *testing.T) {
    // closed 75 days ago → still store_closed
    // ...
}
```

- [ ] **Step 2: Write `queue_hard_delete.go`** — same shape:

```go
where := "status = ? AND status_updated_at < ?"
args  := []any{subscription.StatusStoreClosed, now.Add(-76 * 24 * time.Hour)}
// transition: store_closed → pending_hard_delete
// reason: "lifecycle:queue_hard_delete_day90"
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/queue_hard_delete.go \
        services/marketplace-api/internal/subscription/lifecycle/queue_hard_delete_test.go
git commit -m "feat(lifecycle): 90-day cron — store_closed→pending_hard_delete (§15.2)"
```

---

## Task 7: Hard-delete runner

**Files:**
- Create: `services/marketplace-api/internal/subscription/harddelete/runner.go`
- Create: `services/marketplace-api/internal/subscription/harddelete/runner_test.go`
- Create: `services/marketplace-api/internal/subscription/harddelete/sweeper.go`

**Spec references:** §15.2 (150-day hard-delete), §23 (billing archive).

- [ ] **Step 1: Write failing test — full pipeline**

```go
//go:build integration

func TestHardDelete_FullPipeline(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedPendingHardDelete(150 * 24 * time.Hour) // 150d ago
    suite.SeedOrders(tenantID, storeID, 5)
    suite.SeedCustomers(tenantID, storeID, 3)
    suite.SeedProducts(tenantID, storeID, 8)
    suite.SeedMedia(tenantID, storeID, 12)
    stripeCustomerID := suite.MustStripeCustomer(storeID)

    r := harddelete.NewRunner(suite.DB, suite.Emitter, suite.Stripe, suite.BillingArchiveBuilder)
    require.NoError(t, r.Run(ctx, tenantID, storeID))

    // 1. billing_archive has a row for this store
    var archived int64
    _ = suite.DB.Table("billing_archive").
        Where("store_id=?", storeID).Count(&archived).Error
    require.Equal(t, int64(1), archived)

    // 2. Stripe customer deleted
    require.Contains(t, suite.Stripe.CustomersDeleted, stripeCustomerID)

    // 3. Tenant-scoped sweep — all 5 tables empty for this store
    for _, tbl := range []string{"orders","customers","products","media"} {
        var c int64
        _ = suite.DB.Table(tbl).Where("store_id=?", storeID).Count(&c).Error
        require.Equal(t, int64(0), c, "%s must be empty", tbl)
    }
    // stores table — row deleted
    var storeCount int64
    _ = suite.DB.Table("stores").Where("id=?", storeID).Count(&storeCount).Error
    require.Equal(t, int64(0), storeCount)

    // 4. audit emitted
    require.True(t, suite.Audit.HasEvent("subscription.hard_deleted",
        map[string]string{"store_id": storeID.String()}))

    // 5. statemachine transitioned — subscription row survives in hard_deleted
    var sub subscription.StoreSubscription
    _ = suite.DB.Where("store_id=?", storeID).First(&sub).Error
    require.Equal(t, subscription.StatusHardDeleted, sub.Status)
}

func TestHardDelete_StripeFails_DBSweepNotRun(t *testing.T) {
    // Stripe Customers.Del returns 500 → sweep is NOT invoked; transition NOT made.
    // Orders/products/etc. remain. Billing archive MAY be built (idempotent).
}

func TestHardDelete_ReRun_Idempotent_NoRowsRestored(t *testing.T) {
    // After successful run, a second invocation is a no-op.
}
```

- [ ] **Step 2: Write `sweeper.go`**

```go
package harddelete

// tablesToSweep — order matters: children before parents (FK).
// All tenant-scoped: filter by tenant_id + store_id.
var tablesToSweep = []string{"orders", "customers", "products", "media", "stores"}

type Sweeper struct{ DB GormDB; Emitter statemachine.Emitter }

func (s *Sweeper) Sweep(ctx context.Context, tenantID, storeID uuid.UUID) error {
    for _, tbl := range tablesToSweep {
        pred := "tenant_id = ? AND store_id = ?"
        args := []any{tenantID, storeID}
        if tbl == "stores" {
            pred = "tenant_id = ? AND id = ?"  // stores.id (not stores.store_id)
        }
        res := s.DB.WithContext(ctx).Exec("DELETE FROM "+tbl+" WHERE "+pred, args...)
        if res.Error != nil { return fmt.Errorf("sweep %s: %w", tbl, res.Error) }
        _ = s.Emitter.Emit(ctx, audit.Event{
            Kind: "subscription.hard_delete.sweep",
            Attrs: map[string]any{"table": tbl, "rows": res.RowsAffected, "store_id": storeID},
        })
    }
    return nil
}
```

- [ ] **Step 3: Write `runner.go`**

```go
package harddelete

type Runner struct {
    DB        GormDB
    Emitter   statemachine.Emitter
    Stripe    StripeCustomersAPI
    Archive   billingarchive.Builder  // P10
}

func (r *Runner) Run(ctx context.Context, tenantID, storeID uuid.UUID) error {
    return subscription.WithAdvisoryLock(ctx, r.DB, storeID, func(tx GormDB) error {
        sub, err := subscription.Get(ctx, tx, tenantID, storeID)
        if err != nil { return err }
        if sub.Status != subscription.StatusPendingHardDelete {
            return nil // already advanced or rolled back
        }

        // 1. Build billing archive (idempotent — writes to billing_archive).
        if err := r.Archive.Build(ctx, tx, storeID); err != nil {
            return fmt.Errorf("billing archive: %w", err)
        }

        // 2. Delete Stripe customer.
        if sub.StripeCustomerID != "" {
            if err := r.Stripe.DeleteCustomer(ctx, sub.StripeCustomerID); err != nil {
                return fmt.Errorf("stripe delete: %w", err)
            }
        }

        // 3. Tenant-scoped DELETE sweep across 5 tables.
        sw := &Sweeper{DB: tx, Emitter: r.Emitter}
        if err := sw.Sweep(ctx, tenantID, storeID); err != nil { return err }

        // 4. Final audit.
        _ = r.Emitter.Emit(ctx, audit.Event{
            Kind:  "subscription.hard_deleted",
            Attrs: map[string]any{"tenant_id": tenantID, "store_id": storeID},
        })

        // 5. statemachine transition pending_hard_delete → hard_deleted.
        return statemachine.Transition(ctx, statemachine.TransitionInput{
            DB: tx, Emitter: r.Emitter, TenantID: tenantID, StoreID: storeID,
            From: subscription.StatusPendingHardDelete, To: subscription.StatusHardDeleted,
            Actor: "system:cron", Reason: "lifecycle:hard_delete_day150",
        })
    })
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/harddelete/
git commit -m "feat(harddelete): billing archive + Stripe delete + tenant sweep runner (§15.2/§23)"
```

---

## Task 8: 150-day cron — invokes hard-delete runner

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/hard_delete_cron.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/hard_delete_cron_test.go`

**Spec references:** §15.2.

- [ ] **Step 1: Failing test — 60 days in pending_hard_delete (= day 150 post-expiry)**

```go
func TestHardDeleteCron_Fires_60DaysAfterQueue(t *testing.T) {
    // status=pending_hard_delete with status_updated_at 60 days ago → runner invoked
}

func TestHardDeleteCron_NoopWhenRecent(t *testing.T) {
    // 59 days → still pending_hard_delete
}

func TestHardDeleteCron_PerRow_Isolated(t *testing.T) {
    // Store A fails (Stripe outage); Store B succeeds. Cron doesn't abort on A's error.
}
```

- [ ] **Step 2: Write cron**

```go
package lifecycle

type HardDeleteCron struct {
    DB     GormDB
    Runner *harddelete.Runner
    Clock  func() time.Time
}

func (j *HardDeleteCron) Run(ctx context.Context) error {
    cutoff := j.Clock().Add(-60 * 24 * time.Hour)
    var rows []subscription.StoreSubscription
    if err := j.DB.WithContext(ctx).
        Where("status = ? AND status_updated_at < ?",
              subscription.StatusPendingHardDelete, cutoff).
        Limit(50). // hard-delete is heavy; batch small
        Find(&rows).Error; err != nil {
        return err
    }
    for _, r := range rows {
        if err := j.Runner.Run(ctx, r.TenantID, r.StoreID); err != nil {
            log.WithError(err).WithField("store_id", r.StoreID).Error("hard_delete_failed")
            // Per-row isolation; next run retries.
        }
    }
    return nil
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/hard_delete_cron.go \
        services/marketplace-api/internal/subscription/lifecycle/hard_delete_cron_test.go
git commit -m "feat(lifecycle): 150-day hard-delete cron (§15.2)"
```

---

## Task 9: Win-back cron — day-30 post-expiry

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/winback.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/winback_test.go`

**Spec references:** §15.3.

- [ ] **Step 1: Failing test**

```go
func TestWinback_FiresExactlyDay30(t *testing.T) {
    // cancels_at exactly 30 days ago, status=expired → 1 email
}

func TestWinback_Day29_NoEmail(t *testing.T) {}

func TestWinback_Day31_NoEmail(t *testing.T) {}

func TestWinback_Idempotent_SecondRunSameDay_NoDuplicate(t *testing.T) {
    // Ledger row in `winback_log` (new 1-column table) prevents double-send.
}

func TestWinback_ProAppMerchant_StillReceivesEmail(t *testing.T) {
    // Add-on state is not a filter; §15.3 applies to all expired subscribers.
}
```

- [ ] **Step 2: Write `winback.go`**

```go
package lifecycle

type WinbackJob struct {
    DB        GormDB
    Promo     promo.Service
    Notifier  notification.Client
    Clock     func() time.Time
}

func (j *WinbackJob) Run(ctx context.Context) error {
    now := j.Clock()
    dayStart := now.Add(-30 * 24 * time.Hour).Truncate(24 * time.Hour)
    dayEnd   := dayStart.Add(24 * time.Hour)

    var rows []subscription.StoreSubscription
    if err := j.DB.WithContext(ctx).
        Where("status = ? AND cancels_at >= ? AND cancels_at < ?",
              subscription.StatusExpired, dayStart, dayEnd).
        Find(&rows).Error; err != nil {
        return err
    }

    for _, r := range rows {
        // Idempotency ledger — insert-only, UNIQUE on (store_id, date).
        res := j.DB.Exec(`
          INSERT INTO winback_log (store_id, sent_on)
          VALUES (?, ?) ON CONFLICT (store_id, sent_on) DO NOTHING
        `, r.StoreID, now.Truncate(24*time.Hour))
        if res.RowsAffected == 0 { continue } // already sent today

        code, _ := j.Promo.Generate(ctx, promo.GenerateInput{
            StoreID: r.StoreID, DiscountPct: 20, Cycles: 6, Reason: "winback_day30",
        })
        _ = j.Notifier.SendTemplate(ctx, notification.Message{
            Template: "subscription.winback_day30",
            To:       r.BillingEmail,
            Data:     map[string]any{"promo_code": code.Code, "discount_pct": 20, "cycles": 6},
        })
    }
    return nil
}
```

A tiny migration creates the `winback_log (store_id UUID, sent_on DATE, PRIMARY KEY(store_id, sent_on))` table — fold into the migration file from Task 1.

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/winback.go \
        services/marketplace-api/internal/subscription/lifecycle/winback_test.go \
        services/marketplace-api/db/migrations/20260418_customer_portal.sql
git commit -m "feat(lifecycle): day-30 win-back email with 20%-off-6mo promo (§15.3)"
```

---

## Task 10: Customer portal — HMAC token + order-history endpoint

**Files:**
- Create: `services/marketplace-api/internal/customerportal/token.go`
- Create: `services/marketplace-api/internal/customerportal/handler.go`
- Create: `services/marketplace-api/internal/customerportal/handler_test.go`
- Modify: `services/marketplace-api/internal/handlers/storefront/routes.go`

**Spec references:** §15.4.

- [ ] **Step 1: Failing tests**

```go
func TestToken_SignVerify_RoundTrip(t *testing.T) {
    secret := "a"+strings.Repeat("f", 63)
    tok := customerportal.SignToken(secret, "alice@x.com", "ord_123")
    require.True(t,  customerportal.VerifyToken(secret, "alice@x.com", "ord_123", tok))
    require.False(t, customerportal.VerifyToken(secret, "alice@x.com", "ord_124", tok))
    require.False(t, customerportal.VerifyToken(secret, "bob@x.com",   "ord_123", tok))
}

func TestToken_Verify_ConstantTime(t *testing.T) {
    // uses hmac.Equal — smoke test via benchmarks or a guard-test on imports
}

func TestPortal_GET_MyOrders_ValidToken_ReturnsHistory(t *testing.T) {
    suite := inttest.NewSuite(t)
    store := suite.SeedStoreWithPortalSecret()
    suite.SeedCustomerOrders(store.ID, "alice@x.com", 3)

    tok := customerportal.SignToken(store.PortalSecret, "alice@x.com", "any")
    // Token binds to order_id — use a per-order lookup variant:
    // GET /my-orders/alice@x.com/<tok_for_first_order>
    // Endpoint derives order_id from token → returns all orders for that email on that store.
    w := suite.Storefront(store.ID).GET("/my-orders/alice@x.com/"+tok)
    require.Equal(t, 200, w.Code)
    var body struct{ Orders []any }
    _ = json.Unmarshal(w.Body.Bytes(), &body)
    require.Len(t, body.Orders, 3)
}

func TestPortal_GET_MyOrders_BadToken_403(t *testing.T) {
    w := suite.Storefront(s.ID).GET("/my-orders/alice@x.com/deadbeef")
    require.Equal(t, 403, w.Code)
}

func TestPortal_GET_MyOrders_Past90Days_410Gone(t *testing.T) {
    // store_closed 91 days ago → endpoint returns 410 Gone with "archived" body
}

func TestPortal_GET_MyOrders_StoreNotClosed_Accessible(t *testing.T) {
    // Active store — portal still works (§15.4 says "90 days after store closure" but
    // does not forbid access before closure; access before closure is merchant-admin-driven).
    // Simplest policy: token always works while store is not hard_deleted AND not past 90d post-closure.
}
```

- [ ] **Step 2: Write `token.go`**

```go
package customerportal

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

// SignToken returns hex(HMAC-SHA256(secret, email||"|"||orderID)).
// Binds the token to (store, email, order); token cannot be reused cross-store.
func SignToken(secret, email, orderID string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(strings.ToLower(email) + "|" + orderID))
    return hex.EncodeToString(mac.Sum(nil))
}

func VerifyToken(secret, email, orderID, presented string) bool {
    want, err := hex.DecodeString(presented)
    if err != nil || len(want) != sha256.Size { return false }
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(strings.ToLower(email) + "|" + orderID))
    return hmac.Equal(want, mac.Sum(nil))
}
```

- [ ] **Step 3: Write `handler.go`**

```go
package customerportal

// Handler: GET /my-orders/:email/:order_token
// The route is mounted under the storefront group — tenant + store resolved from host.
func OrderHistoryHandler(deps Deps) gin.HandlerFunc {
    return func(c *gin.Context) {
        storeID := ctxutil.MustStoreID(c)
        email   := strings.ToLower(c.Param("email"))
        tok     := c.Param("order_token")

        store, err := deps.Stores.Get(c, storeID)
        if err != nil { c.JSON(404, gin.H{"error":"store_not_found"}); return }

        // 90-day-post-closure window
        if store.Status == subscription.StatusHardDeleted {
            c.JSON(410, gin.H{"error":"archived","message":"Store data purged."})
            return
        }
        if store.StatusClosedAt != nil &&
            time.Since(*store.StatusClosedAt) > 90*24*time.Hour {
            c.JSON(410, gin.H{"error":"archived","message":"90-day portal window elapsed."})
            return
        }

        // Find any order matching this email + token (token binds to one order,
        // but we return full history once verified — per §15.4 "order history access").
        orders, err := deps.Orders.ListByEmailForStore(c, storeID, email)
        if err != nil || len(orders) == 0 {
            c.JSON(404, gin.H{"error":"not_found"}); return
        }
        valid := false
        for _, o := range orders {
            if VerifyToken(store.PortalSecret, email, o.ID.String(), tok) {
                valid = true; break
            }
        }
        if !valid { c.JSON(403, gin.H{"error":"invalid_token"}); return }

        c.JSON(200, gin.H{"orders": orders})
    }
}
```

- [ ] **Step 4: Mount route**

```go
// storefront/routes.go — unauthenticated group
sf.GET("/my-orders/:email/:order_token", customerportal.OrderHistoryHandler(deps))
```

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/customerportal/token.go \
        services/marketplace-api/internal/customerportal/handler.go \
        services/marketplace-api/internal/customerportal/handler_test.go \
        services/marketplace-api/internal/handlers/storefront/routes.go
git commit -m "feat(customerportal): order-history endpoint with HMAC-SHA256 token (§15.4)"
```

---

## Task 11: Customer erasure endpoint

**Files:**
- Create: `services/marketplace-api/internal/customerportal/erasure.go`
- Create: `services/marketplace-api/internal/customerportal/erasure_test.go`
- Modify: `services/marketplace-api/internal/handlers/storefront/routes.go`

**Spec references:** §15.4 ("merchant cannot bypass Mark8ly for customer erasure requests").

- [ ] **Step 1: Failing tests**

```go
func TestErasure_POST_InsertsRowAndReturns202(t *testing.T) {
    suite := inttest.NewSuite(t)
    store := suite.SeedStore(...)
    w := suite.Storefront(store.ID).POST("/customer-erasure", map[string]any{
        "email":  "alice@x.com",
        "reason": "I no longer want my data retained",
    })
    require.Equal(t, 202, w.Code)

    var count int64
    _ = suite.DB.Table("customer_erasure_requests").
        Where("store_id=? AND customer_email=?", store.ID, "alice@x.com").
        Count(&count).Error
    require.Equal(t, int64(1), count)
}

func TestErasure_NoMerchantAdminExposure(t *testing.T) {
    // No admin route mounted — verify via router dump
    routes := suite.Router().Routes()
    for _, r := range routes {
        require.NotContains(t, r.Path, "customer-erasure",
            "erasure must not be exposed on merchant admin — §15.4")
        // (sf.POST /customer-erasure is fine; admin/** must not have any)
    }
}

func TestErasure_EmptyEmail_Rejected(t *testing.T) {}

func TestErasure_RateLimitedPerStore(t *testing.T) {
    // Optional — 5/hr/store/IP; mark as `t.Skip("rate limit in P17")` if not yet wired
}
```

- [ ] **Step 2: Write `erasure.go`**

```go
package customerportal

type ErasureRequest struct {
    Email  string `json:"email" binding:"required,email"`
    Reason string `json:"reason,omitempty"`
}

func ErasureHandler(deps Deps) gin.HandlerFunc {
    return func(c *gin.Context) {
        var req ErasureRequest
        if err := c.ShouldBindJSON(&req); err != nil {
            c.JSON(400, gin.H{"error":"bad_request","message":err.Error()}); return
        }
        storeID  := ctxutil.MustStoreID(c)
        tenantID := ctxutil.MustTenantID(c)

        row := customerportal.CustomerErasureRequest{
            ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
            CustomerEmail: strings.ToLower(req.Email),
            Reason:        req.Reason,
            RequesterIP:   c.ClientIP(),
            CreatedAt:     time.Now(),
        }
        if err := deps.DB.Create(&row).Error; err != nil {
            log.WithError(err).Error("erasure_insert_failed")
            c.JSON(500, gin.H{"error":"write_failed"}); return
        }

        // Fire-and-forget: notify Mark8ly support queue.
        go deps.SupportQueue.Enqueue(context.Background(), supportqueue.Ticket{
            Kind: "gdpr_customer_erasure",
            RefID: row.ID.String(),
            Subject: fmt.Sprintf("GDPR erasure request from %s on store %s",
                row.CustomerEmail, storeID),
        })

        c.JSON(202, gin.H{"status":"queued", "request_id": row.ID})
    }
}
```

- [ ] **Step 3: Mount route (storefront, NOT admin)**

```go
sf.POST("/customer-erasure", customerportal.ErasureHandler(deps))
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/customerportal/erasure.go \
        services/marketplace-api/internal/customerportal/erasure_test.go \
        services/marketplace-api/internal/handlers/storefront/routes.go
git commit -m "feat(customerportal): append-only customer erasure endpoint (§15.4)"
```

---

## Task 12: End-to-end lifecycle integration test + cron registration + final verification

**Files:**
- Create: `services/marketplace-api/internal/subscription/lifecycle/e2e_test.go`
- Create: `services/marketplace-api/internal/subscription/lifecycle/crons.go` — registers all 5 jobs
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Purpose:** Prove the full chain: cancel → 30d expired → win-back email → 14d store_closed → 76d pending_hard_delete → 60d hard_deleted, all via time-mocked clock + test crons.

- [ ] **Step 1: Write `crons.go`**

```go
package lifecycle

import "github.com/robfig/cron/v3"

// Register wires all five lifecycle jobs onto a cron runner using the schedules
// documented in §15.2 / §15.3. Schedules are UTC.
func Register(c *cron.Cron, deps Deps) error {
    type entry struct {
        spec string
        run  func(context.Context) error
        name string
    }
    entries := []entry{
        {"0 1 * * *",  deps.Finalize.Run,      "finalize"},          // 01:00 UTC
        {"30 1 * * *", deps.Close.Run,         "close_14d"},         // 01:30 UTC
        {"0 2 * * *",  deps.QueueHardDel.Run,  "queue_hard_delete"}, // 02:00 UTC
        {"0 3 * * *",  deps.HardDelete.Run,    "hard_delete"},       // 03:00 UTC
        {"0 10 * * *", deps.Winback.Run,       "winback"},           // 10:00 UTC
    }
    for _, e := range entries {
        e := e
        if _, err := c.AddFunc(e.spec, func() {
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
            defer cancel()
            if err := e.run(ctx); err != nil {
                log.WithError(err).WithField("job", e.name).Error("lifecycle_cron_failed")
            }
        }); err != nil {
            return fmt.Errorf("register %s: %w", e.name, err)
        }
    }
    return nil
}
```

- [ ] **Step 2: Wire in `main.go`**

```go
c := cron.New(cron.WithLocation(time.UTC))
if err := lifecycle.Register(c, lifecycleDeps); err != nil { log.Fatal(err) }
c.Start()
defer c.Stop()
```

- [ ] **Step 3: Write `e2e_test.go` — single test running the full timeline**

```go
//go:build integration

func TestLifecycle_FullTimeline_CancelToHardDelete(t *testing.T) {
    s := inttest.NewSuite(t)
    clk := clock.NewMock(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))

    tenantID, storeID := s.SeedStore(subscription.StatusActive, subscription.PlanStarter)
    _ = s.SeedCustomerOrders(storeID, "bob@x.com", 2)

    // Day 0 — merchant cancels (reject save offer).
    s.AdminPOST(tenantID, storeID, "/subscription/cancel",
        cancel.Request{AcceptSaveOffer:false, SurveyReason:"moving_platforms"})
    s.AssertStatus(storeID, subscription.StatusCancelScheduled)

    // Day 30 — cancels_at passes; finalize cron runs.
    clk.Advance(30 * 24 * time.Hour)
    require.NoError(t, s.Cron.Finalize.Run(ctx))
    s.AssertStatus(storeID, subscription.StatusExpired)
    // Win-back fires same day (day 30 post cancels_at).
    require.NoError(t, s.Cron.Winback.Run(ctx))
    require.Equal(t, 1, s.Notifier.CountSent("subscription.winback_day30", "bob@x.com"))

    // Day 44 — 14d post-expiry; close cron runs.
    clk.Advance(14 * 24 * time.Hour)
    require.NoError(t, s.Cron.Close.Run(ctx))
    s.AssertStatus(storeID, subscription.StatusStoreClosed)

    // GDPR portal still accessible.
    tok := customerportal.SignToken(s.StorePortalSecret(storeID),
        "bob@x.com", s.FirstOrderID(storeID, "bob@x.com").String())
    w := s.Storefront(storeID).GET("/my-orders/bob@x.com/" + tok)
    require.Equal(t, 200, w.Code)

    // Day 120 — 76d post-close; queue hard-delete.
    clk.Advance(76 * 24 * time.Hour)
    require.NoError(t, s.Cron.QueueHardDel.Run(ctx))
    s.AssertStatus(storeID, subscription.StatusPendingHardDelete)

    // Day 180 — 60d in pending; hard-delete runs.
    clk.Advance(60 * 24 * time.Hour)
    require.NoError(t, s.Cron.HardDelete.Run(ctx))
    s.AssertStatus(storeID, subscription.StatusHardDeleted)

    // Portal now returns 410.
    w = s.Storefront(storeID).GET("/my-orders/bob@x.com/" + tok)
    require.Equal(t, 410, w.Code)

    // Orders / customers / products / media purged; billing_archive retained.
    for _, tbl := range []string{"orders","customers","products","media"} {
        s.AssertEmpty(tbl, "store_id=?", storeID)
    }
    s.AssertExists("billing_archive", "store_id=?", storeID)
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Final verification (below), then commit**

```bash
git add services/marketplace-api/internal/subscription/lifecycle/crons.go \
        services/marketplace-api/internal/subscription/lifecycle/e2e_test.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(lifecycle): register 5 daily crons + full-timeline integration test"
```

---

## Final verification

- [ ] `go build ./...` clean across all 5 new packages.
- [ ] `go test -tags=integration ./internal/subscription/... ./internal/customerportal/... -count=1` green.
- [ ] Grep — no direct `UPDATE store_subscriptions SET status` anywhere under `internal/subscription/lifecycle/` or `internal/subscription/harddelete/`; every transition routes through `statemachine.Transition`.
- [ ] Grep — `internal/handlers/admin` contains no `customer-erasure` references (GDPR erasure must not be exposed on merchant admin — §15.4 invariant).
- [ ] Success Criterion #54 test green — save-offer leaves current period untouched, discount attaches at next invoice.
- [ ] Success Criteria #48 + #49 remain green (P3 transitions) — sequential path `expired → store_closed → pending_hard_delete` is enforced by the state machine; no cron attempts a shortcut.
- [ ] Pro+App event `subscription.pro_app_cancelled` is published exactly once per merchant with `has_white_label_app_add_on=true` at finalize time; zero publications for non-add-on merchants.
- [ ] Hard-delete runner is the ONLY code path in the repo that calls `stripe.Customers.Del`.
- [ ] Every cron run that fails a single row logs + continues; crons never abort mid-batch.
- [ ] Daily cron schedule in `crons.go` matches §15.2/§15.3 timings (01:00, 01:30, 02:00, 03:00, 10:00 UTC).

## What's now unlocked

- **P12** (Cloudflare Worker closed.html) — reads `subscription_status='store_closed'` flipped by this plan's 14-day cron; can render `closed.html` with customer-portal link.
- **P15** (white-label app teardown) — consumes `subscription.pro_app_cancelled` event emitted at finalize.
- **P16** (admin frontend) — builds the cancel dialog, save-offer confirmation text ("Your 50% discount applies starting NEXT invoice cycle — no credit for current period"), and billing-archive view.
- **P17** (observability) — cron success/failure counters + hard-delete sweep row-count gauge feed directly off this plan's audit events.
- **Spec §15 closed** — all five subsections (15.1–15.5) implemented end-to-end.

## Execution handoff

Plan complete. Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Prerequisites that must ship first: **P1, P2, P3, P10**. Hard-delete in production requires a dry-run flag for the first two weeks (default `DRY_RUN=true` via env) so operators can confirm sweep counts match `billing_archive` builder output before destructive DELETEs land — fold that flag into `harddelete.Runner.Run` as a cheap safety net.
