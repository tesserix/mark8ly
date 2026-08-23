# Onboarding Funnel Implementation Plan (#283)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose mark8ly's onboarding funnel to the Tesserix platform console — estate-wide counters plus the session rows behind them, agreeing with each other by construction.

**Architecture:** `platform-api` owns both queries, including one shared SQL predicate for "abandoned" that the counters and the rows both use. `marketplace-api` calls them through a new `onboardingfunnel` client and reshapes onto the `platformadmin` surface.

**Tech Stack:** Go 1.26, Gin 1.12, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-23-onboarding-funnel-design.md` — the binding authority. Read its "Definitions" and "The finding that shapes this design" sections before writing any query.

## The finding you must not re-derive

`StatusAbandoned` and `StatusExpired` exist as constants and in migration 0004's CHECK constraint, and the package doc claims sessions transition to them. **Nothing writes either value.** There is no gc. Every production session is `in_progress`, `verifying` or `completed`.

So **`abandoned` is derived from idle time, never read from `status`**. Filed as #322. Do not "fix" this by querying `status = 'abandoned'` — that returns zero forever and looks like a data problem.

## Global Constraints

- **Abandoned** = not completed AND `last_activity_at <= now() - 24 hours`. **In flight** = not completed AND `last_activity_at > now() - 24 hours`. Exactly 24h idle is **abandoned**.
- `24 * time.Hour` lives in ONE exported named constant. No literal `24` in a query.
- **One shared SQL predicate** for abandoned, used by both the funnel aggregate and the sessions list — the way `applyDirectoryFilter` is shared by the directory's count and page queries. Two copies is how the two endpoints come to disagree.
- **Partition invariant:** `completed + in_flight + abandoned == started`, exactly, for any window. Assert it in a test.
- `email_verified` is a **subset counter that cuts across** the partition, not a stage. Do not include it in the sum.
- `median_completion_seconds` is `null` with zero completions. Never `0`.
- `last_24h` is always `now()-24h … now()` and **ignores the window**.
- The funnel returns a single object with **no `pagination` key**. `sessions` uses the standard envelope; empty is `200` with `[]`, allocated via `make([]T, 0, n)`.
- `pagination.limit` reports the **effective** (clamped) limit. Clamp at 500. A missing parameter takes the default and never errors.
- Ids **bare**. Timestamps RFC3339 UTC with offset. Never send a `source` field.
- **`draft` must never reach the console.** It is a JSONB blob of merchant-entered wizard data. Project field by field and assert its absence in a test.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## Two conventions that differ between the services

- **`platform-api` tests use the INTERNAL package** — `package onboarding`, not `onboarding_test`. See `internal/onboarding/completion_integration_test.go`.
- **`marketplace-api` tests use the EXTERNAL package** — `package platformadmin_test`, `package onboardingfunnel_test`.

Both use `//go:build integration` and the `*_integration_test.go` suffix for DB-backed tests.

## Environment

- **Use the LAN IP, not localhost** — a native Postgres squats on 127.0.0.1 on this machine, so `localhost` reports `role "dev" does not exist`:
  - platform: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable'`
  - marketplace: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`
  The committed Makefile says `localhost`, correct everywhere else — **do not change it**.
- Integration runs need `-p 1`. The packages share one local Postgres; parallel execution exhausts its connection limit (`FATAL: sorry, too many clients already`) and presents as data pollution. It is not.
- Do NOT run `make dev` — the migrate containers are broken here.
- Never run anything against a remote or GKE database. The only cluster that exists is production.
- Ignore `go.work requires go >= 1.26.6 (running go 1.26.5)` from vet/LSP — pre-existing drift.

## Time-dependent tests: seed relative to `now()`

Every one of these queries compares against `now()`. Seed fixtures as offsets from the current time (`now - 25h`, `now - 23h`, `now - 24h`), never as absolute timestamps — an absolute fixture passes today and fails tomorrow. Insert `last_activity_at` explicitly rather than letting the column default fire.

## File Structure

**`platform-api`**

| File | Responsibility |
|---|---|
| `internal/onboarding/funnel.go` (create) | The constant, the shared predicate, `FunnelStats`, `SessionRow`, both queries |
| `internal/onboarding/repository.go` (modify) | Two methods on the `Repository` interface |
| `internal/onboarding/service.go` (modify) | Two passthroughs |
| `internal/onboarding/funnel_integration_test.go` (create) | Boundary, partition, median, window, cross-endpoint agreement |
| `internal/onboarding/handler.go` (modify) | `RegisterAnalytics(g *gin.RouterGroup)` plus two handlers |
| `internal/onboarding/funnel_handler_test.go` (create) | Query parsing, clamping, envelope, null median |
| `cmd/server/main.go` (modify) | Mount on the strict group |

**`marketplace-api`**

| File | Responsibility |
|---|---|
| `internal/onboardingfunnel/client.go` (create) | Client, `ErrUnavailable`, both calls |
| `internal/onboardingfunnel/client_test.go` (create) | Envelope parsing, null median, 5xx, transport failure |
| `internal/handlers/platformadmin/onboarding.go` (create) | Both handlers and the wire shapes |
| `internal/handlers/platformadmin/onboarding_test.go` (create) | Golden fixtures, draft-absence, empty-is-array |
| `internal/handlers/platformadmin/testdata/onboarding_funnel.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/testdata/onboarding_sessions.json` (create) | Golden fixture |
| `internal/handlers/platformadmin/routes.go` (modify) | New `Deps` field, mount both |
| `cmd/marketplace-api/main.go` (modify) | Wire the client at BOTH sites |

---

### Task 1: platform-api — the queries

**Files:** `internal/onboarding/funnel.go` (create), `repository.go`, `service.go`, `funnel_integration_test.go` (create)

Create `funnel.go` with:

```go
// AbandonedAfter is how long a non-completed session may sit idle before the
// funnel counts it abandoned.
//
// Derived, not stored: StatusAbandoned exists as a constant and in migration
// 0004's CHECK constraint, but nothing ever writes it and there is no gc
// (#322). Querying status = 'abandoned' returns zero forever.
//
// 24h because onboarding is normally one sitting; the only legitimate long
// pause is waiting on the verification email, which is minutes to hours.
const AbandonedAfter = 24 * time.Hour
```

The shared predicate — **one definition, used by both queries**:

```go
// abandonedExpr is the SQL for "not completed and idle past the cutoff".
// Shared by the funnel aggregate and the sessions list so the two cannot
// disagree about which sessions are abandoned. Exactly AbandonedAfter idle
// counts as abandoned, hence <=.
func abandonedExpr() string {
	return "(onboarding_sessions.status <> 'completed' AND onboarding_sessions.last_activity_at <= now() - INTERVAL '24 hours')"
}
```

Derive the interval from `AbandonedAfter` rather than hardcoding `'24 hours'` twice — e.g. build it with `fmt.Sprintf("%d hours", int(AbandonedAfter.Hours()))` as a bound parameter, or use `now() - make_interval(hours => ?)`. Pick one and make the constant genuinely load-bearing: a test must be able to prove that changing `AbandonedAfter` changes the query's behaviour.

`FunnelStats` and `SessionRow` types, plus a `FunnelFilter` carrying `CreatedFrom`, `CreatedTo`, `Status`, `Abandoned *bool`, `Page`, `Limit`.

**`GetFunnel`** — ONE query with `FILTER (WHERE …)` aggregates. Five separate counts could observe five different database states and break the partition for reasons unrelated to the data:

```sql
SELECT
  COUNT(*)                                        AS started,
  COUNT(*) FILTER (WHERE email_verified_at IS NOT NULL) AS email_verified,
  COUNT(*) FILTER (WHERE status = 'completed')    AS completed,
  COUNT(*) FILTER (WHERE <abandoned>)             AS abandoned,
  COUNT(*) FILTER (WHERE status <> 'completed' AND NOT (<abandoned>)) AS in_flight,
  percentile_cont(0.5) WITH IN GROUP (ORDER BY EXTRACT(EPOCH FROM completed_at - created_at))
    FILTER (WHERE status = 'completed')           AS median_completion_seconds
FROM onboarding_sessions WHERE <window>
```

Note the exact syntax is `percentile_cont(0.5) WITHIN GROUP (ORDER BY …)` — check it compiles rather than copying the line above verbatim.

Scan `median_completion_seconds` into a `*float64` so SQL `NULL` survives as Go `nil`. Scanning into `float64` silently turns "nothing completed" into `0`.

`last_24h` is a second, tiny query over `created_at > now() - INTERVAL '24 hours'` — **not** filtered by the window.

**`ListSessions`** — page + unpaginated total, sharing the same filter builder, ordered `created_at DESC`. Allocate before `Find`. Clamp limit at 500, default 50 (mirror `DefaultDirectoryPageSize`/`MaxDirectoryPageSize` in the tenant package).

**Tests** (`funnel_integration_test.go`, `package onboarding`, `//go:build integration`). Seed relative to `now()`:

- [ ] **Boundary:** sessions at `now-23h` (in flight), `now-25h` (abandoned) and **exactly `now-24h` (abandoned)**. This pins the rule rather than leaving it to whichever operator was typed.
- [ ] **Partition:** over a mixed fixture, `completed + in_flight + abandoned == started`, exactly.
- [ ] **Cross-endpoint agreement:** the funnel's `abandoned` count equals the number of rows `ListSessions` flags abandoned over the same window. This is what proves the predicate is genuinely shared.
- [ ] **Median, odd count:** three completions of 10s, 20s, 60s → 20.
- [ ] **Median, even count:** two completions of 10s and 20s → 15. The even case is where a wrong percentile implementation shows up.
- [ ] **Median, zero completions:** `nil`, not `0`.
- [ ] **Window:** a session created outside `[from, to]` appears in neither the counters nor the rows.
- [ ] **`last_24h` ignores the window:** with a window entirely in the past, `last_24h.started` still counts a session created minutes ago.
- [ ] **A completed session is never abandoned**, however old and idle.

**Verify:** `cd services/platform-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' go test -tags=integration -p 1 -count=1 ./internal/onboarding/...`

**Commit:** `feat(onboarding): funnel counters and session rows (#283)`

---

### Task 2: platform-api — the internal endpoints

**Files:** `internal/onboarding/handler.go`, `internal/onboarding/funnel_handler_test.go` (create), `cmd/server/main.go`

Add `RegisterAnalytics(g *gin.RouterGroup)` to the onboarding `Handler`, mounting `GET /onboarding/funnel` and `GET /onboarding/sessions`. Keep it **separate from `Register`**, whose routes are public wizard routes on `/api/v1` — mirroring how `tenant.RegisterDirectory` is separate from `tenant.Register`, and for the same reason.

In `cmd/server/main.go`, mount it on the **strict** group (the one already built at line ~347 for the tenant directory). Both endpoints return estate-wide data, so an unconfigured deploy must refuse rather than serve the lot.

**Rename that group's variable** from `tenantDirectory` to something like `strictInternal`, since it now carries two features. Update the one existing use. Keep the explanatory comment above it and extend it to say the onboarding analytics routes share the guard for the same reason.

Unlike #279, **this task does change `main.go`** — the onboarding handler has no existing mount on the strict group. That is expected here.

**Tests** (`funnel_handler_test.go`):

- [ ] Funnel returns `200` with the counters under `data`, and **no `pagination` key**.
- [ ] `median_completion_seconds` serialises as JSON `null` when nil — assert on raw JSON, not a decoded struct, since decoding makes `null` and `0` hard to tell apart.
- [ ] Sessions returns the standard envelope; empty is `[]`, not `null`.
- [ ] `limit=9999` clamps to 500 and `pagination.limit` reports **500**.
- [ ] A missing parameter takes the default and does not error.
- [ ] Both routes are reachable under the strict group's prefix and do not collide with the existing `/onboarding/sessions/:id` public route. **Assert both in one test** — `/onboarding/sessions` (static, analytics) and `/onboarding/sessions/:id` (public wizard) live at the same path position, and a future edit could shadow one. This is the #287 class of bug.

**Verify:** package tests plus `go build ./...`.

**Commit:** `feat(onboarding): internal funnel and sessions endpoints (#283)`

---

### Task 3: marketplace-api — the client

**Files:** `internal/onboardingfunnel/client.go` (create), `client_test.go` (create)

A new package modelled on `internal/tenantdirectory` — same `do` helper shape, same `ErrUnavailable` sentinel and reasoning. Separate package on purpose: a funnel read is a different concern from a directory read and the two will diverge.

No `ErrNotFound` is needed: neither endpoint 404s.

`FunnelStats` must carry `MedianCompletionSeconds *float64` so upstream `null` survives as `nil` rather than becoming `0`.

**Tests:**

- [ ] Sends `X-Internal-Auth`; requests the right paths.
- [ ] Parses the funnel envelope including nested `last_24h` and `window`.
- [ ] **`"median_completion_seconds": null` parses to `nil`, not `0`.**
- [ ] Parses the sessions envelope; `"data": []` yields an allocated empty slice, and `"data": null` also yields an allocated empty slice.
- [ ] Upstream 5xx → `ErrUnavailable`. Transport failure → `ErrUnavailable`.
- [ ] Window parameters are URL-encoded correctly (RFC3339 contains `+` in offsets — use an offset timestamp so a naive concatenation fails this test).

**Verify:** `cd services/marketplace-api && go test ./internal/onboardingfunnel/...`

**Commit:** `feat(onboardingfunnel): client for the platform onboarding funnel (#283)`

---

### Task 4: marketplace-api — the platformadmin endpoints

**Files:** `internal/handlers/platformadmin/onboarding.go` (create), `onboarding_test.go` (create), two fixtures under `testdata/`, `routes.go`, `cmd/marketplace-api/main.go`

Declare an `OnboardingFunnel` interface in `onboarding.go` (the subset the handler needs), add an `OnboardingFunnel` field to `platformadmin.Deps`, and mount both routes when it is non-nil — mirroring the existing `TenantDirectory` treatment.

Unlike #279, **this task does add a `Deps` field and does change `main.go`**, because the client is a new type. Wire it at **both** sites (~1917 and ~2003), guarded by `cfg.PlatformAPIURL != ""` exactly as `tenantDirectoryClient` is. Missing one site means the endpoint works in one server mode and 404s in the other.

Wire shapes exactly as the spec's two JSON blocks. `median_completion_seconds` is `*float64` with **no `omitempty`** — the console must receive an explicit `null`, not a missing key.

Session rows are projected field by field. **`draft` must not appear.**

Errors: `ErrUnavailable` → `503 upstream_unavailable`; anything else → `500 internal_error`. Never an empty funnel on failure.

**Tests:**

- [ ] Golden fixture per endpoint, each **proven by mutation**: rename a JSON tag and confirm the test fails; add a field to the response struct and confirm it fails again. Revert both and report both results.
- [ ] **`draft` absent** from a session row even when the stub supplies a populated `draft`. Assert on the raw JSON body, not a decoded struct.
- [ ] Empty sessions is `200` with `[]` — assert the raw `data` is exactly `[]`.
- [ ] `median_completion_seconds` serialises as `null` when nil, and is **present** (not omitted).
- [ ] Upstream unavailable → `503` on both endpoints, and the body carries no counters.
- [ ] `pagination.limit` reflects what the client reported, not what was requested.

**Verify:** `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... ./internal/onboardingfunnel/... && go build ./...`

**Commit:** `feat(platformadmin): onboarding funnel and sessions (#283)`

---

## After the plan

- [ ] `go build ./...` clean in both services.
- [ ] Full unit suites green in every touched package.
- [ ] Integration suite green in `platform-api/internal/onboarding` with `-p 1`.
- [ ] Confirm the `AbandonedAfter` constant is load-bearing: changing it changes query behaviour, proven by a test rather than by inspection.
- [ ] Comment on #283 with the delivered shape, the 24h cutoff, and the derived-not-stored explanation, linking #322.
