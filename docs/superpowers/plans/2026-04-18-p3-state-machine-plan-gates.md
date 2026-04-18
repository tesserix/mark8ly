# P3 — Subscription State Machine + Plan Gates + Read-Only Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the passive subscription row into a deliberate state machine: rewrite `plangate` against the v2.3 four-plan feature matrix, enforce the sequential `expired → store_closed → pending_hard_delete` transition path under advisory lock + CAS guard, and introduce `RequireActive` middleware with a route allowlist (deliberately excluding `payment_action_required`) so the authz chain is `IstioAuth → TenantMiddleware → RequireActive → RequireFeature → handler`.

**Architecture:** A new `internal/subscription/statemachine` package owns the transition table (`map[from]map[to]transitionMeta`), the `Transition(ctx, tx, storeID, from, to, reason, actor)` entry point, and the CAS UPDATE `WHERE status = :expected_from`. Every transition runs inside `subscription.WithAdvisoryLock` (from P1) and emits an `audit.EmitStateTransition`. The P2 webhook handlers stop doing raw `UPDATE store_subscriptions SET status=...` — they call `statemachine.Transition` instead, so idempotent retries are safe. `plangate/gate.go` becomes a thin wrapper around the rewritten `featureMatrix` (v2.3 4-plan + marketplace) and the existing `PlanResolver`. A new `subscription/readonly` middleware intercepts admin routes and returns **HTTP 402 Payment Required** for closed/expired states, respecting a per-route allowlist that always permits billing + subscription + order-export + auth endpoints. A new file `internal/subscription/statemachine/transitions.go` documents all 17 valid transitions per §17.2.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL (no new SQL beyond reusing CHECK constraints from P1), existing `internal/audit`, `subscription.WithAdvisoryLock` (P1), `webhookevents`/`dispatch` (P2).

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §9 (feature matrix), §17.1–17.4 (states, transitions, read-only, concurrency), §17.3 (**`payment_action_required` NOT in read-only**).

**Depends on:** P1 (data model + `WithAdvisoryLock` + `EmitStateTransition`), P2 (webhook dispatcher that will delegate to the new state machine).

**Related plans:**
- **P4** (upgrade/downgrade + store-block) — consumes `statemachine.Transition` + `featureMatrix.StoresLimit()` + the advisory lock
- **P5** (trial card-add deferred charge) — transitions `signup → trialing` and `trialing → active` via this state machine
- **P6** (dunning + `payment_action_required` fallback) — enters the state excluded from read-only by design

---

## Scope Check

In scope:
1. Rewrite `plangate.featureMatrix` — 4-plan × 24-feature grid per §9; remove legacy `PlanFree`/`PlanEnterprise` entries.
2. Define the canonical transition table (§17.1) in one file with inline comments citing spec.
3. `statemachine.Transition` — CAS UPDATE under advisory lock + audit emit.
4. Replace direct status UPDATEs in the P2 dispatcher with `statemachine.Transition` calls.
5. `subscription.RequireActive` middleware with route allowlist (§17.3).
6. `subscription.ReadOnlyStatus` helper returning the set `{expired, store_closed, pending_hard_delete}` — **not** `payment_action_required` (Council finding #3).
7. `plangate.RequireFeature` and `RequirePlan` — keep existing 403 response shape, but source the feature matrix from the new file.
8. `AllFeatureLimits(plan Plan) map[string]int` remains for frontend JSON consumption.

Out of scope:
- Downgrade store-block cron re-check (§4.5.1) → P4.
- Storefront closure (Cloudflare Worker `closed.html`) → P12.
- Actual billing side-effects of transitions (retrying cards, writing refunds) — those land in P5/P6/P10.
- HTTP 402 error body copy beyond a minimal JSON envelope — UI copy is P16.

---

## File Structure

### Create

- `services/marketplace-api/internal/subscription/statemachine/transitions.go` — the transition table + helper functions (pure; no DB)
- `services/marketplace-api/internal/subscription/statemachine/transitions_test.go`
- `services/marketplace-api/internal/subscription/statemachine/machine.go` — `Transition` (the DB-facing entry point)
- `services/marketplace-api/internal/subscription/statemachine/machine_test.go`
- `services/marketplace-api/internal/subscription/readonly/middleware.go` — `RequireActive` Gin middleware + allowlist config
- `services/marketplace-api/internal/subscription/readonly/middleware_test.go`
- `services/marketplace-api/internal/subscription/readonly/allowlist.go` — data-only, no logic — the allowlist patterns
- `services/marketplace-api/internal/plangate/matrix.go` — new feature matrix, extracted from `gate.go`
- `services/marketplace-api/internal/plangate/matrix_test.go`

### Modify

- `services/marketplace-api/internal/plangate/gate.go` — shrink to a thin wrapper; moves matrix into `matrix.go`
- `services/marketplace-api/internal/handlers/admin/routes.go` — insert `readonly.RequireActive` into middleware chain between `StoreMiddleware` and authz
- `services/marketplace-api/internal/billing/dispatch/handlers.go` (from P2) — replace direct `UPDATE store_subscriptions SET status=...` with `statemachine.Transition`
- `services/marketplace-api/internal/handlers/admin/subscription.go` — use `statemachine.Transition` for the manual cancel endpoint (if present; otherwise this lands in P11)
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire `readonly.RequireActive` into the admin deps

### Delete

- Any lingering references to `PlanFree` / `PlanEnterprise` after P1 — final scrub.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Transition table (pure data) + unit tests | — |
| 2 | `statemachine.Transition` with CAS + advisory lock + audit | 1, P1 |
| 3 | Plug state machine into P2 dispatcher | 2, P2 |
| 4 | Rewrite `plangate.featureMatrix` against v2.3 §9 | — |
| 5 | `AllFeatureLimits` + frontend JSON parity test | 4 |
| 6 | `RequireActive` middleware with route allowlist + 402 response | 2 |
| 7 | Wire middleware into admin router | 6 |
| 8 | `payment_action_required` bypass integration test | 6, 7 |
| 9 | Sequential-path integration test (§17.2 §17.2) | 2 |
| 10 | CAS conflict test — concurrent transitions serialize | 2 |
| 11 | Final scrub: grep for legacy plan/status constants | all |

---

## Reusable patterns

**A. Transition table form** — `map[SubscriptionStatus]map[SubscriptionStatus]transitionMeta`. Only transitions present in the map are valid. Every transition carries `mustBeActor` (`ActorSystem` / `ActorUser` / `Any`), `reasonRequired bool`, and `severity audit.Severity`. This keeps `Transition` one compact function.

**B. CAS UPDATE pattern** —
```sql
UPDATE store_subscriptions
SET status = :to, updated_at = now()
WHERE tenant_id = :tenant_id
  AND store_id  = :store_id
  AND status    = :expected_from
RETURNING id
```
`RowsAffected == 0` means the CAS failed (someone else moved the row); return `ErrCASConflict` so the caller decides whether to re-read and retry. P2's dispatcher treats `ErrCASConflict` on idempotent event replays as a no-op.

**C. Advisory-locked transaction** — every call to `statemachine.Transition` wraps the CAS inside `subscription.WithAdvisoryLock(ctx, db, storeID, fn)` (P1 Task 13). Concurrent writers block at the Postgres session level, not the Go level.

**D. `RequireActive` middleware signature** — matches the rest of the router:
```go
func RequireActive(cfg Config) gin.HandlerFunc
```
Returns 402 with `{"error":"subscription_inactive","status":"expired"}` for blocked statuses; `c.Next()` otherwise. Ordering: **after** `StoreMiddleware` (so `store_plan` and `subscription` are already loaded into the Gin context) and **before** authz role checks.

**E. Allowlist** — slice of `(method, pathPattern)` pairs. Patterns use the same tokens as Gin's routes (e.g. `/admin/stores/:storeId/subscription/*path`). A route matches if the method is the same AND the pattern matches `c.FullPath()`. Using `FullPath()` (not `c.Request.URL.Path`) means routes are matched pre-param-substitution, which is stable and doesn't leak store IDs into the allowlist logic.

---

## Task 1: Transition table (pure)

**Files:**
- Create: `services/marketplace-api/internal/subscription/statemachine/transitions.go`
- Create: `services/marketplace-api/internal/subscription/statemachine/transitions_test.go`

**Spec references:** §17.2.

- [ ] **Step 1: Write failing test — valid transitions match §17.2 exactly**

```go
package statemachine_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

func TestValidTransitions_MatchSpecExactly(t *testing.T) {
    want := []struct{ from, to subscription.SubscriptionStatus }{
        {subscription.StatusSignup,    subscription.StatusTrialing},
        {subscription.StatusTrialing,  subscription.StatusActive},
        {subscription.StatusTrialing,  subscription.StatusExpired},
        {subscription.StatusActive,    subscription.StatusPastDue},
        {subscription.StatusActive,    subscription.StatusPaymentActionRequired},
        {subscription.StatusActive,    subscription.StatusCancelScheduled},
        {subscription.StatusPastDue,   subscription.StatusActive},
        {subscription.StatusPastDue,   subscription.StatusExpired},
        {subscription.StatusPaymentActionRequired, subscription.StatusActive},
        {subscription.StatusPaymentActionRequired, subscription.StatusPastDue},
        {subscription.StatusCancelScheduled, subscription.StatusActive},
        {subscription.StatusCancelScheduled, subscription.StatusExpired},
        {subscription.StatusExpired,   subscription.StatusActive},
        {subscription.StatusExpired,   subscription.StatusStoreClosed},
        {subscription.StatusStoreClosed, subscription.StatusActive},
        {subscription.StatusStoreClosed, subscription.StatusPendingHardDelete},
        {subscription.StatusPendingHardDelete, subscription.StatusHardDeleted},
    }

    for _, tc := range want {
        require.True(t, statemachine.IsValidTransition(tc.from, tc.to),
            "expected %s → %s to be valid", tc.from, tc.to)
    }

    // Every transition in the table must be in the `want` list (no extras).
    require.Len(t, statemachine.AllValidTransitions(), len(want),
        "transition table has extra/missing transitions vs §17.2")
}

func TestInvalidTransitions_Rejected(t *testing.T) {
    // The spec forbids direct expired → pending_hard_delete (must go via store_closed).
    require.False(t, statemachine.IsValidTransition(
        subscription.StatusExpired, subscription.StatusPendingHardDelete))

    // No path back from hard_deleted.
    require.False(t, statemachine.IsValidTransition(
        subscription.StatusHardDeleted, subscription.StatusActive))

    // No signup → active shortcut (trial gate is deliberate).
    require.False(t, statemachine.IsValidTransition(
        subscription.StatusSignup, subscription.StatusActive))
}

func TestPaymentActionRequired_IsNotInReadOnlySet(t *testing.T) {
    // Council finding #3: payment_action_required merchants keep full admin.
    require.NotContains(t,
        statemachine.ReadOnlyStatuses(),
        subscription.StatusPaymentActionRequired)
}

func TestReadOnlyStatusSet_MatchesSpec(t *testing.T) {
    require.ElementsMatch(t,
        []subscription.SubscriptionStatus{
            subscription.StatusExpired,
            subscription.StatusStoreClosed,
            subscription.StatusPendingHardDelete,
        },
        statemachine.ReadOnlyStatuses())
}
```

- [ ] **Step 2: Run — expect FAIL (package doesn't exist)**

```bash
cd services/marketplace-api
go test ./internal/subscription/statemachine/... -v
```

- [ ] **Step 3: Write `transitions.go`**

```go
// Package statemachine encodes the v2.3 subscription state machine (§17.2).
// Every transition is explicit; no fallthrough, no branch-on-status sprinkled
// across the codebase. Callers wire webhook + cron + merchant actions through
// one Transition entry point.
package statemachine

import (
    "github.com/tesserix/marketplace-api/internal/subscription"
)

// transitionMeta documents what kind of transition this is. Callers can
// introspect before calling Transition to decide permission/audit fields.
type transitionMeta struct {
    // Severity fed into the audit event. Warning/Error surfaces on dashboards.
    Severity AuditSeverity
    // Whether this transition is reachable from a merchant-driven action
    // (e.g. cancel, reactivate) vs system-only (dunning expiry, hard-delete cron).
    ActorKind ActorKind
    // Human-readable note citing the spec line; surfaced in audit metadata.
    SpecRef string
}

type AuditSeverity string
const (
    SeverityInfo    AuditSeverity = "info"
    SeverityWarning AuditSeverity = "warning"
    SeverityError   AuditSeverity = "error"
)

type ActorKind string
const (
    ActorSystem ActorKind = "system"
    ActorUser   ActorKind = "user"
    ActorAny    ActorKind = "any"
)

// transitionTable encodes §17.2. Every allowed move lives here; nothing else
// is a valid transition.
var transitionTable = map[subscription.SubscriptionStatus]map[subscription.SubscriptionStatus]transitionMeta{
    subscription.StatusSignup: {
        subscription.StatusTrialing: {SeverityInfo, ActorSystem, "§17.2 signup → trialing (email verified)"},
    },
    subscription.StatusTrialing: {
        subscription.StatusActive:  {SeverityInfo, ActorSystem, "§17.2 trialing → active (card added; first charge day 90)"},
        subscription.StatusExpired: {SeverityWarning, ActorSystem, "§17.2 trialing → expired (day 90, no card)"},
    },
    subscription.StatusActive: {
        subscription.StatusPastDue:                {SeverityWarning, ActorSystem, "§17.2 active → past_due (invoice.payment_failed)"},
        subscription.StatusPaymentActionRequired:  {SeverityWarning, ActorSystem, "§17.2 active → payment_action_required (invoice.payment_action_required)"},
        subscription.StatusCancelScheduled:        {SeverityInfo,    ActorUser,   "§17.2 active → cancel_scheduled (merchant cancel)"},
    },
    subscription.StatusPastDue: {
        subscription.StatusActive:  {SeverityInfo,    ActorSystem, "§17.2 past_due → active (retry succeeds)"},
        subscription.StatusExpired: {SeverityError,   ActorSystem, "§17.2 past_due → expired (dunning final fail)"},
    },
    subscription.StatusPaymentActionRequired: {
        subscription.StatusActive:  {SeverityInfo,    ActorSystem, "§17.2 payment_action_required → active (invoice.paid)"},
        subscription.StatusPastDue: {SeverityWarning, ActorSystem, "§17.2 payment_action_required → past_due (invoice unpaid past reminder)"},
    },
    subscription.StatusCancelScheduled: {
        subscription.StatusActive:  {SeverityInfo,    ActorUser,   "§17.2 cancel_scheduled → active (save-offer reversal or card re-added)"},
        subscription.StatusExpired: {SeverityWarning, ActorSystem, "§17.2 cancel_scheduled → expired (current_period_end)"},
    },
    subscription.StatusExpired: {
        subscription.StatusActive:      {SeverityInfo,    ActorUser,   "§17.2 expired → active (card re-added during grace)"},
        subscription.StatusStoreClosed: {SeverityWarning, ActorSystem, "§17.2 expired → store_closed (day 14 post-expiry)"},
    },
    subscription.StatusStoreClosed: {
        subscription.StatusActive:             {SeverityInfo,    ActorUser,   "§17.2 store_closed → active (card re-added during grace)"},
        subscription.StatusPendingHardDelete:  {SeverityError,   ActorSystem, "§17.2 store_closed → pending_hard_delete (day 90 post-expiry)"},
    },
    subscription.StatusPendingHardDelete: {
        subscription.StatusHardDeleted: {SeverityError, ActorSystem, "§17.2 pending_hard_delete → hard_deleted (deletion job)"},
    },
    // StatusHardDeleted is terminal.
}

// IsValidTransition reports whether (from → to) is one of the 17 allowed moves.
func IsValidTransition(from, to subscription.SubscriptionStatus) bool {
    toMap, ok := transitionTable[from]
    if !ok { return false }
    _, ok = toMap[to]
    return ok
}

// AllValidTransitions returns every (from, to) pair in the table — used for
// tests and documentation. Don't call it in a hot path.
type Transition struct {
    From subscription.SubscriptionStatus
    To   subscription.SubscriptionStatus
    Meta transitionMeta
}

func AllValidTransitions() []Transition {
    out := make([]Transition, 0, 32)
    for from, tos := range transitionTable {
        for to, meta := range tos {
            out = append(out, Transition{From: from, To: to, Meta: meta})
        }
    }
    return out
}

// ReadOnlyStatuses returns the statuses in which RequireActive rejects
// non-allowlisted admin routes (§17.3). NOTE: payment_action_required is
// deliberately excluded (Council finding #3).
func ReadOnlyStatuses() []subscription.SubscriptionStatus {
    return []subscription.SubscriptionStatus{
        subscription.StatusExpired,
        subscription.StatusStoreClosed,
        subscription.StatusPendingHardDelete,
    }
}

func IsReadOnly(s subscription.SubscriptionStatus) bool {
    for _, r := range ReadOnlyStatuses() {
        if r == s { return true }
    }
    return false
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/statemachine/transitions{,_test}.go
git commit -m "feat(subscription): canonical transition table + read-only status set"
```

---

## Task 2: `statemachine.Transition` with CAS + advisory lock + audit

**Files:**
- Create: `services/marketplace-api/internal/subscription/statemachine/machine.go`
- Create: `services/marketplace-api/internal/subscription/statemachine/machine_test.go`

- [ ] **Step 1: Failing test — success path**

```go
//go:build integration

package statemachine_test

import (
    "context"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestTransition_HappyPath(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    emitter := audit.NewEmitter(rec)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    c, _ := gin.CreateTestContext(nil)
    c.Set("tenant_id", tenantID.String())

    err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: emitter, GinCtx: c,
        TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusActive, To: subscription.StatusCancelScheduled,
        Actor: "user:" + uuid.New().String(), Reason: "merchant_cancelled",
    })
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusCancelScheduled, sub.Status)

    emitter.FlushForTesting()
    require.Len(t, rec.Events(), 1)
    md := rec.Events()[0].Metadata
    require.Equal(t, "active", md["from_status"])
    require.Equal(t, "cancel_scheduled", md["to_status"])
    require.Equal(t, "merchant_cancelled", md["reason"])
}
```

- [ ] **Step 2: Failing test — CAS conflict**

```go
func TestTransition_CASConflict_WhenStatusAlreadyMoved(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    // Caller expects From=trialing but the row is already active.
    err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusTrialing, To: subscription.StatusActive,
        Actor: "system:webhook", Reason: "invoice.paid",
    })
    require.ErrorIs(t, err, statemachine.ErrCASConflict)
}
```

- [ ] **Step 3: Failing test — invalid transition rejected**

```go
func TestTransition_InvalidTransition_Rejected(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusExpired,
    }).Error)

    err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusExpired, To: subscription.StatusPendingHardDelete, // forbidden shortcut
        Actor: "system:cron", Reason: "day_90",
    })
    require.ErrorIs(t, err, statemachine.ErrInvalidTransition)
}
```

- [ ] **Step 4: Run tests — expect FAIL**

- [ ] **Step 5: Write `machine.go`**

```go
package statemachine

import (
    "context"
    "errors"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

var (
    ErrInvalidTransition = errors.New("statemachine: invalid transition")
    ErrCASConflict       = errors.New("statemachine: CAS conflict (status changed concurrently)")
)

// TransitionInput describes a desired state change. All fields required
// except GinCtx and StripeEventID.
type TransitionInput struct {
    DB            *gorm.DB
    Emitter       *audit.Emitter
    GinCtx        *gin.Context // optional — used for audit emit tagging
    TenantID      uuid.UUID
    StoreID       uuid.UUID
    From          subscription.SubscriptionStatus // expected current status; used for CAS
    To            subscription.SubscriptionStatus
    Actor         string // "user:<uuid>" or "system:webhook:stripe" or "system:cron:trial_expiry"
    Reason        string // free-text but normally mirrors the webhook/cron name
    StripeEventID string // optional — included in audit metadata for traceability
}

// Transition performs a CAS UPDATE under an advisory lock and emits an audit
// event on success. It is the ONLY sanctioned way to change
// store_subscriptions.status outside raw migrations.
//
// Callers should treat ErrCASConflict as "someone else already moved it;
// re-read if your decision depended on the previous state." Idempotent webhook
// retries typically log and swallow the conflict.
func Transition(ctx context.Context, in TransitionInput) error {
    if !IsValidTransition(in.From, in.To) {
        return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, in.From, in.To)
    }

    return subscription.WithAdvisoryLock(ctx, in.DB, in.StoreID, func(tx *gorm.DB) error {
        res := tx.Exec(`
            UPDATE store_subscriptions
            SET status = ?, updated_at = now()
            WHERE tenant_id = ?
              AND store_id  = ?
              AND status    = ?`,
            in.To, in.TenantID, in.StoreID, in.From,
        )
        if res.Error != nil {
            return fmt.Errorf("statemachine: CAS update: %w", res.Error)
        }
        if res.RowsAffected == 0 {
            return ErrCASConflict
        }

        // Audit emit must NOT fail the transition — emitter already handles
        // queue-full drops; any error here is logged internally.
        if in.Emitter != nil {
            in.Emitter.EmitStateTransition(in.GinCtx, audit.StateTransition{
                StoreID:       in.StoreID,
                TenantID:      in.TenantID,
                From:          string(in.From),
                To:            string(in.To),
                Actor:         in.Actor,
                Reason:        in.Reason,
                StripeEventID: in.StripeEventID,
            })
        }
        return nil
    })
}
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/subscription/statemachine/machine{,_test}.go
git commit -m "feat(subscription): statemachine.Transition with CAS + advisory lock + audit"
```

---

## Task 3: Plug state machine into P2 dispatcher

**Files:**
- Modify: `services/marketplace-api/internal/billing/dispatch/handlers.go`
- Modify: `services/marketplace-api/internal/billing/dispatch/dispatcher.go`
- Modify: `services/marketplace-api/internal/billing/dispatch/handlers_test.go`

**Objective:** Replace direct `UPDATE store_subscriptions SET status=...` in `handleInvoicePaymentFailed`, `handleInvoicePaymentActionRequired`, `handleSubscriptionDeleted` with `statemachine.Transition` calls.

- [ ] **Step 1: Failing test — `invoice.payment_failed` goes `active → past_due` via state machine + emits audit**

```go
func TestHandleInvoicePaymentFailed_TransitionsActiveToPastDue(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    raw := []byte(`{"id":"evt_1","type":"invoice.payment_failed","data":{"object":{"id":"in_x","customer":"cus_x","subscription":"sub_x"}}}`)
    d := dispatch.NewWithStateMachine(em)

    err := d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_1", EventType: "invoice.payment_failed", Payload: raw,
    })
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusPastDue, sub.Status)

    em.FlushForTesting()
    require.Len(t, rec.Events(), 1)
    require.Equal(t, "past_due", rec.Events()[0].Metadata["to_status"])
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Update dispatcher constructor**

Change `dispatch.New()` signature to `dispatch.New(em *audit.Emitter) *Dispatcher`. Store the emitter on the struct. Update all handler signatures to receive the emitter via closure.

```go
func New(em *audit.Emitter) *Dispatcher {
    d := &Dispatcher{emitter: em, handlers: map[string]Handler{}}
    d.handlers["customer.subscription.deleted"]   = d.handleSubscriptionDeleted
    d.handlers["invoice.payment_failed"]          = d.handleInvoicePaymentFailed
    d.handlers["invoice.payment_action_required"] = d.handleInvoicePaymentActionRequired
    // ...others unchanged from P2
    return d
}
```

- [ ] **Step 4: Refactor `handleInvoicePaymentFailed`**

```go
func (d *Dispatcher) handleInvoicePaymentFailed(ctx context.Context, tx *gorm.DB, raw []byte) error {
    var e struct {
        Data struct { Object struct { Customer string } } `json:"data"`
    }
    if err := json.Unmarshal(raw, &e); err != nil { return err }
    customer := e.Data.Object.Customer
    if customer == "" { return errors.New("dispatch: missing customer on invoice.payment_failed") }

    var sub subscription.StoreSubscription
    if err := tx.Where("stripe_customer_id=?", customer).First(&sub).Error; err != nil {
        return err
    }

    err := statemachine.Transition(ctx, statemachine.TransitionInput{
        DB: tx, Emitter: d.emitter,
        TenantID: sub.TenantID, StoreID: sub.StoreID,
        From: sub.Status, To: subscription.StatusPastDue,
        Actor: "system:webhook:stripe", Reason: "invoice.payment_failed",
    })
    // CAS conflict is benign on retries — the desired terminal state is already set or
    // the row has moved on to expired via dunning; log and swallow.
    if errors.Is(err, statemachine.ErrCASConflict) {
        return nil
    }
    return err
}
```

Apply the same pattern to:
- `handleInvoicePaymentActionRequired`: re-read the row inside the locked transaction; valid From is `active` only per §17.2 — if already `payment_action_required`, no-op (idempotent replay).
- `handleSubscriptionDeleted`: re-read inside the locked tx; valid From is `active|past_due|cancel_scheduled|payment_action_required`, To=`expired`. If observed status ∉ valid From set (e.g. already `store_closed`), log warning + no-op.
- `handleInvoicePaid`: **re-read inside the locked tx** (same shape as the two above). If current status is `payment_action_required` → transition to `active`. If `past_due` → `active`. Otherwise no-op.

**`handleCheckoutSessionCompleted` is deliberately NOT refactored here.** P2's handler still issues the raw UPDATE that sets `status` from `signup` → `trialing` via a `CASE` inside `COALESCE(billing_currency, ?)`. That UPDATE stays in place until **P5 (trial card-add deferred charge)** reworks the whole signup/trialing/active pathway through `statemachine.Transition`. Until then, Task 11's grep scrub MUST be written to exempt `handleCheckoutSessionCompleted` explicitly — see Task 11 Step 2.

- [ ] **Step 5: Run dispatcher tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/{dispatcher,handlers,handlers_test}.go
git commit -m "refactor(billing): route dispatcher state mutations through statemachine.Transition"
```

---

## Task 4: Rewrite `plangate.featureMatrix` against v2.3 §9

**Files:**
- Create: `services/marketplace-api/internal/plangate/matrix.go`
- Modify: `services/marketplace-api/internal/plangate/gate.go` (shrink)
- Create: `services/marketplace-api/internal/plangate/matrix_test.go`

**Spec references:** §9 (Trial / Starter / Studio / Pro / Pro+App).

> **Note on `Pro + App`**: in the matrix we treat Pro+App as Pro plus the boolean `HasWhiteLabelAppAddOn`. The matrix table keys on plan only; the one add-on feature (`FeatureWhiteLabelApp`) is gated via a separate function that reads `subscription.HasWhiteLabelAppAddOn`.

- [ ] **Step 1: Failing tests per §9 row**

```go
package plangate_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/plangate"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

func TestMatrix_StoresLimits(t *testing.T) {
    require.Equal(t, 1, plangate.Limit(subscription.PlanTrial, plangate.FeatureStores))
    require.Equal(t, 2, plangate.Limit(subscription.PlanStarter, plangate.FeatureStores))
    require.Equal(t, 5, plangate.Limit(subscription.PlanStudio, plangate.FeatureStores))
    require.Equal(t, 10, plangate.Limit(subscription.PlanPro, plangate.FeatureStores))
}

func TestMatrix_ImagesPerProduct_Grandfathering(t *testing.T) {
    require.Equal(t, 25, plangate.Limit(subscription.PlanTrial, plangate.FeatureImagesPerProduct))
    require.Equal(t, 25, plangate.Limit(subscription.PlanStarter, plangate.FeatureImagesPerProduct))
    require.Equal(t, 50, plangate.Limit(subscription.PlanStudio, plangate.FeatureImagesPerProduct))
    require.Equal(t, plangate.Unlimited, plangate.Limit(subscription.PlanPro, plangate.FeatureImagesPerProduct))
}

func TestMatrix_CampaignEmailsPerMonth(t *testing.T) {
    require.Equal(t, 5_000,  plangate.Limit(subscription.PlanTrial, plangate.FeatureCampaignEmailsPerMonth))
    require.Equal(t, 15_000, plangate.Limit(subscription.PlanStarter, plangate.FeatureCampaignEmailsPerMonth))
    require.Equal(t, 50_000, plangate.Limit(subscription.PlanStudio, plangate.FeatureCampaignEmailsPerMonth))
    require.Equal(t, plangate.Negotiated, plangate.Limit(subscription.PlanPro, plangate.FeatureCampaignEmailsPerMonth))
}

func TestMatrix_CustomCSS_StudioAndUp(t *testing.T) {
    require.False(t, plangate.IsAllowed(subscription.PlanTrial,   plangate.FeatureCustomCSS))
    require.False(t, plangate.IsAllowed(subscription.PlanStarter, plangate.FeatureCustomCSS))
    require.True(t,  plangate.IsAllowed(subscription.PlanStudio,  plangate.FeatureCustomCSS))
    require.True(t,  plangate.IsAllowed(subscription.PlanPro,     plangate.FeatureCustomCSS))
}

func TestMatrix_CustomCodeInjection_ProOnly(t *testing.T) {
    require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureCustomCodeInjection))
    require.True(t,  plangate.IsAllowed(subscription.PlanPro,    plangate.FeatureCustomCodeInjection))
}

func TestMatrix_SSO_ProOnly(t *testing.T) {
    require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureSSO))
    require.True(t,  plangate.IsAllowed(subscription.PlanPro,    plangate.FeatureSSO))
}

func TestMatrix_FullReadWriteAPI_ProOnly(t *testing.T) {
    require.True(t,  plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureReadAPI))
    require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureFullAPI))
    require.True(t,  plangate.IsAllowed(subscription.PlanPro,    plangate.FeatureFullAPI))
}

func TestMatrix_WhiteLabelApp_GatedByAddOn(t *testing.T) {
    // The matrix always returns false for FeatureWhiteLabelApp on any plan;
    // the add-on boolean is the gate, not the plan.
    for _, p := range []subscription.SubscriptionPlan{
        subscription.PlanTrial, subscription.PlanStarter, subscription.PlanStudio, subscription.PlanPro,
    } {
        require.False(t, plangate.IsAllowed(p, plangate.FeatureWhiteLabelApp))
    }
    require.True(t, plangate.WhiteLabelAppEnabled(subscription.PlanPro, true))
    require.False(t, plangate.WhiteLabelAppEnabled(subscription.PlanPro, false))
    require.False(t, plangate.WhiteLabelAppEnabled(subscription.PlanStudio, true), "add-on meaningless without Pro")
}

func TestMatrix_AuditRetention(t *testing.T) {
    require.Equal(t, 90,  plangate.Limit(subscription.PlanTrial,  plangate.FeatureAuditRetentionDays))
    require.Equal(t, 90,  plangate.Limit(subscription.PlanStarter, plangate.FeatureAuditRetentionDays))
    require.Equal(t, 365, plangate.Limit(subscription.PlanStudio, plangate.FeatureAuditRetentionDays))
    require.Equal(t, plangate.Unlimited, plangate.Limit(subscription.PlanPro, plangate.FeatureAuditRetentionDays))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `matrix.go`**

```go
package plangate

import (
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type Feature string

const (
    // Limits
    FeatureStores                  Feature = "stores"
    FeatureImagesPerProduct        Feature = "images_per_product"
    FeatureAuditRetentionDays      Feature = "audit_retention_days"
    FeatureCampaignEmailsPerMonth  Feature = "campaign_emails_per_month"
    FeatureTransactionalEmails     Feature = "transactional_emails"
    // Storefront
    FeatureCustomDomain            Feature = "custom_domain"
    FeatureFullColorPalette        Feature = "full_color_palette"
    FeatureAnnouncementBar         Feature = "announcement_bar"
    FeatureRemovePoweredBy         Feature = "remove_powered_by"
    FeatureCustomCSS               Feature = "custom_css"
    FeatureCustomCodeInjection     Feature = "custom_code_injection"
    FeatureWhiteLabelApp           Feature = "white_label_app"   // always false in matrix; gated via add-on
    // Platform
    FeatureCSVImportExport         Feature = "csv_import_export"
    FeatureShippingLabels          Feature = "shipping_labels"
    FeatureReturns                 Feature = "returns"
    FeatureReviews                 Feature = "reviews"
    FeatureTickets                 Feature = "tickets"
    FeatureGiftCards               Feature = "gift_cards"
    FeatureReadAPI                 Feature = "read_api"
    FeatureFullAPI                 Feature = "full_read_write_api"
    FeatureSSO                     Feature = "sso"
    FeatureUptimeSLA               Feature = "uptime_sla"        // add-on only
    // Support
    FeatureStandardEmailSupport    Feature = "standard_email_support"
    FeaturePriorityEmailSupport    Feature = "priority_email_support"
    FeatureNamedCSM                Feature = "named_csm"         // add-on only
)

// Sentinel limit values.
const (
    Disabled   = 0
    Unlimited  = -1
    Negotiated = -2 // surfaced to UI as "contact sales"
)

type planLimits map[Feature]int

// featureMatrix encodes §9. Boolean features use 0/1; numeric limits use actual values;
// Unlimited and Negotiated use sentinels. Pro+App-only features (FeatureWhiteLabelApp,
// FeatureUptimeSLA, FeatureNamedCSM) are Disabled across all plans and turned on by
// WhiteLabelAppEnabled() when the subscription carries the add-on.
var featureMatrix = map[subscription.SubscriptionPlan]planLimits{
    subscription.PlanTrial: {
        FeatureStores:                 1,
        FeatureImagesPerProduct:       25,
        FeatureAuditRetentionDays:     90,
        FeatureCampaignEmailsPerMonth: 5_000,
        FeatureTransactionalEmails:    Unlimited,
        FeatureCustomDomain:           1,
        FeatureFullColorPalette:       1,
        FeatureAnnouncementBar:        1,
        FeatureRemovePoweredBy:        1,
        FeatureCustomCSS:              Disabled,
        FeatureCustomCodeInjection:    Disabled,
        FeatureWhiteLabelApp:          Disabled,
        FeatureCSVImportExport:        1,
        FeatureShippingLabels:         1,
        FeatureReturns:                1,
        FeatureReviews:                1,
        FeatureTickets:                1,
        FeatureGiftCards:              1,
        FeatureReadAPI:                Disabled,
        FeatureFullAPI:                Disabled,
        FeatureSSO:                    Disabled,
        FeatureUptimeSLA:              Disabled,
        FeatureStandardEmailSupport:   1,
        FeaturePriorityEmailSupport:   Disabled,
        FeatureNamedCSM:               Disabled,
    },
    subscription.PlanStarter: { /* …mirrors §9 Starter column… */ },
    subscription.PlanStudio:  { /* …§9 Studio column… */ },
    subscription.PlanPro:     { /* …§9 Pro column; FeatureCustomCodeInjection=1; FeatureFullAPI=1; FeatureSSO=1 … */ },
    // Marketplace: placeholder; empty matrix returns Disabled by default.
}

// IsAllowed reports whether the feature is enabled (>0 or Unlimited).
func IsAllowed(p subscription.SubscriptionPlan, f Feature) bool {
    limits, ok := featureMatrix[p]
    if !ok { return false }
    v, ok := limits[f]
    if !ok { return false }
    return v != Disabled
}

// Limit returns the numeric limit (or Unlimited/Negotiated/Disabled sentinels).
func Limit(p subscription.SubscriptionPlan, f Feature) int {
    return featureMatrix[p][f]
}

// WhiteLabelAppEnabled requires Pro AND the add-on boolean from the subscription row.
func WhiteLabelAppEnabled(p subscription.SubscriptionPlan, hasAddOn bool) bool {
    return p == subscription.PlanPro && hasAddOn
}

// AllFeatureLimits returns a JSON-friendly snapshot for the frontend.
func AllFeatureLimits(p subscription.SubscriptionPlan) map[string]int {
    out := make(map[string]int, 32)
    for f, v := range featureMatrix[p] {
        out[string(f)] = v
    }
    return out
}
```

Fill the Starter/Studio/Pro columns *carefully* against §9. Each row in the spec is one entry; use `grep` to verify every spec feature has a matrix entry:

```bash
grep -E '^\| [A-Z]' docs/superpowers/specs/2026-04-17-subscription-model-design.md | head -40
```

- [ ] **Step 4: Update `gate.go` to import the new matrix**

Delete the old `Feature` constants and `featureMatrix` from `gate.go`. Keep only:
- `type Plan = subscription.SubscriptionPlan` (alias)
- `planOrder` map
- `PlanAtLeast`, `PlanResolver`, `RequireFeature`, `RequirePlan`

Replace the old `IsAllowed` / `GetLimit` calls inside the middleware with the new `plangate.IsAllowed` / `plangate.Limit` from `matrix.go`.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/plangate/{matrix,matrix_test,gate}.go
git commit -m "feat(plangate): rewrite feature matrix to v2.3 4-plan model (§9)"
```

---

## Task 5: `AllFeatureLimits` frontend JSON parity test

**Files:**
- Modify: `services/marketplace-api/internal/plangate/matrix_test.go`
- (possibly) Create: `apps/admin/lib/plangate-schema.ts` stub — NOT in this plan; documented for P16

- [ ] **Step 1: Golden-file test — matrix is stable JSON**

```go
func TestAllFeatureLimits_StableJSON(t *testing.T) {
    for _, p := range []subscription.SubscriptionPlan{
        subscription.PlanTrial, subscription.PlanStarter, subscription.PlanStudio, subscription.PlanPro,
    } {
        t.Run(string(p), func(t *testing.T) {
            m := plangate.AllFeatureLimits(p)
            // Every feature in the enum must be present (zero is fine).
            for _, f := range plangate.AllFeatures() {
                _, ok := m[string(f)]
                require.True(t, ok, "feature %s missing from AllFeatureLimits for %s", f, p)
            }
        })
    }
}
```

Add `func AllFeatures() []Feature` to `matrix.go` returning every `Feature` constant — useful for generating TS types in P16.

- [ ] **Step 2: Run — expect PASS (fix any missing matrix entries)**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/plangate/matrix.go \
        services/marketplace-api/internal/plangate/matrix_test.go
git commit -m "test(plangate): assert every feature is present in every plan's AllFeatureLimits"
```

---

## Task 6: `RequireActive` middleware + allowlist + 402 response

**Files:**
- Create: `services/marketplace-api/internal/subscription/readonly/allowlist.go`
- Create: `services/marketplace-api/internal/subscription/readonly/middleware.go`
- Create: `services/marketplace-api/internal/subscription/readonly/middleware_test.go`

**Spec references:** §17.3.

- [ ] **Step 1: Failing tests — blocked path + allowed path + bypass for `payment_action_required`**

```go
func TestRequireActive_BlocksExpiredOnNonAllowlistedRoute(t *testing.T) {
    h := newHandlerWithStatus(subscription.StatusExpired)
    w := doRequest(h, "GET", "/admin/stores/s1/products", "/admin/stores/:storeId/products")
    require.Equal(t, 402, w.Code)
    require.Contains(t, w.Body.String(), "subscription_inactive")
}

func TestRequireActive_AllowsBillingPathEvenWhenExpired(t *testing.T) {
    h := newHandlerWithStatus(subscription.StatusExpired)
    w := doRequest(h, "POST", "/admin/stores/s1/subscription/checkout",
        "/admin/stores/:storeId/subscription/checkout")
    require.Equal(t, 200, w.Code)
}

func TestRequireActive_AllowsOrderExportEvenWhenStoreClosed(t *testing.T) {
    h := newHandlerWithStatus(subscription.StatusStoreClosed)
    w := doRequest(h, "GET", "/admin/stores/s1/orders/export/csv",
        "/admin/stores/:storeId/orders/export/*path")
    require.Equal(t, 200, w.Code)
}

func TestRequireActive_DoesNotBlockPaymentActionRequired(t *testing.T) {
    // Council finding #3: merchant retains full admin + storefront for SCA completion.
    h := newHandlerWithStatus(subscription.StatusPaymentActionRequired)
    w := doRequest(h, "GET", "/admin/stores/s1/products",
        "/admin/stores/:storeId/products")
    require.Equal(t, 200, w.Code)
}

func TestRequireActive_DoesNotBlockActive(t *testing.T) {
    h := newHandlerWithStatus(subscription.StatusActive)
    w := doRequest(h, "PUT", "/admin/stores/s1/products/1",
        "/admin/stores/:storeId/products/:id")
    require.Equal(t, 200, w.Code)
}
```

- [ ] **Step 2: Write `allowlist.go`**

```go
package readonly

import "net/http"

// AllowedRoute identifies a Gin route by method + FullPath() pattern.
// FullPath is the path registered in the router, pre-substitution — stable and
// testable. Use Gin tokens (:storeId, *path) as documented.
type AllowedRoute struct {
    Method  string
    Pattern string
}

// DefaultAllowlist — §17.3 matches these four groups plus any GET under /admin/
// (view-only). The Gin router is expected to mount admin routes under /admin/,
// with store-scoped routes under /admin/stores/:storeId/.
var DefaultAllowlist = []AllowedRoute{
    {http.MethodPost, "/admin/stores/:storeId/subscription/*path"},
    {http.MethodPost, "/admin/stores/:storeId/billing/*path"},
    {http.MethodGet,  "/admin/stores/:storeId/orders/export/*path"},
    {http.MethodPost, "/admin/auth/*path"},
    // Note: all GET /admin/** is allowed (view-only); encoded separately in
    // middleware.go as a wildcard method short-circuit.
}
```

- [ ] **Step 3: Write `middleware.go`**

```go
package readonly

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

// Config wires the middleware into the admin router.
// Store is expected to be populated on the Gin context by StoreMiddleware at
// ctx.MustGet("subscription_status") (or equivalent). If not present, the
// middleware falls through to c.Next() — it is additive, not authoritative.
type Config struct {
    StatusContextKey string          // default: "subscription_status"
    Allowlist        []AllowedRoute  // default: DefaultAllowlist
}

func RequireActive(cfg Config) gin.HandlerFunc {
    if cfg.StatusContextKey == "" { cfg.StatusContextKey = "subscription_status" }
    if cfg.Allowlist == nil { cfg.Allowlist = DefaultAllowlist }

    return func(c *gin.Context) {
        raw, ok := c.Get(cfg.StatusContextKey)
        if !ok { c.Next(); return }
        status, _ := raw.(subscription.SubscriptionStatus)

        if !statemachine.IsReadOnly(status) {
            c.Next()
            return
        }

        if routeAllowed(c, cfg.Allowlist) {
            c.Next()
            return
        }

        c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
            "error":  "subscription_inactive",
            "status": string(status),
        })
    }
}

func routeAllowed(c *gin.Context, allowlist []AllowedRoute) bool {
    // All read-only routes (GET /admin/**) always allowed.
    if c.Request.Method == http.MethodGet && strings.HasPrefix(c.FullPath(), "/admin/") {
        return true
    }
    full := c.FullPath()
    for _, r := range allowlist {
        if r.Method != "" && r.Method != c.Request.Method {
            continue
        }
        if patternMatches(r.Pattern, full) {
            return true
        }
    }
    return false
}

// patternMatches supports exact match plus a tail wildcard *path — enough for §17.3.
// For more sophisticated matching, replace with the same matcher Gin uses.
func patternMatches(pattern, full string) bool {
    if pattern == full { return true }
    if strings.HasSuffix(pattern, "*path") {
        prefix := strings.TrimSuffix(pattern, "*path")
        return strings.HasPrefix(full, prefix)
    }
    return false
}
```

> **Reviewer note:** the spec states the chain is `IstioAuth → TenantMiddleware → RequireActive → RequireFeature → handler`. This service uses `auth.HeaderTrustAuth` instead of `IstioAuth`, per exploration. The chain is effectively the same shape; `HeaderTrustAuth` sets `tenant_id`/`user_id`, then `StoreMiddleware` loads the subscription onto the context. We insert `RequireActive` right after `StoreMiddleware` — see Task 7.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/subscription/readonly/
git commit -m "feat(subscription): RequireActive middleware with allowlist (402 Payment Required)"
```

---

## Task 7: Wire middleware into admin router

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/internal/stores/middleware.go` (add subscription status to context if not already)
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Ensure `StoreMiddleware` populates `subscription_status`**

Check `internal/stores/middleware.go` — per exploration it already loads the store into context. Add a subscription status read if not already present:

```go
// Inside StoreMiddleware, after loading the store:
sub, err := subscriptionRepo.GetByStoreID(c.Request.Context(), db, tenantID, store.ID)
if err == nil {
    c.Set("subscription_status", sub.Status)
    c.Set("subscription_plan", sub.Plan)
    c.Set("subscription_has_app_addon", sub.HasWhiteLabelAppAddOn)
} else if !errors.Is(err, subscription.ErrNotFound) {
    c.AbortWithStatusJSON(500, gin.H{"error":"subscription_lookup_failed"})
    return
}
```

If the existing store middleware doesn't have access to the subscription repo, accept it as a `MiddlewareConfig` field and wire it in `main.go`. Keep changes surgical.

- [ ] **Step 2: Insert `RequireActive` in `routes.go`**

Find the admin router group construction. The chain is approximately:

```go
admin := router.Group("/admin",
    auth.HeaderTrustAuth(cfg.InternalSecret),
    deps.TenantRouteMiddleware,
)
storeRoute := admin.Group("/stores/:storeId",
    deps.StoreMiddleware, // already loads subscription onto context
    readonly.RequireActive(readonly.Config{}), // NEW
)
// authz checks + RequireFeature follow per-route as before
```

- [ ] **Step 3: Wire `main.go`**

Pass the subscription repo into `StoreMiddlewareConfig`. Keep `RequireActive`'s default config — tests live in the middleware package.

- [ ] **Step 4: Build + integration smoke**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./internal/handlers/admin/... ./internal/subscription/readonly/... -v
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/routes.go \
        services/marketplace-api/internal/stores/middleware.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(admin): wire RequireActive into store-scoped admin chain"
```

---

## Task 8: `payment_action_required` bypass integration test

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/subscription_par_integration_test.go`

**Purpose:** Success criterion 38 — "merchant admin fully editable, storefront live" during `payment_action_required`.

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestAdmin_PaymentActionRequired_AllowsFullAccess(t *testing.T) {
    suite := inttest.NewSuite(t) // project test-harness for booting gin + DB

    tenantID, storeID := suite.SeedStore(subscription.StatusPaymentActionRequired, subscription.PlanStarter)

    // Non-billing, non-export admin route should succeed.
    resp := suite.AdminGET(tenantID, storeID, "/admin/stores/"+storeID.String()+"/products")
    require.Equal(t, 200, resp.Code)

    // Product create (POST) under Starter should also succeed.
    resp = suite.AdminPOST(tenantID, storeID, "/admin/stores/"+storeID.String()+"/products",
        map[string]any{"name":"Test","price":100})
    require.NotEqual(t, 402, resp.Code, "payment_action_required must NOT return 402")
}
```

If `inttest.NewSuite` doesn't exist, use whatever harness already exists in `internal/handlers/admin/*_test.go`.

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/subscription_par_integration_test.go
git commit -m "test(admin): payment_action_required allows full admin access (spec criterion 38)"
```

---

## Task 9: Sequential-path integration test

**Files:**
- Create: `services/marketplace-api/internal/subscription/statemachine/sequential_path_test.go`

**Purpose:** Success criteria 48 + 49 — `expired → pending_hard_delete` requires passage through `store_closed`.

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestSequentialPath_ExpiredMustGoThroughStoreClosed(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusExpired,
    }).Error)

    // Illegal: expired → pending_hard_delete directly.
    err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusExpired, To: subscription.StatusPendingHardDelete,
        Actor: "system:cron", Reason: "day_90_shortcut",
    })
    require.ErrorIs(t, err, statemachine.ErrInvalidTransition)

    // Legal path: expired → store_closed → pending_hard_delete.
    require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusExpired, To: subscription.StatusStoreClosed,
        Actor: "system:cron", Reason: "day_14_post_expiry",
    }))
    require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
        DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
        From: subscription.StatusStoreClosed, To: subscription.StatusPendingHardDelete,
        Actor: "system:cron", Reason: "day_90_post_expiry",
    }))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.Equal(t, subscription.StatusPendingHardDelete, sub.Status)
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/statemachine/sequential_path_test.go
git commit -m "test(subscription): sequential expired→store_closed→pending_hard_delete (criterion 48/49)"
```

---

## Task 10: CAS conflict concurrency test

**Files:**
- Create: `services/marketplace-api/internal/subscription/statemachine/concurrency_test.go`

**Purpose:** Prove `WithAdvisoryLock` + CAS serializes concurrent transitions such that only one writer wins per race.

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestTransition_ConcurrentWriters_ExactlyOneSucceeds(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    // Two racers both try active → cancel_scheduled.
    var wg sync.WaitGroup
    var okCount, conflictCount int32
    for i := 0; i < 2; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
                DB: db, Emitter: em, TenantID: tenantID, StoreID: storeID,
                From: subscription.StatusActive, To: subscription.StatusCancelScheduled,
                Actor: "user:x", Reason: "race",
            })
            switch {
            case err == nil:
                atomic.AddInt32(&okCount, 1)
            case errors.Is(err, statemachine.ErrCASConflict):
                atomic.AddInt32(&conflictCount, 1)
            default:
                t.Errorf("unexpected error: %v", err)
            }
        }()
    }
    wg.Wait()

    require.EqualValues(t, 1, okCount, "exactly one writer must succeed")
    require.EqualValues(t, 1, conflictCount, "the other must see ErrCASConflict")
}
```

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/statemachine/concurrency_test.go
git commit -m "test(subscription): concurrent transitions serialize under advisory lock + CAS"
```

---

## Task 11: Final scrub for legacy constants

- [ ] **Step 1: Grep for legacy symbols**

```bash
cd services/marketplace-api
grep -Rn "PlanFree\|PlanEnterprise\|StatusCancelled\|StatusIncomplete" internal/ cmd/ | grep -v "_test.go" || echo "clean"
```
Expected: `clean`. If any hits remain, fix them (map to the new constants). Tests that deliberately reference legacy values for migration assertions can stay.

- [ ] **Step 2: Grep for direct status UPDATEs bypassing the state machine**

```bash
grep -RnE 'UPDATE\s+store_subscriptions\s+SET\s+status' internal/ \
  | grep -v "_test.go" \
  | grep -v statemachine/ \
  | grep -v 'dispatch/handlers\.go.*handleCheckoutSessionCompleted'
```
Expected: zero hits after the three exclusions. The one legitimate outlier is `handleCheckoutSessionCompleted` in `internal/billing/dispatch/handlers.go`, which still issues a raw UPDATE that sets `status` via a `COALESCE`+`CASE` clause for the `signup → trialing` binding step. That handler will be rewritten to call `statemachine.Transition` in **P5 (trial card-add deferred-charge flow)** — leaving it untouched here is deliberate and must NOT be "fixed".

Any other hits are bugs — route them through `Transition`.

- [ ] **Step 3: Run the full test suite**

```bash
go test -tags=integration ./... -count=1
```
Expected: green.

- [ ] **Step 4: Final commit**

```bash
git add -u
git commit --allow-empty -m "chore: scrub verified — no legacy plan/status symbols outside tests"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] All 17 transitions from §17.2 are in `transitionTable`; `AllValidTransitions()` length == 17.
- [ ] `statemachine.IsReadOnly(subscription.StatusPaymentActionRequired) == false` (Council finding #3).
- [ ] Every mutation in the P2 dispatcher goes through `statemachine.Transition`.
- [ ] Grep confirms no direct status UPDATEs outside `statemachine/`.
- [ ] Integration test: `payment_action_required` merchant can POST to `/admin/stores/:id/products` (not 402).
- [ ] Integration test: `expired` merchant receives 402 on `/admin/stores/:id/products` but 200 on `/admin/stores/:id/subscription/portal`.
- [ ] Integration test: `expired → pending_hard_delete` direct transition returns `ErrInvalidTransition`; sequential path succeeds.

## What's now unlocked

- **P4** (upgrade/downgrade + store-block) builds on `statemachine.Transition` + `plangate.Limit(FeatureStores)` + `WithAdvisoryLock`.
- **P5** (trial card-add deferred charge) transitions `signup → trialing` and `trialing → active` via this state machine.
- **P6** (dunning) owns the `active → past_due → active/expired` ladder and the `payment_action_required` recovery flow — which this plan *deliberately* leaves fully usable by the merchant.
- **P11** (cancellation + save-offer) transitions `active → cancel_scheduled` and the reversal `cancel_scheduled → active` — both already valid here.
- **P16** (admin frontend) reads `AllFeatureLimits(plan)` to drive plan-management UI.
- **P17** (observability) reads `subscription.state_transition` audit events for the state-count gauge and transition dashboards.

## Execution handoff

Plan complete. Three implementation plans are now saved under `docs/superpowers/plans/`:
- `2026-04-18-p1-subscription-data-model.md`
- `2026-04-18-p2-stripe-multicurrency-webhooks.md`
- `2026-04-18-p3-state-machine-plan-gates.md`

Execute each with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**, in order P1 → P2 → P3.
