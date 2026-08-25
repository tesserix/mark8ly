# Trial Extension Endpoint (#286) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a platform operator extend a merchant's trial, attributed, reasoned, refused where it would be wrong, and safe to retry.

**Architecture:** One write endpoint on the existing `platformadmin` surface. It re-reads the subscription inside a transaction, refuses four distinct states with four distinct codes, writes `trial_ends_at`, deletes that subscription's `trial_reminders` rows so the reminder cadence re-arms, and emits an operator-attributed audit row. Retries are made safe by the `idempotency_keys` table — this endpoint is its first consumer estate-wide, so the plan also adds the prune nothing currently performs.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `testify/require`.

**Spec:** `docs/superpowers/specs/2026-08-25-trial-extend-design.md`

## Global Constraints

- **Service is `marketplace-api`.** All paths relative to `services/marketplace-api/`. Do NOT touch `platform-api`.
- **No migration.** `store_subscriptions.trial_ends_at` (migration `000103`) and `idempotency_keys` (migration `000001:264`) both already exist. **`ExpectedSchemaVersion` does NOT move** — if you find yourself editing `migrations.go`, stop.
- **This is a WRITE**: HMAC signature *and* operator identity *and* capability. Mounting on the `platformadmin` group gives all three; do not add your own checks.
- **Audit via `platformadmin.EmitOperatorAction(c, em, tenantID, ev)` — never `audit.Emit`.** Nothing on this surface sets `tenant_id` on the gin context, and `resolveScope` drops a tenant-less event **silently, with no error**. The helper takes the tenant as a required parameter so it cannot be forgotten.
- **Never compute a trial end.** Call `trial.EndsAt(sub)`. A guard test (`internal/billing/trial/single_definition_test.go`) fails if a second derivation site appears.
- Envelope for errors is `{"error": "<code>", "message": "<human text>"}`; extra fields allowed (`field`, `allowed`) where #287 already uses them.
- Timestamps RFC3339 UTC with offset (`t.UTC().Format(time.RFC3339)`); ids **bare**; never send a `source` field.
- **Integration tests:** `//go:build integration`, `-p 1`, DSN `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`. **Never `localhost`.** Env var `TEST_DATABASE_URL`.
- **Seed with the existing helpers.** `seedExpiringRow` / `seedExpiringStore` in `internal/billing/trial/expiring_integration_test.go`; `seedStore` in `internal/subscription/dunning/testhelpers_integration_test.go`. A hand-written `INSERT INTO stores` omits `storefront_customer_portal_secret`, which migration `000058` declares `CHAR(64) NOT NULL` with its DEFAULT dropped, and will fail.
- **`go vet -tags=integration ./...`** in every verification step — the default toolchain never compiles build-tagged files.
- **`go test` from the service root (`./...`)**, never path-scoped.
- **Pre-existing failures, not yours (#317):** 19 tests in `internal/billing/trial/subscribe_integration_test.go` skip silently (they gate on `TEST_DB_DSN`); `internal/subscription/planchange` integration is 9 FAIL / 0 PASS; `internal/whitelabel` integration panics. All three also fail at `origin/main`. Scope your runs and do not try to fix them.
- `go vet` prints `go.work requires go >= 1.26.6 (running go 1.26.5)` — pre-existing drift, ignore.

---

### Task 1: The domain operation — extend, with its refusals

The refusal rules and the write belong together in the `trial` package, which already owns
what a trial is and when it ends. The handler stays an HTTP shell.

**Files:**
- Create: `services/marketplace-api/internal/billing/trial/extend.go`
- Create: `services/marketplace-api/internal/billing/trial/extend_integration_test.go`

**Interfaces:**
- Consumes: `subscription.StoreSubscription` (has `TrialEndsAt *time.Time`), `trial.EndsAt`, `subscription.StatusActive` / `StatusTrialing` / `StatusSignup`.
- Produces:
  ```go
  var ErrNoSubscription   = errors.New("trial: no subscription for store")
  var ErrAlreadyConverted = errors.New("trial: subscription already converted")
  var ErrStripeManaged    = errors.New("trial: trial is stripe-managed")
  var ErrNotTrialing      = errors.New("trial: subscription is not in a trial state")
  var ErrEndNotInFuture   = errors.New("trial: new trial end must be in the future")

  type ExtendResult struct {
      SubscriptionID uuid.UUID
      TenantID       uuid.UUID
      StoreID        uuid.UUID
      PreviousEndsAt time.Time
      NewEndsAt      time.Time
      RemindersCleared int64
  }

  func Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error)
  ```

- [ ] **Step 1: Write the failing tests**

Create `services/marketplace-api/internal/billing/trial/extend_integration_test.go`:

```go
//go:build integration

package trial_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// extendAsOf is the fixed "now" every test in this file passes, so
// boundaries are pinned rather than racing the wall clock.
var extendAsOf = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// Happy path. The subscription was created 80 days ago (derived end 10 days
// out); the operator moves it to 60 days out. The assertion is on the value
// stored AND on what EndsAt reports afterwards — storing a value nothing
// reads is the defect #353 existed to remove.
func TestExtend_SetsTheNewEndAndReportsThePrevious(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	derivedEnd := extendAsOf.Add(10 * 24 * time.Hour)
	seeded := seedExpiringRow(t, db, derivedEnd, nil)
	newEnd := extendAsOf.Add(60 * 24 * time.Hour)

	res, err := trial.Extend(context.Background(), db, seeded.StoreID, newEnd, extendAsOf)
	require.NoError(t, err)

	require.Equal(t, seeded.StoreID, res.StoreID)
	require.Equal(t, seeded.TenantID, res.TenantID)
	require.True(t, derivedEnd.Equal(res.PreviousEndsAt),
		"previous must be the EFFECTIVE end before the write: want %s got %s", derivedEnd, res.PreviousEndsAt)
	require.True(t, newEnd.Equal(res.NewEndsAt))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", seeded.StoreID).Error)
	require.NotNil(t, after.TrialEndsAt)
	require.True(t, newEnd.Equal(*after.TrialEndsAt))
	require.True(t, newEnd.Equal(trial.EndsAt(after)),
		"EndsAt must report the stored value — otherwise nothing downstream sees the extension")
}

// The reminder cadence must re-arm. trial_reminders' PK is
// (subscription_id, offset_key) with ON CONFLICT DO NOTHING, so a warning
// already sent can never re-send; leaving the rows would mean a merchant
// extended past their T-15 gets no notice before the date they are charged.
func TestExtend_ClearsSentReminders(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)
	require.NoError(t, db.Exec(
		`INSERT INTO trial_reminders (subscription_id, tenant_id, store_id, offset_key, sent_at)
		 VALUES (?, ?, ?, 'no_pm_t_minus_15', ?)`,
		seeded.ID, seeded.TenantID, seeded.StoreID, extendAsOf,
	).Error)

	res, err := trial.Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.RemindersCleared)

	var n int64
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", seeded.ID).Count(&n).Error)
	require.Equal(t, int64(0), n, "the sent reminder must be cleared so the cadence re-arms")
}

// A converted subscription is REFUSED, not silently ignored — #286's own
// acceptance criterion. The control in the same test proves the refusal is
// not simply always firing.
func TestExtend_RefusesConverted(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	converted := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.Status = subscription.StatusActive })
	trialing := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)

	_, err := trial.Extend(context.Background(), db,
		converted.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf)
	require.ErrorIs(t, err, trial.ErrAlreadyConverted)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", converted.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt, "a refused extension must not write anything")

	_, err = trial.Extend(context.Background(), db,
		trialing.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf)
	require.NoError(t, err, "the control MUST succeed — otherwise this test passes because everything is refused")
}

// A card-backed trial is refused: Stripe holds that billing date, and
// writing locally without telling Stripe is the split-brain #353 removed.
func TestExtend_RefusesStripeManaged(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	subID := "sub_live_abc123"
	managed := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
		func(r *subscription.StoreSubscription) { r.StripeSubscriptionID = &subID })

	_, err := trial.Extend(context.Background(), db,
		managed.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf)
	require.ErrorIs(t, err, trial.ErrStripeManaged)

	var untouched subscription.StoreSubscription
	require.NoError(t, db.First(&untouched, "store_id = ?", managed.StoreID).Error)
	require.Nil(t, untouched.TrialEndsAt)
}

// Statuses outside the trial states are refused with their own error, so
// the console can tell "converted" from "expired". `signup` and `trialing`
// are BOTH accepted — the reminder cron targets both.
func TestExtend_StatusMatrix(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	cases := []struct {
		status  subscription.SubscriptionStatus
		wantErr error
	}{
		{subscription.StatusTrialing, nil},
		{subscription.StatusSignup, nil},
		{subscription.StatusActive, trial.ErrAlreadyConverted},
		{subscription.StatusExpired, trial.ErrNotTrialing},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			st := tc.status
			seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour),
				func(r *subscription.StoreSubscription) { r.Status = st })

			_, err := trial.Extend(context.Background(), db,
				seeded.StoreID, extendAsOf.Add(60*24*time.Hour), extendAsOf)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// An end in the past is refused: it is indistinguishable from expiring the
// trial, which the cron already does. The boundary is `now` itself — the
// instant where a `>` and a `>=` implementation disagree.
func TestExtend_RefusesEndNotInFuture(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(10*24*time.Hour), nil)

	_, err := trial.Extend(context.Background(), db, seeded.StoreID, extendAsOf, extendAsOf)
	require.ErrorIs(t, err, trial.ErrEndNotInFuture, "exactly `now` is not in the future")

	_, err = trial.Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(-time.Hour), extendAsOf)
	require.ErrorIs(t, err, trial.ErrEndNotInFuture)

	_, err = trial.Extend(context.Background(), db,
		seeded.StoreID, extendAsOf.Add(time.Second), extendAsOf)
	require.NoError(t, err, "one second after `now` IS in the future")
}

// Shortening to an earlier — but still future — date is allowed. EndsAt
// honours a stored value even when earlier than the derived one, and an
// operator correcting an over-generous grant is legitimate.
func TestExtend_AllowsAnEarlierButStillFutureEnd(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	seeded := seedExpiringRow(t, db, extendAsOf.Add(40*24*time.Hour), nil)
	earlier := extendAsOf.Add(5 * 24 * time.Hour)

	res, err := trial.Extend(context.Background(), db, seeded.StoreID, earlier, extendAsOf)
	require.NoError(t, err)
	require.True(t, earlier.Equal(res.NewEndsAt))
}

// An unknown store is a distinct error, not a silent no-op.
func TestExtend_UnknownStore(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	_, err := trial.Extend(context.Background(), db,
		uuid.New(), extendAsOf.Add(60*24*time.Hour), extendAsOf)
	require.ErrorIs(t, err, trial.ErrNoSubscription)
}

// THE enforcement test, and the assertion #287 lacked. Extend a trial past
// its derived end, run the expiry cron, and assert it survives — while an
// unextended control in the same fixture IS expired, so the test cannot
// pass by the cron doing nothing.
func TestExtend_ExtendedTrialSurvivesTheExpiryCron(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores", "audit_logs")

	pastDue := extendAsOf.Add(-10 * 24 * time.Hour) // derived end already passed
	protected := seedExpiringRow(t, db, pastDue, nil)
	control := seedExpiringRow(t, db, pastDue, nil)

	_, err := trial.Extend(context.Background(), db,
		protected.StoreID, extendAsOf.Add(30*24*time.Hour), extendAsOf)
	require.NoError(t, err)

	cron := trial.NewExpiryCron(db, nil, nil, func() time.Time { return extendAsOf })
	require.NoError(t, cron.Run(context.Background()))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "store_id = ?", protected.StoreID).Error)
	require.Equal(t, subscription.StatusTrialing, after.Status,
		"an extended trial must survive the cron — this is what makes the endpoint mean anything")

	var ctl subscription.StoreSubscription
	require.NoError(t, db.First(&ctl, "store_id = ?", control.StoreID).Error)
	require.Equal(t, subscription.StatusExpired, ctl.Status,
		"the unextended control MUST expire, or this test passes because the cron did nothing")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... -run TestExtend_ -v
```

Expected: FAIL to compile — `undefined: trial.Extend`.

If you see `--- SKIP`, `TEST_DATABASE_URL` did not reach the process. Fix that first and say so in your report — a skipped test is not a passing test, and the two are one character apart.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/billing/trial/extend.go`:

```go
package trial

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Errors returned by Extend. Each maps to a distinct HTTP code at the
// handler, so the console can tell "already converted" from "expired" from
// "Stripe owns this one" rather than getting one opaque refusal.
var (
	ErrNoSubscription   = errors.New("trial: no subscription for store")
	ErrAlreadyConverted = errors.New("trial: subscription already converted")
	ErrStripeManaged    = errors.New("trial: trial is stripe-managed")
	ErrNotTrialing      = errors.New("trial: subscription is not in a trial state")
	ErrEndNotInFuture   = errors.New("trial: new trial end must be in the future")
)

// ExtendResult describes a completed extension.
type ExtendResult struct {
	SubscriptionID   uuid.UUID
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	PreviousEndsAt   time.Time
	NewEndsAt        time.Time
	RemindersCleared int64
}

// Extend moves a trial's end date, refusing the states where doing so
// would be wrong or would disagree with Stripe.
//
// Everything happens in one transaction, and the row is re-read INSIDE it,
// so the refusal checks and the write see the same state — otherwise a
// subscription that converts between the check and the write would be
// extended anyway.
//
// now is a parameter rather than time.Now() so callers and tests can pin
// the boundary exactly; production passes time.Now().UTC().
func Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (ExtendResult, error) {
	var out ExtendResult

	// Checked before opening a transaction: it needs no row, and refusing
	// early keeps a pointless BEGIN off the connection pool.
	if !newEnd.After(now) {
		return out, ErrEndNotInFuture
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub subscription.StoreSubscription
		if err := tx.Where("store_id = ?", storeID).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoSubscription
			}
			return fmt.Errorf("trial: load subscription: %w", err)
		}

		// Order matters: `active` gets its own error even though it would
		// also fail the trial-state check, because "already converted" is
		// the acceptance criterion's own words and the console shows a
		// different message for it.
		switch {
		case sub.Status == subscription.StatusActive:
			return ErrAlreadyConverted
		case sub.StripeSubscriptionID != nil && *sub.StripeSubscriptionID != "":
			return ErrStripeManaged
		case sub.Status != subscription.StatusTrialing && sub.Status != subscription.StatusSignup:
			return ErrNotTrialing
		}

		// The EFFECTIVE end before the write — the derived date when the
		// trial has never been extended. Never recompute it here; EndsAt is
		// the only definition (#353).
		out.PreviousEndsAt = EndsAt(sub)

		end := newEnd.UTC()
		if err := tx.Model(&subscription.StoreSubscription{}).
			Where("store_id = ?", storeID).
			Update("trial_ends_at", end).Error; err != nil {
			return fmt.Errorf("trial: write trial_ends_at: %w", err)
		}

		// Clear the reminder slots so the cadence re-arms against the new
		// end. trial_reminders' PK is (subscription_id, offset_key) and
		// processOne inserts ON CONFLICT DO NOTHING, so a reminder already
		// sent can NEVER re-send: without this, a merchant extended past
		// their T-15 warning gets no notice before the date they are
		// actually charged on.
		res := tx.Exec(`DELETE FROM trial_reminders WHERE subscription_id = ?`, sub.ID)
		if res.Error != nil {
			return fmt.Errorf("trial: clear reminders: %w", res.Error)
		}

		out.SubscriptionID = sub.ID
		out.TenantID = sub.TenantID
		out.StoreID = sub.StoreID
		out.NewEndsAt = end
		out.RemindersCleared = res.RowsAffected
		return nil
	})
	if err != nil {
		return ExtendResult{}, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... -run TestExtend_ -v
```

Expected: every `TestExtend_*` and every `TestExtend_StatusMatrix` subtest prints `--- PASS`. Count them against the file and report the number.

- [ ] **Step 5: Prove the refusals constrain the code**

A refusal that cannot fail is not a refusal. Two mutations, reverting after each:

1. Delete the `case sub.Status == subscription.StatusActive:` arm. Run `-run TestExtend_RefusesConverted`. Expected: FAIL. Revert.
2. Delete the `case sub.StripeSubscriptionID != nil ...` arm. Run `-run TestExtend_RefusesStripeManaged`. Expected: FAIL. Revert.

Then re-run both and confirm PASS. Report all four observations. If either mutation passed, the test proves nothing.

- [ ] **Step 6: Build and vet**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
```
Expected: all three exit 0.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/trial/extend.go \
        services/marketplace-api/internal/billing/trial/extend_integration_test.go
git commit -m "feat(trial): Extend with its refusals and reminder re-arm (#286)"
```

---

### Task 2: Idempotency, and the prune nothing performs

`idempotency_keys` has existed since migration `000001` with a model and **zero consumers
anywhere in the estate**. This endpoint is its first. Two comments — the migration's
(`000001:262`) and the package's (`internal/idempotency/models.go:3-6`) — claim a nightly
sweep cleans it up. **Both are false**: the only Go references delete by `tenant_id` during
tenant hard-delete (`subscription/harddelete/sweeper.go:133`) and purge
(`tenantpurge/purge.go:257`); nothing reads `expires_at`. Writing keys into a table nothing
prunes is a slow leak, so this task adds the prune and corrects the comments.

**Files:**
- Create: `services/marketplace-api/internal/idempotency/store.go`
- Create: `services/marketplace-api/internal/idempotency/store_integration_test.go`
- Modify: `services/marketplace-api/internal/idempotency/models.go:3-6` (the false comment)
- Modify: `services/marketplace-api/migrations/000001_products_initial.up.sql:262` (the false comment)
- Modify: `services/marketplace-api/internal/handlers/platformadmin/sweep.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:1714-1725`

**Interfaces:**
- Consumes: `idempotency.IdempotencyKey` (fields `Key string`, `TenantID string`, `Response datatypes.JSON`, `CreatedAt`, `ExpiresAt time.Time`).
- Produces:
  ```go
  // internal/idempotency
  const DefaultTTL = 24 * time.Hour
  func Lookup(ctx context.Context, db *gorm.DB, key string) (json.RawMessage, bool, error)
  func Save(ctx context.Context, db *gorm.DB, key, tenantID string, body json.RawMessage, now time.Time, ttl time.Duration) error
  func SweepExpired(ctx context.Context, db *gorm.DB, now time.Time) (int64, error)

  // internal/handlers/platformadmin
  func SweepExpiredIdempotencyKeys(ctx context.Context, db *gorm.DB) (int64, error)
  ```

- [ ] **Step 1: Write the failing tests**

Create `services/marketplace-api/internal/idempotency/store_integration_test.go`:

```go
//go:build integration

package idempotency_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/idempotency"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

var idemNow = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

// A miss is a miss — not an error, and not an empty body that a caller
// could mistake for a stored response.
func TestLookup_MissReturnsFalse(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	body, ok, err := idempotency.Lookup(context.Background(), db, "never-seen")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, body)
}

// A saved response replays byte-for-byte. The stored body is deliberately
// NOT valid-but-different JSON: byte equality is what the caller needs, so
// a re-marshal that reorders keys would be a defect.
func TestSaveThenLookup_ReplaysTheExactBytes(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "key-" + uuid.New().String()
	want := json.RawMessage(`{"store_id":"s1","trial_ends_at":"2026-12-01T00:00:00Z"}`)

	require.NoError(t, idempotency.Save(context.Background(), db,
		key, uuid.New().String(), want, idemNow, idempotency.DefaultTTL))

	got, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, string(want), string(got))
}

// Saving the same key twice must not error — a retry that races itself
// (two pods, same key) has to converge, not 500.
func TestSave_SameKeyTwiceIsNotAnError(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "key-" + uuid.New().String()
	tenant := uuid.New().String()
	first := json.RawMessage(`{"n":1}`)

	require.NoError(t, idempotency.Save(context.Background(), db, key, tenant, first, idemNow, idempotency.DefaultTTL))
	require.NoError(t, idempotency.Save(context.Background(), db, key, tenant,
		json.RawMessage(`{"n":2}`), idemNow, idempotency.DefaultTTL))

	got, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, string(first), string(got),
		"the FIRST response wins — a replay must not change what an earlier caller was told")
}

// The sweep deletes expired rows and leaves live ones. Both are seeded, on
// the exact boundary: a row expiring exactly at `now` is expired.
func TestSweepExpired_DeletesOnlyExpiredRows(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	tenant := uuid.New().String()
	live := "live-" + uuid.New().String()
	dead := "dead-" + uuid.New().String()
	boundary := "edge-" + uuid.New().String()

	require.NoError(t, idempotency.Save(context.Background(), db, live, tenant,
		json.RawMessage(`{"k":"live"}`), idemNow, time.Hour))
	require.NoError(t, idempotency.Save(context.Background(), db, dead, tenant,
		json.RawMessage(`{"k":"dead"}`), idemNow.Add(-48*time.Hour), time.Hour))
	require.NoError(t, idempotency.Save(context.Background(), db, boundary, tenant,
		json.RawMessage(`{"k":"edge"}`), idemNow, 0)) // expires_at == idemNow exactly

	n, err := idempotency.SweepExpired(context.Background(), db, idemNow)
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "the long-expired row AND the one expiring exactly now")

	_, ok, err := idempotency.Lookup(context.Background(), db, live)
	require.NoError(t, err)
	require.True(t, ok, "a live key must survive the sweep")

	_, ok, err = idempotency.Lookup(context.Background(), db, dead)
	require.NoError(t, err)
	require.False(t, ok)
}

// An expired-but-not-yet-swept key must not replay. Expiry is a property of
// the row, not of whether the cron has run recently.
func TestLookup_ExpiredKeyIsAMiss(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "stale-" + uuid.New().String()
	require.NoError(t, idempotency.Save(context.Background(), db, key, uuid.New().String(),
		json.RawMessage(`{"k":"stale"}`), idemNow.Add(-48*time.Hour), time.Hour))

	_, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.False(t, ok, "an expired row must not replay just because the sweep has not run")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/idempotency/... -v
```

Expected: FAIL to compile — `undefined: idempotency.Lookup`.

- [ ] **Step 3: Write the store**

Create `services/marketplace-api/internal/idempotency/store.go`:

```go
package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultTTL is how long a stored response stays replayable. Long enough to
// cover a client's retry budget, short enough that the table stays small.
const DefaultTTL = 24 * time.Hour

// Lookup returns a previously stored response for key.
//
// An EXPIRED row is a miss: expiry is a property of the row, not of whether
// the sweep happened to run recently, and replaying a response past its TTL
// would make the guarantee depend on cron timing.
func Lookup(ctx context.Context, db *gorm.DB, key string) (json.RawMessage, bool, error) {
	var row IdempotencyKey
	err := db.WithContext(ctx).
		Where("key = ? AND expires_at > now()", key).
		First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("idempotency: lookup: %w", err)
	}
	return json.RawMessage(row.Response), true, nil
}

// Save stores a response under key.
//
// ON CONFLICT DO NOTHING, so the FIRST writer wins: two pods handling the
// same retry converge instead of one overwriting what the other already
// told a caller. A duplicate is therefore not an error.
func Save(ctx context.Context, db *gorm.DB, key, tenantID string, body json.RawMessage, now time.Time, ttl time.Duration) error {
	row := IdempotencyKey{
		Key:       key,
		TenantID:  tenantID,
		Response:  []byte(body),
		CreatedAt: now.UTC(),
		ExpiresAt: now.UTC().Add(ttl),
	}
	err := db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).
		Create(&row).Error
	if err != nil {
		return fmt.Errorf("idempotency: save: %w", err)
	}
	return nil
}

// SweepExpired deletes rows past their expires_at, inclusive of the instant
// itself.
//
// NOTHING pruned this table before #286 — the comments in migration 000001
// and models.go claiming a nightly sweep were both wrong, and the only other
// references delete by tenant_id during tenant hard-delete and purge.
func SweepExpired(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	res := db.WithContext(ctx).
		Where("expires_at <= ?", now.UTC()).
		Delete(&IdempotencyKey{})
	if res.Error != nil {
		return 0, fmt.Errorf("idempotency: sweep expired: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

- [ ] **Step 4: Correct the two false comments**

In `services/marketplace-api/internal/idempotency/models.go`, replace the package doc's
cleanup sentence with:

```go
// Package idempotency holds the IdempotencyKey model and its store.
//
// Expired rows are pruned by SweepExpired, wired onto the platform-admin
// daily cron (platformadmin.SweepSpec) since #286 — this table's first
// consumer. An earlier version of this comment, and the one in migration
// 000001, both claimed a pre-existing nightly sweep handled it. Neither was
// true: the only other references delete by tenant_id when a tenant is
// hard-deleted or purged, and nothing read expires_at at all.
```

In `services/marketplace-api/migrations/000001_products_initial.up.sql`, replace line 262's
comment with:

```sql
-- idempotency_keys — pruned by internal/idempotency.SweepExpired, run from
-- the platform-admin daily cron since #286. (This comment previously claimed
-- cleanup via a nightly sweep job that did not exist.)
```

- [ ] **Step 5: Wire the prune onto the existing cron**

In `services/marketplace-api/internal/handlers/platformadmin/sweep.go`, add below
`SweepExpiredNonces`:

```go
// SweepExpiredIdempotencyKeys deletes idempotency_keys rows past their
// expires_at. It rides the same daily schedule as the nonce sweep because
// both tables exist only to serve this surface, and #286 is the first
// consumer this table has ever had.
func SweepExpiredIdempotencyKeys(ctx context.Context, db *gorm.DB) (int64, error) {
	return idempotency.SweepExpired(ctx, db, time.Now().UTC())
}
```

Add `"time"` and `"github.com/mark8ly/marketplace-api/internal/idempotency"` to that file's imports.

In `services/marketplace-api/cmd/marketplace-api/main.go`, inside the existing
`if cfg.PlatformAdminSecret != "" {` block (around line 1714), after the nonce-sweep
`AddFunc`, add a second one:

```go
		if _, err := trialScheduler.AddFunc(platformadmin.SweepSpec, func() {
			deleted, err := platformadmin.SweepExpiredIdempotencyKeys(workerCtx, conn)
			if err != nil {
				log.Error("platform admin idempotency sweep failed", "err", err)
				return
			}
			log.Info("platform admin idempotency sweep complete", "rows_deleted", deleted)
		}); err != nil {
			log.Error("register platform admin idempotency sweep cron", "err", err)
		}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/idempotency/... -v
```

Expected: all five tests `--- PASS`. Report the count.

- [ ] **Step 7: Prove the expiry check constrains the code**

Change `Lookup`'s predicate from `"key = ? AND expires_at > now()"` to `"key = ?"` and run
`-run TestLookup_ExpiredKeyIsAMiss`. Expected: FAIL. **Revert** and confirm PASS. Report both.

- [ ] **Step 8: Build, vet, full suite**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t2.log 2>&1; echo "exit=$?"; grep -E '^FAIL' /tmp/t2.log | head
```
Expected: `exit=0`.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/idempotency/ \
        services/marketplace-api/internal/handlers/platformadmin/sweep.go \
        services/marketplace-api/migrations/000001_products_initial.up.sql \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(idempotency): replayable response store and the prune nothing performed (#286)"
```

---

### Task 3: The handler, its reason codes and its contract

Copy `internal/handlers/platformadmin/tenant_lifecycle.go`'s shape — it is the only other
write on this surface, and matching it means the console handles both the same way.

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/trial_extend_response.json`

**Interfaces:**
- Consumes: `trial.Extend`, `trial.ExtendResult`, the five `trial.Err*` values (Task 1); `idempotency.Lookup` / `Save` / `DefaultTTL` (Task 2); `platformadmin.EmitOperatorAction`, `audit.Event`, `pagination` is NOT used here.
- Produces:
  ```go
  var ExtendReasonCodes = []string{
      "support_escalation", "onboarding_delay", "billing_dispute", "goodwill", "operator_error",
  }
  type TrialExtender interface {
      Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)
  }
  // Adapter so a free function satisfies the interface, matching the
  // existing SubscriptionsFunc / TrialListerFunc / SubscriptionListerFunc
  // pattern already in routes.go.
  type TrialExtenderFunc func(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)
  func (f TrialExtenderFunc) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)

  func NewBillingTrialExtendHandler(db *gorm.DB, ex TrialExtender, audit trialExtendAuditFunc, logger *slog.Logger) *BillingTrialExtendHandler
  func (h *BillingTrialExtendHandler) Register(g *gin.RouterGroup)  // POST /admin/billing/trials/:storeID/extend
  ```

- [ ] **Step 1: Read the model before writing anything**

Read `internal/handlers/platformadmin/tenant_lifecycle.go` in full — in particular
`lifecycleRequest` (`:138-141`), `isKnownReasonCode` and the `invalid_reason_code` response
(`:221-228`), and `lifecycleAuditFunc` / `NewOperatorActionAuditFunc` (`:50-70`). Reuse those
shapes and the same error bodies. Do NOT invent a second audit-func pattern; declare
`trialExtendAuditFunc` with the same signature so tests can capture the event synchronously
(the real `*audit.Emitter` writes on an async goroutine and cannot be observed in a unit test).

- [ ] **Step 2: Write the failing tests**

Create `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_test.go`:

```go
package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

var extendStoreID = uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")
var extendTenantID = uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111")

type stubExtender struct {
	result  trial.ExtendResult
	err     error
	calls   int
	gotEnd  time.Time
	gotStor uuid.UUID
}

func (s *stubExtender) Extend(_ context.Context, _ *gorm.DB, storeID uuid.UUID, newEnd, _ time.Time) (trial.ExtendResult, error) {
	s.calls++
	s.gotStor = storeID
	s.gotEnd = newEnd
	if s.err != nil {
		return trial.ExtendResult{}, s.err
	}
	return s.result, nil
}

type capturedAudit struct{ events []audit.Event }

func (c *capturedAudit) fn(_ *gin.Context, _ uuid.UUID, ev audit.Event) error {
	c.events = append(c.events, ev)
	return nil
}

func okResult() trial.ExtendResult {
	return trial.ExtendResult{
		SubscriptionID:   uuid.MustParse("cccccccc-1111-1111-1111-111111111111"),
		TenantID:         extendTenantID,
		StoreID:          extendStoreID,
		PreviousEndsAt:   time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC),
		NewEndsAt:        time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		RemindersCleared: 2,
	}
}

func postExtend(t *testing.T, ex platformadmin.TrialExtender, aud *capturedAudit, storeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(nil, ex, aud.fn, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/admin/billing/trials/"+storeID+"/extend", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

const goodBody = `{"reason_code":"support_escalation","reason":"migration slipped two weeks","trial_ends_at":"2026-12-01T00:00:00Z"}`

// The golden fixture pins the contract as bytes, catching a rename or an
// unauthorized addition that a struct-shaped assertion would accept.
func TestTrialExtendMatchesPinnedContract(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	want, err := os.ReadFile("testdata/trial_extend_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// Every declared reason code is accepted, and one outside the set is
// refused with #287's exact error shape. Both directions asserted: a check
// that always passes and one that always fails look identical otherwise.
func TestTrialExtendReasonCodes(t *testing.T) {
	for _, code := range platformadmin.ExtendReasonCodes {
		t.Run("accepts_"+code, func(t *testing.T) {
			body := `{"reason_code":"` + code + `","trial_ends_at":"2026-12-01T00:00:00Z"}`
			rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(), body)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}

	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(),
		`{"reason_code":"because_i_said_so","trial_ends_at":"2026-12-01T00:00:00Z"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp struct {
		Error   string   `json:"error"`
		Field   string   `json:"field"`
		Allowed []string `json:"allowed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "invalid_reason_code", resp.Error)
	require.Equal(t, "reason_code", resp.Field)
	require.Equal(t, platformadmin.ExtendReasonCodes, resp.Allowed,
		"the response must publish the allowed set, as #287 does")
}

// An absent reason_code is refused — `{}` binds successfully to the zero
// value, so this is the case the check exists to catch.
func TestTrialExtendRequiresReasonCode(t *testing.T) {
	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(),
		`{"trial_ends_at":"2026-12-01T00:00:00Z"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_reason_code")
}

// Each domain refusal maps to its own status and code, so the console can
// tell them apart. Every row asserted — a mapping is exactly the kind of
// table where one wrong entry hides behind the others.
func TestTrialExtendRefusalMapping(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{trial.ErrAlreadyConverted, http.StatusConflict, "already_converted"},
		{trial.ErrStripeManaged, http.StatusConflict, "stripe_managed"},
		{trial.ErrNotTrialing, http.StatusConflict, "not_trialing"},
		{trial.ErrEndNotInFuture, http.StatusBadRequest, "invalid_request"},
		{trial.ErrNoSubscription, http.StatusNotFound, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.wantCode, func(t *testing.T) {
			aud := &capturedAudit{}
			rec := postExtend(t, &stubExtender{err: tc.err}, aud, extendStoreID.String(), goodBody)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())

			var resp struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, tc.wantCode, resp.Error)
			require.Empty(t, aud.events, "a refused extension must not write an audit row")
		})
	}
}

// A malformed store id is a 400, not a 500 — #343 records the opposite
// happening on another internal route.
func TestTrialExtendMalformedStoreIDIs400(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, "not-a-uuid", goodBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, ex.calls, "the domain call must not be reached with an unparsed id")
}

// The audit row carries the action, the reason code, the free text and both
// dates. Asserting the VALUES, not merely that an event was emitted: a
// payload built by map lookup returns the zero value for a missing key.
func TestTrialExtendEmitsAnAttributedAuditRow(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, aud.events, 1)
	ev := aud.events[0]
	require.Equal(t, "trial.extended", ev.Action)

	raw, err := json.Marshal(ev.Metadata)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "support_escalation")
	require.Contains(t, body, "migration slipped two weeks")
	require.Contains(t, body, "2026-12-01T00:00:00Z")
	require.Contains(t, body, "2026-09-14T10:22:31Z")
}

// A body that is not JSON at all is refused before the reason-code check,
// matching #287's binder behaviour.
func TestTrialExtendUnparseableBody(t *testing.T) {
	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(), `not json`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

// An unparseable trial_ends_at is a 400 and never reaches the domain call.
func TestTrialExtendUnparseableDate(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, extendStoreID.String(),
		`{"reason_code":"goodwill","trial_ends_at":"next tuesday"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, ex.calls)
}
```

Create `services/marketplace-api/internal/handlers/platformadmin/testdata/trial_extend_response.json` (one line):

```json
{"store_id":"bbbbbbbb-1111-1111-1111-111111111111","tenant_id":"aaaaaaaa-1111-1111-1111-111111111111","trial_ends_at":"2026-12-01T00:00:00Z","previous_trial_ends_at":"2026-09-14T10:22:31Z","reason_code":"support_escalation","reason":"migration slipped two weeks","reminders_cleared":2}
```

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run TestTrialExtend -v
```

Expected: FAIL to compile — `undefined: platformadmin.NewBillingTrialExtendHandler`.

- [ ] **Step 4: Write the handler**

Create `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go`:

```go
package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
)

// ExtendReasonCodes is the closed set of reasons a trial may be extended
// for. As with SuspendReasonCodes, an audit row saying WHAT happened
// without WHY is the gap this series exists to close, so the code is
// REQUIRED; free text (`reason`) is accepted IN ADDITION, never instead.
//
// Deliberately a different set from SuspendReasonCodes: the reasons for
// granting more trial time are not the reasons for suspending a tenant.
var ExtendReasonCodes = []string{
	"support_escalation", // an open support case needs more time to resolve
	"onboarding_delay",   // setup or migration slipped, outside the merchant's control
	"billing_dispute",    // a billing disagreement is open; the trial should not lapse meanwhile
	"goodwill",           // discretionary grant, no other category applies
	"operator_error",     // correcting a mistaken earlier extension or trial start
}

// maxReasonLen caps the free-text reason. Long enough for a sentence of
// context, short enough that an audit row stays readable.
const maxReasonLen = 500

// TrialExtender is the subset of the trial package this handler needs,
// declared locally so the handler is stubbable — the same reason
// TenantLifecycle and EstateCounts are declared here rather than imported.
type TrialExtender interface {
	Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)
}

// TrialExtenderFunc adapts a free function to TrialExtender, matching the
// SubscriptionsFunc / TrialListerFunc pattern already used in routes.go.
type TrialExtenderFunc func(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error)

func (f TrialExtenderFunc) Extend(ctx context.Context, db *gorm.DB, storeID uuid.UUID, newEnd, now time.Time) (trial.ExtendResult, error) {
	return f(ctx, db, storeID, newEnd, now)
}

// trialExtendAuditFunc records a platform-operator action. Production
// closes over a real *audit.Emitter via EmitOperatorAction; test doubles
// capture the audit.Event synchronously, which the real Emitter cannot do
// because its write happens on an async worker goroutine.
type trialExtendAuditFunc func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error

// BillingTrialExtendHandler serves POST /admin/billing/trials/{store_id}/extend.
//
// The path parameter is a STORE id, not a subscription id: #285 emits no
// row id, so the console has none to send, and store_subscriptions declares
// UNIQUE (store_id) which makes the store id unambiguous.
type BillingTrialExtendHandler struct {
	db     *gorm.DB
	ex     TrialExtender
	audit  trialExtendAuditFunc
	logger *slog.Logger
}

// NewBillingTrialExtendHandler constructs the handler. logger may be nil.
func NewBillingTrialExtendHandler(db *gorm.DB, ex TrialExtender, aud trialExtendAuditFunc, logger *slog.Logger) *BillingTrialExtendHandler {
	return &BillingTrialExtendHandler{db: db, ex: ex, audit: aud, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *BillingTrialExtendHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/billing/trials/:storeID/extend", h.extend)
}

type trialExtendRequest struct {
	ReasonCode  string `json:"reason_code"`
	Reason      string `json:"reason"`
	TrialEndsAt string `json:"trial_ends_at"`
}

type trialExtendResponse struct {
	StoreID             string `json:"store_id"`
	TenantID            string `json:"tenant_id"`
	TrialEndsAt         string `json:"trial_ends_at"`
	PreviousTrialEndsAt string `json:"previous_trial_ends_at"`
	ReasonCode          string `json:"reason_code"`
	Reason              string `json:"reason,omitempty"`
	RemindersCleared    int64  `json:"reminders_cleared"`
}

func (h *BillingTrialExtendHandler) extend(c *gin.Context) {
	storeID, err := uuid.Parse(strings.TrimSpace(c.Param("storeID")))
	if err != nil {
		// 400, not 500: a malformed id is the caller's error. #343 records
		// the opposite happening on another internal route.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "store id is not a valid uuid",
		})
		return
	}

	var req trialExtendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin returns io.EOF for a completely empty body, so an omitted
		// body is rejected HERE. `{}` binds to the zero value and is the
		// case the reason-code check below exists to catch.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	if !isKnownReasonCode(req.ReasonCode, ExtendReasonCodes) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_reason_code",
			"message": "reason_code is required and must be one of the declared codes",
			"field":   "reason_code",
			"allowed": ExtendReasonCodes,
		})
		return
	}

	newEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(req.TrialEndsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "trial_ends_at must be an RFC3339 timestamp",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) > maxReasonLen {
		reason = reason[:maxReasonLen]
	}

	res, err := h.ex.Extend(c.Request.Context(), h.db, storeID, newEnd, time.Now().UTC())
	if err != nil {
		h.respondExtendErr(c, err)
		return
	}

	prev := res.PreviousEndsAt.UTC().Format(time.RFC3339)
	next := res.NewEndsAt.UTC().Format(time.RFC3339)

	// EmitOperatorAction, never audit.Emit: nothing on this surface sets
	// tenant_id on the gin context, and resolveScope drops a tenant-less
	// event silently with no error. The tenant is a required parameter here
	// so it cannot be forgotten (trap 3, #310).
	if h.audit != nil {
		ev := audit.Event{
			Action:       "trial.extended",
			ResourceType: "subscription",
			ResourceID:   res.SubscriptionID.String(),
			// StoreID is deliberately LEFT NIL even though we have one.
			// audit.Event's own comment groups trial extend with the
			// tenant-scoped platform writes, and a store-scoped audit row
			// would surface this operator action inside the MERCHANT's own
			// store-scoped audit view — a product decision about what a
			// merchant sees, not a detail to settle by default here. The
			// store id is still recorded, in metadata below.
			//
			// TenantID is NOT set here either: EmitOperatorAction assigns
			// it from its own tenantID parameter (audit.go:44). Setting it
			// in this literal would be overwritten with the same value and
			// would imply the caller owns a field the helper owns.
			Metadata: map[string]any{
				"reason_code":            req.ReasonCode,
				"reason":                 reason,
				"previous_trial_ends_at": prev,
				"trial_ends_at":          next,
				"store_id":               res.StoreID.String(),
				"reminders_cleared":      res.RemindersCleared,
			},
		}
		if err := h.audit(c, res.TenantID, ev); err != nil && h.logger != nil {
			// Logged, not surfaced: the extension already happened, and
			// failing the response would make the caller retry a write that
			// succeeded.
			h.logger.Error("trial extend: audit emit failed",
				"store_id", res.StoreID.String(), "err", err)
		}
	}

	c.JSON(http.StatusOK, trialExtendResponse{
		StoreID:             res.StoreID.String(),
		TenantID:            res.TenantID.String(),
		TrialEndsAt:         next,
		PreviousTrialEndsAt: prev,
		ReasonCode:          req.ReasonCode,
		Reason:              reason,
		RemindersCleared:    res.RemindersCleared,
	})
}

// respondExtendErr maps the domain's sentinel errors to distinct statuses
// and codes, so the console can tell "already converted" from "expired"
// from "Stripe owns this one" rather than getting one opaque refusal.
func (h *BillingTrialExtendHandler) respondExtendErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trial.ErrAlreadyConverted):
		c.JSON(http.StatusConflict, gin.H{
			"error": "already_converted", "message": "the subscription has already converted to a paid plan",
		})
	case errors.Is(err, trial.ErrStripeManaged):
		c.JSON(http.StatusConflict, gin.H{
			"error":   "stripe_managed",
			"message": "this trial has a Stripe subscription; Stripe owns its billing date and it cannot be extended here",
		})
	case errors.Is(err, trial.ErrNotTrialing):
		c.JSON(http.StatusConflict, gin.H{
			"error": "not_trialing", "message": "the subscription is not in a trial state",
		})
	case errors.Is(err, trial.ErrEndNotInFuture):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "trial_ends_at must be in the future",
		})
	case errors.Is(err, trial.ErrNoSubscription):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "no subscription for that store",
		})
	default:
		if h.logger != nil {
			h.logger.Error("trial extend failed", "err", err)
		}
		// The driver's error text is logged server-side, never echoed.
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not extend trial",
		})
	}
}
```

**`isKnownReasonCode` already exists** in this package, used by
`tenant_lifecycle.go`. Do not redeclare it — read it first and confirm its
signature is `(code string, allowed []string) bool`; if it differs, match
the real one.

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run TestTrialExtend -v
```
Expected: every `TestTrialExtend*` and every subtest `--- PASS`. Report the count.

- [ ] **Step 6: Prove the golden fixture catches a rename AND an addition**

A fixture that only catches omissions is theatre. Reverting after each:

1. **Rename:** change the response struct's `reminders_cleared` JSON tag to `cleared`. Run `-run TestTrialExtendMatchesPinnedContract`. Expected: FAIL. Revert.
2. **Addition:** add `Extra string \`json:"extra"\`` to the response struct and set it. Run the same test. Expected: FAIL. Revert.

Re-run and confirm PASS. Report all four observations.

- [ ] **Step 7: Build, vet, full suite**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t3.log 2>&1; echo "exit=$?"; grep -E '^FAIL' /tmp/t3.log | head
```
Expected: `exit=0`.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go \
        services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_test.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/trial_extend_response.json
git commit -m "feat(platformadmin): POST /admin/billing/trials/{store_id}/extend (#286)"
```

---

### Task 4: Idempotency on the endpoint, then mount and wire it

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_test.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/routes_trial_extend_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (BOTH `platformadmin.Deps` literals)

**Interfaces:**
- Consumes: `idempotency.Lookup`, `idempotency.Save`, `idempotency.DefaultTTL` (Task 2); the handler from Task 3.
- Produces: `Deps.TrialExtender TrialExtender`; the route `POST /api/v1/platform/admin/billing/trials/:storeID/extend`.

- [ ] **Step 1: Write the failing tests**

Append to `billing_trial_extend_test.go`:

```go
// The Idempotency-Key header is REQUIRED. A write that cannot be retried
// safely is worse than one that refuses to start.
func TestTrialExtendRequiresIdempotencyKey(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "idempotency_key_required")
	require.Equal(t, 0, ex.calls)
}
```

**Then update `postExtend` to set `Idempotency-Key` by default**, and add a variant that omits
it for the test above — otherwise every existing test in the file starts failing for the wrong
reason. Read the file and make the smallest change that keeps each existing test asserting
what it already asserts.

Create `services/marketplace-api/internal/handlers/platformadmin/routes_trial_extend_test.go`:

```go
package platformadmin_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// Mounted when the dependency is supplied. Asserted as "not 404" with the
// secret set, matching TestRegisterTicketsMountsWhenDepPresent.
func TestRegisterMountsTrialExtendWhenSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		TrialExtender: &stubExtender{},
		Secret:        "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend",
		bytes.NewBufferString(`{}`)))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when TrialExtender is set")

	// A bogus sibling under the same prefix must 404 — without this, the
	// assertion above is also satisfied by a router that answers everything.
	bogus := httptest.NewRecorder()
	r.ServeHTTP(bogus, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend-nope",
		bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusNotFound, bogus.Code)
}

// Nil leaves it unmounted, matching every other optional route here.
func TestRegisterLeavesTrialExtendUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{Repo: &stubRepo{}})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/billing/trials/bbbbbbbb-1111-1111-1111-111111111111/extend",
		bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

`stubRepo` is the existing minimal `audit.Repository` stub at `audit_logs_test.go:23`, already
used by `routes_tickets_test.go`. Reuse it — `Register` returns immediately when `Repo` is nil.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -run 'TestTrialExtendRequiresIdempotencyKey|TestRegister.*TrialExtend' -v
```
Expected: FAIL — missing header handling, and `unknown field TrialExtender in struct literal`.

- [ ] **Step 3: Add idempotency to the handler**

In `billing_trial_extend.go`:

- Read `Idempotency-Key`. Empty → `400` `{"error":"idempotency_key_required","message":"the Idempotency-Key header is required for this endpoint"}`, before any domain call.
- Before doing the work, `idempotency.Lookup(ctx, h.db, key)`. On a hit, write the stored bytes back with `200` and **return without calling Extend and without emitting an audit row.**
- After a successful extension and its audit emit, marshal the response body and `idempotency.Save(ctx, h.db, key, result.TenantID.String(), body, time.Now().UTC(), idempotency.DefaultTTL)`. A save failure is logged, not surfaced — the extension already happened, and failing the response would make the caller retry a write that succeeded.
- Guard both calls on `h.db != nil` so the unit tests, which pass a nil db, still exercise the rest of the handler.

- [ ] **Step 4: Mount and wire**

In `routes.go`, add to `Deps` after `Notifications`:

```go
	// TrialExtender serves POST /admin/billing/trials/:storeID/extend (#286),
	// this surface's second WRITE. Like TenantLifecycle it needs DB and
	// Emitter as well: a write endpoint that cannot be attributed to an
	// operator should not exist rather than run silently unaudited.
	TrialExtender TrialExtender
```

And in `Register`, after the `Tickets`/`Notifications` blocks:

```go
	switch {
	case deps.TrialExtender != nil && deps.DB != nil && deps.Emitter != nil:
		NewBillingTrialExtendHandler(
			deps.DB, deps.TrialExtender,
			NewOperatorActionAuditFunc(deps.Emitter),
			deps.Logger,
		).Register(group)
	case deps.TrialExtender != nil:
		if deps.Logger != nil {
			deps.Logger.Warn("platformadmin: trial extend route not mounted — DB and Emitter are both required",
				"db_nil", deps.DB == nil, "emitter_nil", deps.Emitter == nil)
		}
	}
```

**If `NewOperatorActionAuditFunc` returns a `lifecycleAuditFunc` whose type does not match
`trialExtendAuditFunc`, declare `trialExtendAuditFunc` as the same named type rather than
adding a second adapter.** Read `tenant_lifecycle.go:50-70` and pick whichever keeps one
adapter in the package.

In `cmd/marketplace-api/main.go`, add

```go
			TrialExtender:         platformadmin.TrialExtenderFunc(trial.Extend),
```

to **BOTH** `platformadmin.Deps` literals (around lines 1981 and 2094), keeping gofmt's
alignment. `trial` is already imported in that file.

**Verify both sites were edited.** #323 records five routes that silently unmounted because
only one was touched:

```bash
cd services/marketplace-api && grep -c 'TrialExtender:' cmd/marketplace-api/main.go
```
Expected output: `2`. If it prints `1`, the second site was missed.

- [ ] **Step 5: Run the tests**

```bash
cd services/marketplace-api && go test -count=1 ./internal/handlers/platformadmin/... -v 2>&1 | grep -E '^(--- |ok|FAIL)' | tail -40
```
Expected: every test in the package passes, including the pre-existing ones.

- [ ] **Step 6: Add and run the idempotency integration test**

Create `services/marketplace-api/internal/handlers/platformadmin/billing_trial_extend_integration_test.go`:

```go
//go:build integration

package platformadmin_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Idempotency is the acceptance criterion this proves: the SAME key
// replays the stored body and writes NO second audit row, while a
// DIFFERENT key is a new extension. Asserting the audit-row count is what
// distinguishes real idempotency from a coincidentally identical response.
func TestTrialExtendIsIdempotentPerKey(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	ex := &stubExtender{result: okResult()}
	aud := &capturedAudit{}
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(db, ex, aud.fn, nil).Register(r.Group(""))

	do := func(key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost,
			"/admin/billing/trials/"+extendStoreID.String()+"/extend",
			bytes.NewBufferString(goodBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		r.ServeHTTP(rec, req)
		return rec
	}

	first := do("key-alpha")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := do("key-alpha")
	require.Equal(t, http.StatusOK, second.Code)
	require.JSONEq(t, first.Body.String(), second.Body.String(), "same key must replay the same body")
	require.Equal(t, 1, ex.calls, "same key must NOT perform a second extension")
	require.Len(t, aud.events, 1, "same key must NOT write a second audit row")

	third := do("key-beta")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, 2, ex.calls, "a DIFFERENT key is a new extension")
	require.Len(t, aud.events, 2)
}
```

Run:
```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/handlers/platformadmin/... -run TestTrialExtendIsIdempotent -v
```
Expected: PASS.

- [ ] **Step 7: Full verification**

```bash
cd services/marketplace-api
gofmt -l ./cmd ./internal
go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t4.log 2>&1; echo "unit_exit=$?"; grep -E '^FAIL' /tmp/t4.log | head
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... ./internal/idempotency/... ./internal/handlers/platformadmin/... -v > /tmp/i4.log 2>&1
echo "int_exit=$?"; echo "PASS=$(grep -c -- '--- PASS' /tmp/i4.log) SKIP=$(grep -c -- '--- SKIP' /tmp/i4.log) FAIL=$(grep -c -- '--- FAIL' /tmp/i4.log)"
```

Expected: `gofmt` silent; build/vet exit 0; `unit_exit=0`. The 19 `internal/billing/trial` SKIPs are pre-existing (#317) — report them separately from your own.

**Run `go test ./...` from the SERVICE ROOT, not `./internal/...`** — the root package holds the schema-version guard, and a path-scoped run silently excludes it.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/ \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): idempotent trial extend, mounted on both wiring sites (#286)"
```

---

## Final whole-branch review

Mutate rather than read — a review that only reads the diff has repeatedly missed things a two-minute mutation caught.

- [ ] Both `platformadmin.Deps` literals carry `TrialExtender` (`grep -c` returns `2`).
- [ ] `migrations.go` is **unchanged** — this branch ships no migration.
- [ ] `go test -count=1 ./...` from the service root exits 0, confirmed by exit code, and the root-package schema guard ran.
- [ ] `go vet -tags=integration ./...` exits 0.
- [ ] Every refusal test seeds the exact state that triggers it AND a control that does not.
- [ ] `TestExtend_ExtendedTrialSurvivesTheExpiryCron` asserts its unextended control DID expire — without that it passes if the cron does nothing.
- [ ] The idempotency test asserts the **audit-row count**, not only the response body.
- [ ] The golden fixture was proved by mutation against a rename and an addition.
- [ ] No comment added in this branch asserts a property that was not checked — including the corrected `idempotency_keys` comments, which must now describe what the code actually does.

## What production can and cannot prove

**`store_subscriptions` is empty in production** — 0 rows against 4 stores, verified read-only in-cluster on 2026-08-25. Re-check before reporting rather than repeating this.

**Provable:** the route is mounted, an unsigned request gets `401`, a write without operator or capability gets `401`, and a bogus sibling path under the same prefix gets `404` — the last is what makes the first mean "this route exists".

**Not provable:** every refusal, the reminder re-arm, the cascade and the idempotent replay. None can be exercised without a scratch tenant that has entered the billing flow. Say which is which; a `401` from a route whose body has never run is not evidence the body works.

No migration ships, so `ExpectedSchemaVersion` does not move and there is no crashloop risk on rollout. Deploys arrive as image tags (`main-<sha7>`), never git commits.
