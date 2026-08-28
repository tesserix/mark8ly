# Break-Glass Inventory (#333) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the emergency-account audit trail readable outside mark8ly. `break_glass_accounts` already records when an emergency credential was used; nothing outside this service can see it, so in practice the control on a deliberate auth bypass is rotation, not oversight. This adds the read — and only the read — that closes that gap.

**Architecture:** `GET /api/v1/platform/admin/break-glass` on the `platformadmin` surface. A new cross-tenant read in `internal/breakglass` (the existing `Repository` is tenant-scoped by construction and cannot serve this) feeds a handler that mirrors `email_sends.go` (#348D) exactly: same envelope, same `...Func` adapter wiring, same "excluded by construction" discipline on the wire struct. The route is additionally gated on an **exact** `rotate-credentials` capability via a new `RequiredReadCapabilities` map — the first read on this surface to require anything beyond a valid signature.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, testify. One service: `services/marketplace-api`.

**Issue:** https://github.com/tesserix/mark8ly/issues/333

---

## Global Constraints

- **No migration.** `000072` and `000073` already hold everything this reads. `ExpectedSchemaVersion` does not move. If you find yourself writing DDL, the plan is wrong.
- **Never load `breakglass.Account` for this endpoint.** That struct carries `SecretPath`, `PasswordHash` and `TOTPSecretRef`. The platform read uses its own row type and an **explicit column list** — never `SELECT *`, never a GORM `Find` into `Account`. A column added to the table tomorrow must not be able to reach the wire.
- **Integration tests gate on `TEST_DATABASE_URL`**, never `TEST_DB_DSN` — files using the latter skip silently (#317).
- **Integration tests run with `-p 1`** and `//go:build integration`; `go vet -tags=integration ./...` is the only command that compiles them, and is part of every task's verification.
- **`go test ./...` from the service root**, never path-scoped, or the root schema-version guard silently does not run. Use `-count=1`.
- Envelope is exactly `{"data": …, "pagination": …}`; timestamps RFC3339 UTC; ids bare strings; never a `source` field.
- Allocate slices with `make([]T, 0, n)` — a nil slice marshals to `null` and defeats the caller's `?? []`.
- marketplace-api test packages are **external** (`package platformadmin_test`).
- Commit messages: conventional, single line, no signature.

---

## File Structure

| file | responsibility |
|---|---|
| `internal/breakglass/platform_list.go` (create) | `PlatformListFilter`, `PlatformRow`, `PlatformListResult`, `ListPlatform` — the cross-tenant read |
| `internal/breakglass/platform_list_test.go` (create) | filter/sort semantics against sqlite where expressible; the struct-shape leak guard |
| `internal/breakglass/platform_list_integration_test.go` (create) | real Postgres: lockout join, NULLS LAST ordering, explicit column list |
| `internal/handlers/platformadmin/break_glass.go` (create) | `BreakGlassLister` iface + `...Func` adapter, handler, wire struct, filter parsing |
| `internal/handlers/platformadmin/break_glass_test.go` (create) | envelope, empty `200 []`, filter/sort plumbing, **marshalled-response leak test** |
| `internal/handlers/platformadmin/middleware.go` (modify) | `RequiredReadCapabilities` + the read branch of `RequirePlatformAuth` |
| `internal/handlers/platformadmin/middleware_test.go` (modify) | read-gate: match, mismatch, absent, undeclared-read-route-still-open |
| `internal/handlers/platformadmin/routes.go` (modify) | `Deps.BreakGlass`, nil-safe mount |
| `internal/handlers/platformadmin/routes_capability_coverage_test.go` (modify) | assert the break-glass GET is declared, and that no OTHER read route accidentally is |
| `cmd/marketplace-api/main.go` (modify) | wire `BreakGlassListerFunc(breakglass.ListPlatform)` at **both** Deps sites (≈2135, ≈2287) |

---

## Contract

`GET /api/v1/platform/admin/break-glass`

**Headers:** standard platform signature set, plus `X-Platform-Capability: rotate-credentials` (exact) and a non-empty `X-Platform-Operator`.

**Query:** `tenant_id`, `used_after`, `used_before` (RFC3339, filter on `last_used_at`), `used` (`true`/`false` — has the account ever been used), `locked` (`true`/`false`), `sort` (`last_used_at` | `-last_used_at`, default `-last_used_at`), `page`, `limit`. A malformed parameter takes the default and never errors, matching every other read on this surface.

**Row:**

```json
{
  "tenant_id": "…",
  "tenant_name": "Acme Ltd",
  "totp_enrolled": true,
  "last_used_at": "2026-08-27T09:14:00Z",
  "last_rotated_at": "2026-08-27T09:14:00Z",
  "rotation_scheduled_at": "2026-08-28T09:14:00Z",
  "locked_out": false,
  "lockout_expires_at": null,
  "created_at": "2026-04-18T00:00:00Z"
}
```

**Absent by construction:** `password_hash`, `secret_path`, `totp_secret_ref`.

---

## Two places the issue text and the schema disagree — resolved here

**1. Lockouts are per-IP, not per-account.** `break_glass_lockouts`' primary key is `(ip_hash, locked_until)`; `tenant_id` is nullable and is only populated when the failed login actually resolved a tenant (`internal/handlers/admin/break_glass_login.go:180-184`). `locked_out` therefore means **"at least one active lockout row currently names this tenant"**, not "this account is locked". Attempts that never resolved a tenant carry `tenant_id IS NULL` and are invisible to this view. Say exactly that in the struct comment; do not let the field name imply account-level lockout.

**2. `last_used_at` is nullable.** A never-used account sorts *last* under `-last_used_at`, not first. `ORDER BY last_used_at DESC NULLS LAST` — the surface exists to answer "which emergency accounts have been used recently", and floating never-used rows to the top defeats it. Assert this in the integration test; sqlite and Postgres disagree on default NULL ordering, so it must be explicit in the SQL rather than inherited.

---

## The read gate

Reads on this surface currently require **nothing** beyond a valid signature — `RequirePlatformAuth`'s operator/capability block is inside `if isWrite(...)`. This adds a per-route requirement for reads:

```go
// RequiredReadCapabilities declares reads that require a specific
// capability VALUE, not merely a valid signature.
//
// Unlike RequiredWriteCapabilities this map is NOT gated on
// CapabilityValueChecked and its values are NOT empty: it is opt-in per
// route, and a read route ABSENT from it keeps today's behaviour
// (signature only, no operator, no capability). That asymmetry is
// deliberate — flipping value enforcement on for every write at once is
// #364's problem and needs the console to send more than one value;
// requiring one named capability on one route does not.
var RequiredReadCapabilities = map[string]string{
    CapabilityKey(http.MethodGet, "/api/v1/platform/admin/break-glass"): "rotate-credentials",
}
```

Enforcement mirrors the write branch: declared route ⇒ operator required, capability required, exact string equality, `403 capability_insufficient` on mismatch. No lattice, no prefix match, no implication — per #275 mark8ly records the capability and refuses a mismatch; it never reasons about what one capability implies about another.

**What this pins on the console.** platform-api has no break-glass module today (`aiusage audit billing crm entities health inbox kpis tenants tickets tools`), so **no request is being refused by this that succeeds today**. But the audit module sets `X-Platform-Capability: platform` for its own calls, so when the break-glass module is built it must send `rotate-credentials` — matching what the console's own route config already declares for `platform.breakGlass`. Put that sentence in the code comment, so whoever builds the module finds the requirement from this file rather than from a 403.

---

## Tasks

### Task 1 — The leak guard, first and failing

- [ ] Write `TestPlatformRowCannotCarryCredentialFields` in `internal/breakglass/platform_list_test.go`: reflect over `breakglass.PlatformRow`'s fields and JSON tags, fail if any field name or tag matches `secret_path`, `password_hash`, `totp_secret_ref`, or case-insensitively contains `secret`, `hash` or `password`.
- [ ] Write the handler-side twin in `break_glass_test.go`: marshal a response built from a fully-populated `PlatformRow` and assert none of the three strings appears anywhere in the JSON bytes.
- [ ] Run both. They MUST fail to compile (no `PlatformRow` yet). Record that.

**Verify:** `go test ./internal/breakglass/... -count=1` fails for the stated reason, not another.

### Task 2 — `ListPlatform`

- [ ] `internal/breakglass/platform_list.go`: `DefaultPlatformPageSize = 50`, `MaxPlatformPageSize = 200` (mirror `emaillog`).
- [ ] `PlatformListFilter{TenantID *uuid.UUID; UsedAfter, UsedBefore *time.Time; Used, Locked *bool; SortDesc bool; Page, Limit int}`.
- [ ] `PlatformRow` with the contract fields above and nothing else.
- [ ] `ListPlatform(ctx, db, f, asOf)`: `db.Table("break_glass_accounts a")`, **explicit** `Select` naming only the six safe columns plus the two derived lockout columns. Active lockout via correlated subqueries against `break_glass_lockouts` filtered `tenant_id = a.tenant_id AND locked_until > ?asOf` — `EXISTS` for `locked_out`, `MAX(locked_until)` for `lockout_expires_at`.
- [ ] `Count` BEFORE `Select`/paging, so `Total` is the full match count.
- [ ] `ORDER BY last_used_at DESC NULLS LAST, tenant_id` (or `ASC NULLS LAST` when `SortDesc` is false). The tie-break on `tenant_id` makes paging deterministic across the many rows sharing a NULL `last_used_at`.
- [ ] Task 1's struct test now passes.

**Verify:** `go test ./internal/breakglass/... -count=1` and `go vet ./...`.

### Task 3 — Integration test for the read

- [ ] `platform_list_integration_test.go`, `//go:build integration`, gated on `TEST_DATABASE_URL`.
- [ ] Seed: three accounts (never used; used 2h ago; used 30d ago), one active lockout naming tenant B, one **expired** lockout naming tenant C, one active lockout with `tenant_id IS NULL`.
- [ ] Assert: default sort puts the 2h row first and the never-used row last; B is `locked_out` with the right `lockout_expires_at`; C is not (expired); the NULL-tenant lockout affects no row; `used_after` narrows correctly; `Total` is the unpaginated count.
- [ ] Assert the generated SQL names an explicit column list — capture it via a GORM session logger or `db.ToSQL`, and fail if it contains `*`, `secret_path`, `password_hash` or `totp_secret_ref`.

**Verify:** `go vet -tags=integration ./...` then `go test -tags=integration -p 1 -count=1 ./internal/breakglass/...`.

### Task 4 — Handler

- [ ] `BreakGlassLister` interface (one method) + `BreakGlassListerFunc` adapter, mirroring `EmailSendLister`.
- [ ] `BreakGlassHandler` with `db`, `repo`, `dir TenantDirectory`, `logger`, `now`.
- [ ] `Register` mounts `GET /admin/break-glass`.
- [ ] Wire struct populated **field by field** from `PlatformRow` — never embedded, never re-marshalled from the model.
- [ ] `tenant_name` enrichment via `lookupTenantNames` over the page's distinct tenant ids, `omitempty`, and a directory failure degrades to ids rather than failing the request — mirror `BillingSubscriptionsHandler.lookupTenantNames` exactly.
- [ ] `parseFilter` never errors; `effectiveBreakGlassLimit` reports the limit actually applied.
- [ ] Handler tests: envelope shape; empty result is `200` with `"data": []`; each filter reaches the repo stub; both sort values; unknown `sort` falls back to the default; the Task 1 marshal test now passes.

**Verify:** `go test ./internal/handlers/platformadmin/... -count=1`.

### Task 5 — Read gate

- [ ] Add `RequiredReadCapabilities` to `middleware.go` with the comment text above.
- [ ] Add `AuthConfig.RequiredReadCaps map[string]string` (nil ⇒ production map), matching the existing test-override pattern for writes.
- [ ] Add the read branch to `RequirePlatformAuth`: for a non-write method, look up `CapabilityKey(method, c.FullPath())`; if undeclared, proceed exactly as today; if declared, require non-empty operator, non-empty capability, and exact equality — `401 operator_required` / `401 capability_required` / `403 capability_insufficient`.
- [ ] Tests: declared route + correct capability ⇒ 200; declared + `platform` ⇒ 403; declared + absent ⇒ 401; **undeclared read route with no capability at all ⇒ still 200** (this is the regression that would break `/admin/audit-logs`, `/admin/health` and every shipped read, so it is the most important assertion in the task).
- [ ] Extend `routes_capability_coverage_test.go`: build the real router, assert `GET /api/v1/platform/admin/break-glass` is present in `RequiredReadCapabilities`, and assert no other mounted read route is — so a future read cannot pick up an unintended gate by accident.

**Verify:** `go test ./internal/handlers/platformadmin/... -count=1`.

### Task 6 — Wiring

- [ ] `Deps.BreakGlass BreakGlassLister` with the doc comment convention used by `EmailSends`; nil leaves the route unmounted.
- [ ] Mount in `Register` under `if deps.BreakGlass != nil`.
- [ ] Wire `platformadmin.BreakGlassListerFunc(breakglass.ListPlatform)` at **both** `platformadmin.Deps` sites in `main.go` (≈2135 and ≈2287). Missing the second is the standard failure here — grep for `EmailSends:` and match the count.
- [ ] Full-surface route test: the route exists, answers `200` with a valid signature and `rotate-credentials`, and `403`s on `platform`.

**Verify:** `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1` from the service root.

### Task 7 — Close out

- [ ] Re-read the issue's acceptance list and tick each item against a named test.
- [ ] Comment on #333 with what shipped, the two schema-vs-issue-text resolutions above, and the explicit statement that platform-api's break-glass module must send `rotate-credentials`.

---

## Out of scope — deliberately

- **`CapabilityValueChecked` / `RequiredWriteCapabilities` stay untouched.** Those gate the four POST routes. Per #364 a single global value is the wrong shape, and per the issue's own second comment no route anywhere in the estate requires a verb capability yet — making `purge` the first route gated on `hard-delete` deserves its own review, not a ride-along with a read.
- **`FEDERATION_MARK8LY_ENDPOINTS` is unset in tesserix-home**, so platform-api believes mark8ly implements no optional contract endpoints. This is why #284/#285 shipped and the console's Billing page still says nothing federates billing. Not a mark8ly change; file it against tesserix-home and reference it from #333 so the dependency is visible before this endpoint ships and shows nothing.
- **platform-api's break-glass module and the console's `pending: true` route.** Both tesserix-home side.
