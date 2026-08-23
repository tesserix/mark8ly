# Admin Billing Trials Implementation Plan (#285)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Answer "which trials expire this week" for the whole estate — and correct #282's already-live `trials_expiring` counter, which asks the wrong question.

**Architecture:** `internal/billing/trial` becomes the single owner of "what a trial is and when it ends", holding both the count and the listing beside the expiry cron that defines the rule. `marketplace-api`'s platformadmin surface exposes the listing; the KPI handler repoints to the corrected count. Tenant names come from one batch lookup through the existing `tenantdirectory` client.

**Tech Stack:** Go 1.26, Gin 1.12, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-24-admin-billing-trials-design.md` — the binding authority. **Read its "The correction" section before writing any code**; the whole plan follows from it.

## Global Constraints

- **An expiring trial is all three of:** `status = 'trialing'` AND `stripe_subscription_id IS NULL` AND `created_at + TrialDays` within the window. Any query that omits the `stripe_subscription_id` clause is counting converting trials too, and is wrong.
- `TrialDays = 90` (`internal/billing/trial/subscribe.go`) is the trial **length**. The **window** is how far ahead we look — the endpoint's `days`, defaulting to 7.
- Window is **half-open left, inclusive right**: `created_at + TrialDays > asOf` and `<= asOf + window`. An already-expired trial is not "expiring"; one ending exactly at the edge is.
- **`trial_ends_at` on the wire is `created_at + TrialDays`** — the same value the merchant-facing endpoint at `internal/handlers/admin/subscription.go:197` computes. If the console and the merchant quote different dates, one is lying.
- `asOf` is a **parameter**, never `time.Now()` inside a query, so tests can pin boundaries. Production passes `time.Now()`.
- **No `amount` key.** Not `null`, not `0`. mark8ly holds no prices — `PriceIDFor` returns a Stripe price ID. Carry `billing_currency`, `plan`, `period`.
- `billing_currency` is nullable; **omit** it when absent rather than shipping empty.
- **No `dunning_state` field.** A trial cannot be in dunning — the ladder selects `status IN (past_due, expired, store_closed)` and `status` is single-valued. Ship `payment_method_on_file` instead.
- Standard envelope `{"data": [...], "pagination": {...}}`; empty is `200` with `[]`, allocated via `make([]T, 0, n)`. Ordered **soonest-expiry first**.
- `pagination.limit` reports the **effective** (clamped) limit. Ids bare, timestamps RFC3339 UTC, no `source` field.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## The import constraint that decides file placement

`internal/billing/trial` imports `internal/subscription` (`subscribe.go:17`). Therefore `internal/subscription` **cannot** import `internal/billing/trial` — that is why `CountTrialsExpiring` cannot reference `TrialDays` from where it currently lives, and why it moves rather than being patched in place.

`internal/subscription/dunning` already imports `internal/billing/trial` and is unaffected (`trial` does not import `dunning`).

## Environment

- **Use the LAN IP, not localhost** — a native Postgres squats on 127.0.0.1:
  - marketplace: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`
  - platform: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable'`
  The committed Makefile says `localhost`, correct everywhere else — **do not change it**.
- Integration runs need `-p 1 -count=1`.
- **`internal/subscription` has pre-existing `store_subscriptions_store_id_fkey` failures**, tracked as **#317** and verified pre-existing. Scope runs with `-run` and do not try to fix them.
- **platform-api seeds its own reference data**: `GB`/`GBP`/`Europe/London` are in the seed, `IE`/`Europe/Dublin` are not.
- Do NOT run `make dev`. Never run anything against a remote or GKE database.
- Ignore `go.work requires go >= 1.26.6 (running go 1.26.5)` — pre-existing drift.

## File Structure

**`marketplace-api`**

| File | Responsibility |
|---|---|
| `internal/billing/trial/expiring.go` (create) | `DefaultExpiryWindow`, `ExpiringRow`, `CountExpiring`, `ListExpiring` |
| `internal/billing/trial/expiring_integration_test.go` (create) | Boundary, card-present exclusion, ordering, window |
| `internal/subscription/repository.go` (modify) | **Remove** `CountTrialsExpiring` from the interface and impl |
| `internal/subscription/models.go` (modify) | **Remove** `TrialExpiryHorizon` |
| `internal/subscription/repository_kpi_integration_test.go` (delete) | Its subject moves to `trial` |
| `internal/subscription/planchange/{cron_test,planchange_test}.go` (modify) | Drop the now-absent fake method |
| `internal/handlers/platformadmin/kpis.go` (modify) | Repoint to `trial.CountExpiring` |
| `internal/handlers/platformadmin/kpis_test.go` (modify) | Stub follows the new interface |
| `internal/tenantdirectory/client.go` (modify) | `IDs` on `ListParams` |
| `internal/handlers/platformadmin/billing_trials.go` (create) | Handler and wire shape |
| `internal/handlers/platformadmin/billing_trials_test.go` (create) | Contract + golden |
| `internal/handlers/platformadmin/testdata/billing_trials_response.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/routes.go` (modify) | New `Deps` field, mount |
| `cmd/marketplace-api/main.go` (modify) | Wire at BOTH sites |

**`platform-api`**

| File | Responsibility |
|---|---|
| `internal/tenant/directory.go` (modify) | `IDs` on `DirectoryFilter`, applied in the shared filter |
| `internal/tenant/directory_integration_test.go` (modify) | `ids` filtering |
| `internal/tenant/handler.go` (modify) | Parse `ids` |

---

### Task 1: the corrected trial queries

**Files:** `internal/billing/trial/expiring.go` (create), `internal/billing/trial/expiring_integration_test.go` (create)

**Interfaces:**
- Produces: `trial.DefaultExpiryWindow` (`time.Duration`), `trial.ExpiringRow`, `trial.CountExpiring(ctx, db *gorm.DB, asOf time.Time, window time.Duration) (int64, error)`, `trial.ListExpiring(ctx, db *gorm.DB, asOf time.Time, window time.Duration, page, limit int) ([]ExpiringRow, int64, error)`.

Nothing is removed in this task — the old code keeps working until Task 2. That keeps this task independently reviewable.

```go
// DefaultExpiryWindow is how far ahead the console looks for expiring trials
// when a caller supplies no `days`. Shared by GET /admin/kpis's
// trials_expiring counter and GET /admin/billing/trials, so the two cannot
// report different numbers for the same word.
const DefaultExpiryWindow = 7 * 24 * time.Hour

// MaxExpiryWindow clamps `days`. Beyond the trial length the window stops
// meaning anything — every live trial is inside it.
const MaxExpiryWindow = time.Duration(TrialDays) * 24 * time.Hour

// ExpiringRow is one trial about to expire.
type ExpiringRow struct {
	TenantID        string
	StoreID         string
	TrialEndsAt     time.Time
	Plan            string
	Period          string
	BillingCurrency *string
	HasPaymentMethod bool
	Status          string
}
```

The shared predicate — **one definition, used by both queries**:

```go
// expiringScope narrows to trials that will actually EXPIRE, in the window
// (asOf, asOf+window].
//
// All three clauses matter, and the third is the one #282 originally missed:
//
//   - status = 'trialing'
//   - stripe_subscription_id IS NULL — no card. A trialing subscription WITH
//     a card has a Stripe subscription and will CONVERT, not expire; its
//     renewal date comes from Stripe, not from created_at.
//   - created_at + TrialDays inside the window. This is the same rule
//     expiry_cron.go applies and the same date the merchant is shown.
//
// Half-open left so an already-expired trial is not "expiring"; inclusive
// right so one ending exactly at the edge is.
func expiringScope(db *gorm.DB, asOf time.Time, window time.Duration) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Model(&subscription.StoreSubscription{}).
		Where("status = ?", subscription.StatusTrialing).
		Where("stripe_subscription_id IS NULL").
		Where("created_at > ?", asOf.Add(-trialLen)).
		Where("created_at <= ?", asOf.Add(window).Add(-trialLen))
}
```

Note the algebra: `created_at + TrialDays > asOf` is `created_at > asOf - TrialDays`. Doing it this way keeps the comparison on a plain indexed column instead of an expression. Say so in a comment — the next reader will otherwise "simplify" it back.

`CountExpiring` counts over that scope. `ListExpiring` selects the row fields, orders by `created_at ASC` (soonest trial end first, since every row shares the same trial length), applies `Offset`/`Limit`, and returns the unpaginated total from a second count over the same scope. Allocate the slice with `make([]ExpiringRow, 0, limit)` before scanning.

Compute `TrialEndsAt` as `row.CreatedAt.Add(trialLen)` in Go rather than in SQL, so it cannot drift from the merchant-facing calculation.

**Tests** (`expiring_integration_test.go`, `package trial_test`, `//go:build integration`). Seed relative to a fixed `asOf`:

- [ ] A card-less trial ending in 3 days counts and appears.
- [ ] **A trial WITH `stripe_subscription_id` set does NOT count**, even though its `created_at` puts its trial end inside the window. *This is the assertion that would have caught the original defect.*
- [ ] **A row with `current_period_end` NULL still appears** — the population the old query silently dropped. Set it explicitly to NULL.
- [ ] **Exactly at the right edge counts:** `created_at` such that `created_at + TrialDays == asOf + window`.
- [ ] **Exactly at `asOf` does not count:** `created_at + TrialDays == asOf` (half-open left).
- [ ] A trial ending beyond the window does not count.
- [ ] `status = 'active'` does not count, however recent.
- [ ] Ordering: three rows whose insertion order differs from expiry order come back soonest-first.
- [ ] `CountExpiring` equals `len(rows)` from `ListExpiring` for the same `asOf`/window with a large limit.
- [ ] Pagination: `limit=1` returns one row and a `total` of the full match count.

**Verify:** `cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go test -tags=integration -p 1 -count=1 ./internal/billing/trial/...`

**Commit:** `feat(trial): expiring-trial count and listing on the authoritative rule (#285)`

---

### Task 2: land the correction

**Files:** `internal/subscription/repository.go`, `internal/subscription/models.go`, `internal/subscription/repository_kpi_integration_test.go` (delete), `internal/subscription/planchange/cron_test.go`, `internal/subscription/planchange/planchange_test.go`, `internal/handlers/platformadmin/kpis.go`, `internal/handlers/platformadmin/kpis_test.go`

**Interfaces:**
- Consumes: `trial.CountExpiring`, `trial.DefaultExpiryWindow` from Task 1.

This task makes `/admin/kpis` report a true number. Until it lands, `trials_expiring` is structurally `0`.

- [ ] Remove `CountTrialsExpiring` from `subscription.Repository` (interface **and** the `gormRepository` implementation).
- [ ] Remove `TrialExpiryHorizon` from `subscription/models.go`.
- [ ] Delete `internal/subscription/repository_kpi_integration_test.go` — its subject moved to `trial` in Task 1, where equivalent and better tests now live. Do not port the old `current_period_end` tests; they assert the defect.
- [ ] Remove the now-orphaned `CountTrialsExpiring` stubs from the two hand-rolled fakes at `planchange/cron_test.go:74` and `planchange/planchange_test.go:75`.
- [ ] In `platformadmin/kpis.go`, change the narrow `Subscriptions` interface (line ~30) to `CountExpiring(ctx context.Context, db *gorm.DB, asOf time.Time, window time.Duration) (int64, error)` — **the same method name as the package function it fronts**, so a reader tracing the call does not have to notice a rename. Keep the interface rather than calling `trial.CountExpiring` directly: it is what lets `kpis_test.go` stub the value, and the existing stub pattern should survive this change.
- [ ] Update the call at line ~168 to pass `trial.DefaultExpiryWindow`.
- [ ] Update `kpis_test.go`'s stub to the new shape.

**Tests:**

- [ ] The existing KPI tests still pass with the new source.
- [ ] Add one test asserting `trials_expiring` reflects the value the trial source returns — with a **distinct non-zero** stub value, so a fabricated `0` cannot satisfy it. (The registry lookup is already guarded; this pins the wiring.)
- [ ] `grep` confirms no reference to `TrialExpiryHorizon` or `CountTrialsExpiring` remains anywhere.

**Verify:** `cd services/marketplace-api && go build ./... && go test ./internal/handlers/platformadmin/... ./internal/subscription/planchange/...` plus the Task 1 integration run.

**Commit:** `fix(kpis): count expiring trials on the authoritative rule, not current_period_end (#285)`

---

### Task 3: batch tenant lookup by ids

**Files:** `platform-api/internal/tenant/directory.go`, `directory_integration_test.go`, `handler.go`; `marketplace-api/internal/tenantdirectory/client.go`

**Interfaces:**
- Produces: `DirectoryFilter.IDs []string` (platform-api) and `tenantdirectory.ListParams.IDs []string` (marketplace-api), sent as `?ids=a,b,c`.

Extend the **existing** directory rather than adding an endpoint — the umbrella comment on #260 asks for extensions to be additive.

In `applyDirectoryFilter`, add:

```go
	if len(f.IDs) > 0 {
		q = q.Where("tenants.id IN ?", f.IDs)
	}
```

An empty `IDs` slice must add **no** clause — otherwise an unfiltered call silently returns nothing. Parse `ids` in the handler by splitting on `,` and dropping empty segments; if nothing remains, treat it as absent.

On the client, add `IDs []string` to `ListParams` and set `q.Set("ids", strings.Join(p.IDs, ","))` when non-empty.

**Tests:**

- [ ] Seeding three tenants and filtering by two ids returns exactly those two.
- [ ] An unknown id in the list is ignored, not an error, and the known ones still return.
- [ ] **An empty `IDs` slice returns everything** — the guard against a silently-empty result.
- [ ] `ids` combines with `status` rather than replacing it.
- [ ] Client: `ids=a,b` appears in the query string; assert on the received `r.URL.RawQuery`.

**Verify:** platform-api `-run Directory` integration run plus `cd services/marketplace-api && go test ./internal/tenantdirectory/...`

**Commit:** `feat(tenant): filter the directory by ids for batch lookup (#285)`

---

### Task 4: the endpoint

**Files:** `internal/handlers/platformadmin/billing_trials.go` (create), `billing_trials_test.go` (create), `testdata/billing_trials_response.json` (create), `routes.go`, `cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `trial.ListExpiring`, `trial.DefaultExpiryWindow`, `trial.MaxExpiryWindow`, `tenantdirectory` `List` with `IDs`.

Wire shape exactly as the spec's JSON block. Project field by field.

- [ ] Parse `days` (default `DefaultExpiryWindow`, clamp to `MaxExpiryWindow`), `page`, `limit` (default 50, clamp 500). A missing parameter takes the default and never errors.
- [ ] Call `trial.ListExpiring`.
- [ ] Collect the **distinct** tenant ids from the page and make **one** `tenantdirectory.List` call with `IDs` set. Not one call per row.
- [ ] Map ids to names; a tenant missing from the response gets its `tenant_name` omitted rather than blank — but log it, because it means the two services disagree about which tenants exist.
- [ ] `trial_ends_at` RFC3339 UTC; `days_remaining` computed from the same instant used for the query, not a fresh `time.Now()`.
- [ ] `billing_currency` omitted when nil.
- [ ] **No `amount` key.** **No `dunning_state` key.**
- [ ] Upstream unreachable (`tenantdirectory.ErrUnavailable`) → `503 upstream_unavailable`; any other error → `500 internal_error`, matching `respondErr`'s reasoning on this surface.
- [ ] Mount in `routes.go` behind a nil guard; wire at **BOTH** `main.go` sites (~1917 and ~2003) — production runs two deployments from this binary and missing one makes the endpoint 404 depending on which served the request.

**Tests** (`package platformadmin_test`):

- [ ] Golden fixture via `require.JSONEq`, **proven by mutation**: rename a JSON key and confirm failure; add a field to the response struct and confirm failure. Revert both and report both results.
- [ ] Empty result is `200` with `data` exactly `[]` — assert on raw JSON.
- [ ] Rows are ordered soonest-first as returned; the handler must not reorder.
- [ ] `days=9999` clamps; `days` absent takes the default; both asserted via the value passed to the stub.
- [ ] `pagination.limit` reports the clamped value.
- [ ] **One tenant lookup for a page with several rows sharing a tenant** — assert the stub was called exactly once and received deduplicated ids.
- [ ] A tenant absent from the directory response omits `tenant_name`; the row still appears.
- [ ] `ErrUnavailable` → `503`, and assert the raw body carries **no** `data` key.
- [ ] No `amount` or `dunning_state` key appears in any response — assert on the raw body.
- [ ] **The cross-endpoint invariant.** With one shared fixture set, `/admin/kpis`'s `trials_expiring` equals this endpoint's `pagination.total` at the default window. Drive both handlers in the same test rather than asserting each separately: the point is that the two agree, and two independent assertions against the same stub would pass even if the handlers read different sources. This is the assertion whose absence let #282 ship a structurally-zero counter.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... && go build ./...`

**Commit:** `feat(platformadmin): GET /admin/billing/trials (#285)`

---

## After the plan

- [ ] `go build ./...` clean in both services.
- [ ] Integration suites green: `marketplace-api/internal/billing/trial`, `platform-api/internal/tenant` (scoped), and the platformadmin unit suite.
- [ ] `grep` confirms `TrialExpiryHorizon` and `CountTrialsExpiring` are gone.
- [ ] Confirm the KPI's `trials_expiring` and the listing's `pagination.total` agree at the default window — verify against production after deploy, where a **non-zero** value is itself evidence the correction worked.
- [ ] Comment on #282 explaining that its counter was corrected here and why, so its "verified" comment is not left standing unqualified.
- [ ] Comment on #285 with the delivered shape, the `payment_method_on_file` substitution, and the missing-amount rationale.
