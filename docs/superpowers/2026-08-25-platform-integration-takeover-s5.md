# Takeover prompt — Platform integration v1 (session 5)

Paste everything below into a new session.

> The previous takeover (`2026-08-24-...-s4.md`) was **deleted**, not superseded in place.
> It said #332 was next and that #286 was the large one; both are now delivered. A stale
> takeover is worse than none, because it is written to be trusted wholesale.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, a multi-service
Go + Next.js workspace).

Read first, in order: `docs/superpowers/2026-08-23-platform-integration-handoff.md`
(the working doc — traps, conventions, environment) · the latest comment on **#260** ·
`docs/superpowers/specs/2026-08-22-platform-admin-surface-design.md` (the foundation).

The two most recent specs are the best models, because each had to argue against its own
issue's premise before building anything:
`2026-08-25-trial-end-storable-design.md` (#353) and `2026-08-25-trial-extend-design.md` (#286).

## State

**Sixteen delivered:** #274, #275, #276, #277, #279, #282, #283, #284, #285, #289,
#281(b), #287, #329, #332, #353, #286. (#326 closed as a side effect of #353.)

**Take #288 next — but read the warning below before you touch it.** Then #319. #281(a)
remains open.

**Blocked, do not start:** #278 (needs a console-side decision), #280 (its stated
rationale is void — see the working doc), #290 (blocked on the console publishing
`@tesserix/admin-conformance`), #331 (behind #336), #333 (a decision, not work), #330
(an Otto decision). New and unblocked but not sequenced: #348, #358.

## #288 is the irreversible one, and it is last by design

`POST /admin/tenants/{id}/purge`. Everything else in this milestone can be rolled back by
reverting a deploy. This cannot. Before designing it:

- Read `internal/tenantpurge/purge.go`. It already exists and already enumerates the
  tenant-scoped tables. **Verify that list against the current schema rather than trusting
  it** — it is a list maintained by hand, and this milestone has repeatedly punished
  trusting maintained lists (see trap 12).
- `subscription/harddelete/sweeper.go` is a second, overlapping enumeration. Two lists of
  "every tenant-scoped table" that must agree and have no test forcing them to.
- Production has **4 tenants and 4 stores**. There is no scratch tenant. Purging to verify
  is not available to you, and a dry-run mode is probably part of the design rather than a
  nicety.

## What the last two issues cost, and why

Both #353 and #286 were re-scoped during design because the issue's premise did not hold.
Expect the same and budget for it.

- **#286's path parameter did not exist.** It says `{id}`; `/admin/billing/trials` (#285)
  emits `tenant_id` and `store_id` and no row id, so the console had nothing to send. Ruled
  to the **store id**, which `UNIQUE (store_id)` makes unambiguous. That was the **fourth**
  such instance, after #277's tenant slug, #276's `metadata` shape and #329's assignee.
- **#286 could not be built at all** until #353 shipped: trial end was recomputed at seven
  sites and stored at none, so an "extension" would have changed a number nothing read.

**Check the issue's nouns against the schema before you design anything.** It has been
right about the intent every time and wrong about the fields more often than not.

## The failure that matters most, because it recurred at the last possible moment

#286's final whole-branch review found a **Critical** that four task reviews and the
controller all missed: the idempotency key was **global**, so reusing a key against a
different store returned the *first* store's `store_id`, `tenant_id` and dates — a
cross-tenant read on a governance surface — while the second store was never extended and
the operator saw success.

It survived every earlier gate **because the integration test written to prove that exact
acceptance criterion used one store for every call.** No fixture distinguished the broken
implementation from the correct one.

That is trap 6 in its purest form, and the lesson is narrower than "write good tests":
**a test for a property that discriminates between two values must contain both values.**
One store cannot prove cross-store scoping. One status cannot prove a status filter. One
tenant cannot prove tenant isolation.

Related, from the same round: a fix wave introduced a **new Critical** (a reserved
idempotency key never released on failure, so a mistyped reason code bricked that key for
24 hours). **New Critical breakage in a fix diff joins the open findings — it is not a
residual to defer.**

## Twelve traps are in the working doc. These four bit hardest this round

- **Trap 6 — the fixture beside the property.** See above. It has now cost a Critical.
- **Trap 10 — running the search is not the same as reading it.** A claim that "no
  sent-mail record exists" was written from a grep whose output contained the
  disconfirming file. When you write a negative, name the thing you found that came
  *closest* to disconfirming it; that sentence is checkable and "X does not exist" is not.
- **Trap 12 — a pre-existing comment is a claim of the same kind.** Two comments claimed
  `idempotency_keys` was swept nightly. Nothing swept it; the table had **zero consumers
  estate-wide**. Two comments agreeing is usually one comment copied.
- **Trap 8's corollary, in your own shell.** A `&&` chain aborted mid-way while a trailing
  `echo ok` on its own line still printed "ok", so a ledger entry silently never got
  written. Check exit codes, not the last line of output.

## Environment

- LAN IP `192.168.1.110`, never `localhost` · `-p 1` on integration runs · `go vet
  -tags=integration ./...` is the only thing that compiles build-tagged files ·
  `go test ./...` from the **service root**, never path-scoped, or the schema-version guard
  silently does not run.
- **Production Postgres is in-cluster and reachable**, which the earlier takeovers did not
  say: CloudNativePG at `mark8ly-postgres-rw.mark8ly.svc.cluster.local`, database
  `mark8ly_marketplace_api`. Read-only checks:
  `kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- psql -U postgres -d mark8ly_marketplace_api -tAc "<query>"`.
  Use it to replace stale claims with measured ones instead of repeating them.
- **`store_subscriptions` has 0 rows** (4 stores), measured 2026-08-25. Every billing
  behaviour in #353 and #286 is unexercised in production. Re-measure rather than repeat.
- `psql` is **not** installed locally; `docker run --rm postgres:15 psql …` works.
- Migrations apply via a `migrate` **initContainer on `marketplace-api-admin` only**;
  `-storefront` has none and runs the same image with the same `AssertVersion` check, so a
  storefront pod restarting mid-rollout crashloops until its own rollout lands. Expected,
  self-healing, and indistinguishable from a real failure if unexpected.
- Deploys arrive as **image tags** `main-<sha7>`, never git commits. Watch the tag.
- One GKE cluster and it is production.

## Pre-existing test failures — not yours, do not fix, do not let them mask yours

Each confirmed at `origin/main` in a clean worktree:

- 19 tests in `internal/billing/trial/subscribe_integration_test.go` **skip silently** —
  they gate on `TEST_DB_DSN` while the repo sets `TEST_DATABASE_URL` — and **all 19 fail**
  when actually run. Broken *and* never executed. Full analysis on **#317**.
- `internal/subscription/planchange` integration: 9 FAIL / 0 PASS.
- `internal/whitelabel` integration: a nil-pointer panic.

The first nearly cost real money on #353: the natural home for a Stripe `trial_end`
assertion was that exact file, which would have made a money-critical check silently never
execute.

## Verification, stated the way this milestone has learned to state it

Measure the endpoint's paths **before and after** the rollout. A `401` alone cannot
distinguish "mounted" from "this prefix refuses everything". The shape that works:

| path | before | after |
|---|---|---|
| the new route | `404` | `401` |
| a bogus sibling under the same prefix | `404` | `404` |
| an already-live route on the prefix | `401` | `401` |

Exactly one should move. Then check the body says `unauthenticated`, not `not_configured` —
the latter means the secret is unset and the surface is inert.

Then say plainly which of your checks are **data-independent** (routing, mounting,
signature refusal, clamps) and which merely mean **no data reached this code**. An empty
`200` is not a passing integration check, and neither is a `401` from a route whose body
has never run.

## Process

Per endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development`.

The pre-flight scan before Task 1 has caught a real plan defect on three consecutive
branches — a raw `INSERT INTO stores` omitting a `NOT NULL` column, a duplicate import, a
mount guard contradicting its own test. Run it properly; it is not ceremony.

Reviews earn their cost when they **mutate rather than read**. Every branch this round
proved its golden fixture by renaming a field and adding one, and proved each refusal by
deleting its guard.

Ask before merging anything.
