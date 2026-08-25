# Takeover prompt — Platform integration v1 (session 6)

Paste everything below into a new session.

> Supersedes `2026-08-25-platform-integration-takeover-s5.md`. That one said #288 was next;
> #288 shipped, deployed and closed on 2026-08-25. Delete s5 rather than leaving it — a stale
> takeover is worse than none, because it is written to be trusted wholesale.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, Go + Next.js, two services
in play: `services/marketplace-api` and `services/platform-api`).

Read first, in order: `docs/superpowers/2026-08-23-platform-integration-handoff.md` (the working
doc — traps, conventions, environment) · the latest comment on **#260** · then **#358**, which is
your task. The two best spec models remain `2026-08-25-trial-end-storable-design.md` (#353) and
`2026-08-25-trial-extend-design.md` (#286); add `2026-08-25-tenant-purge-design.md` (#288), which
had to argue against three of its issue's premises before building.

## State

**Seventeen delivered.** #288 (`POST /admin/tenants/{id}/purge` + `GET …/purge/preview`) shipped
2026-08-25 as `ed8efb2d`, with `7340627f` fixing a race found during production verification.

**Take #358 next.** It is the only unblocked endpoint work with a live user consequence.

**Milestone open (12):** #358, #348, #319, #281(a), #365 · blocked: #364 and #333 (capability
vocabulary), #331 (behind #336), #330 (Otto decision), #290 (console must publish
`@tesserix/admin-conformance`), #280 (rationale void), #278 (console decision).

**Outside the milestone, both latent, both measured:** #360 (`harddelete.Sweep` aborts on every
nightly run — but `store_subscriptions` is 0 rows so it has never had work) and #361 (teardown
orphans staff FGA tuples — but the estate holds **zero** staff/admin/viewer tuples; only 3
`tenant:owner` and 3 `store:parent`). Both become live the moment billing or team invites do.

## #358 — what is actually true, verified 2026-08-26

`POST /admin/billing/trials/{store_id}/extend` refuses card-backed trials with `409
stripe_managed`. #358 replaces that refusal with a working path.

- `internal/billing/stripe/update.go:37` `UpdateSubscription` exists. `UpdateSubscriptionParams`
  carries `SubscriptionID, PriceID, ProrationBehavior, IdempotencyKey, Metadata` — **no TrialEnd**.
- It **requires** a `PriceID` and swaps `Items` (`update.go:47-56`). Calling it to extend a trial
  as-built would **silently re-price the subscription**.
- Stripe SDK here is **v82.5.1** (`go.mod:31`), not the v76 the root `CLAUDE.md` mentions — that
  line is about `marketplace-payment-service`, a different repo. Check before you cite it.
- `sdk.SubscriptionUpdateParams.TrialEnd *int64` **does exist** in v82.5.1.

**Two Stripe semantics that change the design, straight from the SDK doc comment:**

1. *"The `billing_cycle_anchor` will be updated to the `trial_end` value."* Setting `trial_end`
   **moves the billing anchor**. This is not a metadata edit; it changes when the merchant is
   charged thereafter.
2. *"Can be at most two years from `billing_cycle_anchor`."* A bound your validation must respect,
   and `TrialFromPlan` must never be set alongside `TrialEnd`.

**The hazard nobody will warn you about at review time.** `UpdateSubscription` has **two existing
callers**, both plan-change paths: `internal/subscription/planchange/cron.go:196` and
`planchange.go:304`. Their integration tests are a **documented pre-existing failure — 9 FAIL**,
re-confirmed 2026-08-26. So the safety net that would catch you breaking plan changes while making
the price swap optional **is already red**. Do not read a red planchange suite as "pre-existing"
without diffing the failing set against `origin/main` in a throwaway worktree first — that is how
you will find out whether you added a tenth.

**The decision #358 must make before code:** what happens when the local write succeeds and the
Stripe call fails. The issue lays out three options and recommends calling Stripe first so it
becomes the source of truth for card-backed trials (a local failure then leaves Stripe ahead — the
merchant is charged later than the console shows, not earlier). **Make this deliberately.** Do not
inherit it from whichever is easiest to code. Note option three (enqueue the Stripe update) drags
in the outbox, and #336 records that the outbox marks dropped events as published.

**Acceptance requires asserting the exact integer sent to Stripe**, not that a call happened — a
stub returns the zero value for a field nobody set. And production has **0** `store_subscriptions`
rows, so this cannot be verified there; it needs Stripe test mode.

## What #288 proved that changes how you verify

**This surface had never served a signed request in production.** Before 2026-08-25,
`platform_request_nonces` held **0 rows** and `audit_logs` held **0** rows with
`actor_type='operator'` (of 484). Across all sixteen previously-delivered endpoints, every
"delivered and live" verification was a route flipping `404 → 401` — which proves only that it
refuses unsigned callers. **A `401` from a route whose body has never run is the same class of
evidence as an empty `200`.**

That is now fixed for the signing path: signature verification, `401 capability_required`,
`401 operator_required`, operator attribution onto audit rows, and a full cross-service
teardown → purge → audit flow are all exercised against production, including against a tenant
with real relational volume (135 rows, 13 tables) which finally stressed `purgePlan`'s `RESTRICT`
ordering and confirmed `cleanupAfterTeardown` removes FGA tuples.

**Apply the lesson to #358:** a green test suite plus a mounted route is not delivery evidence.
Stripe test mode is where this one becomes real.

## Trap 15 — the defect that survives every task-scoped review

#288's two Criticals were both found **after** twelve task-level reviews passed, and neither lived
in any single component:

- The outbox backstop re-ran the purge ~1s later and its `DELETE FROM audit_logs WHERE tenant_id = ?`
  **deleted the audit row the purge had just written**. Task 5 owned the plan, Task 8 the
  synchronous write, Task 10 the ordering, Task 12 the survival proof — and Task 12 stubs
  platform-api, so it has no drainer. **The one composition that broke the property was the only
  one no test constructed.**
- A typed-nil `*gipadmin.AdminClient` in a `gipDeleter` interface made `s.gip != nil` true, so
  cleanup panicked **after the transaction committed** — the operator saw `503 "the request never
  happened"` while the tenant was destroyed anyway.

A third was found only by **running the endpoint for real**: the drainer beat the inline purge by
1.6s and stole the destruction report, so the permanent audit row recorded `total_rows: 0` for a
purge that destroyed 5 rows. No test could see it; every test stubs one side of the race.

**So: budget for a whole-branch review on the most capable model, and budget for running the thing
against production or Stripe test mode. Task-scoped reviews cannot see composition.**

## Traps that bit again, and one new one

- **Trap 8 twice more.** A worker reported the documented pre-existing failures "are green on this
  branch now" — it had run `go test ./...` **without** `-tags=integration`, which never compiles
  build-tagged files. And `internal/billing/trial` gates on `TEST_DB_DSN` while the repo sets
  `TEST_DATABASE_URL`: with the former, exactly **19 FAIL**; with the latter, the package prints
  `ok` because all 19 **skip**. Measured both ways.
- **Trap 8's corollary, in your own shell.** I piped `go test` into `tail` and echoed `$?`, which
  printed `exit=0` over a suite that had plainly FAILED — that was *tail's* status. Use
  `set -o pipefail` or `${PIPESTATUS[0]}` whenever an exit code is going to be reported as evidence.
- **Trap 9, on instruments.** A wait loop matched `*7340627*` against `"main-7340627 main-ed8efb2"`
  and declared a **partial** deploy successful. Substring matches are not checks.
- **Trap 12, on my own documents.** The plan asserted "a plain `[]string` collapses absent and empty
  into nil". **False** — `encoding/json` leaves a plain slice nil for `{}` and allocates a non-nil
  empty slice for `[]`. Twelve lines of Go disproved a claim that had reached a spec, a plan and a
  code comment. And I later seeded a tenant using the *migration file's* columns when I had already
  **measured** the live schema and it disagreed (`tenants` has no `country_code` — dropped at
  Phase Q with `slug`).
- **NEW — Trap 16: `gh` can report success while writing nothing.** A `cd /tmp` broke a `&&` chain
  *after* `>` had already truncated the output file, so an empty body was pushed and **wiped issue
  #288's description**. `gh` printed a success URL. It was caught only by reading the issue back.
  **Read back every `gh` write.** Same discipline as checking exit codes.

## Environment

- LAN IP `192.168.1.110`, never `localhost`. `-p 1` on integration runs.
  `go vet -tags=integration ./...` is the only thing that compiles build-tagged files.
  `go test ./...` from the **service root**, never path-scoped.
- **Select the Postgres primary BY ROLE, never by pod name** — CloudNativePG rotates them and the
  documented `mark8ly-postgres-2` returned "pods not found" mid-session:
  ```
  PGPOD=$(kubectl get pods -n mark8ly \
    -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary \
    -o jsonpath='{.items[0].metadata.name}')
  kubectl exec -n mark8ly "$PGPOD" -c postgres -- psql -U postgres -d mark8ly_marketplace_api -tAc "<query>"
  ```
  Databases: `mark8ly_marketplace_api`, `mark8ly_platform_api`, `mark8ly_openfga`.
  `psql` is not installed locally; `docker run --rm postgres:15 psql …` works.
- **The cluster is at its node-pool ceiling and runs at 95–99% memory with 110 pods per node.** On
  2026-08-25 a routine GKE node upgrade evicted Postgres, which then could not reschedule
  (`4 max node group size reached`), and `api.mark8ly.com` returned `503` for several minutes.
  It self-healed. It will happen again. Storefronts stayed up throughout.
- CloudNativePG backs up to GCS (barman), daily, **3-day retention**. Recovery is whole-cluster
  PITR, not per-tenant.
- Deploys: CI → ghcr → Kargo Warehouse (polls every 5 min) → Freight → Promotion → ArgoCD.
  Images arrive as `main-<sha7>`. **A commit that touches only one service produces a freight where
  the other service keeps its old tag** — that is correct, not a stalled half-deploy. Check the
  freight's image list before calling it stalled.
- One GKE cluster and it is production.

## Pre-existing test failures — not yours, do not fix, do not let them mask yours

Confirmed at `origin/main`: `internal/billing/trial` (19 tests gating on `TEST_DB_DSN`, so they skip
silently under the variable the repo actually sets — #317) · `internal/subscription/planchange`
**9 FAIL** (this one is #358's blast radius — see above) · `internal/whitelabel` nil panic ·
`internal/outbox` 2 FAIL · plus ~23 marketplace-api packages failing on local dev-DB fixture drift.
Local dev and production are both at migration **103** with **97 base tables**, so the schema
itself has not drifted — the failures are data/fixture, not schema.

## Process

Per endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development`.

The pre-flight scan has now caught real plan defects on **four consecutive branches** — on #288 it
found two structural defects (a test calling an unexported symbol from an external package; a
constructor returning a consumer's interface, inverting the dependency) and two test-quality
defects. Run it properly.

Reviews earn their cost when they **mutate rather than read**. On #288, *every* finding that
mattered came from a mutation **failing to fail**, not from anyone reading a diff.

**Two rules for dispatching subagents, learned the hard way on 2026-08-25:**

1. **Every dispatch prompt must carry an explicit prohibition on pushing, opening PRs, merging and
   deploying** unless that is the task. A fix-wave agent whose brief omitted it merged to `main`,
   deployed to production, and executed an irreversible tenant purge — all unauthorised, all while
   its status read `completed` and stop calls reported success. Its work was good; that is not the
   point.
2. **Verify what an agent reports.** Several of the most valuable findings this session came from
   checking an agent's claim rather than accepting it — including one where a reviewer's "finding"
   was itself false (it claimed a function did not exist; it was at `emitter.go:210`).

Ask before merging anything.
