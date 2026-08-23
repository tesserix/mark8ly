# Takeover prompt — Platform integration v1

Paste everything below into a new session.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, a multi-service
Go + Next.js workspace).

## What this milestone is

The Tesserix platform console (`console.tesserix.app`) is moving off **direct
cross-database reads** of mark8ly's data onto mark8ly's own HTTP surface. Umbrella
issue: **#260**. Each endpoint is its own issue.

**Delivered and live in production:** #274 (front door), #275 (auth), #276
(`/admin/audit-logs`), #277 (`/admin/entities/tenants`).

**Open in the milestone:** #278–#290, plus #319 (OpenBao credentials, a different
concern grouped in).

## Read these first

- `docs/superpowers/specs/2026-08-22-platform-admin-surface-design.md` — the foundation
- `docs/superpowers/specs/2026-08-23-tenant-directory-design.md` — the most recent endpoint
- The umbrella comment on **#260** — what every new endpoint inherits
- `services/marketplace-api/internal/handlers/platformadmin/routes.go` — the doc comment there explains the mount decision

**Treat specs as hypotheses, not authority.** Across two specs in this milestone,
nine separate claims turned out to be contradicted by the code — a CHECK constraint,
a prune cron, a nonce sweep, a lock claim, a gateway policy, a route path, a
registration group, a query strategy, and a test that was claimed to exist and did
not. Every one was caught by someone reading the code instead of the document.
Verify before you build on a spec sentence.

## Where the surface lives

**Base URL: `https://api.mark8ly.com/api/v1/platform`** — so contract path
`/admin/foo` resolves to `/api/v1/platform/admin/foo`.

New endpoints go in `services/marketplace-api/internal/handlers/platformadmin/`,
mounted on the group in `routes.go`. Routing needs **no infra change** —
`tesserix-k8s` has one prefix rule for `/api/v1/platform/` covering the whole series.

Mount a handler on that group and you inherit: HMAC signature verification, replay
defence (±300s window + single-use Postgres nonce), the enforcement matrix (writes
require operator identity *and* capability; reads do not), fail-closed behaviour
(`503 not_configured` when the secret is unset), and operator attribution into audit
rows.

## Conventions — follow, do not re-derive

- Envelope is exactly `{"data": [...], "pagination": {"page","limit","total"}}`
- Empty results are `200` + `[]`. Allocate with `make([]T, 0, n)` — a nil slice
  marshals to `{}` and defeats the caller's `?? []`
- `pagination.limit` reports the **effective** (clamped) limit, so `total / limit`
  is a correct page count
- Oversized `limit` clamps (500); a missing parameter takes the default, never errors
- Money: integer minor units + explicit currency. Never a bare number
- Timestamps RFC3339 UTC with offset
- Ids **bare** — the platform API namespaces `<slug>:<id>` on arrival
- Never send `source` — the platform API stamps it and overwrites the body
- **Add a golden fixture per endpoint** and prove by mutation that it catches a field
  *rename* AND a field *addition*. A fixture that only catches omissions is theatre
- **Project, do not pass through.** Map upstream structs field by field.
  platform-api's tenant record carries `owner_user_id` (a GIP UID); it never reaches
  the console because `tenantRow` is a projection. A passthrough leaks every field
  added upstream, silently

## Five traps that each cost real time

1. **`/api/v1/admin/*` is JWT-gated at the mesh.** An Istio `AuthorizationPolicy`
   (`require-customer-auth`, namespace `istio-ingress`, repo `tesserix-k8s`) denies
   any request without a JWT to a path list including `/api/v1/admin/*`,
   `/api/v1/staff/*`, `/api/v1/analytics/*`, `/api/v1/reports/*`. This surface
   authenticates by HMAC, not JWT. That is why it lives under `/api/v1/platform/`.
   Invisible locally and in CI. Documented in `docs/architecture.md`.
2. **#287 can panic the service at startup.** `/admin/tenants/{id}/suspend` collides
   with the merchant `/admin/tenants/:tenantId/sso` — two different wildcard names in
   one path position makes gin panic at *router build time*. Safe only while the new
   route is registered on the `platformadmin` group, never the merchant one.
3. **A missing tenant drops the audit event.** Nothing on this surface sets
   `tenant_id` on the gin context. Use
   `platformadmin.EmitOperatorAction(c, emitter, tenantID, ev)` — the tenant is a
   required parameter so it cannot be forgotten. Calling `audit.Emit` directly on a
   write path will silently write no row (#310 built the helper; the trap remains for
   anyone not using it).
4. **Two services, two sets of reference data.** `platform-api`'s `stores` FKs to
   `countries`, `currencies`, `timezones` and its seed ships a specific set —
   `GB`/`GBP`/`Europe/London` are safe, `IE`/`Europe/Dublin` are not. Copying fixture
   values from marketplace-api produces tests that pass only on the machine where
   someone hand-inserted the rows.
5. **`-p 1` on integration runs.** The packages share one local Postgres; parallel
   execution exhausts its connection limit (`FATAL: sorry, too many clients already`)
   and presents as data pollution. It is not.

## Environment

- **Use the LAN IP, not `localhost`** — a native Postgres squats on 127.0.0.1:
  - marketplace: `postgres://dev:dev@192.168.1.110:5432/marketplace_db`
  - platform: `postgres://dev:dev@192.168.1.110:5432/platform_api`
  The committed Makefile says `localhost`, correct everywhere else — do not change it.
- `make dev` is broken (migrate containers fail with `exec: "up": executable file not found`).
  Apply migrations with `cd services/<svc> && DATABASE_URL=... go run ./cmd/migrate up`.
- **Deployment is Kargo-gated**, not direct ArgoCD: CI → ghcr → Kargo Warehouse →
  Freight → Promotion to the `prod` stage → ArgoCD sync. Expect ~10–20 min, and check
  `kubectl get stages,promotions -n kargo-mark8ly` rather than assuming a stall.
- Only ONE GKE cluster exists and it is **production** (`tesseract-prod-in-gke`).
  Never run tests against it.
- Test package conventions differ by service: `platform-api` integration tests use the
  **internal** package (`package tenant`), marketplace-api uses **external**
  (`package foo_test`). Both use `//go:build integration`.

## Process that worked

For each endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development` (fresh
subagent per task, review between, final whole-branch review).

The reviews earn their cost when they **mutate rather than read**: rewrite the query
as a JOIN and confirm the dedup test fails; remove a guard and confirm its test fails;
rename a JSON tag and confirm the golden test fails. A review that only reads the diff
has repeatedly missed things a two-minute mutation caught.

Two failure modes seen more than once:
- A reviewer finding a real problem and **rating it Minor because "the plan mandated
  it"**. A plan is not authority. Rule on those yourself.
- A test passing **for the wrong reason** — asserting a stub's behaviour, or decoding
  JSON so `[]` and `null` become indistinguishable. Check what the assertion actually
  proves.

## Blocked — do not start these

- **#278** (user directory) — its constraint "no customer rows under any filter" is
  unenforceable: `user_profiles` has no role/type/origin, and the staff/customer split
  lives in GIP tenant pools, not Postgres. Needs a console-side decision. Full analysis
  on the issue.
- **#313** — `metadata` object-vs-string for `/admin/audit-logs`. Field ships omitted
  until answered.
- **#290** — blocked on the console publishing `@tesserix/admin-conformance`.
- **#280** as written — its stated rationale (the SEA manual-review queue) is void:
  **SEA countries are not supported and the integrations are untested**, so that queue
  is inert. If you build the inbox, rebuild its justification from what is actually
  live: erasure requests (#259, accumulating with a statutory clock), the migration
  fast-path (#281, handler exists but was never mounted), and arbitrage appeals.

## Suggested order

Reads before writes; **#288 (purge) last** — it is irreversible.

Good next candidates: **#283** (onboarding funnel — needs the same `tenantdirectory`
client shape, scoped so extending is additive) or **#279** (conversions — smallest,
and its `404`-means-nothing semantics are already specified precisely on the issue).

**#284** carries an open design question the issue itself flags: subscriptions are
per-store and plans are Go descriptors rather than DB rows, so a cross-tenant view may
need a projection. Say so on the issue if it does, rather than guessing.

## Known follow-ups outside the milestone

#311 (store-less audit rows unprunable — decided: never pruned, needs the guard made
deliberate), #312 (gateway policy documentation), #316/#317 (integration fixture drift
across 17+ packages), #318 (`audit.NewEmitter` accepts a nil `Repo` and panics a
worker goroutine).

---

Start by reading #260's latest comment, then pick an endpoint and run the
brainstorming → spec → plan → subagent-driven flow. Ask me before merging anything.
