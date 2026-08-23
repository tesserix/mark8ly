# Admin KPIs Implementation Plan (#282)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the platform console's product tile headline counters for mark8ly, and a way to say "not instrumented" that cannot be mistaken for zero.

**Architecture:** A key registry in `marketplace-api` drives both the `200` payload and the `501` responses. The handler composes three sources: a new `estatecounts` client to platform-api, the existing `onboardingfunnel` client from #283, and marketplace-api's own subscription repository.

**Tech Stack:** Go 1.26, Gin 1.12, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-23-admin-kpis-design.md` — the binding authority. Read its "The failure this endpoint exists to prevent" and "The key registry" sections before writing any code.

## Global Constraints

- **No key is ever omitted to mean "unavailable", and no unavailable value is ever rendered as `0`.** This is the entire point of the endpoint. The console has rendered em-dashes-that-look-like-zeroes for a year because an existing route falls through to `{}`.
- Three states must stay distinguishable: instrumented (`200` + value), known-but-uninstrumented (`501`), unknown key (`501`). The last two share a status deliberately; they differ only in message.
- Every `501` body carries a machine-readable `key` field so the console can mark one tile rather than the whole panel.
- **`onboarding_in_flight` reuses #283's funnel** — do not recompute it. One definition, one implementation.
- **`trials_expiring` horizon is a single exported constant.** #285 will reuse it. A second literal `7` anywhere is a defect.
- **Partial results are forbidden.** If any upstream fails, the whole endpoint returns `503`. Never return the subset that succeeded.
- Money keys are declared and uninstrumented. Do NOT implement GMV — there is no FX source and currency is per-store and per-order.
- Envelope is `{"data": {...}}`. Values are integers, never floats or strings.
- Ids bare, timestamps RFC3339 UTC, no `source` field.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## Conventions that differ, and will bite you

- **marketplace-api's `subscription.Repository` is STATELESS.** `NewRepository()` takes no arguments and every method takes `db *gorm.DB` as its first real parameter (see `internal/subscription/repository.go:82`). platform-api's repositories hold their own `db`. Writing `subscription.NewRepository(conn)` will not compile.
- **`platform-api` tests use the INTERNAL package** (`package tenant`, `package store`). **`marketplace-api` tests use EXTERNAL** (`package foo_test`). Both use `//go:build integration` for DB-backed tests.
- The subscription `Repository` interface carries an explicit warning that tenant-agnostic lookups must not be exposed through tenant-facing APIs. The new count in Task 3 **is** deliberately estate-wide. It is justified because the platformadmin surface is HMAC-gated and has no tenant context at all — but it must carry its own warning comment saying so, in the same style as `GetByStripeCustomerID`'s. Do not quietly add an unscoped query to that interface without the comment.

## Environment

- **Use the LAN IP, not localhost** — a native Postgres squats on 127.0.0.1:
  - platform: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable'`
  - marketplace: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`
  The committed Makefile says `localhost`, correct everywhere else — **do not change it**.
- Integration runs need `-p 1 -count=1`. The packages share one local Postgres; parallel execution exhausts its connection limit (`FATAL: sorry, too many clients already`) and looks like data pollution.
- **platform-api seeds its own reference data.** `stores` FKs to `countries`, `currencies`, `timezones`; `GB`/`GBP`/`Europe/London` are in the seed, `IE`/`Europe/Dublin` are not. Do not copy fixture values from marketplace-api.
- Do NOT run `make dev` — the migrate containers are broken here.
- Never run anything against a remote or GKE database. The only cluster that exists is production.
- Ignore `go.work requires go >= 1.26.6 (running go 1.26.5)` from vet/LSP — pre-existing drift.
- `TestIntegration_Complete_TenantAndOutboxCommitAtomically` in platform-api's onboarding package fails with a nil-storeRepo panic. Pre-existing and unrelated; scope runs with `-run` to avoid it.

## File Structure

**`platform-api`**

| File | Responsibility |
|---|---|
| `internal/estate/counts.go` (create) | `Counts` type and the two count queries |
| `internal/estate/handler.go` (create) | `GET /internal/estate/counts` |
| `internal/estate/counts_integration_test.go` (create) | Status filtering, empty estate |
| `internal/estate/handler_test.go` (create) | Envelope, error mapping |
| `cmd/server/main.go` (modify) | Mount on the strict group |

**`marketplace-api`**

| File | Responsibility |
|---|---|
| `internal/estatecounts/client.go` (create) | Client + `ErrUnavailable` |
| `internal/estatecounts/client_test.go` (create) | Parsing, 5xx, transport failure |
| `internal/subscription/repository.go` (modify) | `CountTrialsExpiring` + the horizon constant |
| `internal/subscription/repository_kpi_integration_test.go` (create) | Horizon boundary, status filtering |
| `internal/handlers/platformadmin/kpis.go` (create) | Registry, handler, wire shape |
| `internal/handlers/platformadmin/kpis_test.go` (create) | Registry-driven tests, 501s, 503 |
| `internal/handlers/platformadmin/testdata/kpis_response.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/routes.go` (modify) | Two new `Deps` fields, mount |
| `cmd/marketplace-api/main.go` (modify) | Wire at BOTH sites |

---

### Task 1: platform-api — estate counts

**Files:** `internal/estate/counts.go`, `internal/estate/handler.go`, `internal/estate/counts_integration_test.go`, `internal/estate/handler_test.go`, `cmd/server/main.go`

**Interfaces:**
- Produces: `estate.Counts{TenantsActive int64; StoresActive int64}`, `estate.NewRepository(db *gorm.DB) Repository` with `Get(ctx) (*Counts, error)`, `estate.NewHandler(repo Repository) *Handler` with `Register(g *gin.RouterGroup)` mounting `GET /estate/counts`.

A new small package rather than bolting onto `tenant` or `store`: it spans both, and neither owns it.

```go
// Package estate serves platform-wide counts for the Tesserix console's
// product tile (#282). It reads across tenants and stores, which is why it
// is its own package rather than living in either.
package estate

// Counts is the estate-wide headline count set.
type Counts struct {
	TenantsActive int64 `json:"tenants_active"`
	StoresActive  int64 `json:"stores_active"`
}
```

The repository runs two counts:

```go
func (r *gormRepository) Get(ctx context.Context) (*Counts, error) {
	var c Counts
	if err := r.db.WithContext(ctx).Table("tenants").
		Where("status = ?", tenant.StatusActive).
		Count(&c.TenantsActive).Error; err != nil {
		return nil, fmt.Errorf("estate: count active tenants: %w", err)
	}
	if err := r.db.WithContext(ctx).Table("stores").
		Where("status = ?", store.StatusActive).
		Count(&c.StoresActive).Error; err != nil {
		return nil, fmt.Errorf("estate: count active stores: %w", err)
	}
	return &c, nil
}
```

Use the existing `tenant.StatusActive` and `store.StatusActive` constants — do not write `"active"` as a literal. Check both exist and are exported before writing this (they are, at `internal/tenant/models.go` and `internal/store/models.go`).

Handler:

```go
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/estate/counts", h.get)
}

func (h *Handler) get(c *gin.Context) {
	counts, err := h.repo.Get(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": counts})
}
```

`respondError` is package-local here — check how `internal/tenant/handler.go` maps errors and mirror it rather than inventing a second convention. If that helper is not exported from a shared place, write a minimal local equivalent.

In `cmd/server/main.go`, mount on the **strict** group (the `strictInternal` variable that #283 renamed), alongside the tenant directory and onboarding analytics. Extend the comment above that group to mention estate counts. These are estate-wide reads: an unconfigured deploy must refuse, not serve them.

**Tests** (`counts_integration_test.go`, `package estate`, `//go:build integration`):

- [ ] Counts only `active` tenants: seed one `active` and one `suspended`, assert `TenantsActive == 1`.
- [ ] Counts only `active` stores: same shape.
- [ ] An empty estate returns zeros and **no error** — zero is a real answer here, distinct from the endpoint being unavailable.
- [ ] Tenants and stores are counted independently: a tenant with two stores yields `TenantsActive=1, StoresActive=2`.

Seed stores with `GB`/`GBP`/`Europe/London` — those are in platform-api's reference seed. `IE`/`Europe/Dublin` are not and will fail the FK.

**Tests** (`handler_test.go`): envelope is `{"data":{...}}` with both keys present; a repository error maps to the package's error convention, not a bare 500 string.

**Verify:** `cd services/platform-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' go test -tags=integration -p 1 -count=1 ./internal/estate/... && go build ./...`

**Commit:** `feat(estate): platform-wide tenant and store counts (#282)`

---

### Task 2: marketplace-api — the estate counts client

**Files:** `internal/estatecounts/client.go`, `internal/estatecounts/client_test.go`

**Interfaces:**
- Consumes: platform-api `GET /internal/estate/counts` → `{"data":{"tenants_active":N,"stores_active":N}}`.
- Produces: `estatecounts.NewClient(baseURL, secret string, httpClient *http.Client) *Client` with `Get(ctx) (*Counts, error)`; `estatecounts.Counts{TenantsActive, StoresActive int64}`; `estatecounts.ErrUnavailable`.

Model this on `internal/tenantdirectory/client.go` — read it first. Same `do` helper shape, same `maxBody` cap, same `X-Internal-Auth` header, same `ErrUnavailable` doc-comment reasoning.

No `ErrNotFound`: this endpoint does not 404. Do not add a sentinel for an impossible case.

**Tests** (`package estatecounts_test`):

- [ ] Sends `X-Internal-Auth` and requests exactly `/internal/estate/counts` — assert on `r.URL.Path`, not merely that a request arrived.
- [ ] Parses the envelope into `Counts`.
- [ ] Upstream 5xx → `ErrUnavailable`.
- [ ] Transport failure (closed server) → `ErrUnavailable`.
- [ ] A `200` with `{"data":{}}` yields zeros and no error — an empty estate is a real answer.

**Verify:** `cd services/marketplace-api && go test ./internal/estatecounts/...`

**Commit:** `feat(estatecounts): client for platform-wide estate counts (#282)`

---

### Task 3: marketplace-api — the expiring-trials count

**Files:** `internal/subscription/repository.go`, `internal/subscription/repository_kpi_integration_test.go`

**Interfaces:**
- Produces: `subscription.TrialExpiryHorizon` (a `time.Duration` constant) and `Repository.CountTrialsExpiring(ctx context.Context, db *gorm.DB, asOf time.Time) (int64, error)`.

Add the constant near the status constants:

```go
// TrialExpiryHorizon is how far ahead a trial counts as "expiring" for the
// platform console's KPI tile (#282).
//
// SHARED, deliberately: #285 (GET /admin/billing/trials) needs the same
// notion of "expiring". If it declares its own, the console shows two
// different numbers for the same word on two screens. Reuse this constant.
const TrialExpiryHorizon = 7 * 24 * time.Hour
```

Add to the `Repository` interface, with a warning comment in the same style as `GetByStripeCustomerID`'s:

```go
	// CountTrialsExpiring counts trialing subscriptions whose current period
	// ends within TrialExpiryHorizon of asOf.
	//
	// ESTATE-WIDE, deliberately unscoped by tenant: this serves the platform
	// console's KPI tile, which is HMAC-gated on the platformadmin surface and
	// has no tenant context at all. DO NOT call it from any tenant-facing
	// handler — those must stay tenant-scoped like GetByStoreID.
	//
	// asOf is a parameter rather than now() so a test can pin the window
	// boundary exactly; production passes time.Now().
	CountTrialsExpiring(ctx context.Context, db *gorm.DB, asOf time.Time) (int64, error)
```

Implementation — note the receiver style is `(gormRepository)`, matching its neighbours, and the repository is **stateless**, so `db` comes from the parameter:

```go
func (gormRepository) CountTrialsExpiring(ctx context.Context, db *gorm.DB, asOf time.Time) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&StoreSubscription{}).
		Where("status = ?", StatusTrialing).
		Where("current_period_end IS NOT NULL").
		Where("current_period_end > ?", asOf).
		Where("current_period_end <= ?", asOf.Add(TrialExpiryHorizon)).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("subscription: count trials expiring: %w", err)
	}
	return n, nil
}
```

The window is half-open on the left (`> asOf`) so an **already-expired** trial does not count as "expiring", and inclusive on the right so a trial ending exactly at the horizon does. State that in the doc comment; a reviewer will otherwise have to guess which side is which.

**Tests** (`repository_kpi_integration_test.go`, `package subscription_test`, `//go:build integration`). Seed relative to a fixed `asOf`:

- [ ] A trial ending in 3 days counts.
- [ ] A trial ending in 10 days does **not** count (outside the horizon).
- [ ] A trial ending exactly at `asOf + TrialExpiryHorizon` **does** count (inclusive right edge).
- [ ] A trial that already ended (`current_period_end` before `asOf`) does **not** count.
- [ ] A subscription with `status = 'active'` ending in 3 days does **not** count — only `trialing`.
- [ ] A trialing subscription with `current_period_end IS NULL` does not count and does not error.
- [ ] Counts across tenants: two trialing subscriptions under different `tenant_id`s both count. This is the estate-wide behaviour, asserted rather than assumed.

**Verify:** `cd services/marketplace-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go test -tags=integration -p 1 -count=1 ./internal/subscription/...`

**Commit:** `feat(subscription): count expiring trials for the KPI tile (#282)`

---

### Task 4: marketplace-api — the registry and the endpoint

**Files:** `internal/handlers/platformadmin/kpis.go`, `kpis_test.go`, `testdata/kpis_response.json`, `routes.go`, `cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `estatecounts.Client.Get`, `onboardingfunnel.Client.GetFunnel` (existing, from #283 — its `FunnelStats.InFlight` is the value), `subscription.Repository.CountTrialsExpiring`.

The registry is the heart of this task:

```go
// kpiKey is one metric the console may ask for.
type kpiKey struct {
	Name         string
	Instrumented bool
}

// kpiRegistry declares EVERY key mark8ly knows. Both the 200 payload and the
// 501 responses are driven from here rather than from conditionals in the
// handler, so a key cannot be silently dropped.
//
// An uninstrumented key is NOT omitted and NOT zero — the console has
// rendered em-dashes that look like zeroes for a year because a sibling
// route falls through to {}. See the spec.
var kpiRegistry = []kpiKey{
	{Name: "tenants_active", Instrumented: true},
	{Name: "stores_active", Instrumented: true},
	{Name: "onboarding_in_flight", Instrumented: true},
	{Name: "trials_expiring", Instrumented: true},
	// GMV is genuinely not computable: currency is per store and per order,
	// and there is no FX source anywhere in the workspace. 501 is the honest
	// answer; 0 would be a lie and omitting it would be ambiguous.
	{Name: "gmv_today", Instrumented: false},
	{Name: "gmv_month", Instrumented: false},
}
```

Handler behaviour:

- [ ] Parse `?keys=` as a comma-separated list, trimming each. Empty or absent means "all instrumented keys".
- [ ] For each requested key, resolve it against the registry and respond `501` immediately — do not accumulate partial results first. The two cases share the status and the `key` field but differ in `message`, because the console cannot act differently on them while a human reading logs can:
  - **in the registry, `Instrumented: false`** → `{"error":"not_implemented","message":"kpi \"<key>\" is known but not instrumented","key":"<key>"}`
  - **not in the registry at all** → `{"error":"not_implemented","message":"kpi \"<key>\" is not a recognised key","key":"<key>"}`
- [ ] Otherwise gather values from the three sources.
- [ ] **Any** upstream error → `503 upstream_unavailable`. Never a partial object. `ErrUnavailable` from either client, and any repository error, all take this path.
- [ ] Respond `200 {"data": {...}}` with integer values.

Add **two** fields to `platformadmin.Deps` (`EstateCounts` and `Subscriptions`) plus reuse of the existing `OnboardingFunnel` field from #283. Mount only when all three are non-nil, mirroring the existing nil-guard pattern.

Wire in `cmd/marketplace-api/main.go` at **BOTH** sites (~1917 and ~2003), guarded by `cfg.PlatformAPIURL != ""` exactly as the existing clients are. There are two because production runs two deployments from this binary (`marketplace-api-admin` and `marketplace-api-storefront`); missing one makes the endpoint 404 depending on which served the request.

**Tests** (`package platformadmin_test`):

- [ ] **Registry-driven completeness:** iterate `kpiRegistry`; every instrumented key appears in the `200` payload, and every uninstrumented key returns `501` when requested by name. Written as a loop over the registry, so adding a key without handling it fails the test.
- [ ] An unknown key (`?keys=not_a_real_kpi`) returns `501`, the body's `key` field names it, and the message says **not a recognised key**.
- [ ] A known-uninstrumented key (`?keys=gmv_today`) returns `501`, names it, and the message says **known but not instrumented**. Assert the two messages differ — they are the only thing distinguishing the cases.
- [ ] `?keys=tenants_active,gmv_today` returns `501` — **not** a partial `200` with just `tenants_active`. Assert the body has no `data` key at all.
- [ ] **Anti-drift:** `onboarding_in_flight` equals the `InFlight` the funnel stub reports. Use one stub value and assert both, so a future handler that recomputes rather than reuses fails.
- [ ] An `ErrUnavailable` from the estate client → `503`, and assert the raw body contains **none** of the counter key names. A partial object is the failure mode this asserts against.
- [ ] The same for an `ErrUnavailable` from the funnel client, and for a subscription repository error.
- [ ] Every value in the `200` payload decodes as an integer — assert on raw JSON so a float `4.0` or a string `"4"` fails.
- [ ] Golden fixture via `require.JSONEq` against `testdata/kpis_response.json`. **Prove it by mutation:** rename a JSON key and confirm failure; add a field to the response struct and confirm failure. Revert both and report both results.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... ./internal/estatecounts/... && go build ./...`

**Commit:** `feat(platformadmin): GET /admin/kpis with an explicit not-instrumented contract (#282)`

---

## After the plan

- [ ] `go build ./...` clean in both services.
- [ ] Full unit suites green in every touched package.
- [ ] Integration suites green in `platform-api/internal/estate` and `marketplace-api/internal/subscription` with `-p 1`.
- [ ] Confirm `TrialExpiryHorizon` is the only place `7` appears for this purpose — `grep` for a stray literal.
- [ ] Confirm no code path can return a partial KPI object: every error branch ends in `501` or `503`.
- [ ] Comment on #282 with the delivered keys, the `501` contract, and the reason GMV ships uninstrumented — so the console can wire its registry against the real shape.
- [ ] Note on #285 that `TrialExpiryHorizon` exists and must be reused rather than redeclared.
