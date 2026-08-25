# Storable Trial End (#353) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store a trial's end date so it can be extended, and make all seven sites that currently recompute it read one accessor instead.

**Architecture:** A nullable `store_subscriptions.trial_ends_at` column, where NULL means "never extended" and the effective end is `COALESCE(trial_ends_at, created_at + 90 days)`. One Go accessor, `trial.EndsAt`, plus three SQL scope helpers whose two-branch predicates keep an index on both the common and the extended path. The duplication across those branches is made safe by an agreement test that cross-checks every scope against the Go accessor.

**Tech Stack:** Go 1.26, GORM, PostgreSQL, `golang-migrate`, `testify/require`.

**Spec:** `docs/superpowers/specs/2026-08-25-trial-end-storable-design.md`

## Global Constraints

- **Service is `marketplace-api`.** All paths relative to `services/marketplace-api/`. Do NOT touch `platform-api` — separate migration sequence, separate `notification` package, trap 4.
- **Effective trial end is `COALESCE(trial_ends_at, created_at + 90 days)`.** NULL means never extended.
- **`trial.TrialDays` is `90`** and lives at `internal/billing/trial/subscribe.go:21`. Never retype the number; reference the constant.
- **No backfill.** The migration adds a nullable column and nothing else touches existing rows.
- **`IF NOT EXISTS` on every `CREATE INDEX`.** A bare `CREATE INDEX` that collides with a hand-created one dirties the schema version, and `AssertVersion`'s exact-equality check then crashloops every pod.
- **`ExpectedSchemaVersion` must move with the migration**, in the root-package `migrations.go`. Its guard test lives in the root package, which every path-scoped `go test ./internal/...` excludes.
- **Integration tests:** `//go:build integration`, `-p 1`, DSN `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`. **Never `localhost`** — a different native Postgres squats on 127.0.0.1. Env var is `TEST_DATABASE_URL`.
- **`go vet -tags=integration ./...` in every verification step.** The default toolchain never compiles build-tagged files.
- **`go test` from the service root (`./...`)**, never path-scoped, or the schema-version guard silently does not run.
- **Pre-existing unrelated failures** live in `internal/subscription` (`store_subscriptions_store_id_fkey`, issue #317). Scope runs with `-run`; do not try to fix them.
- **`psql` is not installed on this machine; `docker` is.** Use `docker run --rm postgres:15 psql '<DSN>' -c '...'`.
- `go vet` prints `go.work requires go >= 1.26.6 (running go 1.26.5)`. Pre-existing drift, not a failure.

---

### Task 1: Migration `000103`, the model field, and the version bump

**Files:**
- Create: `services/marketplace-api/migrations/000103_store_subscriptions_trial_ends_at.up.sql`
- Create: `services/marketplace-api/migrations/000103_store_subscriptions_trial_ends_at.down.sql`
- Modify: `services/marketplace-api/migrations.go:17`
- Modify: `services/marketplace-api/internal/subscription/models.go` (add a field near `CreatedAt`, currently line 169)
- Test: `services/marketplace-api/migrations_test.go` (already exists — it is the failing test)

**Interfaces:**
- Consumes: nothing.
- Produces: column `store_subscriptions.trial_ends_at TIMESTAMPTZ NULL`; index `ss_trial_ends_at_idx`; struct field `subscription.StoreSubscription.TrialEndsAt *time.Time`; `marketplaceapi.ExpectedSchemaVersion == 103`.

- [ ] **Step 1: Write the migration files**

No new test is written — `TestExpectedSchemaVersionMatchesHighestMigration` in `services/marketplace-api/migrations_test.go` already asserts the constant tracks the highest migration, and adding these files is what makes it fail.

`migrations/000103_store_subscriptions_trial_ends_at.up.sql`:
```sql
-- Trial end becomes storable so a platform operator can extend it (#353,
-- unblocking #286). Until now it was recomputed as created_at + 90 days at
-- seven independent sites, so there was nothing to extend.
--
-- NULL means "never extended": the effective end is
-- COALESCE(trial_ends_at, created_at + interval '90 days'). Deliberately no
-- backfill — existing rows keep deriving, and the migration cannot corrupt
-- billing data.
ALTER TABLE store_subscriptions ADD COLUMN IF NOT EXISTS trial_ends_at TIMESTAMPTZ;

-- Partial, because extensions are rare: this index stays small while giving
-- the extended branch of the trial-window queries something to use. The
-- unextended branch keeps using ss_status_created_at_idx from migration 087.
CREATE INDEX IF NOT EXISTS ss_trial_ends_at_idx
    ON store_subscriptions (trial_ends_at)
    WHERE trial_ends_at IS NOT NULL;
```

`migrations/000103_store_subscriptions_trial_ends_at.down.sql`:
```sql
-- DESTRUCTIVE: dropping this column discards every operator-granted trial
-- extension. There is no derivation that can recover them — that is the whole
-- point of the column. Rolling back past 103 means those trials silently
-- revert to created_at + 90 days.
DROP INDEX IF EXISTS ss_trial_ends_at_idx;
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS trial_ends_at;
```

- [ ] **Step 2: Run the guard test to verify it fails**

Run: `cd services/marketplace-api && go test -count=1 -run TestExpectedSchemaVersionMatchesHighestMigration ./ -v`

Expected: FAIL — `ExpectedSchemaVersion = 102, but highest migration on disk is 103`.

Confirm the line reads `--- FAIL`. A `--- SKIP` is not a failure and the two are one character apart.

- [ ] **Step 3: Bump the constant and add the model field**

In `services/marketplace-api/migrations.go` line 17:
```go
const ExpectedSchemaVersion uint = 103
```

In `services/marketplace-api/internal/subscription/models.go`, immediately **above** the existing `CreatedAt` field (currently line 169), add:
```go
	// TrialEndsAt is the operator-extended trial end (migration 103). NULL —
	// the common case — means the trial has never been extended and its end
	// is created_at + trial.TrialDays. Never read this field directly to
	// answer "when does this trial end": call trial.EndsAt, which is the only
	// definition of that.
	TrialEndsAt *time.Time `gorm:"column:trial_ends_at"`
```

- [ ] **Step 4: Run the guard test to verify it passes**

Run: `cd services/marketplace-api && go test -count=1 -run TestExpectedSchemaVersionMatchesHighestMigration ./ -v`

Expected: PASS.

- [ ] **Step 5: Apply the migration and confirm the column and index exist**

`make dev` is broken in this repo (migrate containers fail with `exec: "up": executable file not found`). Apply directly:

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go run ./cmd/migrate up

docker run --rm postgres:15 psql \
  'postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  -c "select column_name, is_nullable from information_schema.columns where table_name='store_subscriptions' and column_name='trial_ends_at'" \
  -c "select indexname from pg_indexes where tablename='store_subscriptions' and indexname='ss_trial_ends_at_idx'"
```

Expected: the first query returns one row, `trial_ends_at | YES`. The second returns `ss_trial_ends_at_idx`. If either is empty the migration did not apply — stop, do not proceed.

- [ ] **Step 6: Build and vet**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
```
Expected: all three exit 0.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/migrations/000103_store_subscriptions_trial_ends_at.up.sql \
        services/marketplace-api/migrations/000103_store_subscriptions_trial_ends_at.down.sql \
        services/marketplace-api/migrations.go \
        services/marketplace-api/internal/subscription/models.go
git commit -m "feat(subscription): storable trial_ends_at column and schema version 103 (#353)"
```

---

### Task 2: The accessor, three scope helpers, and the agreement test

This is the task the rest of the plan depends on. `EndsAt` is the single Go definition of trial end; the three scopes are its SQL counterparts.

**The two-branch predicate is duplicated across three helpers on purpose.** Building it by interpolating comparison operators into a shared SQL string would read like injection and be harder to review than the duplication. What makes the duplication safe is the **agreement test** in Step 5: for a seeded matrix of extended and unextended rows around each boundary, every scope must return exactly the rows `EndsAt` says it should. If any branch drifts, that test fails.

**Files:**
- Create: `services/marketplace-api/internal/billing/trial/endsat.go`
- Create: `services/marketplace-api/internal/billing/trial/endsat_test.go`
- Create: `services/marketplace-api/internal/billing/trial/endsat_integration_test.go`

**Interfaces:**
- Consumes: `subscription.StoreSubscription` (with `TrialEndsAt *time.Time` from Task 1); `trial.TrialDays`.
- Produces:
  ```go
  func EndsAt(sub subscription.StoreSubscription) time.Time
  func EndedBeforeScope(db *gorm.DB, at time.Time) *gorm.DB      // effective end <  at
  func EndsBetweenScope(db *gorm.DB, lo, hi time.Time) *gorm.DB  // effective end in (lo, hi]
  func EndsWithinDayScope(db *gorm.DB, dayStart time.Time) *gorm.DB // [dayStart, dayStart+24h)
  ```

- [ ] **Step 1: Write the failing unit test**

Create `services/marketplace-api/internal/billing/trial/endsat_test.go`:

```go
package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// An unextended trial ends created_at + TrialDays. The fixture uses a
// created_at that is NOT a round number of days from any boundary, so an
// implementation that truncated to a day would give a different answer.
func TestEndsAt_UnextendedDerivesFromCreatedAt(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created}

	got := trial.EndsAt(sub)

	require.Equal(t, created.Add(trial.TrialDays*24*time.Hour).UTC(), got)
}

// An extended trial ends at the stored value. The stored value is
// deliberately NOT created_at + 90d — an implementation that ignored the
// column would return the derived date, and this asserts it does not.
func TestEndsAt_ExtendedUsesStoredValue(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	extended := time.Date(2026, 9, 30, 8, 15, 0, 0, time.UTC)
	require.NotEqual(t, created.Add(trial.TrialDays*24*time.Hour).UTC(), extended.UTC(),
		"fixture must distinguish the stored value from the derived one")

	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created, TrialEndsAt: &extended}

	require.Equal(t, extended.UTC(), trial.EndsAt(sub))
}

// A trial can be extended BACKWARDS (shortened). Nothing in the accessor
// should assume the stored value is later than the derived one.
func TestEndsAt_StoredValueEarlierThanDerivedIsHonoured(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	earlier := created.Add(10 * 24 * time.Hour)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created, TrialEndsAt: &earlier}

	require.Equal(t, earlier.UTC(), trial.EndsAt(sub))
}

// The return is always UTC, whatever the driver hands back.
func TestEndsAt_AlwaysUTC(t *testing.T) {
	loc := time.FixedZone("IST", 5*3600+1800)
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, loc)
	sub := subscription.StoreSubscription{ID: uuid.New(), CreatedAt: created}

	require.Equal(t, time.UTC, trial.EndsAt(sub).Location())

	stored := time.Date(2026, 9, 30, 8, 15, 0, 0, loc)
	sub.TrialEndsAt = &stored
	require.Equal(t, time.UTC, trial.EndsAt(sub).Location())
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test -count=1 ./internal/billing/trial/... -run TestEndsAt -v`

Expected: FAIL to compile — `undefined: trial.EndsAt`.

- [ ] **Step 3: Write the implementation**

Create `services/marketplace-api/internal/billing/trial/endsat.go`:

```go
package trial

import (
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// EndsAt returns when this subscription's trial ends.
//
// THIS IS THE ONLY DEFINITION OF TRIAL END. Before #353 the same arithmetic
// was repeated at seven sites, so an operator had nothing to extend: the
// expiry cron, Stripe, the merchant's own screen and the platform console
// each recomputed it from created_at and would have ignored any stored value.
//
// A nil TrialEndsAt — the common case — means the trial has never been
// extended. A non-nil one is authoritative even when it is EARLIER than the
// derived date: shortening is as legitimate as extending, and nothing here
// second-guesses the stored value.
func EndsAt(sub subscription.StoreSubscription) time.Time {
	if sub.TrialEndsAt != nil {
		return sub.TrialEndsAt.UTC()
	}
	return sub.CreatedAt.Add(TrialDays * 24 * time.Hour).UTC()
}

// The three scopes below are EndsAt's SQL counterparts. Each one is a
// two-branch predicate rather than a COALESCE expression, because
// migration 087's (status, created_at) index serves the unextended branch and
// a COALESCE would defeat it; migration 103's partial index serves the
// extended branch, and stays small because extensions are rare.
//
// The branches are duplicated across the three helpers deliberately —
// building them from interpolated comparison operators would read like SQL
// injection and be harder to review. What keeps them honest is
// TestScopesAgreeWithEndsAt, which cross-checks every scope against EndsAt
// over a matrix of extended and unextended rows on each boundary. If a branch
// drifts, that test fails.

// EndedBeforeScope narrows to rows whose effective trial end is strictly
// before at. Used by the expiry cron.
func EndedBeforeScope(db *gorm.DB, at time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Where(
		"(trial_ends_at IS NULL AND created_at < ?) OR (trial_ends_at IS NOT NULL AND trial_ends_at < ?)",
		at.Add(-trialLen), at,
	)
}

// EndsBetweenScope narrows to rows whose effective trial end lies in the
// half-open-left, inclusive-right interval (lo, hi]. Used by the expiring
// queries: half-open left so an already-expired trial is not "expiring",
// inclusive right so one ending exactly at the edge is.
func EndsBetweenScope(db *gorm.DB, lo, hi time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Where(
		"(trial_ends_at IS NULL AND created_at > ? AND created_at <= ?) OR "+
			"(trial_ends_at IS NOT NULL AND trial_ends_at > ? AND trial_ends_at <= ?)",
		lo.Add(-trialLen), hi.Add(-trialLen), lo, hi,
	)
}

// EndsWithinDayScope narrows to rows whose effective trial end falls inside
// the 24 hours beginning at dayStart — [dayStart, dayStart+24h). Used by the
// reminder cron, which fires once per calendar day per offset.
//
// Note the brackets differ from EndsBetweenScope: a day bucket is inclusive
// on the left and exclusive on the right so consecutive days neither overlap
// nor leave a gap.
func EndsWithinDayScope(db *gorm.DB, dayStart time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	dayEnd := dayStart.Add(24 * time.Hour)
	return db.Where(
		"(trial_ends_at IS NULL AND created_at >= ? AND created_at < ?) OR "+
			"(trial_ends_at IS NOT NULL AND trial_ends_at >= ? AND trial_ends_at < ?)",
		dayStart.Add(-trialLen), dayEnd.Add(-trialLen), dayStart, dayEnd,
	)
}
```

- [ ] **Step 4: Run the unit tests to verify they pass**

Run: `cd services/marketplace-api && go test -count=1 ./internal/billing/trial/... -run TestEndsAt -v`

Expected: all four `TestEndsAt*` print `--- PASS`.

- [ ] **Step 5: Write the agreement integration test**

This is what makes the duplicated branches safe. Create
`services/marketplace-api/internal/billing/trial/endsat_integration_test.go`:

```go
//go:build integration

package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedSub inserts one store_subscriptions row. store_id references stores(id),
// so a parent store row is created first — unlike notifications, this table
// DOES have that FK (migration 000015). stripe_customer_id is NOT NULL.
func seedSub(t *testing.T, db *gorm.DB, createdAt time.Time, trialEndsAt *time.Time) subscription.StoreSubscription {
	t.Helper()

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone)
		 VALUES (?, ?, ?, ?, 'active', 'GB', 'GBP', 'Europe/London')`,
		storeID, tenantID, "seed-"+storeID.String()[:8], "seed-"+storeID.String()[:8],
	).Error)

	sub := subscription.StoreSubscription{
		ID:               uuid.New(),
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + storeID.String()[:8],
		Status:           subscription.StatusTrialing,
		CreatedAt:        createdAt,
		TrialEndsAt:      trialEndsAt,
	}
	require.NoError(t, db.Create(&sub).Error)
	return sub
}

// The scopes are SQL restatements of EndsAt, and the two-branch predicates
// are duplicated across three helpers. This test is what stops them drifting:
// for a matrix of extended and unextended rows placed on and around each
// boundary, every scope must return EXACTLY the rows EndsAt says it should.
//
// Delete either branch of any scope and this fails.
func TestScopesAgreeWithEndsAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	anchor := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	trialLen := time.Duration(trial.TrialDays) * 24 * time.Hour

	// Each row is built so its EFFECTIVE end sits at a named instant, via
	// both routes: unextended rows get created_at = end - 90d, extended rows
	// get created_at far away (200 days back) so the derived date is nowhere
	// near the stored one. An implementation that ignored trial_ends_at would
	// place the extended rows ~110 days in the past and fail every case.
	ends := []time.Time{
		anchor.Add(-48 * time.Hour), // well before
		anchor,                      // exactly on the anchor
		anchor.Add(72 * time.Hour),  // inside a 7-day window
		anchor.Add(30 * 24 * time.Hour), // well after
	}

	var all []subscription.StoreSubscription
	for _, e := range ends {
		all = append(all, seedSub(t, db, e.Add(-trialLen), nil)) // unextended
		stored := e
		all = append(all, seedSub(t, db, anchor.Add(-200*24*time.Hour), &stored)) // extended
	}

	// Sanity: EndsAt agrees with where we intended to put each row. Without
	// this, a bug in the fixture could make the comparisons below vacuous.
	for i, sub := range all {
		require.Equal(t, ends[i/2].UTC(), trial.EndsAt(sub),
			"fixture row %d is not where the test thinks it is", i)
	}

	idsFrom := func(rows []subscription.StoreSubscription) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, r := range rows {
			m[r.ID] = true
		}
		return m
	}
	// expected computes the answer in GO, from EndsAt — never from the SQL
	// under test.
	expected := func(pred func(time.Time) bool) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, s := range all {
			if pred(trial.EndsAt(s)) {
				m[s.ID] = true
			}
		}
		return m
	}
	query := func(scope func(*gorm.DB) *gorm.DB) map[uuid.UUID]bool {
		var got []subscription.StoreSubscription
		require.NoError(t, scope(db.Model(&subscription.StoreSubscription{})).Find(&got).Error)
		return idsFrom(got)
	}

	t.Run("EndedBeforeScope", func(t *testing.T) {
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndedBeforeScope(d, anchor) })
		want := expected(func(e time.Time) bool { return e.Before(anchor) })
		require.Equal(t, want, got)
		require.NotEmpty(t, want, "the predicate must match something, or this proves nothing")
	})

	t.Run("EndsBetweenScope is half-open left, inclusive right", func(t *testing.T) {
		hi := anchor.Add(7 * 24 * time.Hour)
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsBetweenScope(d, anchor, hi) })
		want := expected(func(e time.Time) bool { return e.After(anchor) && !e.After(hi) })
		require.Equal(t, want, got)
		// The row sitting exactly on `anchor` must be EXCLUDED, and that is
		// the case a `>=` implementation would get wrong.
		require.NotEmpty(t, want)
	})

	t.Run("EndsBetweenScope right edge is inclusive", func(t *testing.T) {
		hi := anchor.Add(72 * time.Hour) // a row ends exactly here
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsBetweenScope(d, anchor, hi) })
		want := expected(func(e time.Time) bool { return e.After(anchor) && !e.After(hi) })
		require.Equal(t, want, got)
		require.Len(t, want, 2, "both the extended and unextended row on that instant must be included")
	})

	t.Run("EndsWithinDayScope is inclusive left, exclusive right", func(t *testing.T) {
		dayStart := anchor // a row ends exactly at dayStart
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsWithinDayScope(d, dayStart) })
		want := expected(func(e time.Time) bool {
			return !e.Before(dayStart) && e.Before(dayStart.Add(24*time.Hour))
		})
		require.Equal(t, want, got)
		require.Len(t, want, 2, "the row exactly on dayStart must be INCLUDED — that is the bracket that differs from EndsBetweenScope")
	})
}
```

- [ ] **Step 6: Run the agreement test**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... -run TestScopesAgreeWithEndsAt -v
```

Expected: the parent test and all four subtests print `--- PASS`.

If you see `--- SKIP`, `TEST_DATABASE_URL` did not reach the process — fix that first and say so in your report. A skipped test is not a passing test.

- [ ] **Step 7: Prove the agreement test constrains the code**

Temporarily delete the extended branch of `EndsBetweenScope` — change its `Where` to only the first clause:

```go
	return db.Where(
		"(trial_ends_at IS NULL AND created_at > ? AND created_at <= ?)",
		lo.Add(-trialLen), hi.Add(-trialLen),
	)
```

Re-run Step 6. Expected: FAIL. **Revert** and re-run to confirm PASS. If the mutated version passed, the test proves nothing and must be fixed before continuing.

- [ ] **Step 8: Build and vet**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
```
Expected: all exit 0.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/billing/trial/endsat.go \
        services/marketplace-api/internal/billing/trial/endsat_test.go \
        services/marketplace-api/internal/billing/trial/endsat_integration_test.go
git commit -m "feat(trial): EndsAt accessor and the three effective-end scopes (#353)"
```

---

### Task 3: Teach the expiry cron and the expiring queries

Both read the effective end in SQL. `expiry_cron` is the site whose omission would mean an extension does not extend anything.

`ListExpiring` also carries a second defect this task fixes: it orders by `created_at ASC` on the stated assumption that "every row shares the same trial length" (`expiring.go:80-82`). Extensions break that assumption, so the ordering must move to the effective end.

**Files:**
- Modify: `services/marketplace-api/internal/billing/trial/expiry_cron.go:46-56`
- Modify: `services/marketplace-api/internal/billing/trial/expiring.go:53-115`
- Test: `services/marketplace-api/internal/billing/trial/expiry_extension_integration_test.go` (create)

**Interfaces:**
- Consumes: `trial.EndsAt`, `trial.EndedBeforeScope`, `trial.EndsBetweenScope` (Task 2).
- Produces: no new exported names. `ExpiringRow.TrialEndsAt` now carries the effective end.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/billing/trial/expiry_extension_integration_test.go`:

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

// THE test for this issue. A subscription created 100 days ago is past its
// derived 90-day end and the cron expires it today. Give it a trial_ends_at
// 30 days in the future and it must SURVIVE.
//
// Delete the extended branch of EndedBeforeScope and this fails — which is
// exactly what "an extension actually extends something" means.
func TestExpiryCron_DoesNotExpireAnExtendedTrial(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-100 * 24 * time.Hour)
	extended := now.Add(30 * 24 * time.Hour)

	unextended := seedSub(t, db, created, nil)
	protected := seedSub(t, db, created, &extended)

	cron := trial.NewExpiryCron(db, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	var after subscription.StoreSubscription
	require.NoError(t, db.First(&after, "id = ?", protected.ID).Error)
	require.Equal(t, subscription.StatusTrialing, after.Status,
		"an extended trial must not be expired by the day-90 rule")

	var control subscription.StoreSubscription
	require.NoError(t, db.First(&control, "id = ?", unextended.ID).Error)
	require.Equal(t, subscription.StatusExpired, control.Status,
		"the unextended control MUST expire — otherwise this test passes because the cron did nothing")
}

// An extended trial appears in the expiring window at its NEW end, and is
// absent from the window around its old one.
func TestListExpiring_UsesTheExtendedEnd(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-88 * 24 * time.Hour) // derived end is 2 days away
	extended := now.Add(40 * 24 * time.Hour) // real end is 40 days away
	seedSub(t, db, created, &extended)

	// A 7-day window around now would catch the DERIVED end and must not.
	rows, total, err := trial.ListExpiring(context.Background(), db, now, 7*24*time.Hour, 1, 50)
	require.NoError(t, err)
	require.Equal(t, int64(0), total, "the old derived end must not put an extended trial in the window")
	require.Empty(t, rows)

	// A window that reaches the NEW end must catch it, and report that date.
	rows, total, err = trial.ListExpiring(context.Background(), db, now, 45*24*time.Hour, 1, 50)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, extended.UTC(), rows[0].TrialEndsAt,
		"the row must report its effective end, not created_at + 90d")
}

// ListExpiring orders by soonest-ending first. Before #353 it ordered by
// created_at on the assumption that every row shared one trial length;
// extensions break that, so an older row with a later extended end must sort
// AFTER a newer row ending sooner.
func TestListExpiring_OrdersByEffectiveEndNotCreatedAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	// Older row, extended to end LATER.
	laterEnd := now.Add(20 * 24 * time.Hour)
	older := seedSub(t, db, now.Add(-120*24*time.Hour), &laterEnd)
	// Newer row, unextended, ending SOONER.
	sooner := seedSub(t, db, now.Add(-85*24*time.Hour), nil) // ends in 5 days

	rows, total, err := trial.ListExpiring(context.Background(), db, now, 30*24*time.Hour, 1, 50)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	require.Equal(t, sooner.StoreID.String(), rows[0].StoreID,
		"soonest effective end must come first; ordering by created_at would invert these")
	require.Equal(t, older.StoreID.String(), rows[1].StoreID)
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... -run 'TestExpiryCron_DoesNotExpire|TestListExpiring_' -v
```

Expected: FAIL. `TestExpiryCron_DoesNotExpireAnExtendedTrial` fails because the extended row was expired; the `ListExpiring` tests fail on the window and the ordering.

- [ ] **Step 3: Teach `expiry_cron.go`**

In `services/marketplace-api/internal/billing/trial/expiry_cron.go`, replace the body of `Run` from the `cutoff` line through the `Find` call with:

```go
	// Effective trial end, not created_at + TrialDays: a trial an operator
	// extended must survive its original day 90. EndedBeforeScope carries
	// both branches — see endsat.go.
	now := c.clock().UTC()
	var rows []subscription.StoreSubscription
	err := EndedBeforeScope(
		c.db.WithContext(ctx).
			Where("status = ?", subscription.StatusTrialing).
			Where("stripe_subscription_id IS NULL"),
		now,
	).Find(&rows).Error
	if err != nil {
		return err
	}
```

Delete the now-unused `cutoff` variable and update the doc comment on `Run` (currently lines 43-45) to say it selects trialing stores without a card **whose effective trial end has passed**, rather than "whose created_at is older than TrialDays".

- [ ] **Step 4: Teach `expiring.go`**

In `services/marketplace-api/internal/billing/trial/expiring.go`:

Replace `expiringScope` (and the paragraph of its doc comment describing the `created_at` algebra) with:

```go
// expiringScope narrows to trials that will actually EXPIRE, in the window
// (asOf, asOf+window].
//
// All three clauses matter, and the third is the one #282 originally missed:
//
//   - status = 'trialing'
//   - stripe_subscription_id IS NULL — no card. A trialing subscription WITH
//     a card has a Stripe subscription and will CONVERT, not expire; its
//     renewal date comes from Stripe, not from us.
//   - effective trial end inside the window. This is the same rule
//     expiry_cron.go applies and the same date the merchant is shown.
//
// The window's brackets and the index-preserving two-branch predicate both
// live in EndsBetweenScope — see endsat.go.
func expiringScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	return EndsBetweenScope(
		db.Model(&subscription.StoreSubscription{}).
			Where("status = ?", subscription.StatusTrialing).
			Where("stripe_subscription_id IS NULL"),
		asOf, asOf.Add(window),
	)
}
```

In `ListExpiring`, delete the local `trialLen` variable, change the ordering, and use the accessor for the row's date:

```go
	// Soonest effective end first. This used to order by created_at on the
	// assumption that every row shared one trial length — extensions break
	// that, so an older row extended further out must sort after a newer one
	// ending sooner.
	err = expiringScope(db.WithContext(ctx), asOf, window).
		Order("COALESCE(trial_ends_at, created_at + INTERVAL '" + strconv.Itoa(TrialDays) + " days') ASC").
		Offset(offset).
		Limit(limit).
		Find(&raw).Error
```

and in the row loop:

```go
			TrialEndsAt:      EndsAt(r),
```

Add `"strconv"` to the file's imports.

- [ ] **Step 5: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/billing/trial/... -v
```

Expected: every test in the package prints `--- PASS`, including Task 2's agreement test. Report the `--- PASS` and `--- SKIP` counts.

- [ ] **Step 6: Build, vet, and run the unit suite**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./... && go test -count=1 ./... > /tmp/t3.log 2>&1; echo "exit=$?"; grep -E '^(FAIL|---)' /tmp/t3.log | head -20
```
Expected: `exit=0`.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/trial/expiry_cron.go \
        services/marketplace-api/internal/billing/trial/expiring.go \
        services/marketplace-api/internal/billing/trial/expiry_extension_integration_test.go
git commit -m "feat(trial): expiry cron and expiring queries read the effective trial end (#353)"
```

---

### Task 4: Teach the two Stripe sites and the merchant-facing display

Three Go-value sites. Two of them send `trial_end` to **Stripe**, so getting these wrong means the console quotes one date while Stripe bills another. `planchange.go` also closes **#326**, which is a hardcoded `90` that does not reference `TrialDays` at all.

**Files:**
- Modify: `services/marketplace-api/internal/billing/trial/subscribe.go:129-131`
- Modify: `services/marketplace-api/internal/subscription/planchange/planchange.go:224-225`
- Modify: `services/marketplace-api/internal/handlers/admin/subscription.go:197`
- Test: `services/marketplace-api/internal/billing/trial/subscribe_trialend_test.go` (create)
- Test: `services/marketplace-api/internal/handlers/admin/subscription_trialend_test.go` (create)

**Interfaces:**
- Consumes: `trial.EndsAt` (Task 2).
- Produces: no new exported names.

- [ ] **Step 1: Find how each site is already tested**

Before writing tests, read these to reuse the existing fakes rather than inventing new ones — a second Stripe fake in the same package will collide:

```bash
cd services/marketplace-api
grep -rn "TrialEnd" internal/billing/trial/*_test.go | head
grep -rn "TrialEnd\|CreateSubscription" internal/subscription/planchange/*_test.go | head
grep -rn "enrichTrialBanner\|TrialEndsAt" internal/handlers/admin/*_test.go | head
```

Use whatever stub each package already has. If a package has no existing Stripe stub, add one **in your new test file** and name it after the test (`trialEndStubStripe`), so it cannot collide with a future one.

- [ ] **Step 2: Write the failing tests**

Create `services/marketplace-api/internal/billing/trial/subscribe_trialend_test.go`:

```go
package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// The value sent to Stripe as trial_end must be the EFFECTIVE end. If this
// uses created_at + 90d, Stripe bills on a date the console does not show and
// the merchant is charged early.
//
// Asserting the VALUE, not that a call happened: a test that only checks a
// call was made passes against a fabricated zero.
func TestTrialEndValueSentToStripeUsesEffectiveEnd(t *testing.T) {
	created := time.Date(2026, 3, 4, 13, 47, 11, 0, time.UTC)
	extended := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	require.NotEqual(t, created.Add(trial.TrialDays*24*time.Hour).Unix(), extended.Unix(),
		"fixture must distinguish the extended value from the derived one")

	sub := subscription.StoreSubscription{
		ID: uuid.New(), CreatedAt: created, TrialEndsAt: &extended,
	}

	require.Equal(t, extended.UTC().Unix(), trial.EndsAt(sub).Unix(),
		"whatever computes the Stripe trial_end must go through EndsAt")
}
```

The test above pins the accessor's contract. The two **call-site** assertions —
that `subscribe.go` and `planchange.go` actually hand `trial.EndsAt(...).Unix()` to
Stripe — need each package's existing fake, and are added in Step 5. Both fakes are
already named `fakeStripe`, so do NOT declare a third:

- **`internal/billing/trial`** — `fakeStripe` lives in `subscribe_integration_test.go`
  and already captures `lastTrialEnd int64`. **That file carries `//go:build integration`**,
  so the subscribe call-site assertion must go in an integration-tagged file too; it
  cannot live in the plain unit test created above.
- **`internal/subscription/planchange`** — `fakeStripe` is in `planchange_test.go:20`
  and its `CreateSubscription` **discards its input** (`:31`). Add a capture field to
  that existing fake (`lastTrialEnd int64`, assigned from `in.TrialEnd`) rather than
  writing a second fake.

Create `services/marketplace-api/internal/handlers/admin/subscription_trialend_test.go`:

```go
package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// The date the MERCHANT is shown must be the effective end, and days_remaining
// must be counted from it. A merchant granted an extension who still sees the
// old date has been told something false by their own dashboard.
func TestEnrichTrialBanner_UsesExtendedEnd(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	created := now.Add(-88 * 24 * time.Hour) // derived end: 2 days away
	extended := now.Add(20 * 24 * time.Hour) // real end: 20 days away

	sub := subscription.StoreSubscription{
		ID: uuid.New(), Status: subscription.StatusTrialing,
		CreatedAt: created, TrialEndsAt: &extended,
	}

	var resp SubscriptionResponse
	enrichTrialBanner(&resp, sub, now)

	require.NotNil(t, resp.TrialEndsAt)
	require.Equal(t, extended.UTC().Format("2006-01-02T15:04:05Z"), *resp.TrialEndsAt,
		"the merchant must be shown the extended end, not created_at + 90d")
	require.NotNil(t, resp.DaysRemainingInTrial)
	require.Equal(t, 20, *resp.DaysRemainingInTrial,
		"days_remaining must count to the effective end; the derived end would give 2")
}
```

The response fields are `TrialEndsAt *string` and `DaysRemainingInTrial *int`
(`internal/handlers/admin/subscription.go:119-120`) — both pointers, hence the
nil checks.

- [ ] **Step 3: Run the tests to verify they fail**

```bash
cd services/marketplace-api
go test -count=1 ./internal/billing/trial/... -run TestTrialEndValueSentToStripe -v
go test -count=1 ./internal/handlers/admin/... -run TestEnrichTrialBanner_UsesExtendedEnd -v
```

Expected: the admin test FAILS on both assertions (it shows the derived date). The trial test passes already if Task 2 is in — that is fine, it pins the contract the sites must use.

- [ ] **Step 4: Make the three edits**

`internal/billing/trial/subscribe.go` — replace the `trialEnd` line and its comment:
```go
	// trial_end is the EFFECTIVE end: an operator-extended trial must bill on
	// the extended date, not created_at + TrialDays (#353).
	trialEnd := EndsAt(row).Unix()
```

`internal/subscription/planchange/planchange.go` — replace the hardcoded constant (closes #326):
```go
	// trial_end is the EFFECTIVE end (#353). This previously hardcoded
	// 90 * 24 * time.Hour without referencing trial.TrialDays at all (#326),
	// so a change to the trial length would silently have disagreed with
	// Stripe about a billing date.
	trialEnd := trial.EndsAt(sub).Unix()
```
Add `"github.com/mark8ly/marketplace-api/internal/billing/trial"` to that file's imports if it is not already there, and remove any now-unused import.

`internal/handlers/admin/subscription.go` — replace the `endsAt` line:
```go
	endsAt := trial.EndsAt(s)
```
`trial` is already imported in that file (it referenced `trial.TrialDays`); confirm the import survives and remove it only if nothing else uses it.

- [ ] **Step 5: Add the two Stripe call-site assertions, then prove them by mutation**

Add one assertion per call site, using the fakes named above.

**`internal/billing/trial`** — in an integration-tagged test alongside `fakeStripe`,
seed a subscription row whose `trial_ends_at` is set to a value distinct from
`created_at + 90d`, run the subscribe path, and assert
`fake.lastTrialEnd == <that value>.Unix()`.

**`internal/subscription/planchange`** — add `lastTrialEnd int64` to the existing
`fakeStripe` (`planchange_test.go:20`) and assign it inside `CreateSubscription`
from `in.TrialEnd`. Then in a test, give the subscription a `TrialEndsAt` distinct
from `created_at + 90d` and assert `f.lastTrialEnd` equals it.

In both cases assert the **exact integer**. A test that only checks a call happened
passes against a fabricated zero.

Then prove both assertions bite. Temporarily revert `subscribe.go`'s line to `row.CreatedAt.Add(TrialDays * 24 * time.Hour).Unix()` and re-run that package's tests: expected FAIL. Revert. Do the same for `planchange.go` with `sub.CreatedAt.Add(90 * 24 * time.Hour).Unix()`: expected FAIL. Revert.

Report both failures and both post-revert passes. An assertion that passes under mutation is not testing the Stripe value.

- [ ] **Step 6: Run the tests and the full unit suite**

```bash
cd services/marketplace-api
go test -count=1 ./internal/billing/trial/... ./internal/subscription/planchange/... ./internal/handlers/admin/... -v 2>&1 | grep -E '^(--- |ok|FAIL)' | tail -30
go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t4.log 2>&1; echo "exit=$?"; grep -E '^FAIL' /tmp/t4.log | head
```
Expected: `exit=0`.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/trial/subscribe.go \
        services/marketplace-api/internal/billing/trial/subscribe_trialend_test.go \
        services/marketplace-api/internal/subscription/planchange/planchange.go \
        services/marketplace-api/internal/handlers/admin/subscription.go \
        services/marketplace-api/internal/handlers/admin/subscription_trialend_test.go
git add -u services/marketplace-api/internal/subscription/planchange/ services/marketplace-api/internal/billing/trial/
git commit -m "feat(trial): Stripe trial_end and the merchant banner read the effective end; closes #326 (#353)"
```

---

### Task 5: Teach the trial-reminder cron

The one site that is not a substitution. `trial_reminders.go` never computes a date — it computes an **offset** (`dayOffset := trial.TrialDays - t.DaysBefore`) and day-buckets on `created_at`. An extended trial therefore keeps getting reminders on the original schedule and gets none before its real end: "your trial ends in 3 days", a month early, then silence.

**Files:**
- Modify: `services/marketplace-api/internal/subscription/dunning/trial_reminders.go:104-118`
- Test: `services/marketplace-api/internal/subscription/dunning/trial_reminders_extension_integration_test.go` (create)

**Interfaces:**
- Consumes: `trial.EndsWithinDayScope` (Task 2).
- Produces: no new exported names.

- [ ] **Step 1: Write the failing test**

Create `services/marketplace-api/internal/subscription/dunning/trial_reminders_extension_integration_test.go`:

```go
//go:build integration

package dunning_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingEmail captures which template went to whom, so the assertions can
// be about the VALUES sent rather than merely that a send happened.
type recordingEmail struct{ sent []string }

func (r *recordingEmail) Send(_ context.Context, template email.TemplateID, to string, _ map[string]any) error {
	r.sent = append(r.sent, string(template)+"->"+to)
	return nil
}

func seedTrialSub(t *testing.T, db *gorm.DB, createdAt time.Time, trialEndsAt *time.Time, hasPM bool) subscription.StoreSubscription {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, name, slug, status, country_code, currency_code, timezone)
		 VALUES (?, ?, ?, ?, 'active', 'GB', 'GBP', 'Europe/London')`,
		storeID, tenantID, "seed-"+storeID.String()[:8], "seed-"+storeID.String()[:8],
	).Error)
	sub := subscription.StoreSubscription{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		StripeCustomerID:        "cus_" + storeID.String()[:8],
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: hasPM,
		CreatedAt:               createdAt,
		TrialEndsAt:             trialEndsAt,
	}
	require.NoError(t, db.Create(&sub).Error)
	return sub
}

// An extended trial must be reminded relative to its NEW end. Before #353 the
// cron bucketed on created_at and would have fired T-15 based on the original
// 90-day schedule — a month early — then nothing before the real end.
func TestTrialReminders_FireRelativeToTheExtendedEnd(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Derived end is 15 days away — the OLD schedule would fire T-15 today.
	created := now.Add(-75 * 24 * time.Hour)
	// Real end is 60 days away, so nothing should fire today.
	extended := now.Add(60 * 24 * time.Hour)
	sub := seedTrialSub(t, db, created, &extended, false)

	rec := &recordingEmail{}
	cron := dunning.NewSendTrialReminders(db, rec, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, rec.sent,
		"no reminder is due 60 days before the effective end; bucketing on created_at would have sent T-15")

	var n int64
	require.NoError(t, db.Table("trial_reminders").Where("subscription_id = ?", sub.ID).Count(&n).Error)
	require.Equal(t, int64(0), n, "no idempotency slot should be claimed when nothing is due")
}

// The converse, and the one that proves the cron still works at all: with the
// effective end exactly 15 days out, T-15 fires.
func TestTrialReminders_FireOnTheExtendedEndsT15(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Created 200 days ago — the derived end is long past, so ONLY the stored
	// value can put this row in the T-15 bucket.
	created := now.Add(-200 * 24 * time.Hour)
	extended := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC) // 15 days out
	sub := seedTrialSub(t, db, created, &extended, false)

	rec := &recordingEmail{}
	cron := dunning.NewSendTrialReminders(db, rec, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	require.Len(t, rec.sent, 1, "exactly the T-15 reminder is due")

	var keys []string
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", sub.ID).Pluck("offset_key", &keys).Error)
	require.Equal(t, []string{"no_pm_t_minus_15"}, keys)
}
```

**Before running:** the file needs the `email` import for `recordingEmail`'s signature — add
`"github.com/mark8ly/marketplace-api/internal/email"`. Check `email.Client`'s exact method
signature in `internal/email/client.go` and match it; if `NewSendTrialReminders`'s counter
parameter cannot be nil, pass whatever no-op the package's existing tests use.

- [ ] **Step 2: Run it to verify it fails**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/subscription/dunning/... -run TestTrialReminders_ -v
```

Expected: FAIL. The first test fails because a T-15 fires on the old schedule; the second because the row is never selected.

- [ ] **Step 3: Rewrite `runForOffset`'s selection**

In `services/marketplace-api/internal/subscription/dunning/trial_reminders.go`, replace the `dayOffset`/`dayStart`/`dayEnd` computation and the `Where("created_at >= ? AND created_at < ?", ...)` clause with:

```go
	// Target subscriptions whose EFFECTIVE trial end falls on the day that is
	// DaysBefore days from now. This used to work backwards from a fixed trial
	// length and bucket on created_at, which meant an operator-extended trial
	// kept its original reminder schedule and got nothing before its real end
	// (#353).
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, t.DaysBefore)

	var rows []subscription.StoreSubscription
	err := trial.EndsWithinDayScope(
		s.db.WithContext(ctx).
			Model(&subscription.StoreSubscription{}).
			Where("status IN ?", []subscription.SubscriptionStatus{
				subscription.StatusSignup,
				subscription.StatusTrialing,
			}).
			Where("has_default_payment_method = ?", t.HasPM),
		dayStart,
	).Find(&rows).Error
	if err != nil {
		return err
	}
```

Add `"github.com/mark8ly/marketplace-api/internal/billing/trial"` to the imports if the file does not already have it (it referenced `trial.TrialDays`, so it likely does — confirm rather than assume, and remove the import only if nothing else in the file uses it).

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -count=1 -p 1 -tags=integration ./internal/subscription/dunning/... -v
```

Expected: both new tests PASS and every pre-existing test in the package still passes. If a pre-existing reminder test now fails, that is a real regression in the new selection — fix it, do not adjust the old test to match.

Report the `--- PASS`, `--- SKIP` and `--- FAIL` counts.

- [ ] **Step 5: Build, vet, full suite**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t5.log 2>&1; echo "exit=$?"; grep -E '^FAIL' /tmp/t5.log | head
```
Expected: `exit=0`.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/subscription/dunning/trial_reminders.go \
        services/marketplace-api/internal/subscription/dunning/trial_reminders_extension_integration_test.go
git commit -m "feat(dunning): trial reminders fire relative to the effective trial end (#353)"
```

---

### Task 6: The guard that stops an eighth site appearing

The spec's central claim is that **nothing except `trial.EndsAt` computes a trial end**. That is only true today; the guard is what keeps it true.

**Files:**
- Create: `services/marketplace-api/internal/billing/trial/single_definition_test.go`

**Interfaces:**
- Consumes: nothing at runtime — it scans source files.
- Produces: no exported names.

- [ ] **Step 1: Write the guard test**

Create `services/marketplace-api/internal/billing/trial/single_definition_test.go`:

```go
package trial_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Trial end is derived at exactly one place: trial.EndsAt. Before #353 the
// same arithmetic lived at seven sites, which is why an operator had nothing
// to extend — the expiry cron, Stripe, the merchant's screen and the platform
// console each recomputed it and would have ignored a stored value.
//
// This test fails when an eighth site appears. If you are here because it
// failed: call trial.EndsAt instead of doing the arithmetic, or — if your site
// genuinely is a new definition — change EndsAt and say so.
func TestTrialEndIsDerivedInExactlyOnePlace(t *testing.T) {
	// Matches `<something>.Add(TrialDays * 24 * time.Hour)` and the hardcoded
	// 90-day form that #326 was, in either spacing.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`Add\(\s*(trial\.)?TrialDays\s*\*\s*24\s*\*\s*time\.Hour\s*\)`),
		regexp.MustCompile(`Add\(\s*90\s*\*\s*24\s*\*\s*time\.Hour\s*\)`),
		regexp.MustCompile(`AddDate\(\s*0\s*,\s*0\s*,\s*-?\s*(trial\.)?TrialDays\s*\)`),
	}

	// The one legitimate site. Paths are relative to the service root.
	allowed := map[string]bool{
		filepath.Join("internal", "billing", "trial", "endsat.go"): true,
	}

	root := filepath.Join("..", "..", "..") // -> services/marketplace-api
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		if allowed[rel] {
			return nil
		}
		src, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		for _, re := range patterns {
			if re.Match(src) {
				offenders = append(offenders, rel)
				return nil
			}
		}
		return nil
	})
	require.NoError(t, err)

	require.Empty(t, offenders,
		"these files derive a trial end themselves instead of calling trial.EndsAt — "+
			"that is how #353 happened. Call trial.EndsAt, or change EndsAt if this is genuinely a new definition.")
}
```

- [ ] **Step 2: Run it — it must PASS, because Tasks 3-5 removed every other site**

Run: `cd services/marketplace-api && go test -count=1 ./internal/billing/trial/... -run TestTrialEndIsDerivedInExactlyOnePlace -v`

Expected: PASS.

**If it FAILS, do not weaken the test.** It has found a site Tasks 3-5 missed, which is precisely its job. Fix that site to call `trial.EndsAt` and re-run.

- [ ] **Step 3: Prove the guard actually bites**

A guard that cannot fail is worse than none. Temporarily add this to
`internal/handlers/admin/subscription.go`, inside `enrichTrialBanner`:

```go
	_ = s.CreatedAt.Add(trial.TrialDays * 24 * time.Hour)
```

Re-run Step 2. Expected: FAIL, naming `internal/handlers/admin/subscription.go`. **Revert** and re-run to confirm PASS.

Report both observations. If the mutated version passed, the regex does not match real code and the test is theatre.

- [ ] **Step 4: Build, vet, full suite**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
go test -count=1 ./... > /tmp/t6.log 2>&1; echo "exit=$?"; grep -E '^FAIL' /tmp/t6.log | head
```
Expected: `exit=0`.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/trial/single_definition_test.go
git commit -m "test(trial): guard that trial end is derived in exactly one place (#353)"
```

---

## Final whole-branch review

Verify as a whole, and mutate rather than read — a review that only reads the diff has repeatedly missed things a two-minute mutation caught.

- [ ] `go test -count=1 ./...` from the **service root** exits 0, and the root-package schema guard is confirmed RUN and PASS **by name**, not inferred from a summary line.
- [ ] `go vet -tags=integration ./...` exits 0.
- [ ] The full integration suite runs with `-p 1` and reports `--- SKIP` = 0 for the touched packages. A skipped test is not a passing test, and `testdb.NewDB` skips silently when `TEST_DATABASE_URL` is unset.
- [ ] `TestTrialEndIsDerivedInExactlyOnePlace` passes, and was demonstrated to fail under an injected eighth site.
- [ ] `TestScopesAgreeWithEndsAt` passes, and was demonstrated to fail with a branch deleted.
- [ ] Both Stripe `trial_end` values were asserted **as values** and demonstrated to fail under mutation. This is the one that costs real money if wrong.
- [ ] `git diff main --stat` shows no change under `services/platform-api/`.
- [ ] No comment added in this branch asserts a property that was not checked. In particular, any comment claiming a site is "the only" anything.

## What production can and cannot prove

`store_subscriptions` was reported **empty** in production earlier in this milestone. **That claim's freshness has expired — re-check it before verifying, do not assume it.** Query the row count via the platform surface or a read-only path before writing a verification report.

If it still holds, then **every behaviour this branch changes is unexercised in production**: the deploy proves the migration applied and the service starts, and nothing more. Say that explicitly rather than reporting a green deploy as a working feature.

Deploys arrive as image tags (`main-<sha7>`), never git commits. `marketplace-api-admin` carries the `migrate` initContainer and `marketplace-api-storefront` does not, so a storefront pod restarting mid-rollout crashloops until its own rollout lands — expected, self-healing, and indistinguishable from a real failure if you are not expecting it.
