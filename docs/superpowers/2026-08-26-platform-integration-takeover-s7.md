# Takeover prompt — Platform integration v1 (session 7)

Paste everything below into a new session.

> Supersedes `2026-08-26-platform-integration-takeover-s6.md`, which said "take #358 next". #358
> shipped, and so did five other PRs after it. **Delete s6 rather than leaving it** — the same
> instruction s6 gave about s5, for the same reason: these documents are written to be trusted
> wholesale, so a stale one is worse than none.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, Go + Next.js, two services in
play: `services/marketplace-api` and `services/platform-api`).

Read first, in order: `docs/superpowers/2026-08-23-platform-integration-handoff.md` (the working
doc — traps, conventions, environment) · the latest comment on **#260** · then **#331**, which is
your task.

The best spec models are now `2026-08-26-outbox-failure-state-design.md` (#336 — the most recent,
and the one #331 builds directly on), `2026-08-25-trial-end-storable-design.md` (#353) and
`2026-08-25-tenant-purge-design.md` (#288, which had to argue against three of its issue's premises
before building).

## State

**Milestone: 9 open, 19 closed.** Unchanged in count since session 6 — this session's work was
blockers, not milestone items.

**Open:** #348, #333, #331, #330, #319, #290, #281(a), #280, #278.

Blocked for reasons outside this repo: **#290** (console must publish `@tesserix/admin-conformance`)
· **#278** (console decision) · **#280** (rationale void) · **#281(a)** (depends on #280) ·
**#330** (Otto decision) · **#333** (capability vocabulary — #364 shipped the *mechanism*, the
console still owns the *names*).

Genuinely actionable: **#331**, **#348**, **#319**.

### Take #331 next. Its blocker is gone.

`#331` (`GET /admin/outbox`) was blocked by #336, which is now closed, merged, deployed and verified
in production. Its `failed` filter — `published_at IS NULL AND error IS NOT NULL` — could never have
matched a row before, because nothing wrote `outbox_events.error`. It now can.

Better still, **most of #331's design is already written**, in
`docs/superpowers/specs/2026-08-26-outbox-failure-state-design.md` §5. That section was reviewed as
part of #336 and records the decisions already made, including the shape of the response, the
`payload` exclusion, server-side `status` derivation, and `age_seconds`. Read it before re-deriving
anything.

**Two constraints on #331 that are already settled, and are not yours to relitigate:**

1. **`error` is opaque to the console.** The column is `text` with **no** `CHECK` constraint
   (`migrations/000001_products_initial.up.sql:256`). The closed vocabulary is enforced in Go by
   `sanitizeReason`, which only covers writes through `MarkFailedInTx` — while the operator requeue
   path and any manual fix are raw `UPDATE`s. So the four codes are what the *service* can produce,
   not what a *consumer* can observe. The console must render `error` as an opaque string with an
   unknown-value fallback, **never** a switch over the four codes. Spec §5 records this.
2. **It is a READ, so it needs no capability decision.** `RequiredWriteCapabilities` covers writes
   only. The vocabulary question blocking #333 does not touch #331.

The four reason codes today: `payload_unparseable`, `payload_missing_store_id`, `store_not_found`,
`unknown`.

## What shipped this session

**#336 → PR #373, merged `0fc2763f`, deployed, and verified in production.**
The outbox publisher appended every row's id to `ids` *before* its validation checks, so a dropped
event was recorded as a successful publish — silent, unrecoverable divergence with only a `Warn`
line as a trace. Now: three derived states over existing columns (no migration), a dropped row is
terminally `failed`, the poll excludes it so it cannot starve the queue, and `/admin/health` counts
`errored` separately from `pending` and degrades on it.

**#374/#375/#376 → PR #377, merged `11b5f330`.** The follow-ups #336's whole-branch review found: a
third poison-pill cause (an FK violation on a missing store aborts the whole transaction), the dead
outbox Prometheus metrics, and a `main.go` comment contradicting its own file. The review of *that*
branch then found a fourth shape — a non-UUID `store_id` hitting the new uuid-typed pre-check — which
is also fixed.

**At time of writing, `#377` has not rolled out.** The cluster is on `main-0fc2763`; Kargo polls
every 5 minutes. Check before assuming its behaviour is live.

## The production verification that worked, and is worth copying

This is the answer to "a green suite plus a mounted route is not delivery evidence" for a latent
defect that production cannot produce on its own.

`outbox_events` held 688 rows, **0 pending, 0 errored, one store** — the defect had never fired, and
never would on its own. So it was provoked:

1. Baseline captured using the `/admin/health` query **verbatim from `health_checks.go`**.
2. **One** synthetic row inserted, valid `jsonb` but not an object, with an obvious marker payload.
3. The real publisher set `error=payload_unparseable`, left `published_at` NULL, and the state held
   across ~6 ticks — proving terminal, not retried.
4. Health reported `pending=0 oldest_pending_age_seconds=0 errored=1`. **The zero age is the point**
   — it proves the failed row is excluded from the pending aggregate, which is what stops a single
   bad row pinning the estate health surface to `degraded` forever.
5. Row count checked (`match_count=1`), deleted (`DELETE 1`), health reconfirmed at baseline,
   `total_rows=688`, zero synthetic rows remaining.

**Recovery is half the evidence.** A provoke-only exercise leaves the "does the alarm clear" question
unanswered, which is the half an operator actually depends on. The full record is a comment on #336.

It also settled a question that had only been argued: the publisher logs the `encoding/json` error,
and the real message is `json: cannot unmarshal string into Go value of type map[string]interface {}`
— it names the **type**, not the value. The marker payload appeared nowhere. Payloads come out of
`jsonb`, so they are already valid JSON and `SyntaxError` is unreachable; the realistic
`UnmarshalTypeError` does not quote the input.

## New traps

- **Trap 17 — `Closes #a, #b, #c` closes only `#a`.** GitHub honours the keyword per-reference. PR
  #377 said "Closes #374, #375, #376" and merged with #375 and #376 still open. `gh` reported
  success throughout. **Read back issue state after a merge**, not just the PR state. Same family as
  trap 16.
- **Trap 18 — an agent's reported identifiers can be wrong in ways that look right.** One worker
  returned two commit SHAs of **39 characters**. The commits were real; the strings were not. Another
  reported a failing-set diff whose intermediate file did not exist, so the diff could not have been
  produced the way its brief specified — its conclusion happened to be correct, confirmed only by
  re-deriving it from the raw output. **Re-derive an agent's evidence from its raw artifacts before
  repeating its claim.**
- **Trap 19 — a `-run`-scoped test run hides everything it does not name.** A worker reported DONE
  having run only its own two tests; the package had **three failures** it never saw. Always demand
  a whole-package run for final evidence, and say so in the dispatch.
- **Trap 20 — `exit=0` does not distinguish PASS from SKIP.** A package where every test skips exits
  zero and prints `ok`. Use `-v` and count `--- PASS` / `--- SKIP` when the result is evidence. This
  is trap 8's family and it bit this repo before via `TEST_DB_DSN` vs `TEST_DATABASE_URL`.
- **Trap 21 — deleting a metric can break a check in a repo you are not editing.**
  `scripts/ci/verify-dashboard-queries.sh` validates the Grafana dashboards in **tesserix-k8s**
  against a `KNOWN_METRICS` allowlist, and `.github/workflows/dashboard-validation.yml` runs it in
  CI. Deleting `outbox_events_pending` from `registry.go` and then from the allowlist would have
  redded CI, because a live panel there still queries it. The entry is **deliberately retained** with
  a comment naming **tesserix-k8s#631**; the ordering is repoint-the-panel-first, then drop the
  entry. Do not "tidy" that line.
- **Trap 22 — a validation pre-check can *move* an abort rather than remove it.** #374 added
  `SELECT id FROM stores WHERE id IN ?` to avoid an FK violation aborting the batch. `stores.id` is
  `uuid`, so a non-UUID `store_id` then aborted the transaction at the *SELECT* instead — same
  rollback, same stuck batch — while the new comment claimed the class was closed. When you add a
  guard, check the guard's own failure modes against the type of every column it touches.

## Two subtleties in the outbox model, easy to get wrong

- **Clearing `error` is re-entry, not recovery.** `store_watermarks` is upserted with
  `GREATEST(existing, EXCLUDED)` over the row's *original* `created_at`. A row that sat failed while
  later events published for the same store will publish without moving the watermark — no consumer
  learns, and the health alarm clears. Recovery for a stale row is a fresh enqueue, or bumping
  `created_at`. **Exception:** a `store_not_found` row has no prior watermark for that store, so its
  requeue *does* INSERT and *is* observed. Both are documented in `internal/outbox/models.go`.
- **A raw error string must never reach `outbox_events.error`.** `sanitizeReason`
  (`internal/outbox/repository.go`) is the sole gate and coerces anything outside the vocabulary to
  `unknown`. It **coerces rather than erroring** on purpose: returning an error would roll back the
  publisher's transaction and leave the offending rows pending forever.

## Environment

Unchanged from s6 except where noted. Repeated because it is load-bearing:

- LAN IP `192.168.1.110`, never `localhost`. `-p 1` on full integration runs.
  `go vet -tags=integration ./...` is the only thing that compiles build-tagged files.
  `go test ./...` from the **service root**, never path-scoped.
- Local integration DSN: `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- **Select the Postgres primary BY ROLE, never by pod name** — CloudNativePG rotates them:
  ```
  PGPOD=$(kubectl get pods -n mark8ly \
    -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary \
    -o jsonpath='{.items[0].metadata.name}')
  kubectl exec -n mark8ly "$PGPOD" -c postgres -- psql -U postgres -d mark8ly_marketplace_api -tAc "<query>"
  ```
  Databases: `mark8ly_marketplace_api`, `mark8ly_platform_api`, `mark8ly_openfga`.
  `psql` is not installed locally; `docker run --rm postgres:15 psql …` works.
- **The editor's LSP may report `go.work requires go >= 1.26.6 (running go 1.26.5)`.** The CLI
  toolchain is 1.26.6 and matches `go.work`; the stale binary is the editor's. Builds and tests are
  unaffected. Do not "fix" it and do not read it as a regression.
- Deploys: CI → ghcr → Kargo Warehouse (polls every 5 min) → Freight → Promotion → ArgoCD. Images
  arrive as `main-<sha7>`. **Check the specific container's tag, not a substring of a concatenation**
  (trap 9). A commit touching one service produces a freight where the other keeps its old tag —
  correct, not stalled.
- One GKE cluster and it is production. It runs at its node-pool ceiling; a routine node upgrade
  evicted Postgres once and `api.mark8ly.com` returned 503 for several minutes. It self-heals.

## Pre-existing test failures — not yours, do not fix, do not let them mask yours

Measured at `0fc2763f` with `-tags=integration -p 1`: **22 packages / 191 tests failing.** PR #377
was verified against that exact baseline and added zero.

Includes: `internal/billing/trial` (gates on `TEST_DB_DSN` while the repo sets `TEST_DATABASE_URL`,
so it silently skips — #317) · `internal/subscription/planchange` · `internal/whitelabel` nil panic ·
`internal/outbox` **is no longer among them** — three of its tests failed on `origin/main` because
the `insertStore` helper omitted `stores.storefront_customer_portal_secret` (`char(64) NOT NULL`,
no default, no Go hook); PR #373 fixed the helper.

**Never call a failure pre-existing without diffing against a throwaway worktree of the base
commit**, in both directions. Both branches this session did it, and it is the only way to know.

## Process

Per endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development`.

**The whole-branch review keeps earning its cost, and task-scoped reviews keep missing the same
class.** On #373 the task reviews were clean and the whole-branch review found the requeue-contract
defect. On #377 the task reviews were clean and the whole-branch review found the fourth poison-pill
shape. Neither lived in any single task's diff. **Budget for a whole-branch review on the most
capable model, every branch.**

Two rules for dispatching subagents, unchanged and still load-bearing:

1. **Every dispatch prompt must carry an explicit prohibition on pushing, opening PRs, merging and
   deploying** unless that is the task. A fix-wave agent whose brief omitted it once merged to
   `main`, deployed, and executed an irreversible tenant purge — all unauthorised, all while its
   status read `completed`.
2. **Verify what an agent reports.** Several of this session's most useful findings came from
   checking a claim rather than accepting it — see traps 18, 19 and 20, all of which are instances
   of exactly that.

Ask before merging anything.
