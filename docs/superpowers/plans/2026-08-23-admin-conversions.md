# Admin Conversions Implementation Plan (#279)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Answer, for one lead email, whether that lead has become a mark8ly tenant — so the platform CRM's conversion column stops resolving to `unknown` for every row.

**Architecture:** `platform-api` gains one internal endpoint that owns the lookup — an exact, case-insensitive match on `tenants.owner_email`, hitting the `tenants_owner_email_unique` index from migration 0014. `marketplace-api` calls it through the existing `tenantdirectory` client and reshapes the answer onto the `platformadmin` surface, which already carries signature verification, replay defence and the pinned envelope.

**Tech Stack:** Go 1.26, Gin 1.12, GORM, PostgreSQL 15, testify.

**Issue:** #279. Design approved in-session (bounded change; no separate spec file). The response-semantics table on the issue is the binding contract.

## The contract — this is the whole point of the endpoint

`404` **cannot** mean "not converted", because `404` is also what a framework returns for a route that does not exist. The two are indistinguishable on the wire, so the console can never trust `404` to carry a definite answer.

| case | marketplace-api response |
|---|---|
| owner email matches a tenant | `200 {"state":"converted","ref":"<bare tenant id>","label":"<tenant name>","observed_at":"<created_at, RFC3339 UTC>"}` |
| no tenant owns that email | `200 {"state":"none"}` — the **only** honest way to assert this |
| `email` missing or blank | `400 {"error":"validation_error","message":...}` — a caller bug, not an answer |
| upstream unreachable or 5xx | `503 {"error":"upstream_unavailable",...}` — never `{"state":"none"}` |

The `404` from platform-api is swallowed **in the marketplace-api handler** and converted to `200 {"state":"none"}`. That is the single most important line in this plan.

## Global Constraints

- **`observed_at` is the tenant's `created_at`** — when the conversion happened. Never `time.Now()`.
- Ids go out **bare** — no `mark8ly:` prefix. The platform API namespaces `<slug>:<id>` on arrival; prefixing here yields `mark8ly:mark8ly:...`.
- Never send a `source` field. The platform API stamps it and overwrites the body.
- Timestamps are RFC3339, UTC, with offset.
- **Conversion means owning a tenant.** `tenants.owner_email` only. A lead later invited as staff on someone else's tenant reads as `none`. Staff membership lives in OpenFGA, not queryably in Postgres, and only `owner_email` has a unique index behind it.
- **Project, do not pass through.** The marketplace-api response is built field by field. `owner_user_id` is a GIP UID and must never reach the console.
- This endpoint returns a single object, not a page. It has **no** `pagination` key. Do not add one.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## Verified facts — do not re-derive, do not doubt

- **Migration 0014** puts `CREATE UNIQUE INDEX tenants_owner_email_unique ON tenants (lower(owner_email))`. An email owns **at most one** tenant. There is no "which one wins" case to handle, and no `ErrMultiple` to invent.
- `repository.OwnerEmailExists` (`internal/tenant/repository.go:109`) already normalises with `strings.ToLower(strings.TrimSpace(email))` and queries `WHERE lower(owner_email) = ?`. **Copy that normalisation exactly** — a different one would miss the index or disagree with onboarding's duplicate check.
- **Gin 1.12 accepts `/internal/tenants/by-owner-email` as a static sibling of `:id`.** Verified in this session against the real two-group shape from `cmd/server/main.go` (strict directory group + permissive internal group, directory registered first): no panic at router build time, the static route wins over `:id`, and `/tenants/:id`, `/:id/detail`, `/:id/members` all still resolve to their own handlers under their own middleware. This is the one place trap #2 (the #287 gin panic) could have bitten; it does not.
- `RegisterDirectory` is already mounted in `cmd/server/main.go:348` behind `middleware.RequireInternalAuthStrict`. Adding a route inside it needs **no** `main.go` change.
- `platformadmin.Deps.TenantDirectory` is already wired at **two** sites in `cmd/marketplace-api/main.go` (~1917 and ~2003). Extending the existing `TenantDirectory` interface means the conversions handler reuses that dependency and needs **no** `main.go` change either. Do not add a third Deps field.
- `require.JSONEq` compares both directions, so a golden fixture asserted with it catches a field **rename** and a field **addition**, not only an omission. Prove it by mutation anyway (see Task 4).

## Two conventions that differ between the services

- **`platform-api` integration tests use the INTERNAL package** — `package tenant`, not `tenant_test`. See `internal/tenant/repository_integration_test.go`. They import `github.com/mark8ly/platform-api/pkg/testdb`.
- **`marketplace-api` tests use the EXTERNAL package** — `package platformadmin_test`, `package tenantdirectory_test`.

Both use `//go:build integration` and the `*_integration_test.go` suffix for DB-backed tests.

## Environment

- Local docker Postgres is running. **Use the LAN IP, not localhost** — a native Postgres squats on 127.0.0.1 on this machine, so `localhost` reaches the wrong server and reports `role "dev" does not exist`:
  - marketplace-api: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`
  - platform-api: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable'`
  The committed Makefile says `localhost`, which is correct everywhere else — **do not change it**.
- Integration runs need `-p 1`. These packages share one local Postgres and parallel package execution exhausts its connection limit (`FATAL: sorry, too many clients already`), which looks like data pollution and is not.
- **`platform-api` seeds its own reference data.** `stores` FKs to `countries`, `currencies`, `timezones`. `GB`/`GBP`/`Europe/London` are in the seed; `IE`/`Europe/Dublin` are not. Do not copy fixture values from marketplace-api. This task touches only `tenants`, so it should not need a store at all — if you find yourself inserting one, stop and ask whether the test needs it.
- Do NOT run `make dev` — the migrate containers are broken on this machine.
- Never run anything against a remote or GKE database. The only cluster that exists is production.

## File Structure

**`platform-api`**

| File | Responsibility |
|---|---|
| `internal/tenant/directory.go` (modify) | `GetByOwnerEmail` repository query |
| `internal/tenant/repository.go` (modify) | `GetByOwnerEmail` on the `Repository` interface |
| `internal/tenant/service.go` (modify) | `GetByOwnerEmail` service passthrough |
| `internal/tenant/directory_integration_test.go` (modify) | Exact-match, case/whitespace, substring-is-not-a-match, miss |
| `internal/tenant/handler.go` (modify) | `GET /tenants/by-owner-email` on `RegisterDirectory` |
| `internal/tenant/directory_handler_test.go` (modify) | Route wiring, missing-email, hit and miss shapes |

**`marketplace-api`**

| File | Responsibility |
|---|---|
| `internal/tenantdirectory/client.go` (modify) | `FindByOwnerEmail` |
| `internal/tenantdirectory/client_test.go` (modify) | Auth header, query encoding, 404→`ErrNotFound`, 5xx→`ErrUnavailable` |
| `internal/handlers/platformadmin/conversions.go` (create) | The handler and the wire shape |
| `internal/handlers/platformadmin/conversions_test.go` (create) | All four contract rows plus the golden fixture |
| `internal/handlers/platformadmin/testdata/conversions_converted.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/entities_tenants.go` (modify) | Extend the `TenantDirectory` interface |
| `internal/handlers/platformadmin/routes.go` (modify) | Mount the handler |

---

### Task 1: platform-api — the owner-email lookup query

**Files:** `internal/tenant/directory.go`, `internal/tenant/repository.go`, `internal/tenant/service.go`, `internal/tenant/directory_integration_test.go`

Add to the `Repository` interface in `repository.go`, beside `GetWithStores`:

```go
	// GetByOwnerEmail returns the tenant owned by the given email, or
	// apperrors.NotFound when no tenant is. Comparison is case-insensitive
	// and trims surrounding whitespace, matching OwnerEmailExists and the
	// tenants_owner_email_unique index (migration 0014) — which is also why
	// this returns at most one row rather than a slice.
	GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error)
```

Implement in `directory.go` (it belongs with the directory reads, not with the onboarding-facing queries in `repository.go`):

```go
func (r *gormRepository) GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error) {
	// Same normalisation as OwnerEmailExists: the unique index is on
	// lower(owner_email), so anything else silently misses it.
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return nil, apperrors.NotFound("tenant_not_found", "no tenant owns that email")
	}

	var t Tenant
	err := r.db.WithContext(ctx).
		Where("lower(owner_email) = ?", normalized).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("tenant_not_found", "no tenant owns that email")
	}
	if err != nil {
		return nil, fmt.Errorf("tenant: get by owner email: %w", err)
	}
	return &t, nil
}
```

Check the exact `apperrors.NotFound` signature and the error-code string used by the neighbouring `GetByID` before writing this — match it rather than inventing a new code.

Add the service passthrough in `service.go`, beside `GetWithStores`:

```go
// GetByOwnerEmail returns the tenant owned by the given email (#279).
func (s *Service) GetByOwnerEmail(ctx context.Context, email string) (*Tenant, error) {
	return s.repo.GetByOwnerEmail(ctx, email)
}
```

**Tests** (`directory_integration_test.go`, `package tenant`, `//go:build integration`). Write these first and watch them fail:

- [ ] Exact match returns the tenant: seed `founder@acme.example`, look it up, assert the id and name.
- [ ] **Case-insensitive:** seeding `founder@acme.example` and querying `Founder@ACME.example` returns the same tenant.
- [ ] **Whitespace-trimmed:** querying `"  founder@acme.example  "` returns the same tenant.
- [ ] **A substring is NOT a match:** seed `bob@acme.example`, query `ob@acme.example`, assert `apperrors` not-found. This is the regression guard against anyone later "simplifying" this into the directory's `ILIKE '%q%'` search.
- [ ] An unseeded email returns not-found, not an empty `Tenant` and not a nil error.
- [ ] Empty string and whitespace-only both return not-found without touching the DB.

Seed with the package's existing tenant helper if one exists; this test needs no store, no country and no currency.

**Verify:**

```
cd services/platform-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' go test -tags=integration -p 1 -run 'OwnerEmail' ./internal/tenant/...
```

**Commit:** `feat(tenant): look up a tenant by owner email (#279)`

---

### Task 2: platform-api — the internal endpoint

**Files:** `internal/tenant/handler.go`, `internal/tenant/directory_handler_test.go`

Add the route inside `RegisterDirectory`, next to the existing two:

```go
		// Static sibling of :id. Verified against gin 1.12 with main.go's
		// real two-group shape: no router-build panic, and /tenants/:id,
		// /:id/detail and /:id/members all still resolve to their own
		// handlers. Path chosen over a filter on the directory list because
		// that list matches with ILIKE '%q%' — a substring match would
		// report the wrong lead converted.
		t.GET("/by-owner-email", h.getTenantByOwnerEmail)
```

The handler:

```go
func (h *Handler) getTenantByOwnerEmail(c *gin.Context) {
	t, err := h.svc.GetByOwnerEmail(c.Request.Context(), c.Query("email"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}
```

A missing `email` reaches the repository as `""` and comes back not-found, so it answers `404` — which is correct on **this** hop. The `400` in the contract belongs to marketplace-api, which knows it is serving a console caller.

**Tests** (`directory_handler_test.go`, existing package and style):

- [ ] A hit returns `200` with `data.id` and `data.owner_email` matching the seeded tenant.
- [ ] A miss returns `404`.
- [ ] A missing `email` parameter returns `404`, not `500` and not a panic.
- [ ] The route is reachable at exactly `/internal/tenants/by-owner-email` when mounted the way `main.go` mounts it, and registering it does **not** break `/internal/tenants/:id/detail`. Assert both in the same test so a future route change cannot silently shadow one.

**Verify:** the package's unit tests, plus `go build ./...`.

**Commit:** `feat(tenant): internal by-owner-email lookup endpoint (#279)`

---

### Task 3: marketplace-api — the client method

**Files:** `internal/tenantdirectory/client.go`, `internal/tenantdirectory/client_test.go`

Add, following the shape of the existing `Get`:

```go
// FindByOwnerEmail returns the tenant owned by the given email.
//
// ErrNotFound means no tenant owns it — a definite answer, and the caller
// MUST turn it into a 200 "none" rather than propagating a 404. See #279:
// a 404 on the wire is indistinguishable from a route that does not exist,
// so it can never carry "not converted".
func (c *Client) FindByOwnerEmail(ctx context.Context, email string) (*Tenant, error) {
	var envelope struct {
		Data Tenant `json:"data"`
	}
	q := url.Values{}
	q.Set("email", email)
	if err := c.do(ctx, "/internal/tenants/by-owner-email?"+q.Encode(), &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}
```

**Tests** (`client_test.go`, `package tenantdirectory_test`):

- [ ] Sends `X-Internal-Auth` and requests path `/internal/tenants/by-owner-email` with `email` correctly URL-encoded — use an address containing `+` (`founder+tag@acme.example`) so a naive string concatenation fails this test.
- [ ] Parses the envelope into a `Tenant`, including `created_at`.
- [ ] Upstream `404` → `ErrNotFound`.
- [ ] Upstream `5xx` → `ErrUnavailable`.
- [ ] A closed server (transport failure) → `ErrUnavailable`.

**Verify:** `cd services/marketplace-api && go test ./internal/tenantdirectory/...`

**Commit:** `feat(tenantdirectory): find a tenant by owner email (#279)`

---

### Task 4: marketplace-api — the conversions endpoint

**Files:** `internal/handlers/platformadmin/conversions.go` (create), `conversions_test.go` (create), `testdata/conversions_converted.json` (create), `entities_tenants.go` (modify), `routes.go` (modify)

Extend the existing interface in `entities_tenants.go` — do **not** declare a second one, and do **not** add a new `Deps` field. The client already satisfies it, so both `main.go` wiring sites keep working untouched:

```go
	FindByOwnerEmail(ctx context.Context, email string) (*tenantdirectory.Tenant, error)
```

Any existing test stub implementing `TenantDirectory` needs the new method too.

`conversions.go` — the wire shape, projected field by field:

```go
// conversionResponse is the CRM's answer for one lead email.
//
// The zero-value response is {"state":"none"} — every other field is
// omitempty, so a miss cannot leak an empty ref the console would render.
type conversionResponse struct {
	State      string `json:"state"`
	Ref        string `json:"ref,omitempty"`
	Label      string `json:"label,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
}
```

Register `GET /admin/conversions`. The handler:

- [ ] Trims `email`; blank → `400 validation_error`.
- [ ] Calls `FindByOwnerEmail`.
- [ ] `ErrNotFound` → `200 {"state":"none"}`. **Comment this line** with why a 404 must not be propagated.
- [ ] `ErrUnavailable` → `503 upstream_unavailable`, logged. Never `{"state":"none"}` — "we could not ask" is not "not converted", and the console renders the two differently.
- [ ] Anything else → `500 internal_error`, matching `respondErr`'s existing reasoning about a wrong shared secret. Reuse `respondErr` for the 503/500 branches if it fits without contortion; if reusing it would force the 404-to-200 conversion into the shared function, write a local switch instead — the shared helper must keep mapping `ErrNotFound` to `404` for the tenants routes.
- [ ] Success → `converted`, with `Ref` the **bare** id, `Label` the tenant name, `ObservedAt` the tenant's `CreatedAt` as RFC3339 UTC.

Mount it in `routes.go` inside the existing `if deps.TenantDirectory != nil` block.

**Tests** (`conversions_test.go`, `package platformadmin_test`):

- [ ] Converted: golden fixture via `require.JSONEq` against `testdata/conversions_converted.json`.
- [ ] **Prove the fixture by mutation.** Temporarily rename a JSON tag (`ref` → `reference`) and confirm the test fails; temporarily add a field to the struct and confirm it fails again. Revert both. Record both results in your report — a fixture that only catches omissions is theatre.
- [ ] Miss: body is exactly `{"state":"none"}` — assert on the decoded raw JSON, so an extra `"ref":""` would fail. Assert the status is `200` and **explicitly assert it is not 404**.
- [ ] Missing `email`, and `email=` blank, and `email=%20%20`: all `400`.
- [ ] `ErrUnavailable` → `503`, and assert the body does **not** contain `"state"`. A 503 that also says `none` would be read as a definite answer by a caller that only checks the body.
- [ ] An unexpected error → `500`.
- [ ] The email reaches the client verbatim after trimming — assert the stub received exactly what you expect, so a future normalisation change here cannot silently diverge from platform-api's.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... && go build ./...`

**Commit:** `feat(platformadmin): GET /admin/conversions for the CRM (#279)`

---

## After the plan

- [ ] `go build ./...` clean in both services.
- [ ] Full unit suites green in both touched packages.
- [ ] Integration suite green in `platform-api/internal/tenant` with `-p 1`.
- [ ] Confirm no `main.go` in either service was modified — if one was, that is a signal the interface extension was done wrong.
- [ ] Comment on #279 with the delivered shape and the owner-email-only scoping decision, so the console side can object before it ships.
