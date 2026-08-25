# Trial extension pushes `trial_end` to Stripe (#358) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `POST /admin/billing/trials/{store_id}/extend`'s blanket `409 stripe_managed` with a path that moves the trial end in Stripe and locally, atomically, without ever changing the subscription's price.

**Architecture:** `trial.Extend` becomes a method on `trial.Extender` carrying a `StripeTrialUpdater`. One transaction takes `SELECT … FOR UPDATE` on the `store_subscriptions` row and holds it across a bounded (10s) Stripe call: validate → Stripe → local write → commit. Stripe is the source of truth for card-backed trials, so a failure leaves Stripe *ahead*, never behind. A nil updater reproduces today's refusal exactly.

**Tech Stack:** Go 1.26, Gin, GORM, `github.com/stripe/stripe-go/v82` v82.5.1, Postgres 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-26-trial-extend-stripe-design.md` — read it before Task 1. It records the four decisions this plan implements and why each alternative was rejected.

## Global Constraints

- Stripe SDK is **v82.5.1**. The root `CLAUDE.md` says v76; that line describes `marketplace-payment-service`, a different repo. Do not cite it.
- Service root for every command: `services/marketplace-api`. Never path-scope `go test ./...`.
- Integration runs: `-tags=integration -p 1 -count=1`, env `TEST_DATABASE_URL` (**not** `TEST_DB_DSN` — under that name `internal/billing/trial` silently skips 19 tests and prints `ok`).
- `TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"` — the LAN IP, never `localhost`; a native Postgres squats on 127.0.0.1.
- `go vet -tags=integration ./...` is the ONLY command that compiles `//go:build integration` files. It is part of the verification set for every task.
- Use `set -o pipefail` whenever a command's exit code will be reported as evidence.
- Money and dates: integer minor units with explicit currency; RFC3339 UTC on the wire; Unix seconds to Stripe.
- Never set `TrialFromPlan` anywhere on the extend path — Stripe rejects it alongside `trial_end`.
- **Do not** push, open a PR, merge, or deploy. Do not fix pre-existing failures listed below.

### Measured baseline — do not re-derive, do not "fix"

Measured 2026-08-26 in a throwaway worktree at `origin/main` (`68ac8bfa`):

`internal/subscription/planchange` fails **exactly 9** tests, every one with
`insert or update on table "store_subscriptions" violates foreign key constraint "store_subscriptions_store_id_fkey"` — the tests insert a subscription without seeding its parent `stores` row (#317 fixture drift, not logic):

```
TestCriterion39_DowngradeBlockedOverQuota_AtPeriodEnd
TestDowngradeRecheckCron_CommitsWhenEligible_Integration
TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected
TestExecute_Downgrade_StudioToStarter_ParksPending
TestExecute_RejectsCurrencyChange
TestExecute_Upgrade_Rejected_WhenStatusReadOnly
TestExecute_Upgrade_StarterToStudio_CommitsImmediately
TestGrandfathering_StudioToStarter_ImagesAllowed
TestPeriodSwitch_MonthlyToAnnual_Pro_CommitsImmediately
```

**Consequence for this branch:** those tests never reach the plan-change logic, so they are **not** a safety net for making the price swap optional. The regression proof lives in Task 1's `internal/billing/stripe` unit tests, which do run. After any task that touches `internal/billing/stripe`, re-run the planchange suite and compare the failing set to this list **by name**. A tenth name, or a different failure message, is yours.

Also pre-existing, also not yours: `internal/whitelabel` nil panic, `internal/outbox` 2 FAIL, ~23 marketplace-api packages failing on local dev-DB fixture drift.

## File Structure

| file | responsibility |
|---|---|
| `internal/billing/stripe/update.go` (modify) | `TrialEnd` on the params; `Items` only when `PriceID != ""`; new `UpdateTrialEnd` wrapper |
| `internal/billing/stripe/subscription.go` (modify) | project `TrialEnd` + `BillingCycleAnchor` off the SDK object |
| `internal/billing/stripe/update_test.go` (modify) | exact-form assertions: `trial_end` sent, `items[0][price]` absent, plan change unchanged |
| `internal/billing/trial/extend.go` (modify) | `Extender`, `StripeTrialUpdater`, the locked transaction, the card-backed branch, new sentinels |
| `internal/billing/trial/extend_stripe_integration_test.go` (create) | card-backed behaviour, boundaries, ordering-on-failure |
| `internal/handlers/platformadmin/billing_trial_extend.go` (modify) | new sentinels → status codes; three new response fields; audit metadata |
| `internal/billing/trial/expiring.go` (modify) | `ListOptions{IncludeStripeManaged}` on the list + its count |
| `internal/handlers/platformadmin/billing_trials.go` (modify) | `?include_stripe_managed=` and the `stripe_managed` row field |
| `cmd/marketplace-api/main.go` (modify) | build the updater, guard the typed nil, wire both `Register` sites |
| `scripts/verify-358-stripe.sh` (create) | the Stripe test-mode verification runbook, executable |

---

### Task 1: Stripe client can set a trial end without re-pricing

**Files:**
- Modify: `services/marketplace-api/internal/billing/stripe/update.go:24-66`
- Modify: `services/marketplace-api/internal/billing/stripe/subscription.go:14-25,44-51`
- Test: `services/marketplace-api/internal/billing/stripe/update_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `stripe.UpdateSubscriptionParams` gains `TrialEnd *int64`
  - `func UpdateTrialEnd(ctx context.Context, c *Client, in UpdateTrialEndParams) (*Subscription, error)`
  - `type UpdateTrialEndParams struct { SubscriptionID string; TrialEnd int64; IdempotencyKey string; Metadata map[string]string }`
  - `stripe.Subscription` gains `TrialEnd int64` and `BillingCycleAnchor int64`

- [ ] **Step 1: Write the failing tests**

Append to `internal/billing/stripe/update_test.go`:

```go
// subTrialJSON returns a trialing subscription with a trial_end and a
// billing_cycle_anchor, so a caller can read back what Stripe holds.
func subTrialJSON(subID, itemID, priceID string, trialEnd, anchor int64) string {
	return `{
		"id":"` + subID + `","status":"trialing","currency":"usd",
		"trial_end":` + strconv.FormatInt(trialEnd, 10) + `,
		"billing_cycle_anchor":` + strconv.FormatInt(anchor, 10) + `,
		"items":{"data":[{"id":"` + itemID + `","price":{"id":"` + priceID + `","currency":"usd"}}]}
	}`
}

// The acceptance criterion of #358: the EXACT integer must reach Stripe. A
// stub returns the zero value for a field nobody set, so asserting that a
// call happened proves nothing.
func TestUpdateTrialEnd_SendsExactTrialEndAndNoPrice(t *testing.T) {
	const (
		subID     = "sub_trial"
		itemID    = "si_trial"
		priceID   = "price_current"
		newEnd    = int64(1_790_000_000)
		oldEnd    = int64(1_780_000_000)
		oldAnchor = int64(1_780_000_000)
	)

	var postBody url.Values
	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID:
			postCount++
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subTrialJSON(subID, itemID, priceID, newEnd, newEnd)))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sub, err := billingstripe.UpdateTrialEnd(context.Background(), c, billingbillingstripe.UpdateTrialEndParams{
		SubscriptionID: subID,
		TrialEnd:       newEnd,
		IdempotencyKey: "trial_extend:store-1:abc",
		Metadata:       map[string]string{"mark8ly_reason_code": "goodwill"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, postCount)

	// The exact integer, not "a call happened".
	require.Equal(t, strconv.FormatInt(newEnd, 10), postBody.Get("trial_end"))
	// The price must be untouched: no items key of any kind.
	for k := range postBody {
		require.NotContains(t, k, "items[", "extend must not send items: %s=%s", k, postBody.Get(k))
	}
	require.Equal(t, "none", postBody.Get("proration_behavior"))
	require.Empty(t, postBody.Get("trial_from_plan"), "trial_from_plan is rejected by Stripe alongside trial_end")
	require.Equal(t, "goodwill", postBody.Get("metadata[mark8ly_reason_code]"))

	// The mapped result carries what Stripe now holds.
	require.Equal(t, newEnd, sub.TrialEnd)
	require.Equal(t, newEnd, sub.BillingCycleAnchor)
	_ = oldEnd
	_ = oldAnchor
}

// GetSubscription must expose trial_end and billing_cycle_anchor, or the
// two-year bound cannot be validated and the anchor move cannot be confirmed.
func TestGetSubscription_MapsTrialEndAndAnchor(t *testing.T) {
	const subID = "sub_read"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(subTrialJSON(subID, "si_1", "price_1", 1_781_000_000, 1_779_000_000)))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sub, err := billingstripe.GetSubscription(context.Background(), c, subID)
	require.NoError(t, err)
	require.Equal(t, int64(1_781_000_000), sub.TrialEnd)
	require.Equal(t, int64(1_779_000_000), sub.BillingCycleAnchor)
	require.Equal(t, "trialing", sub.Status)
}

// A plan change must STILL swap the price. This is the regression guard for
// making Items conditional — and it is the only one that runs, because the
// planchange integration suite is 9 FAIL on fixture drift and never reaches
// the logic (see the plan's measured baseline).
func TestUpdateSubscription_StillSendsPrice_WhenPriceIDSet(t *testing.T) {
	const (
		subID    = "sub_plan"
		itemID   = "si_plan"
		newPrice = "price_new"
	)
	var postBody url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, "price_old")))
		case r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, newPrice)))
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    subID,
		PriceID:           newPrice,
		ProrationBehavior: billingstripe.ProrationCreateProrations,
	})
	require.NoError(t, err)
	require.Equal(t, itemID, postBody.Get("items[0][id]"))
	require.Equal(t, newPrice, postBody.Get("items[0][price]"))
	require.Empty(t, postBody.Get("trial_end"), "a plan change must not move the trial end")
}

// Neither a price nor a trial end is a no-op update, and a no-op update that
// silently succeeds is how a caller comes to believe something changed.
func TestUpdateSubscription_RejectsEmptyUpdate(t *testing.T) {
	c := billingstripe.New("sk_test_x")
	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID: "sub_x",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price_id or trial_end required")
}
```

Add `"strconv"` to that file's imports.

- [ ] **Step 2: Run the tests and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 -run 'TestUpdateTrialEnd_|TestGetSubscription_MapsTrialEnd|TestUpdateSubscription_StillSendsPrice|TestUpdateSubscription_RejectsEmptyUpdate' ./internal/billing/stripe/ -v 2>&1 | tail -30
```

Expected: compile failure — `undefined: billingstripe.UpdateTrialEnd`, `sub.TrialEnd undefined`.

- [ ] **Step 3: Add the projected fields to the mapped Subscription**

In `internal/billing/stripe/subscription.go`, add to `type Subscription struct` after `Customer`:

```go
	// TrialEnd and BillingCycleAnchor are Unix seconds, 0 when Stripe
	// reports none. Projected explicitly rather than embedding the SDK
	// object: a passthrough leaks every field Stripe adds upstream.
	// BillingCycleAnchor is needed because SubscriptionUpdateParams.TrialEnd
	// is bounded at two years FROM THE ANCHOR, not from now.
	TrialEnd           int64 `json:"trial_end"`
	BillingCycleAnchor int64 `json:"billing_cycle_anchor"`
```

and in `mapSubscription`, inside the `out := &Subscription{...}` literal:

```go
		TrialEnd:           s.TrialEnd,
		BillingCycleAnchor: s.BillingCycleAnchor,
```

- [ ] **Step 4: Make the price swap optional and add TrialEnd**

In `internal/billing/stripe/update.go`, replace the `UpdateSubscriptionParams` struct and the body of `UpdateSubscription`:

```go
// UpdateSubscriptionParams captures the inputs for a subscription update.
// The current subscription item ID is resolved internally via GetSubscription
// when — and only when — a PriceID is supplied.
type UpdateSubscriptionParams struct {
	SubscriptionID    string
	PriceID           string // optional: when empty, the price is left alone
	TrialEnd          *int64 // optional: Unix seconds; moves billing_cycle_anchor
	ProrationBehavior ProrationBehavior
	IdempotencyKey    string
	Metadata          map[string]string
}

// UpdateSubscription updates an existing Stripe Subscription.
//
// Items are sent ONLY when PriceID is set. This matters: before #358 the
// price swap was unconditional, so calling this to move a trial end would
// have re-priced the subscription silently. The two plan-change callers
// (subscription/planchange) always set PriceID and are unaffected.
//
// Setting TrialEnd also moves Stripe's billing_cycle_anchor to that value —
// Stripe's own documented behaviour, not ours — so it changes when the
// merchant is billed thereafter. Stripe bounds it at two years from the
// current anchor. TrialFromPlan is never set here; Stripe rejects it
// alongside trial_end.
func UpdateSubscription(ctx context.Context, c *Client, in UpdateSubscriptionParams) (*Subscription, error) {
	if in.PriceID == "" && in.TrialEnd == nil {
		return nil, errors.New("stripe: UpdateSubscription: price_id or trial_end required")
	}

	params := &sdk.SubscriptionUpdateParams{
		ProrationBehavior: sdk.String(string(in.ProrationBehavior)),
	}

	if in.PriceID != "" {
		current, err := GetSubscription(ctx, c, in.SubscriptionID)
		if err != nil {
			return nil, err
		}
		if len(current.Items.Data) == 0 {
			return nil, errors.New("stripe: subscription has no items, cannot update")
		}
		params.Items = []*sdk.SubscriptionUpdateItemParams{
			{
				ID:    sdk.String(current.Items.Data[0].ID),
				Price: sdk.String(in.PriceID),
			},
		}
	}

	if in.TrialEnd != nil {
		params.TrialEnd = sdk.Int64(*in.TrialEnd)
	}

	for k, v := range in.Metadata {
		params.AddMetadata(k, v)
	}
	if in.IdempotencyKey != "" {
		params.SetIdempotencyKey(in.IdempotencyKey)
	}

	sdkSub, err := c.sdk.V1Subscriptions.Update(ctx, in.SubscriptionID, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapSubscription(sdkSub), nil
}

// UpdateTrialEndParams captures a trial-end move and nothing else.
type UpdateTrialEndParams struct {
	SubscriptionID string
	TrialEnd       int64 // Unix seconds — required
	IdempotencyKey string
	Metadata       map[string]string
}

// UpdateTrialEnd moves a subscription's trial end without touching its price.
//
// A narrow wrapper rather than a documented convention on UpdateSubscription:
// the extend path must be structurally incapable of acquiring a PriceID
// later, and a struct with no PriceID field is the only way to guarantee that
// against a future edit. proration_behavior is `none` because the anchor move
// this causes must not generate a proration invoice.
func UpdateTrialEnd(ctx context.Context, c *Client, in UpdateTrialEndParams) (*Subscription, error) {
	if in.TrialEnd <= 0 {
		return nil, errors.New("stripe: UpdateTrialEnd: trial_end required")
	}
	return UpdateSubscription(ctx, c, UpdateSubscriptionParams{
		SubscriptionID:    in.SubscriptionID,
		TrialEnd:          &in.TrialEnd,
		ProrationBehavior: ProrationNone,
		IdempotencyKey:    in.IdempotencyKey,
		Metadata:          in.Metadata,
	})
}
```

- [ ] **Step 5: Run the tests and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 ./internal/billing/stripe/... 2>&1 | tail -5
go vet -tags=integration ./... 2>&1 | tail -5
```

Expected: `ok  …/internal/billing/stripe`, vet clean.

- [ ] **Step 6: Prove the price guard by mutation**

Temporarily change `if in.PriceID != ""` to `if true` and re-run
`TestUpdateTrialEnd_SendsExactTrialEndAndNoPrice`. It MUST fail (the loop over `postBody` finds `items[0][id]`). Then revert. A guard whose test passes with the guard removed is not a test. Record the observed failure line in the commit message.

- [ ] **Step 7: Confirm the planchange baseline is unmoved**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 ./internal/subscription/planchange/... 2>&1 \
  | grep '^--- FAIL' | sed 's/ *(.*//' | sort > /tmp/planchange-now.txt
diff /tmp/planchange-now.txt <(printf '%s\n' \
  '--- FAIL: TestCriterion39_DowngradeBlockedOverQuota_AtPeriodEnd' \
  '--- FAIL: TestDowngradeRecheckCron_CommitsWhenEligible_Integration' \
  '--- FAIL: TestExecute_Downgrade_StudioToStarter_OverQuota_Rejected' \
  '--- FAIL: TestExecute_Downgrade_StudioToStarter_ParksPending' \
  '--- FAIL: TestExecute_RejectsCurrencyChange' \
  '--- FAIL: TestExecute_Upgrade_Rejected_WhenStatusReadOnly' \
  '--- FAIL: TestExecute_Upgrade_StarterToStudio_CommitsImmediately' \
  '--- FAIL: TestGrandfathering_StudioToStarter_ImagesAllowed' \
  '--- FAIL: TestPeriodSwitch_MonthlyToAnnual_Pro_CommitsImmediately') \
  && echo "BASELINE UNCHANGED"
```

Expected: `BASELINE UNCHANGED`. Any diff is yours to explain — do not proceed.

- [ ] **Step 8: Commit**

```bash
git add internal/billing/stripe/update.go internal/billing/stripe/subscription.go internal/billing/stripe/update_test.go
git commit -m "feat(stripe): optional price swap and trial_end on subscription update (#358)"
```

---

### Task 2: `trial.Extender` carries the Stripe dependency, nil reproduces today

**Files:**
- Modify: `services/marketplace-api/internal/billing/trial/extend.go`
- Test: `services/marketplace-api/internal/billing/trial/extend_stripe_integration_test.go` (create)

**Interfaces:**
- Consumes: `stripe.Subscription` (Task 1), `stripe.UpdateTrialEndParams` (Task 1).
- Produces:
  - `type StripeTrialUpdater interface { GetSubscription(ctx, id string) (*billingstripe.Subscription, error); UpdateTrialEnd(ctx, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) }`
  - `type Extender struct{ Stripe StripeTrialUpdater }`
  - `func NewExtender(su StripeTrialUpdater) *Extender`
  - `func (e *Extender) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error)`

This task is a pure refactor: no card-backed behaviour yet. `NewExtender(nil).Extend` must behave byte-for-byte as today's `trial.Extend`, and every existing test in `internal/billing/trial` must still pass unchanged.

- [ ] **Step 1: Write the failing test**

Create `internal/billing/trial/extend_stripe_integration_test.go`:

```go
//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var stripeExtendAsOf = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// A card-less trial must behave IDENTICALLY through the Extender as it did
// through the package function. This is the regression guard for the
// refactor: if the new code path changes the common support case, this fails.
func TestExtender_CardlessPathUnchanged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedExpiringRow(t, db, derivedEnd, nil)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	res, err := trial.NewExtender(nil).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)
	require.True(t, derivedEnd.Equal(res.PreviousEndsAt))
	require.True(t, newEnd.Equal(res.NewEndsAt))
	require.False(t, res.StripeApplied, "a card-less extension touches no Stripe subscription")

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
}

// A card-backed trial on a build with NO Stripe configured must refuse
// exactly as it does today. This is the guarantee that an unconfigured pod
// can never silently extend a Stripe-managed trial locally.
func TestExtender_NilUpdater_RefusesCardBacked(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	subID := "sub_nil_updater"
	seeded := seedExpiringRow(t, db, stripeExtendAsOf.Add(10*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })

	_, err := trial.NewExtender(nil).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "a refused extension must write nothing")
}
```

- [ ] **Step 2: Run it and verify it fails**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 -run 'TestExtender_' ./internal/billing/trial/ -v 2>&1 | tail -20
```

Expected: compile failure — `undefined: trial.NewExtender`, `res.StripeApplied undefined`.

Confirm from the verbose output that the tests RUN. A `--- SKIP` means `TEST_DATABASE_URL` is unset and you are proving nothing.

- [ ] **Step 3: Introduce the Extender**

In `internal/billing/trial/extend.go`, add above `Extend`:

```go
// StripeTrialUpdater is the subset of the Stripe client this package needs,
// declared here rather than imported as a concrete type so the extension can
// be tested without a live Stripe and so the dependency points inward.
type StripeTrialUpdater interface {
	GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error)
	UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error)
}

// Extender owns "move a trial's end date", for both card-less and
// card-backed trials.
//
// A nil Stripe field is a SUPPORTED configuration, not a degraded one: a
// build without STRIPE_BILLING_SECRET_KEY refuses card-backed trials with
// ErrStripeManaged, exactly as this endpoint did before #358. Callers MUST
// leave the interface nil rather than assigning a nil *stripe.Client into
// it — a typed nil makes `e.Stripe != nil` true and panics on first use.
type Extender struct {
	Stripe StripeTrialUpdater
}

// NewExtender constructs an Extender. su may be nil; see the type's comment.
func NewExtender(su StripeTrialUpdater) *Extender {
	return &Extender{Stripe: su}
}
```

Add `billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"` to the imports — every other file in this package uses that alias (`subscribe.go:17`, `adapter_test.go:12`), and a second spelling for one import is the kind of inconsistency that makes a later reader think two packages are involved.

Add `StripeApplied bool` to `ExtendResult` with a comment:

```go
	// StripeApplied is true only when this extension moved the trial end in
	// Stripe. False for every card-less extension. The handler surfaces it so
	// an operator learns from the same call whether a billing anchor moved.
	StripeApplied bool
```

Change `func Extend(...)` to `func (e *Extender) Extend(...)` with the identical signature and body, and delete the package-level `Extend`. Then update its existing callers:

- `internal/billing/trial/*_test.go`: replace `trial.Extend(` with `trial.NewExtender(nil).Extend(`.
- `cmd/marketplace-api/main.go:2021` and `:2145` are updated in Task 5; for now change them to `platformadmin.TrialExtenderFunc(trial.NewExtender(nil).Extend)` so the build stays green.

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 ./internal/billing/trial/ -v 2>&1 | grep -E '^(--- |ok|FAIL)' | head -40
go build ./... && go vet -tags=integration ./... 2>&1 | tail -5
```

Expected: the two new tests PASS and every previously-passing test in the package still passes. Count the `--- PASS` lines against the pre-change run; a test that changed to `--- SKIP` is a regression, not a pass.

- [ ] **Step 5: Commit**

```bash
git add internal/billing/trial/ cmd/marketplace-api/main.go
git commit -m "refactor(trial): Extend becomes a method on Extender carrying the Stripe dependency (#358)"
```

---

### Task 3: the card-backed extension path

**Files:**
- Modify: `services/marketplace-api/internal/billing/trial/extend.go`
- Test: `services/marketplace-api/internal/billing/trial/extend_stripe_integration_test.go`

**Interfaces:**
- Consumes: `Extender`, `StripeTrialUpdater`, `ExtendResult.StripeApplied` (Task 2); `stripe.UpdateTrialEndParams`, `stripe.Subscription.TrialEnd/BillingCycleAnchor` (Task 1).
- Produces:
  - `var ErrStripeStateConflict`, `var ErrTrialEndTooFar`, `var ErrStripeCall`
  - `ExtendResult` gains `StripeSubscriptionID string`, `StripeTrialEnd int64`, `PreviousStripeTrialEnd int64`, `PreviousBillingAnchor int64`
  - `const maxStripeTrialWindow = 2 * 365 * 24 * time.Hour`
  - `const stripeCallTimeout = 10 * time.Second`

- [ ] **Step 1: Write the failing tests**

Append to `internal/billing/trial/extend_stripe_integration_test.go`:

```go
// fakeUpdater records what it was asked to do and returns what it is told to.
type fakeUpdater struct {
	get        *billingstripe.Subscription
	getErr     error
	updated    *billingstripe.Subscription
	updateErr  error
	seenParams billingstripe.UpdateTrialEndParams
	updateCalls int
}

func (f *fakeUpdater) GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.get, nil
}

func (f *fakeUpdater) UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	f.updateCalls++
	f.seenParams = in
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updated, nil
}

func seedCardBacked(t *testing.T, db *gorm.DB, subID string, derivedEnd time.Time) subscription.StoreSubscription {
	t.Helper()
	return seedExpiringRow(t, db, derivedEnd, func(s *subscription.StoreSubscription) {
		s.StripeSubscriptionID = &subID
	})
}

// The acceptance criterion, at the domain layer: the EXACT Unix second must
// be handed to Stripe, and the local row must agree with it.
func TestExtender_CardBacked_SendsExactUnixSecondAndWritesLocally(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_ok"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{
			ID: subID, Status: "trialing",
			TrialEnd:           derivedEnd.Unix(),
			BillingCycleAnchor: derivedEnd.Unix(),
		},
		updated: &billingstripe.Subscription{
			ID: subID, Status: "trialing",
			TrialEnd:           newEnd.Unix(),
			BillingCycleAnchor: newEnd.Unix(),
		},
	}

	res, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)

	require.Equal(t, 1, f.updateCalls)
	require.Equal(t, newEnd.Unix(), f.seenParams.TrialEnd,
		"the exact integer sent to Stripe is the acceptance criterion")
	require.Equal(t, subID, f.seenParams.SubscriptionID)

	require.True(t, res.StripeApplied)
	require.Equal(t, subID, res.StripeSubscriptionID)
	require.Equal(t, newEnd.Unix(), res.StripeTrialEnd, "read back from Stripe's reply, not echoed from the request")
	require.Equal(t, derivedEnd.Unix(), res.PreviousStripeTrialEnd)
	require.Equal(t, derivedEnd.Unix(), res.PreviousBillingAnchor)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
}

// THE FAILURE ORDERING, AS A TEST. Stripe fails => nothing is written
// locally. This is the decision the issue required be made deliberately;
// deleting the rollback must break this.
func TestExtender_CardBacked_StripeFailure_WritesNothingLocally(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_fail"
	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedCardBacked(t, db, subID, derivedEnd)
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		 VALUES (?, ?, ?, 'no_pm_t_minus_15', ?)`,
		seeded.ID, seeded.TenantID, seeded.StoreID, stripeExtendAsOf,
	).Error)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updateErr: errors.New("stripe: boom"),
	}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeCall)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "Stripe failed: the local row must be untouched")

	var reminders int64
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", seeded.ID).Count(&reminders).Error)
	require.Equal(t, int64(1), reminders, "Stripe failed: the reminder rows must survive")
}

// Local says trialing, Stripe says active. Refuse rather than reconcile —
// and do not call update.
func TestExtender_CardBacked_StripeNotTrialing_Refuses(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	const subID = "sub_card_active"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))

	f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "active",
		TrialEnd: 0, BillingCycleAnchor: stripeExtendAsOf.Unix()}}

	_, err := trial.NewExtender(f).Extend(context.Background(), db,
		seeded.StoreID, stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeStateConflict)
	require.Equal(t, 0, f.updateCalls, "a refusal must not reach Stripe's update")
}

// THE BOUND, ON THE BOUNDARY. Stripe caps trial_end at two years from the
// CURRENT anchor. Exactly two years passes; one second past it refuses.
// "Close to the edge" is not the edge.
func TestExtender_CardBacked_TwoYearBound_AtTheBoundary(t *testing.T) {
	anchor := stripeExtendAsOf
	twoYears := 2 * 365 * 24 * time.Hour

	t.Run("exactly two years from anchor is allowed", func(t *testing.T) {
		db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
		const subID = "sub_bound_ok"
		seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
		newEnd := anchor.Add(twoYears)

		f := &fakeUpdater{
			get: &billingstripe.Subscription{ID: subID, Status: "trialing",
				TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()},
			updated: &billingstripe.Subscription{ID: subID, Status: "trialing",
				TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
		}
		_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
		require.NoError(t, err)
		require.Equal(t, 1, f.updateCalls)
	})

	t.Run("one second past two years refuses", func(t *testing.T) {
		db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
		const subID = "sub_bound_bad"
		seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))
		newEnd := anchor.Add(twoYears + time.Second)

		f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "trialing",
			TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()}}
		_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
		require.ErrorIs(t, err, trial.ErrTrialEndTooFar)
		require.Equal(t, 0, f.updateCalls)
	})
}

// The bound is measured from Stripe's ANCHOR, not from now. This is the
// fixture that discriminates between the two implementations: the anchor is
// deliberately far from `now`, so a now-based bound gives a different answer.
func TestExtender_CardBacked_BoundIsMeasuredFromAnchorNotNow(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
	const subID = "sub_anchor_far"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))

	// Anchor is 18 months in the PAST, so anchor+2y is only 6 months out.
	anchor := stripeExtendAsOf.Add(-18 * 30 * 24 * time.Hour)
	newEnd := stripeExtendAsOf.Add(12 * 30 * 24 * time.Hour) // legal under now+2y, illegal under anchor+2y

	f := &fakeUpdater{get: &billingstripe.Subscription{ID: subID, Status: "trialing",
		TrialEnd: stripeExtendAsOf.Unix(), BillingCycleAnchor: anchor.Unix()}}
	_, err := trial.NewExtender(f).Extend(context.Background(), db, seeded.StoreID, newEnd, stripeExtendAsOf)
	require.ErrorIs(t, err, trial.ErrTrialEndTooFar,
		"a now-based bound would allow this; the bound is from the anchor")
	require.Equal(t, 0, f.updateCalls)
}

// Two stores, one Extender: a card-backed extension must move exactly the
// subscription it was asked for. One store cannot prove scoping (trap 13,
// which cost #286 a Critical).
func TestExtender_CardBacked_ScopedToTheRequestedStore(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := stripeExtendAsOf.Add(10 * 24 * time.Hour)
	target := seedCardBacked(t, db, "sub_target", derivedEnd)
	other := seedCardBacked(t, db, "sub_other", derivedEnd)
	newEnd := stripeExtendAsOf.Add(60 * 24 * time.Hour)

	f := &fakeUpdater{
		get: &billingstripe.Subscription{ID: "sub_target", Status: "trialing",
			TrialEnd: derivedEnd.Unix(), BillingCycleAnchor: derivedEnd.Unix()},
		updated: &billingstripe.Subscription{ID: "sub_target", Status: "trialing",
			TrialEnd: newEnd.Unix(), BillingCycleAnchor: newEnd.Unix()},
	}
	_, err := trial.NewExtender(f).Extend(context.Background(), db, target.StoreID, newEnd, stripeExtendAsOf)
	require.NoError(t, err)
	require.Equal(t, "sub_target", f.seenParams.SubscriptionID)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", other.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt, "the other store must not be extended")
}
```

Add `"errors"`, `"gorm.io/gorm"` and `billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"` to that file's imports (the alias every other file in this package uses).

- [ ] **Step 2: Run and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 -run 'TestExtender_CardBacked' ./internal/billing/trial/ -v 2>&1 | tail -20
```

Expected: compile failure — `undefined: trial.ErrStripeCall`, `res.StripeTrialEnd undefined`.

- [ ] **Step 3: Implement the card-backed branch**

In `internal/billing/trial/extend.go`, add to the sentinel block:

```go
	// ErrStripeStateConflict: the local row says trialing but Stripe does
	// not. Reconciling silently is not this endpoint's job.
	ErrStripeStateConflict = errors.New("trial: stripe subscription is not trialing")
	// ErrTrialEndTooFar: Stripe bounds trial_end at two years FROM THE
	// CURRENT billing_cycle_anchor — not from now, which is a different
	// instant whenever the anchor is not near now.
	ErrTrialEndTooFar = errors.New("trial: new trial end is more than two years from the stripe billing anchor")
	// ErrStripeCall: Stripe was reached for and did not succeed. Nothing was
	// written locally; the caller may retry.
	ErrStripeCall = errors.New("trial: stripe call failed")
```

Add the constants:

```go
// maxStripeTrialWindow mirrors Stripe's documented bound on
// SubscriptionUpdateParams.TrialEnd: "Can be at most two years from
// billing_cycle_anchor". Validated locally so the operator gets our error
// envelope and the actual bound, rather than an opaque Stripe 400.
const maxStripeTrialWindow = 2 * 365 * 24 * time.Hour

// stripeCallTimeout bounds how long the store_subscriptions row lock is held
// across the external call. The lock is deliberate — it is what removes the
// window in which the row could convert while Stripe is in flight — and this
// ceiling is what keeps "deliberate" from becoming "indefinite".
const stripeCallTimeout = 10 * time.Second
```

Add the four fields to `ExtendResult`:

```go
	// The Stripe-side facts, populated only when StripeApplied is true.
	// StripeTrialEnd is read from Stripe's REPLY, never echoed from our
	// request: what we asked for and what Stripe stored are two claims.
	StripeSubscriptionID   string
	StripeTrialEnd         int64
	PreviousStripeTrialEnd int64
	PreviousBillingAnchor  int64
```

Inside the transaction, take the row lock and replace the card-backed refusal:

```go
		var sub subscription.StoreSubscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("store_id = ?", storeID).First(&sub).Error; err != nil {
```

(add `"gorm.io/gorm/clause"` to the imports)

then replace the `case sub.StripeSubscriptionID != nil && *sub.StripeSubscriptionID != "":` arm so it no longer refuses unconditionally — the switch becomes:

```go
		switch {
		case sub.Status == subscription.StatusActive:
			return ErrAlreadyConverted
		case sub.Status != subscription.StatusTrialing && sub.Status != subscription.StatusSignup:
			return ErrNotTrialing
		}
```

and after the `EndsAt(sub)` check and the `out.PreviousEndsAt` assignment, before the local `Update`:

```go
		// Card-backed: Stripe owns the billing date, so Stripe moves FIRST
		// and is the source of truth. The row lock taken above is held
		// across this call, so nothing can convert or re-extend underneath
		// it; stripeCallTimeout bounds how long that lock can live.
		//
		// The ordering is the decision #358 required be made deliberately:
		// if this call fails the transaction rolls back and NOTHING is
		// written locally. If instead the commit below fails, Stripe is
		// AHEAD of us — the merchant is billed LATER than the console shows,
		// which is the safe direction. The reverse ordering fails the other
		// way.
		if stripeID := stripeSubscriptionID(sub); stripeID != "" {
			if e.Stripe == nil {
				// No Stripe configured: refuse exactly as this endpoint did
				// before #358. A local-only extension of a Stripe-managed
				// trial would put the console and Stripe in disagreement
				// about when a real merchant is charged.
				return ErrStripeManaged
			}

			sctx, cancel := context.WithTimeout(ctx, stripeCallTimeout)
			defer cancel()

			current, err := e.Stripe.GetSubscription(sctx, stripeID)
			if err != nil {
				return fmt.Errorf("%w: get subscription: %v", ErrStripeCall, err)
			}
			if current.Status != "trialing" {
				return ErrStripeStateConflict
			}
			anchor := time.Unix(current.BillingCycleAnchor, 0).UTC()
			if end.After(anchor.Add(maxStripeTrialWindow)) {
				return ErrTrialEndTooFar
			}

			updated, err := e.Stripe.UpdateTrialEnd(sctx, billingstripe.UpdateTrialEndParams{
				SubscriptionID: stripeID,
				TrialEnd:       end.Unix(),
				// Derived from the store, so a retry of the SAME extension
				// cannot move the date twice, while a different extension of
				// the same store still can.
				IdempotencyKey: "trial_extend:" + sub.StoreID.String() + ":" + strconv.FormatInt(end.Unix(), 10),
				Metadata: map[string]string{
					"mark8ly_store_id":  sub.StoreID.String(),
					"mark8ly_tenant_id": sub.TenantID.String(),
				},
			})
			if err != nil {
				return fmt.Errorf("%w: update trial end: %v", ErrStripeCall, err)
			}

			out.StripeApplied = true
			out.StripeSubscriptionID = stripeID
			out.StripeTrialEnd = updated.TrialEnd
			out.PreviousStripeTrialEnd = current.TrialEnd
			out.PreviousBillingAnchor = current.BillingCycleAnchor
		}
```

Move `end := newEnd.UTC()` above this block so it is in scope. Add `"strconv"` to the imports, and add the helper at the bottom of the file:

```go
// stripeSubscriptionID returns the subscription's Stripe id, or "" when it
// has none. A nil pointer and a pointer to "" mean the same thing here — the
// trial is card-less — and collapsing them at one site keeps every caller
// from having to remember that.
func stripeSubscriptionID(sub subscription.StoreSubscription) string {
	if sub.StripeSubscriptionID == nil {
		return ""
	}
	return *sub.StripeSubscriptionID
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 ./internal/billing/trial/ -v 2>&1 | grep -E '^(--- |ok|FAIL)' | head -50
```

Expected: every `TestExtender_*` PASSes and no previously-passing test in the package regresses.

- [ ] **Step 5: Prove the ordering by mutation**

Move the `if stripeID := …` block to AFTER the local `Update` and the reminder `DELETE`, and re-run `TestExtender_CardBacked_StripeFailure_WritesNothingLocally`. It MUST fail — the row now carries a `trial_ends_at` a failed Stripe call never earned. Revert, and record the observed failure in the commit message. This is the test that encodes the decision; if it passes under both orderings it encodes nothing.

- [ ] **Step 6: Commit**

```bash
git add internal/billing/trial/extend.go internal/billing/trial/extend_stripe_integration_test.go
git commit -m "feat(trial): move a card-backed trial end in Stripe before writing locally (#358)"
```

---

### Task 4: the handler surfaces the new outcomes

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go:105-115,225-260,340-382`
- Test: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_test.go`

**Interfaces:**
- Consumes: `trial.ErrStripeStateConflict`, `trial.ErrTrialEndTooFar`, `trial.ErrStripeCall`, `ExtendResult.StripeApplied/StripeSubscriptionID/StripeTrialEnd/PreviousStripeTrialEnd/PreviousBillingAnchor` (Task 3).
- Produces: response fields `stripe_subscription_id`, `stripe_trial_end`, `billing_anchor_moved`; audit metadata keys `stripe_subscription_id`, `stripe_trial_end_unix`, `previous_stripe_trial_end_unix`, `previous_billing_anchor_unix`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/handlers/platformadmin/billing_trial_extend_test.go`. It already provides `stubExtender`, `capturedAudit`, `okResult()`, `postExtend()` and `goodBody` — reuse them rather than inventing a second setup.

```go
// stripeOKResult is okResult() plus the Stripe-side facts a card-backed
// extension produces. Every value is DISTINCT and non-zero: a payload
// assembled by map lookup returns the zero value for a key nobody set, so
// identical or zero fixtures would let a broken mapping pass.
func stripeOKResult() trial.ExtendResult {
	r := okResult()
	r.StripeApplied = true
	r.StripeSubscriptionID = "sub_verify_358"
	r.StripeTrialEnd = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).Unix()
	r.PreviousStripeTrialEnd = time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC).Unix()
	r.PreviousBillingAnchor = time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC).Unix()
	return r
}

// The new refusals must be distinguishable by the console. 502 is
// deliberately NOT the handler's existing 503 `unavailable`: 503 means our
// own idempotency store is unreachable, 502 means the dependency refused —
// and, critically, that no local write happened.
func TestExtend_StripeRefusalsMapToDistinctStatuses(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unconfigured", trial.ErrStripeManaged, http.StatusConflict, "stripe_managed"},
		{"state conflict", trial.ErrStripeStateConflict, http.StatusConflict, "stripe_state_conflict"},
		{"too far", trial.ErrTrialEndTooFar, http.StatusBadRequest, "trial_end_too_far"},
		{"call failed", fmt.Errorf("%w: update trial end: boom", trial.ErrStripeCall), http.StatusBadGateway, "stripe_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := &capturedAudit{}
			rec := postExtend(t, &stubExtender{err: tc.err}, aud, extendStoreID.String(), goodBody)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())

			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tc.wantCode, got["error"])

			// A refusal is not an operator action: nothing was extended, so
			// nothing may be audited as extended.
			require.Empty(t, aud.events, "a refused extension must emit no audit event")

			// The driver's own text must never be echoed to the caller.
			msg, _ := got["message"].(string)
			require.NotContains(t, msg, "boom")
		})
	}
}

// A card-backed extension must disclose that the billing anchor moved, and
// must echo STRIPE's value rather than the request's.
func TestExtend_CardBacked_ResponseDisclosesStripeFacts(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: stripeOKResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "sub_verify_358", got["stripe_subscription_id"])
	require.Equal(t, "2026-12-01T00:00:00Z", got["stripe_trial_end"])
	require.Equal(t, true, got["billing_anchor_moved"])
}

// A card-less extension must carry NONE of those keys — not null, not false,
// absent. Asserted on the RAW BYTES: a decoded map cannot tell an absent key
// from a false one, which is exactly the distinction being made here.
func TestExtend_CardLess_OmitsStripeFields(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := rec.Body.String()
	require.NotContains(t, body, "stripe_subscription_id")
	require.NotContains(t, body, "stripe_trial_end")
	require.NotContains(t, body, "billing_anchor_moved")
}

// The audit row must carry the exact integer sent to Stripe. An audit that
// records only "extended" cannot answer "extended to what, in Stripe?" —
// which is the question this whole series exists to be able to answer.
func TestExtend_CardBacked_AuditCarriesExactUnixSecond(t *testing.T) {
	aud := &capturedAudit{}
	res := stripeOKResult()
	rec := postExtend(t, &stubExtender{result: res}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, aud.events, 1)
	md := aud.events[0].Metadata
	require.Equal(t, "sub_verify_358", md["stripe_subscription_id"])
	require.Equal(t, res.StripeTrialEnd, md["stripe_trial_end_unix"])
	require.Equal(t, res.PreviousStripeTrialEnd, md["previous_stripe_trial_end_unix"])
	require.Equal(t, res.PreviousBillingAnchor, md["previous_billing_anchor_unix"])

	// The two anchors are DIFFERENT values in this fixture, so a mapping
	// that swapped them would be caught. Identical fixtures prove nothing.
	require.NotEqual(t, md["stripe_trial_end_unix"], md["previous_billing_anchor_unix"])
}

// A card-less extension must add none of the Stripe keys to the audit row
// either — the metadata says what happened, and nothing Stripe-shaped did.
func TestExtend_CardLess_AuditHasNoStripeKeys(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, aud.events, 1)
	for _, k := range []string{"stripe_subscription_id", "stripe_trial_end_unix",
		"previous_stripe_trial_end_unix", "previous_billing_anchor_unix"} {
		_, present := aud.events[0].Metadata[k]
		require.False(t, present, "card-less audit must not carry %s", k)
	}
}
```

Add `"fmt"` to that file's imports.

- [ ] **Step 2: Run and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 -run 'TestExtend_Stripe|TestExtend_CardBacked|TestExtend_CardLess' ./internal/handlers/platformadmin/ -v 2>&1 | tail -25
```

Expected: FAIL — the new codes map to the default 500 branch and the response has no Stripe fields.

- [ ] **Step 3: Extend the response struct**

In `billing_trial_extend.go`:

```go
type trialExtendResponse struct {
	StoreID             string `json:"store_id"`
	TenantID            string `json:"tenant_id"`
	TrialEndsAt         string `json:"trial_ends_at"`
	PreviousTrialEndsAt string `json:"previous_trial_ends_at"`
	ExtendedAt          string `json:"extended_at"`
	ReasonCode          string `json:"reason_code"`
	Reason              string `json:"reason,omitempty"`
	RemindersCleared    int64  `json:"reminders_cleared"`

	// Present only for a card-backed extension. omitempty throughout: a
	// card-less extension carries no Stripe keys at all, rather than nulls
	// or a `false` that reads as "we checked and it did not move".
	StripeSubscriptionID string `json:"stripe_subscription_id,omitempty"`
	StripeTrialEnd       string `json:"stripe_trial_end,omitempty"`
	BillingAnchorMoved   bool   `json:"billing_anchor_moved,omitempty"`
}
```

- [ ] **Step 4: Populate it and the audit metadata**

After `resp := trialExtendResponse{…}` is built, before the idempotency save:

```go
	if res.StripeApplied {
		// stripe_trial_end is Stripe's own reply, not our request: the two
		// are different claims and only one of them is authoritative.
		resp.StripeSubscriptionID = res.StripeSubscriptionID
		resp.StripeTrialEnd = time.Unix(res.StripeTrialEnd, 0).UTC().Format(time.RFC3339)
		// Stripe moves billing_cycle_anchor to trial_end on every trial_end
		// update — its documented behaviour, confirmed against the API in
		// #358's verification. The operator learns the merchant's billing
		// date moved from the same response that moved it.
		resp.BillingAnchorMoved = true
	}
```

and inside the `if h.audit != nil` block, after the existing `Metadata` map is built:

```go
		if res.StripeApplied {
			ev.Metadata["stripe_subscription_id"] = res.StripeSubscriptionID
			ev.Metadata["stripe_trial_end_unix"] = res.StripeTrialEnd
			ev.Metadata["previous_stripe_trial_end_unix"] = res.PreviousStripeTrialEnd
			ev.Metadata["previous_billing_anchor_unix"] = res.PreviousBillingAnchor
		}
```

- [ ] **Step 5: Map the new sentinels**

In `respondExtendErr`, before the `default:` arm:

```go
	case errors.Is(err, trial.ErrStripeStateConflict):
		c.JSON(http.StatusConflict, gin.H{
			"error":   "stripe_state_conflict",
			"message": "stripe reports this subscription is no longer trialing; it cannot be extended until the two agree",
		})
	case errors.Is(err, trial.ErrTrialEndTooFar):
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "trial_end_too_far",
			"message": "trial_ends_at is more than two years after the stripe billing anchor, which stripe does not allow",
			"field":   "trial_ends_at",
		})
	case errors.Is(err, trial.ErrStripeCall):
		// 502, not the 503 above: 503 means OUR idempotency store is
		// unreachable, 502 means the dependency failed. The distinction
		// matters to the operator because a 502 here also guarantees
		// nothing was written locally.
		h.logger.Error("trial extend: stripe call failed", "err", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "stripe_unavailable", "message": "could not update the trial end in stripe; nothing was changed",
		})
```

Also update `ErrStripeManaged`'s message, which currently says the trial "cannot be extended here":

```go
			"message": "this trial has a stripe subscription and stripe billing is not configured on this service, so its billing date cannot be moved",
```

- [ ] **Step 6: Run the tests and verify they pass**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 ./internal/handlers/platformadmin/... 2>&1 | tail -5
go vet -tags=integration ./... 2>&1 | tail -5
```

- [ ] **Step 7: Confirm the golden fixture did NOT move**

`testdata/trial_extend_response.json` is a byte-for-byte golden, and `TestTrialExtendMatchesPinnedContract` drives it with `okResult()` — a **card-less** result. All three new fields carry `omitempty`, so the golden must still pass **unchanged**:

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 -run TestTrialExtendMatchesPinnedContract ./internal/handlers/platformadmin/ -v 2>&1 | tail -10
git diff --stat internal/handlers/platformadmin/testdata/trial_extend_response.json
```

Expected: PASS, and an EMPTY diff for that file.

**If that golden fails, do not regenerate it.** A card-less response that carries Stripe keys is the defect the `omitempty` exists to prevent; regenerating the fixture would ship it and pin it as correct. Fix the code.

- [ ] **Step 8: Prove the omission by mutation**

Remove `omitempty` from `BillingAnchorMoved` and re-run `TestExtend_CardLess_OmitsStripeFields` and `TestTrialExtendMatchesPinnedContract`. BOTH must fail. Revert.

- [ ] **Step 9: Commit**

```bash
git add internal/handlers/platformadmin/billing_trial_extend.go internal/handlers/platformadmin/billing_trial_extend_test.go
git commit -m "feat(platformadmin): surface stripe trial-end outcomes on the extend endpoint (#358)"
```

---

### Task 5: wiring, and the typed-nil guard

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:2021,2145` and the block at `:566-830`
- Test: `services/marketplace-api/internal/billing/trial/extend_stripe_integration_test.go`

**Interfaces:**
- Consumes: `trial.NewExtender`, `trial.StripeTrialUpdater` (Task 2); `billingstripe.UpdateTrialEnd`, `billingstripe.GetSubscription` (Task 1).
- Produces: `type trialStripeAdapter struct{ c *billingstripe.Client }` in `main.go`, implementing `trial.StripeTrialUpdater`.

- [ ] **Step 1: Write the failing test**

A comment telling `main.go` not to construct a typed nil is exactly the kind of prose these traps warn about — it redirects the next reader away from checking. Make `NewExtender` enforce it instead, so the guard is code and has a test.

Append to `internal/billing/trial/extend_stripe_integration_test.go`:

```go
// A typed nil in an interface is NOT nil. Assigning a nil
// *stripe.Client into StripeTrialUpdater makes `e.Stripe != nil` TRUE, and
// the first method call panics — after the row lock has been taken and
// inside a transaction. That is #288's second Critical (a typed-nil
// *gipadmin.AdminClient that panicked after the purge transaction had
// already committed) in a new location, so it gets a guard rather than a
// warning comment.
func TestNewExtender_TypedNilUpdaterIsTreatedAsAbsent(t *testing.T) {
	var typedNil *fakeUpdater // nil POINTER; a non-nil INTERFACE once assigned

	e := trial.NewExtender(typedNil)
	require.Nil(t, e.Stripe,
		"a typed-nil updater must be normalised to a true nil, or every card-backed extension panics")
}

// And the behaviour that guard buys: a card-backed trial on a typed-nil
// build refuses exactly as an unconfigured one does, rather than panicking
// mid-transaction.
func TestExtender_TypedNilUpdater_RefusesInsteadOfPanicking(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")
	subID := "sub_typed_nil"
	seeded := seedCardBacked(t, db, subID, stripeExtendAsOf.Add(10*24*time.Hour))

	var typedNil *fakeUpdater
	e := trial.NewExtender(typedNil)

	var err error
	require.NotPanics(t, func() {
		_, err = e.Extend(context.Background(), db, seeded.StoreID,
			stripeExtendAsOf.Add(60*24*time.Hour), stripeExtendAsOf)
	})
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.Nil(t, after.TrialEndsAt, "a refused extension must write nothing")
}
```

- [ ] **Step 2: Run and verify it fails**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 -run 'TypedNil' ./internal/billing/trial/ -v 2>&1 | tail -20
```

Expected: `TestNewExtender_TypedNilUpdaterIsTreatedAsAbsent` FAILs (`e.Stripe` is a non-nil interface), and `TestExtender_TypedNilUpdater_RefusesInsteadOfPanicking` FAILs with a nil-pointer panic inside `require.NotPanics`.

Confirm both RAN. A `--- SKIP` means `TEST_DATABASE_URL` is unset.

- [ ] **Step 3: Normalise the typed nil in the constructor**

In `internal/billing/trial/extend.go`, replace `NewExtender`:

```go
// NewExtender constructs an Extender.
//
// su may be nil, and a TYPED nil — a nil *stripe.Client assigned into the
// interface — is normalised to a true nil here rather than being left to
// panic at first use. Both mean the same thing to a reader ("no Stripe
// configured"), but only one of them means it to Go: an interface holding a
// nil pointer is itself non-nil, so `e.Stripe != nil` would be true and the
// first method call would panic INSIDE the transaction, after the row lock
// was taken — with the operator seeing a 500 for a request that changed
// nothing, or worse, in a variant of this shape, for one that changed
// everything (#288).
//
// Enforced here rather than documented at the call site because a call site
// can be copied wrongly and a comment cannot fail a test.
func NewExtender(su StripeTrialUpdater) *Extender {
	if su != nil {
		if v := reflect.ValueOf(su); v.Kind() == reflect.Ptr && v.IsNil() {
			su = nil
		}
	}
	return &Extender{Stripe: su}
}
```

Add `"reflect"` to the imports.

- [ ] **Step 4: Add the adapter and wire both sites**

In `cmd/marketplace-api/main.go`, near the other adapters (`stripeClientAdapter`, around `:141`):

```go
// trialStripeAdapter adapts the billing Stripe client to
// trial.StripeTrialUpdater. It exists so the trial package depends on a
// two-method interface it declares itself rather than on the whole client.
type trialStripeAdapter struct{ c *billingstripe.Client }

func (a *trialStripeAdapter) GetSubscription(ctx context.Context, id string) (*billingstripe.Subscription, error) {
	return billingstripe.GetSubscription(ctx, a.c, id)
}

func (a *trialStripeAdapter) UpdateTrialEnd(ctx context.Context, in billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	return billingstripe.UpdateTrialEnd(ctx, a.c, in)
}
```

Immediately before the `mode.Both` engine construction (so it precedes BOTH `platformadmin.Register` call sites):

```go
	// trialStripe stays a TRUE nil interface when Stripe is not configured.
	// Assigning &trialStripeAdapter{c: nil} unconditionally would make
	// Extender.Stripe != nil TRUE and panic on the first card-backed
	// extension, after the row lock is taken — the same shape as #288's
	// typed-nil gipDeleter. The nil interface is a supported configuration:
	// card-backed trials get ErrStripeManaged, exactly as before #358.
	var trialStripe trial.StripeTrialUpdater
	if billingStripeClient != nil {
		trialStripe = &trialStripeAdapter{c: billingStripeClient}
	} else {
		log.Warn("STRIPE_BILLING_SECRET_KEY not set — card-backed trials cannot be extended (409 stripe_managed)")
	}
```

At both `platformadmin.Register` sites, replace the `TrialExtender` line with:

```go
			TrialExtender:         trial.NewExtender(trialStripe),
```

`*trial.Extender` already satisfies `platformadmin.TrialExtender`, so the `TrialExtenderFunc` wrapper is not needed here.

- [ ] **Step 5: Verify both sites and the whole build**

```bash
cd services/marketplace-api
set -o pipefail
grep -n "TrialExtender:" cmd/marketplace-api/main.go
grep -n "trialStripe = " cmd/marketplace-api/main.go
go build ./... && go vet -tags=integration ./... 2>&1 | tail -5
```

Expected: exactly TWO `TrialExtender:` lines, both `trial.NewExtender(trialStripe)`; exactly ONE guarded assignment. #323 records five separate occasions where one of two wiring sites was missed — count them, do not assume.

- [ ] **Step 6: Run the full service test suite**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 ./... 2>&1 | grep -Ev '^ok|no test files' | head -40
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 ./internal/billing/... ./internal/handlers/platformadmin/... 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | head -30
```

Compare every failure against the plan's measured baseline. A new name is yours.

- [ ] **Step 7: Commit**

```bash
git add cmd/marketplace-api/main.go internal/billing/trial/extend_stripe_integration_test.go
git commit -m "feat(marketplace-api): wire the stripe trial updater with a typed-nil guard (#358)"
```

---

### Task 6: card-backed trials become visible in `GET /admin/billing/trials`

Without this, #358 delivers a path the console cannot reach: `trial.ListExpiring` filters `stripe_subscription_id IS NULL` (`internal/billing/trial/expiring.go:57`), so an operator can extend a card-backed trial only if they already know its store id.

**Files:**
- Modify: `services/marketplace-api/internal/billing/trial/expiring.go:53-59,66-72,78-110`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/billing_trials.go:25-35,86-115,118-135`
- Test: `services/marketplace-api/internal/billing/trial/expiring_integration_test.go`, `services/marketplace-api/internal/handlers/platformadmin/billing_trials_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type ListOptions struct{ IncludeStripeManaged bool }`
  - `func ListExpiring(ctx, db, asOf, window, page, limit int, opts ListOptions) ([]ExpiringRow, int64, error)` — signature gains a trailing `opts`
  - `ExpiringRow` gains `StripeManaged bool`
  - `trialRow` gains `StripeManaged bool \`json:"stripe_managed"\``
  - query parameter `include_stripe_managed` (default false)

`CountExpiring`'s signature does **not** change: it backs #282's `trials_expiring` KPI, whose meaning is "trials that will EXPIRE". A card-backed trial converts; counting it there would silently change a delivered metric.

- [ ] **Step 1: Write the failing tests**

Append to `internal/billing/trial/expiring_integration_test.go`:

```go
// The default must be unchanged: a card-backed trial is not "expiring".
// This is the contract #285 already ships.
func TestListExpiring_DefaultStillExcludesStripeManaged(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_listed"
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "the card-backed row must not appear by default")
	require.Len(t, rows, 1)
	require.False(t, rows[0].StripeManaged)
}

// With the flag, BOTH appear and each is labelled. Two rows of different
// kinds in one fixture: one kind cannot prove a filter.
func TestListExpiring_IncludeStripeManaged_ReturnsBothLabelled(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_listed_2"
	cardBacked := seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	cardLess := seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	rows, total, err := trial.ListExpiring(context.Background(), db,
		expiringAsOf, trial.DefaultExpiryWindow, 1, 10, trial.ListOptions{IncludeStripeManaged: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	byStore := map[string]trial.ExpiringRow{}
	for _, r := range rows {
		byStore[r.StoreID] = r
	}
	require.True(t, byStore[cardBacked.StoreID.String()].StripeManaged)
	require.False(t, byStore[cardLess.StoreID.String()].StripeManaged)
}

// The KPI must NOT move. CountExpiring keeps its "will expire" meaning.
func TestCountExpiring_UnaffectedByStripeManagedRows(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	subID := "sub_kpi"
	seedExpiringRow(t, db, expiringAsOf.Add(3*24*time.Hour),
		func(s *subscription.StoreSubscription) { s.StripeSubscriptionID = &subID })
	seedExpiringRow(t, db, expiringAsOf.Add(4*24*time.Hour), nil)

	n, err := trial.CountExpiring(context.Background(), db, expiringAsOf, trial.DefaultExpiryWindow)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "trials_expiring counts trials that EXPIRE; a card-backed trial converts")
}
```

And in `internal/handlers/platformadmin/billing_trials_test.go`.

**First, the two existing implementations must gain the new parameter** — there are exactly two, and the second is easy to miss:

- `stubTrialLister.ListExpiring` (`:40`) — add `opts trial.ListOptions` and record it on the struct as `gotOpts trial.ListOptions`. Do NOT add a second stub; this one already records the params it was called with, which is precisely what the new test needs.
- `sharedTrialsFixture.ListExpiring` (`:708`) — add `_ trial.ListOptions`.

Then add:

```go
// The query parameter must reach the lister, and the row must be labelled
// on the wire. Both directions matter: a handler that always passed true
// would widen a live contract, and one that never passed it would leave
// #358's endpoint undiscoverable.
func TestBillingTrials_IncludeStripeManagedReachesTheLister(t *testing.T) {
	dir := &stubBillingDirectory{names: map[string]string{}}

	t.Run("with the flag", func(t *testing.T) {
		rows := billingTrialsFixtureRows(billingTrialsFixtureAsOf)
		rows[0].StripeManaged = true
		trials := &stubTrialLister{rows: rows, total: int64(len(rows))}

		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=true", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.True(t, trials.gotOpts.IncludeStripeManaged)
		// Raw bytes: a decoded map cannot distinguish an absent key from a
		// false one, and telling those apart is this field's whole job.
		require.Contains(t, rec.Body.String(), `"stripe_managed":true`)
	})

	t.Run("without the flag the default is unchanged", func(t *testing.T) {
		trials := &stubTrialLister{rows: nil, total: 0}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.False(t, trials.gotOpts.IncludeStripeManaged,
			"the default must stay #285's shipped contract")
	})

	t.Run("anything other than true is false", func(t *testing.T) {
		trials := &stubTrialLister{rows: nil, total: 0}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=1", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.False(t, trials.gotOpts.IncludeStripeManaged,
			"a widening flag must require the exact value, never a truthy-looking one")
	})

	t.Run("a card-less row is labelled false, not omitted", func(t *testing.T) {
		rows := billingTrialsFixtureRows(billingTrialsFixtureAsOf)
		for i := range rows {
			rows[i].StripeManaged = false
		}
		trials := &stubTrialLister{rows: rows, total: int64(len(rows))}
		rec := httptest.NewRecorder()
		billingTrialsRouter(t, trials, dir).ServeHTTP(rec, httptest.NewRequest(
			http.MethodGet, "/admin/billing/trials?include_stripe_managed=true", nil))
		require.Contains(t, rec.Body.String(), `"stripe_managed":false`,
			"every row states its kind; an omitted false is indistinguishable from an older server")
	})
}
```

- [ ] **Step 2: Run and verify they fail**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -tags=integration -count=1 -p 1 -run 'TestListExpiring_|TestCountExpiring_' ./internal/billing/trial/ -v 2>&1 | tail -20
```

Expected: compile failure — `trial.ListOptions` undefined, `ListExpiring` takes 6 arguments.

- [ ] **Step 3: Add the option to the query layer**

In `internal/billing/trial/expiring.go`:

```go
// ListOptions varies what ListExpiring returns. The zero value is the
// contract #285 already ships, so an omitted option can never widen a live
// result set by accident.
type ListOptions struct {
	// IncludeStripeManaged adds trials that have a Stripe subscription.
	// They do not EXPIRE — they convert — so they are excluded by default
	// and from CountExpiring entirely. They are listable because #358 makes
	// them extendable, and an endpoint the console cannot discover a store
	// id for is unreachable in practice.
	IncludeStripeManaged bool
}
```

Split the scope helper so the shared predicate has one definition:

```go
func expiringScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	return trialingInWindowScope(db, asOf, window).Where("stripe_subscription_id IS NULL")
}

// trialingInWindowScope is expiringScope without the card filter: status is
// trialing and the effective end lies in the window.
func trialingInWindowScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	return EndsBetweenScope(
		db.Model(&subscription.StoreSubscription{}).
			Where("status = ?", subscription.StatusTrialing),
		asOf, asOf.Add(window),
	)
}

func listScope(db *gorm.DB, asOf time.Time, window time.Duration, opts ListOptions) *gorm.DB {
	if opts.IncludeStripeManaged {
		return trialingInWindowScope(db, asOf, window)
	}
	return expiringScope(db, asOf, window)
}
```

Give `ListExpiring` the trailing `opts ListOptions` parameter, replace its two `expiringScope(` calls with `listScope(…, opts)`, and compute its `total` with a count over `listScope` rather than by calling `CountExpiring` (which must keep its narrower meaning). Add `StripeManaged bool` to `ExpiringRow` and populate it in the row loop:

```go
			StripeManaged:    r.StripeSubscriptionID != nil && *r.StripeSubscriptionID != "",
```

- [ ] **Step 4: Thread it through the handler**

In `billing_trials.go`, add `opts trial.ListOptions` to the `TrialLister` interface, `TrialListerFunc` type and its method; add `StripeManaged bool \`json:"stripe_managed"\`` to `trialRow` (no `omitempty` — it is a fact about every row, and an absent `false` would be indistinguishable from an old server); parse the flag in `list`:

```go
	// Default false: #285's live contract lists trials that will EXPIRE. The
	// flag widens it to trials the console can now EXTEND (#358), which is a
	// different question and an explicit one.
	includeStripeManaged := c.Query("include_stripe_managed") == "true"
```

and pass `trial.ListOptions{IncludeStripeManaged: includeStripeManaged}` to `ListExpiring`, mapping `StripeManaged` onto each `trialRow`.

Update the wiring in `cmd/marketplace-api/main.go` at both sites — `platformadmin.TrialListerFunc(trial.ListExpiring)` still compiles once the signatures match; verify, do not assume.

- [ ] **Step 5: Update the golden fixture deliberately**

```bash
cd services/marketplace-api
set -o pipefail
go test -count=1 ./internal/handlers/platformadmin/ -run 'Trials' -v 2>&1 | tail -30
```

The golden fixture for `/admin/billing/trials` WILL fail on the added `stripe_managed` key. That is the fixture working — it is supposed to catch field additions. Update `internal/handlers/platformadmin/testdata/` by hand, and state in the commit message that the addition was intentional.

- [ ] **Step 6: Run and verify everything passes**

```bash
cd services/marketplace-api
set -o pipefail
export TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable"
go test -count=1 ./internal/handlers/platformadmin/... 2>&1 | tail -5
go test -tags=integration -count=1 -p 1 ./internal/billing/trial/ 2>&1 | tail -5
go vet -tags=integration ./... 2>&1 | tail -5
```

- [ ] **Step 7: Prove the default by mutation**

Change `includeStripeManaged` to be unconditionally `true` and re-run `TestListExpiring_DefaultStillExcludesStripeManaged` plus the handler test. They MUST fail. Revert.

- [ ] **Step 8: Commit**

```bash
git add internal/billing/trial/expiring.go internal/billing/trial/expiring_integration_test.go \
        internal/handlers/platformadmin/billing_trials.go internal/handlers/platformadmin/billing_trials_test.go \
        internal/handlers/platformadmin/testdata cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): list stripe-managed trials behind include_stripe_managed (#358)"
```

---

### Task 7: the Stripe test-mode verification runbook

Production holds 0 `store_subscriptions` rows, so the deploy can only prove the route is mounted and refuses unsigned callers — the same class of evidence #288 showed to be nearly empty. This task produces the script that makes the path real.

**Files:**
- Create: `services/marketplace-api/scripts/verify-358-stripe.sh`

**Interfaces:**
- Consumes: everything above.
- Produces: an executable script that creates a test-mode trialing subscription, extends it, and reads Stripe back.

- [ ] **Step 1: Write the script**

```bash
#!/usr/bin/env bash
# Verifies #358 against Stripe TEST MODE. Never run this with a live key.
#
# The key is read from GCP Secret Manager at run time and never written to
# disk, a log line, or the shell history file.
set -euo pipefail

KEY="$(gcloud secrets versions access latest --secret=prod-mark8ly-stripe-billing-secret-key)"
case "$KEY" in
  sk_test_*) ;;
  *) echo "REFUSING: key is not sk_test_ — this script must never touch live billing" >&2; exit 1 ;;
esac

api() { curl -sS -u "$KEY:" "https://api.stripe.com/v1/$1" "${@:2}"; }

echo "==> creating a test-mode customer + trialing subscription"
CUS=$(api customers -d "description=mark8ly-358-verify" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
PM=$(api payment_methods -d "type=card" -d "card[token]=tok_visa" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
api "payment_methods/$PM/attach" -d "customer=$CUS" >/dev/null
PRICE=$(api prices -d "unit_amount=2900" -d "currency=gbp" -d "recurring[interval]=month" \
        -d "product_data[name]=mark8ly-358-verify" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

TRIAL_END=$(( $(date +%s) + 10*24*3600 ))
SUB_JSON=$(api subscriptions -d "customer=$CUS" -d "items[0][price]=$PRICE" \
           -d "trial_end=$TRIAL_END" -d "proration_behavior=none")
SUB=$(echo "$SUB_JSON" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
echo "    subscription=$SUB trial_end=$TRIAL_END"
echo "$SUB_JSON" | python3 -c 'import json,sys;d=json.load(sys.stdin);print("    anchor=",d["billing_cycle_anchor"])'

cat <<NEXT

==> now, by hand:
  1. seed a local store_subscriptions row for a known store id with
     stripe_subscription_id = $SUB, status = 'trialing'
  2. POST /api/v1/platform/admin/billing/trials/<store_id>/extend
     with a signed request, an Idempotency-Key, reason_code=goodwill,
     and trial_ends_at = 60 days out
  3. re-run this script with:  $0 check $SUB <expected_unix>

NEXT

if [ "${1:-}" = "check" ]; then
  api "subscriptions/$2" | python3 -c '
import json,sys
d=json.load(sys.stdin)
exp=int(sys.argv[1])
print("trial_end          =",d["trial_end"],  "expected",exp, "OK" if d["trial_end"]==exp else "MISMATCH")
print("billing_cycle_anchor=",d["billing_cycle_anchor"], "OK" if d["billing_cycle_anchor"]==exp else "ANCHOR DID NOT MOVE")
print("status             =",d["status"])
print("items[0].price     =",d["items"]["data"][0]["price"]["id"])
' "$3"
fi
```

- [ ] **Step 2: Make it executable and run the creation half**

```bash
cd services/marketplace-api
chmod +x scripts/verify-358-stripe.sh
set -o pipefail
./scripts/verify-358-stripe.sh
```

Expected: a `sub_…` id printed, and an anchor equal to the `trial_end` just set.

- [ ] **Step 3: Exercise the endpoint and check Stripe**

Follow the script's printed instructions, then run the `check` form. Record, in the issue comment:

- `trial_end` matches the exact integer the endpoint sent — **this is AC4 and AC5**
- `billing_cycle_anchor` moved to the same value — **this confirms the spec's claim about Stripe's behaviour rather than repeating the SDK's comment**
- `items[0].price` is the ORIGINAL price id — the extension did not re-price
- the local row, the reminder rows and the audit row all agree

- [ ] **Step 4: Commit**

```bash
git add scripts/verify-358-stripe.sh
git commit -m "chore(scripts): stripe test-mode verification runbook for trial extension (#358)"
```

---

## After the last task

1. **Whole-branch review on the most capable model.** #288's two Criticals both survived twelve task-scoped reviews because neither lived in a single component. The composition questions here: the row lock held across a network call versus the connection pool and the idempotency `Reserve` above it; the interaction between the handler's stored idempotent response and a Stripe call that already happened; whether `stripe_managed` now means two different things to the console.
2. **Mutation, not reading.** Every finding that mattered on #288 came from a mutation failing to fail.
3. **Do not push, open a PR, merge, or deploy.** Ask first.
