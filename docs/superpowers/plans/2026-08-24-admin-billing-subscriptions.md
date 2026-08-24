# Admin Billing Subscriptions Implementation Plan (#284, #328)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A cross-tenant view of every subscription on the platform, with plan, status and price — and, in the same change, add the price to `/admin/billing/trials`, which shipped without one on a mistaken conclusion.

**Architecture:** One money resolver, shared by both billing endpoints, reading `internal/billing/pricing`'s catalog. A cross-tenant list query in `internal/subscription`. Tenant names via the `ids` batch filter built in #285.

**Tech Stack:** Go 1.26, Gin 1.12, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-24-admin-billing-subscriptions-design.md` — the binding authority. Read its "The correction this folds in" and "The money resolver" sections before writing any code.

## Global Constraints

- **Never call `pricing.MustGet`.** It **panics** on a miss (`panic("pricing: no amount for plan=…")`). A console read must not panic on a plan/currency combination the catalog lacks. Use `pricing.DevelopedCurrencyOptions` then `pricing.LookupPPPOption` — the pair `MustGet` wraps, minus the panic.
- **Omit, never fake.** No `billing_currency`, or no catalog entry, means **no `amount` key** — not `null`, not `0`, not a guessed currency.
- **Catalog keys are lowercase ISO 4217** (`"gbp"`); `billing_currency` is `char(3)`; the wire contract is **uppercase** (`"GBP"`). Both conversions explicit.
- **All eight `SubscriptionStatus` values** are valid filters and returned values: `signup`, `trialing`, `active`, `past_due`, `payment_action_required`, `cancel_scheduled`, `expired`, `store_closed`. Do not truncate to the four the issue names.
- An unrecognised `status` or `plan` filter is **`400 validation_error`** naming the parameter — an empty list must mean "none match", never "I did not understand you".
- Empty result is `200` with `[]`, **never `501`**. mark8ly has a billing concept.
- `TaxBehavior` is **not** carried on the wire (decided in the spec).
- Ids bare, timestamps RFC3339 UTC, no `source` field. `pagination.limit` reports the **effective** clamped value.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## Two facts that shape the work

**1. The catalog has no price for `trial` or `marketplace` plans.** `internal/billing/pricing/catalog.go` says so explicitly: *"PlanTrial and PlanMarketplace have no Price objects — excluded."*

Subscriptions reach `plan='trial'` by one path (`internal/subscription/service.go:124`) and a merchant-chosen `starter`/`studio`/`pro` by another (`internal/billing/trial/subscribe.go:140`). So a row with `plan='trial'` correctly carries **no amount** — there is no price for it, and that is by design rather than a gap.

This makes #328's value **partial and honest**: `/admin/billing/trials` gains an amount for rows on a real plan, and correctly omits one for `plan='trial'` rows. Production's current golden fixture uses `plan: "trial"`, so its rows will keep carrying no amount. Do not "fix" that by inventing a trial price.

**2. `store_subscriptions` is empty in production.** Verified read-only against the replica: 4 tenants and 4 stores exist, but subscriptions require an explicit merchant-initiated call with a Stripe customer, and none has been made. The new endpoint will correctly serve `[]` until the first merchant subscribes — so it will not be exercised by production data on day one, and the tests are the only evidence its row-shaping is right.

## Conventions that will bite you

- **`subscription.Repository` is STATELESS.** `NewRepository()` takes no arguments; every method takes `db *gorm.DB` as a parameter; receivers are value receivers on `gormRepository`. `NewRepository(conn)` will not compile.
- That interface **warns against tenant-agnostic lookups** (see `GetByStripeCustomerID`). The new cross-tenant list is deliberately unscoped and must carry its own warning comment in the same style, as `CountExpiring` does.
- Tests: `marketplace-api` uses the EXTERNAL package (`package foo_test`); `//go:build integration` for DB-backed ones.
- **`internal/subscription` has pre-existing `store_subscriptions_store_id_fkey` failures** tracked as **#317**. Scope runs with `-run` and do not try to fix them. Seed a `stores` row before a `store_subscriptions` row, as `internal/billing/trial`'s tests do.

## Environment

- **Use the LAN IP, not localhost**: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, with `-p 1 -count=1`. The committed Makefile says `localhost`, correct everywhere else — do not change it.
- Never run anything against a remote or GKE database. Do not run `make dev`.
- Ignore `go.work requires go >= 1.26.6 (running go 1.26.5)` — pre-existing drift.

## File Structure

| File | Responsibility |
|---|---|
| `internal/handlers/platformadmin/money.go` (create) | The shared resolver and the `money` wire type |
| `internal/handlers/platformadmin/money_test.go` (create) | Tier selection, omission rules, case handling |
| `internal/subscription/repository.go` (modify) | `ListAllSubscriptions` cross-tenant query |
| `internal/subscription/repository_crosstenant_integration_test.go` (create) | Filters, ordering, pagination |
| `internal/handlers/platformadmin/billing_subscriptions.go` (create) | Handler and wire shape |
| `internal/handlers/platformadmin/billing_subscriptions_test.go` (create) | Contract, filters, golden |
| `internal/handlers/platformadmin/testdata/billing_subscriptions_response.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/billing_trials.go` (modify) | Add `amount` via the shared resolver (#328) |
| `internal/handlers/platformadmin/billing_trials_test.go` (modify) | Amount cases + cross-endpoint agreement |
| `internal/handlers/platformadmin/testdata/billing_trials_response.json` (modify only if values change) | |
| `internal/handlers/platformadmin/routes.go` (modify) | New `Deps` field, mount |
| `cmd/marketplace-api/main.go` (modify) | Wire at BOTH sites |

---

### Task 1: the shared money resolver

**Files:** `internal/handlers/platformadmin/money.go` (create), `money_test.go` (create)

**Interfaces:**
- Produces: `type money struct { Amount int64 \`json:"amount"\`; Currency string \`json:"currency"\` }` and `func resolveMoney(plan, period string, billingCurrency *string, tier subscription.PriceTier) (money, bool)`.

Read `internal/billing/pricing/catalog.go` first — particularly `MustGet`, so you can see exactly what you must not call and why.

```go
// resolveMoney returns the catalog price for a subscription's plan, period
// and currency, or ok=false when no price can be determined.
//
// ok=false is a normal outcome, not an error:
//   - billing_currency is NULL (the merchant never chose one), or
//   - the plan has no price at all — the catalog excludes `trial` and
//     `marketplace` by design ("no Price objects"), or
//   - the catalog has no entry for that plan/period/currency combination.
//
// Callers OMIT the amount key entirely in that case. Never null, never 0,
// never a guessed currency: a number the system cannot determine must not
// be reported as a number.
//
// Deliberately does NOT use pricing.MustGet, which panics on a miss. A
// console read must not panic on an unpriced combination. This calls the
// same two lookups MustGet wraps, minus the panic.
func resolveMoney(plan, period string, billingCurrency *string, tier subscription.PriceTier) (money, bool) {
	if billingCurrency == nil || strings.TrimSpace(*billingCurrency) == "" {
		return money{}, false
	}
	// Catalog keys are lowercase ISO 4217; the column is char(3) of
	// unspecified case; the wire contract is uppercase.
	cur := strings.ToLower(strings.TrimSpace(*billingCurrency))

	p, per := pricing.Plan(plan), pricing.Period(period)

	if tier == subscription.PriceTierPPP {
		if amt, ok := pricing.LookupPPPOption(p, per, cur); ok {
			return money{Amount: amt.UnitAmountMinor, Currency: strings.ToUpper(amt.Currency)}, true
		}
		return money{}, false
	}

	if opts, ok := pricing.DevelopedCurrencyOptions(p, per); ok {
		if amt, present := opts[cur]; present {
			return money{Amount: amt.UnitAmountMinor, Currency: strings.ToUpper(amt.Currency)}, true
		}
	}
	return money{}, false
}
```

Check the exact `subscription.PriceTier` constant names before writing this (`PriceTierDeveloped`, `PriceTierPPP` at `internal/subscription/models.go:59-60`) and the `pricing` function signatures — do not trust the snippet over the source.

**Tests** (`package platformadmin`, internal — this is an unexported function; check whether the package's existing tests are external and, if so, put these in a `package platformadmin` file alongside, noting it in your report):

- [ ] A developed-tier row (`plan=starter`, `period=monthly`, `currency=gbp`, `tier=developed`) resolves to `{1500, "GBP"}`. **Assert the uppercase currency** — a lowercase `"gbp"` on the wire is a contract break.
- [ ] A PPP-tier row resolves from the PPP table, not the developed one. Pick a plan/currency present in `pppAmounts` and assert the value differs from the developed price for the same plan, so a tier mix-up is visible.
- [ ] **`plan="trial"` resolves `ok=false`** — the catalog has no trial price by design.
- [ ] `plan="marketplace"` resolves `ok=false`.
- [ ] A nil `billingCurrency` resolves `ok=false`.
- [ ] An empty-string and whitespace-only `billingCurrency` both resolve `ok=false`.
- [ ] An **uppercase** input currency (`"GBP"`) resolves the same as lowercase — proving the lookup lowercases rather than missing.
- [ ] An unknown currency (`"zzz"`) resolves `ok=false` and **does not panic**. Assert non-panic explicitly with `require.NotPanics`.
- [ ] An unknown plan (`"nonexistent"`) resolves `ok=false` and does not panic.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/...`

**Commit:** `feat(platformadmin): shared catalog-backed money resolver (#284)`

---

### Task 2: the cross-tenant subscription query

**Files:** `internal/subscription/repository.go`, `internal/subscription/repository_crosstenant_integration_test.go` (create)

**Interfaces:**
- Produces: `Repository.ListAllSubscriptions(ctx context.Context, db *gorm.DB, f CrossTenantFilter) ([]StoreSubscription, int64, error)` and `type CrossTenantFilter struct { Status, Plan string; Page, Limit int }`.

Add to the `Repository` interface with a warning comment in the same style as `GetByStripeCustomerID`'s:

```go
	// ListAllSubscriptions returns a page of subscriptions across EVERY
	// tenant, plus the unpaginated total.
	//
	// ESTATE-WIDE, deliberately unscoped by tenant: this serves the platform
	// console's cross-tenant billing view, which is HMAC-gated on the
	// platformadmin surface and has no tenant context at all. DO NOT call it
	// from any tenant-facing handler — those must stay tenant-scoped like
	// GetByStoreID.
	ListAllSubscriptions(ctx context.Context, db *gorm.DB, f CrossTenantFilter) ([]StoreSubscription, int64, error)
```

Implementation notes:
- Value receiver `(gormRepository)`, `db` from the parameter — the repository is stateless.
- Build the filter once and use it for **both** the count and the page query, so the two cannot drift. `applyDirectoryFilter` in `platform-api/internal/tenant/directory.go` is the established shape.
- An empty `Status` or `Plan` adds **no** clause. Guard each with `if f.Status != ""`, not a bare `Where`.
- Order `created_at DESC`.
- Default limit 50, clamp 500.
- Allocate with `make([]StoreSubscription, 0, limit)` before `Find`.

**Tests** (`package subscription_test`, `//go:build integration`). Seed a `stores` row before each `store_subscriptions` row — the FK is enforced (#317):

- [ ] Returns subscriptions across **two different tenants** — asserted explicitly, since this is the estate-wide behaviour.
- [ ] `Status` filter narrows correctly; an empty `Status` returns everything (the guard).
- [ ] `Plan` filter narrows correctly; an empty `Plan` returns everything.
- [ ] Both filters combine (AND), rather than one replacing the other.
- [ ] Ordering is `created_at DESC` — seed three rows in a different order from their timestamps.
- [ ] Pagination: `Limit: 1` returns one row with `total` equal to the full match count.
- [ ] `Limit: 9999` clamps to 500 and reports the clamped value through the caller's page size.
- [ ] An empty table returns an **allocated empty slice** and `total=0`, not nil and not an error.

**Verify:** `cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go test -tags=integration -p 1 -count=1 -run CrossTenant ./internal/subscription/...`

**Commit:** `feat(subscription): cross-tenant subscription listing for the platform console (#284)`

---

### Task 3: the subscriptions endpoint

**Files:** `internal/handlers/platformadmin/billing_subscriptions.go` (create), `billing_subscriptions_test.go` (create), `testdata/billing_subscriptions_response.json` (create), `routes.go`, `cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `resolveMoney` (Task 1), `ListAllSubscriptions` + `CrossTenantFilter` (Task 2), `tenantdirectory` `List` with `IDs` (built in #285).

Wire shape exactly as the spec's JSON block:

```json
{ "tenant_id": "<bare>", "tenant_name": "Acme Trading", "store_id": "<bare>",
  "plan": "studio", "period": "monthly", "status": "active",
  "amount": {"amount": 1500, "currency": "GBP"},
  "current_period_end": "2026-09-24T00:00:00Z", "cancel_at_period_end": false }
```

- [ ] Validate `status` against the **eight** `subscription.Status*` constants; validate `plan` against the five `subscription.Plan*` constants. An unrecognised value is `400 validation_error` naming the parameter.
- [ ] `amount` **omitted** when `resolveMoney` returns `ok=false`. Use `*money` with `omitempty`, or build the response with a map — whichever your golden test can assert cleanly. Never `null`.
- [ ] `current_period_end` omitted when NULL.
- [ ] One batch `tenantdirectory.List` call per page with **distinct** ids; a tenant missing from the response omits `tenant_name` and the row still appears.
- [ ] `tenantdirectory.ErrUnavailable` → `503 upstream_unavailable`; anything else → `500 internal_error`. Never a partial result.
- [ ] Mount in `routes.go` behind a nil guard on both the new dependency and `TenantDirectory`. Wire at **BOTH** `main.go` sites — production runs two deployments from this binary.

**Tests** (`package platformadmin_test`):

- [ ] Golden fixture via `require.JSONEq`, **proven by mutation**: rename a JSON key and confirm failure; add a field to the response struct and confirm failure. Revert both; report both results.
- [ ] A row whose plan has no catalog price (`plan="trial"`) has **no `amount` key at all** — assert on the raw body, not a decoded struct.
- [ ] A row with a priced plan carries `amount` with an **uppercase** currency.
- [ ] Empty result is `200` with `data` exactly `[]`.
- [ ] Each of the **eight** statuses is accepted as a filter — table-driven over the constants, so a new status added to `models.go` without handler support fails the test.
- [ ] `status=nonsense` and `plan=nonsense` each return `400` naming the parameter, not an empty `200`.
- [ ] One deduplicated tenant lookup per page; no lookup for an empty page.
- [ ] `ErrUnavailable` → `503` with no `data` key on the raw body.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... && go build ./...`

**Commit:** `feat(platformadmin): GET /admin/billing/subscriptions cross-tenant (#284)`

---

### Task 4: add the amount to `/admin/billing/trials` (#328)

**Files:** `internal/handlers/platformadmin/billing_trials.go`, `billing_trials_test.go`, `testdata/billing_trials_response.json` (only if values change)

**Interfaces:**
- Consumes: `resolveMoney` (Task 1).

`/admin/billing/trials` shipped with no `amount` because a previous conclusion held that mark8ly has no prices. It does — see the spec's correction section. This task closes that.

- [ ] `trial.ExpiringRow` already carries `Plan`, `Period` and `BillingCurrency`. It does **not** carry `PriceTier`. Check whether it needs to: `resolveMoney` takes a tier, and a PPP-tier trial would otherwise resolve against the developed table. If the tier is absent from `ExpiringRow`, add it to the row struct and the query's SELECT in `internal/billing/trial/expiring.go`, and extend that package's mapping test to assert it — the same test that pins the other five fields. **Report which you found.**
- [ ] Add `amount` to the trial row wire shape via `resolveMoney`, omitted on `ok=false`.
- [ ] Update the golden fixture **only if its values change**. Its current rows use `plan: "trial"`, which has no catalog price, so they should keep carrying no amount. If the fixture changes, say why in your report — an unexpected change means the resolver is pricing something it should not.

**Tests:**

- [ ] A trial row on a **priced** plan (`starter`/`studio`/`pro`) carries `amount` with an uppercase currency.
- [ ] A trial row with `plan="trial"` carries **no `amount` key** — assert on the raw body.
- [ ] **Cross-endpoint agreement:** the same plan/period/currency resolves to the same amount on `/admin/billing/trials` and `/admin/billing/subscriptions`. **Drive both handlers in one test over one fixture** — two separate assertions against the same catalog would pass even if the handlers resolved differently, which is the failure mode that let a structurally-zero counter ship in this codebase.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... ./internal/billing/trial/... && go build ./...` plus the trial integration suite if `expiring.go` changed.

**Commit:** `feat(platformadmin): carry the catalog amount on billing trials rows (#328)`

---

## After the plan

- [ ] `go build ./...` clean.
- [ ] Unit suites green; the cross-tenant and trial integration suites green with `-p 1`.
- [ ] `grep` confirms `pricing.MustGet` is not called anywhere under `internal/handlers/`.
- [ ] Confirm both `main.go` sites are wired.
- [ ] Comment on #284 with the delivered shape and the confirmed no-projection answer.
- [ ] Comment on #328 recording that the amount is now carried, and that `plan="trial"` rows correctly have none.
