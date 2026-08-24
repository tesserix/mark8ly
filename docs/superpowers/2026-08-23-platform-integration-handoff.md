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
(`/admin/audit-logs`), #277 (`/admin/entities/tenants`), #279 (`/admin/conversions`),
#282 (`/admin/kpis`), #283 (`/admin/onboarding/funnel` + `/sessions`), #284
(`/admin/billing/subscriptions`), #285 (`/admin/billing/trials`), #289
(`/admin/health`), **#287** (`/admin/tenants/{id}/suspend` + `/unsuspend`, #340),
**#329** (`/admin/tickets`, #346), and **#281 part (b) only** — the CSM migration
fast-path review route, mounted and audited (#338); #281 stays OPEN for part (a).

**The map changed on 2026-08-24 — this doc's "everything remaining is a write" is
no longer true.** Five issues joined the milestone: #329 (`/admin/tickets`), #330
(Otto cross-tenant live chat — a decision, not an endpoint), #331 (`/admin/outbox`),
#332 (`/admin/notifications`), #333 (`/admin/break-glass`). Four of those are reads.

**Effort ordering, re-derived 2026-08-24 (session 3 close)** — reads-before-writes no
longer sorts this queue:

**#332 → #333 → #286 → #288 (purge, last) → #319.**

#287 and #329 are now DELIVERED. **#333 is blocked on a decision, not on work:** its
acceptance requires gating on the operator holding the `rotate-credentials` **capability**,
but this surface checks capability **presence only, never the value**
(`internal/handlers/platformadmin/middleware.go:153`). #287 deliberately did not invent a
value vocabulary — the console asserts these names and a guess would refuse every real
request. Settle the console's capability names first, or ship #333 with the gate
explicitly deferred and say so. **#332 has no blocker and is the right next task.**

**#286 is NOT the small one its acceptance criteria suggest.** There is **no stored
trial-end column anywhere** — searched every migration and every Go site. Trial end is
*derived* as `created_at + TrialDays` in five places: `handlers/admin/subscription.go:197`
(what the merchant sees), `billing/trial/subscribe.go:131` (**the value sent to Stripe
as `trial_end`**), `subscription/dunning/trial_reminders.go:108`, `billing/trial/expiring.go`,
and `planchange.go:224` (plus #326's hardcoded 90 in the same role). "Extend a trial" is
not representable today: it needs a new column AND all five consumers taught to respect
it. Miss the Stripe path and the console quotes one date while Stripe bills another.

**#331 is blocked by #336.** It defines `failed` as `published_at IS NULL AND error IS
NOT NULL`, but nothing in the service ever writes `outbox_events.error` — so that filter
can never match. #336 also records the worse half: `Publisher.Tick` appends every row id
to `ids` *before* its two validation checks and then marks all of `ids` published, so an
event with an unparseable payload or missing `store_id` is dropped without its watermark
being bumped **and recorded as successfully published**. Invisible to any monitor.

**Open in the milestone:** #281 (part (a) only), #286, #288, plus the three remaining new
reads #329/#331/#332/#333 and the #330 decision, plus the blocked #278/#280/#290, plus
#319 (OpenBao credentials, a different concern grouped in).

The remaining **writes** are #286, #287, #288 and #281(a) — see trap 3, and trap 2
before touching #287's routing. They are no longer the only work left: see the four
new reads above.

**Reusable pieces the next endpoint inherits**, beyond the surface itself:

| package | what it gives you |
|---|---|
| `marketplace-api/internal/tenantdirectory` | tenant list/detail/by-owner-email, and an `IDs` filter for batch lookup |
| `marketplace-api/internal/onboardingfunnel` | funnel counters + session rows |
| `marketplace-api/internal/estatecounts` | active tenant/store counts |
| `marketplace-api/internal/billing/trial` | `TrialDays`, `DefaultExpiryWindow`, `CountExpiring`, `ListExpiring` — owns "what a trial is and when it ends" |
| `marketplace-api/internal/billing/pricing` | the price catalog, in **minor units**, by plan/period/currency with developed and PPP tiers |
| `platformadmin.resolveMoney` | the shared money resolver both billing endpoints use |
| `subscription.AllStatuses()` | all ten statuses, verified against the DB CHECK constraint |
| `platform-api` `strictInternal` group | the fail-closed internal mount (`cmd/server/main.go`) |

All three clients share one shape (a `do` helper, a `maxBody` cap, `X-Internal-Auth`,
and an `ErrUnavailable` that must never be conflated with an empty result). Copy the
nearest one rather than inventing a fourth.

**Money is available locally.** `pricing.MustGet` **panics** on a miss — never call it
from a handler. Use `resolveMoney`, which omits the amount rather than faking one.

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

## Nine traps that each cost real time

> Trap 9 was added at the close of session 3, and traps 4, 6, 7 and 8 all bit again during
> it — including in prose I wrote myself while restating the rule they encode.

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
4. **Two services, two sets of reference data — and the trap runs BOTH ways.**
   `platform-api`'s `stores` FKs to `countries`, `currencies`, `timezones` and its
   seed ships a specific set — `GB`/`GBP`/`Europe/London` are safe,
   `IE`/`Europe/Dublin` are not. Copying fixture values from marketplace-api
   produces tests that pass only on the machine where someone hand-inserted the rows.

   **`marketplace-api`'s `stores` has NO such FKs.** It is a *local projection* of
   platform-api's (`migrations/000001_products_initial.up.sql:13-26`): plain
   `country_code char(2)`, `currency_code char(3)`, `timezone varchar(64)`, whose only
   constraints are `stores_slug_unique` and `stores_status_valid`. Any string inserts.

   Always say WHICH SERVICE. Read the unqualified sentence above as applying to
   marketplace-api and you will "fix" a constraint that does not exist — that happened
   on 2026-08-24, and the false claim reached a committed code comment before a
   reviewer read the migration and caught it. Naming the service is the whole trap.
5. **`-p 1` on integration runs.** The packages share one local Postgres; parallel
   execution exhausts its connection limit (`FATAL: sorry, too many clients already`)
   and presents as data pollution. It is not.
6. **The fixture that sits *beside* the property instead of *on* it.** This one hit
   **seven times across three branches** and was never once caught by reading a diff —
   only by mutation. The shape is always the same: the assertion names a property, but
   the test data contains no case where a wrong implementation would give a different
   answer.

   | what was claimed | the fixture | why it proved nothing |
   |---|---|---|
   | exactly 24h idle is abandoned | Go's `now` vs Postgres `now()` | sub-second gap, inside tolerance |
   | `idle_hours` shares `abandoned`'s instant | `asOf = time.Now()` | shared and independent clocks coincide |
   | `last_24h` is pinned to `asOf` | a 6h-past `asOf` | still inside the 24h window measured |
   | the two abandoned predicates are shared | idles of 5h/40h/30h | nothing in the 24–25h band the drift moves |
   | the trial window is half-open left | `asOf - 1h` | excluded under both `>` and `>=` |
   | the two 501 messages differ | two *different* keys | interpolation made them differ regardless |
   | active counts exclude non-active | only `active` + `suspended` seeded | a third status, `archived`, was never tested |

   Two rules that would have caught all seven:
   - **A test must fail if the property it names is deleted.** If the assertion would
     still pass with the behaviour removed, it is testing something else. Check by
     removing it, not by reading it.
   - **Put the fixture on the exact value where the candidate implementations
     disagree** — the boundary instant itself, every value of an enum the filter
     discriminates on, the same key in both messages. "Close to the edge" is not the
     edge, and an offset that looks historical can still sit inside the window being
     measured (6 hours did; 10 days was needed).

   Corollary seen twice: **asserting presence when the value is what matters.** A
   payload assembled by map lookup returns the zero value for a missing key, so a
   test that checks a key exists passes on a fabricated `0`. Give each stub a
   distinct non-zero value and assert the values.

7. **A claim is not a guarantee — including your own.** Trap 6's cousin, and the
   costliest thing in this milestone so far. **Eleven instances**, and the three
   worst were *conclusions about the code*, not weak tests:

   | claim | reality |
   |---|---|
   | "`current_period_end` is the trial-end column" (#282) | The rule lives in `expiry_cron.go`: `created_at + TrialDays` with no Stripe subscription. The counter was **structurally zero** in production and reported as verified. |
   | "mark8ly holds no prices" (#285 spec) | `internal/billing/pricing/catalog.go` holds them in minor units. The conclusion came from finding `PriceIDFor` and stopping. |
   | "there are eight subscription statuses" | There are **ten**. Read the first eight of a const block and concluded. |
   | "the test loops over these constants so a ninth fails loudly" (a code comment) | It hand-wrote the list. Adding a constant changed nothing. |
   | that comment's *fix*, claiming the same guarantee | The hand-written list had merely **moved** into `AllStatuses()`. |

   Two rules, and the second is the one people skip:

   - **For every computed value, check the rule against whatever else in the system
     enforces it.** The cron that expires trials, the endpoint the merchant sees, the
     DB CHECK constraint. Mutation testing proves a test *constrains* the code; it
     cannot prove the code asks the right question. Nothing internal to a feature can.
   - **Concluding that something does not exist requires a search, not a single
     lookup.** "There is no price table" and "there is no other definition of trial
     end" were both wrong, and both were one `grep` away from being caught.

   And when you write a comment asserting a property — that a test covers something,
   that a value can never be zero — **check it the way you would check code.** Three
   of the eleven were prose that ran ahead of the implementation, twice in a row on
   the same line. A confident comment is worse than none: it redirects the next
   reader away from looking.

   Where a real authority exists, test against it. `subscription.AllStatuses()` is
   verified against the `store_subscriptions.status` CHECK constraint, because Go
   cannot enumerate a type's constants and Postgres enforces that list on every write.

8. **Build-tagged code is invisible to the default toolchain, and a skip reads as a
   pass.** Two independent failure modes that compound, both hit in one file on
   2026-08-24 (`internal/billing/migration/repository_integration_test.go`):

   - `go build ./...`, `go vet ./...` and `go test ./...` **never compile**
     `//go:build integration` files. A signature change broke two call sites there and
     every one of those commands stayed green. Only `go vet -tags=integration ./...`
     found it. **Put that command in the standard verification set.**
   - `testdb.NewDB` calls `t.Skip()` when its env var is unset, and that file gated on
     **`TEST_DB_DSN`** while the repo uses **`TEST_DATABASE_URL`** (#317's split). Under
     the variable everyone else sets, its tests skipped silently.

   Compounded: those two tests were **broken AND had never run** — they failed at the
   merge base on a missing parent `stores` row — while every summary line read `ok`.
   A test you have never watched RUN is not a test. Confirm from verbose output that
   it ran; `--- SKIP` and `--- PASS` are one character apart in a wall of output.

   Corollary for verification generally: an `echo "OK"` after a `;` in a shell chain
   prints on failure too, and a cached `ok` is not a fresh run. Use `-count=1` and
   check exit codes when the result is going to be reported as evidence.

9. **A claim's freshness expires, and "it does not exist" needs a search.** Two shapes of
   the same failure, both hit repeatedly in session 3.

   **Freshness:** at the end of that session I reported "three instances of a nil check
   that cannot fail", then re-read the merged code before filing the issue — two had
   already been fixed during review. One was live. A summary you wrote twenty minutes ago
   is a claim like any other, and code moved underneath it.

   **Search, not lookup:** the tenant gate in #287 was designed for "the four admin route
   groups" because I read one file. There are **five** — `RegisterAdminMobile` lives in a
   different file and is mounted right beside the call I did read. The gate's own doc
   comment then asserted it covered the admin surface, which made the gap invisible to
   every reviewer who trusted it. The final whole-branch review caught it only by
   searching for other `Register*` functions.

   Same shape as trap 7's `outbox_events.error` and the `stores` FK claim: a negative
   established by one lookup instead of a search, then written into a comment that
   redirected the next reader away from checking.

   **A third variant reached the instruments, not the code.** A Kargo watch reading an
   empty jsonpath reported `Healthy` for 30 minutes while observing nothing; its
   replacement matched a mark8ly commit SHA against `tesserix-k8s` commits, when mark8ly
   changes arrive as **image tags**. A check that cannot observe its subject is
   indistinguishable from a subject that is not moving.

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

**Sort by effort, not by read-vs-write** — the reads-before-writes heuristic stopped
sorting the queue once four new reads joined. **#288 (purge) stays last** regardless;
it is irreversible.

Order re-derived 2026-08-24:

1. **#287** (suspend/unsuspend) — the smallest remaining write that can fully close an
   issue. Needs a reason-code set defined (a product decision, not a code one). Trap 2
   is already de-risked by the `/api/v1/platform` prefix, but check it before touching
   routing.
2. **The new reads — #332, #333, #329.** Unsized; reads have been the cheaper half all
   milestone. **#331 is blocked by #336** (see above), so it is not in this group.
3. **#286** (trial extend) — reads small, is not. See the five derivation sites above.
4. **#288** (purge) — irreversible, last by design.
5. **#319** (OpenBao) — different concern, grouped in, last of all.

Every write needs `EmitOperatorAction` (trap 3) — except one mounted on `/internal`
rather than the platform surface, where you set `audit.Event.TenantID` and `StoreID`
explicitly from the row instead. #338 is the worked example: `EmitOperatorAction`
belongs to the HMAC platform-admin surface and importing it elsewhere is the wrong
dependency.

**#289 is done** (`/admin/health`, #335). Its lesson, which the next endpoint should
inherit: it originally specified a metric over a column nothing writes, and it shipped
stale-heartbeat checks that were blind to `heartbeat_at IS NULL` — the signature of the
very failure they existed to detect. Both survived task-level review. See trap 7.

**Production data shapes what verification can prove.** `store_subscriptions` is
**empty** (4 tenants, 4 stores, no merchant has entered the billing flow — it needs
an explicit call with a Stripe customer). So the billing endpoints correctly serve
`[]`, and their row-shaping is unexercised in production. When you verify a new
endpoint, say which of your checks are data-independent — status codes, validation,
clamps — and which are merely "no data reached this code". An empty `200` is not a
passing integration check.

**Write endpoints** (#281, #286, #287, #288) all need `EmitOperatorAction` — see trap
3. #287 additionally needs trap 2 checked before you touch routing.

## Known follow-ups outside the milestone

- **#322** — `onboarding` declares `StatusAbandoned`/`StatusExpired` and its package
  doc claims a gc, but nothing writes either value and no gc exists. #283 derives
  abandoned from idle time because of this. Fix the doc at minimum.
- **#323** — neither service's `main.go` wiring is verified by any test. **Five
  instances now**, with three different failure modes, none of them chosen: routes
  silently unmounted (most), a **nil interface that panics at runtime** (the KPI
  `Subscriptions` dependency), and one that fails to *compile* by accident of an
  unused variable while a two-line deletion still unmounts silently. The fix is
  extracting the wiring into a testable function; the assertion should be that every
  dependency a mounted route dereferences is non-nil at **every** site, and that the
  two sites construct equivalent `Deps`.
- **#341** (a nil check that cannot fail), **#342** (three tests that do not prove what
  they name), **#343** (500 rather than 400 on a malformed internal `:id`), **#344** (a
  failed projection update cannot be retried), **#345** (`tenantgate` cache has no
  eviction or absolute staleness ceiling) — all filed at the close of session 3 off #287.
- **#336** — the outbox publisher marks dropped events as published, and
  `outbox_events.error` is never written. Blocks #331's `failed` status. Filed
  2026-08-24 out of #289; full analysis on the issue.
- **#326** — `planchange.go:225` hardcodes `90 * 24 * time.Hour` instead of
  `trial.TrialDays`, and sends it to **Stripe** as `trial_end`. A missed edit there
  means disagreeing with Stripe about a billing date.
- **#311** (store-less audit rows unprunable — decided: never pruned, needs the guard
  made deliberate), **#312** (gateway policy documentation), **#316/#317** (integration
  fixture drift; #317 covers the `store_subscriptions_store_id_fkey` failures you will
  see in `internal/subscription` — pre-existing, scope your runs with `-run`. It also
  covers an **env-var split**: `internal/billing/trial`'s older tests gate on
  `TEST_DB_DSN` while the repo uses `TEST_DATABASE_URL`, so 8 failures were *skipping
  silently* — a skip and a pass look identical in a summary line),
  **#318** (`audit.NewEmitter` accepts a nil `Repo` and panics a worker goroutine).
- **Local toolchain drift:** vet/LSP report `go.work requires go >= 1.26.6 (running
  go 1.26.5)`. Pre-existing, harmless to tests, noisy in diagnostics.

---

Start by reading #260's latest comment, then pick an endpoint and run the
brainstorming → spec → plan → subagent-driven flow. Ask me before merging anything.
