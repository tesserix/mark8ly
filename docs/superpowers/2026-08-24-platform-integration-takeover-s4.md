# Takeover prompt — Platform integration v1 (session 4)

Paste everything below into a new session.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, a multi-service
Go + Next.js workspace).

## Read these first, in this order

1. **`docs/superpowers/2026-08-23-platform-integration-handoff.md`** — the working
   document: traps, conventions, environment, what each endpoint inherits.
2. The **latest comment on #260** — the umbrella, updated at the end of session 3.
3. `docs/superpowers/specs/2026-08-22-platform-admin-surface-design.md` — the foundation.

The two most recent specs are worth reading as models, because both had to argue
against their own issue's premise: `2026-08-24-tenant-suspend-design.md` (#287) and
`2026-08-24-platform-tickets-design.md` (#329).

## Where things stand

**Thirteen delivered:** #274, #275, #276, #277, #279, #282, #283, #284, #285, #289,
**#281 part (b)**, **#287**, **#329**.

**Remaining, ordered by effort** — reads-before-writes stopped sorting this queue once
four new reads joined:

1. **#332** — `GET /admin/notifications`, the sent-mail log. **No blocker; start here.**
   Data is in `notifications` (migrations `000016`, `000091`). Must NOT return message
   bodies: a rendered notification contains whatever the template interpolated.
2. **#333** — `GET /admin/break-glass`. **Blocked on a decision, not on work** — see below.
3. **#286** — trial extend. **Far larger than its acceptance criteria suggest** — see below.
4. **#288** — purge. Irreversible, last by design.
5. **#319** — OpenBao credentials, a different concern grouped in.

**Blocked:** #278, #280, #290, and **#331** behind **#336**.

### #333 needs a decision before anyone builds it

Its acceptance requires the endpoint be gated on the operator holding the
`rotate-credentials` **capability**. This surface checks capability **presence only,
never the value** (`internal/handlers/platformadmin/middleware.go:153`), and #287
deliberately did not invent a value vocabulary: the console asserts these names, and a
guess would silently refuse every real request.

So either settle the console's capability names first, or ship #333 with the gate
explicitly deferred and say so in the response. Do not invent a capability string.

### #286 is not the small one it looks like

Its acceptance reads modestly — require a reason, refuse on already-converted, be
idempotent. But **there is no stored trial-end column anywhere.** Trial end is *derived*
as `created_at + TrialDays` in five places:

- `handlers/admin/subscription.go:197` — what the merchant sees
- `billing/trial/subscribe.go:131` — **the value sent to Stripe as `trial_end`**
- `subscription/dunning/trial_reminders.go:108`
- `billing/trial/expiring.go`
- `planchange.go:224` (plus #326's hardcoded `90` in the same role)

"Extend a trial" is not representable today. It needs a new column **and** all five
consumers taught to respect it. Miss the Stripe path and the console quotes one date
while Stripe bills another.

## The one thing to internalise

**Check the claim against the code — including your own claims, and including claims
you made an hour ago.**

Three issues in this series have asked for a field that does not exist: #277's tenant
slug, #276's `metadata` shape, #329's `assignee`. Three for three is not coincidence.
**Read the model before implementing a field name from the issue text**, and when the
field is absent, report it rather than inventing the concept.

It runs deeper than field names. #287 asked for enforcement that **nothing in the estate
performed** — `tenant.StatusSuspended` was declared, never written, and never read to
deny anything, so the endpoint as specified would have changed nothing observable. Its
acceptance also named an `auth-bff` gate that could not work, because `auth-bff` has no
platform-api client at all.

And the freshness of a claim expires fast. At the end of session 3 I reported "three
instances of a nil check that cannot fail", then re-read the merged code before filing
the issue: two had already been fixed during review. One was live. **A summary you wrote
twenty minutes ago is a claim like any other.**

**Concluding that something does not exist requires a search, not a lookup.** Session 3's
worst miss: the tenant gate was designed for "the four admin route groups" because I read
one file. There are five — `RegisterAdminMobile` lives in another file and was mounted
right beside the call I did read. The gate's own doc comment then asserted it covered the
admin surface, which made the gap invisible to every reviewer who trusted it.

## Verification: three blind spots that all report green

Each of these had the tooling reporting success over ground it never covered.

1. **Build-tagged files are invisible to the default toolchain.** `go build`, `go vet`
   and `go test` never compile `//go:build integration` files. A signature change broke
   two call sites and all three stayed green. **Put `go vet -tags=integration ./...` in
   your standard set.**
2. **A silent skip reads exactly like a pass.** `testdb` skips when its env var is unset,
   and one file gated on `TEST_DB_DSN` while the repo uses `TEST_DATABASE_URL` (#317).
   Two tests had **never run** and were broken underneath. Confirm from verbose output
   that a test RAN; `--- SKIP` and `--- PASS` are one character apart in a wall of text.
3. **A root-package test is excluded by every path-scoped command.** platform-api's
   `ExpectedSchemaVersion` must move with each migration, and only `go test ./...` at the
   service root catches it. Its failure text names the consequence: the migrate
   initContainer advances the DB past what `cmd/server` accepts and the service
   **crashloops on rollout**. **Adding a platform-api migration means bumping that
   constant** (marketplace-api has no such rule — trap 4's two-services lesson again).

The same failure shape reached my own instruments in session 3: a Kargo watch reading an
empty jsonpath reported `Healthy` for 30 minutes while observing nothing, and its
replacement matched a mark8ly commit SHA against `tesserix-k8s` commits — mark8ly
changes arrive as **image tags** (`main-<sha7>`), so it could never match. A check that
cannot observe its subject is indistinguishable from a subject that is not moving.

## Process that works

Per endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development` (fresh subagent
per task, review between, whole-branch review at the end).

What actually earns its cost:

- **Reviews that verify rather than read.** Every real finding came from someone
  recomputing a value, searching for a second call site, or reading a test's own doc
  comment instead of accepting a summary.
- **Telling the reviewer where the fixture might fail to discriminate.** Session 3's
  sharpest catch: nothing pinned tenant scoping, so deleting `tenant_id = ?` would have
  suspended **every store in the estate** with the whole suite green.
- **Mutation proofs that name the failing test and quote its text.** "Failed as expected"
  hides a mutation that failed for the wrong reason — one proof fired on a downstream
  panic rather than the assertion it was meant to exercise.
- **Writing plan code against the file, not against memory.** Six defects in session 3's
  plans were mine: a `repo` field that did not exist, a logger on a struct that has none,
  sentinel comparisons against a package that uses constructors, a wrong import path, a
  fabricated FK claim, and fixtures placed outside the window where behaviour changes.
  Five were compile-time. The sixth would have shipped green.

Two recurring reviewer failures to watch for: downgrading a real finding because "the
plan mandated it" (a plan is not authority — rule on it yourself and record the ruling),
and reporting a mutation "failed as expected" without saying which test failed.

## Environment

- **Use the LAN IP, not `localhost`** — a native Postgres squats on 127.0.0.1:
  - marketplace: `postgres://dev:dev@192.168.1.110:5432/marketplace_db`
  - platform: `postgres://dev:dev@192.168.1.110:5432/platform_api`
- `-p 1` on integration runs; the packages share one local Postgres.
- Deploys are **Kargo-gated**, and mark8ly changes arrive as **image tags**, not git
  commits — the Warehouse's git subscription watches `tesserix-k8s`. To confirm a deploy,
  read `.status.freightHistory[0]` on the `prod` stage and look at the **image tags**
  (`main-<sha7>`), not the commit list.
- Only ONE GKE cluster exists and it is **production**. Never test against it.
- **Docs changes go through a PR now**, not direct to `main` (settled in #339): direct
  pushes bypass the branch-protection rule, and a stray `git add -A` once swept six
  unrelated files into a docs commit.

## Open follow-ups

**From session 3:** **#336** (the outbox publisher marks dropped events as published, and
`outbox_events.error` is never written — **blocks #331's `failed` status**), **#341** (a
nil check that cannot fail), **#342** (three tests that do not prove what they name),
**#343** (500 instead of 400 on a malformed internal `:id`), **#344** (a failed projection
update cannot be retried), **#345** (`tenantgate` cache has no eviction or ceiling).

**Pre-existing:** #322, #323, #326, #311, #312, #316/#317, #318, #313.

## Unfinished business

- **#287's enforcement is unverified in production.** The deploy checks confirmed
  platform-api starts (so the migration and `ExpectedSchemaVersion` agree), the routes are
  mounted and refuse unsigned requests, and the audit emitter is wired. The cascade and all
  five enforcement points need a real suspension to prove — which needs a **scratch
  tenant**, not one of the four live merchants. Ask before creating one.
- **#281 stays open** for part (a); its console-facing half needs reviewer identity
  reworked, since `Review` demands a UUID while the platform surface's operator id is an
  opaque string.

---

Start by reading the handoff doc and #260's latest comment, then take **#332** and run the
brainstorming → spec → plan → subagent-driven flow. Ask before merging anything.
