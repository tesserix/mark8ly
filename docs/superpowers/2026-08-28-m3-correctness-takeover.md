# Takeover: mark8ly milestone 3 — Correctness & data integrity

You are taking over an in-progress effort. Eight issues remain in milestone **"Correctness & data integrity"** (`gh api repos/tesserix/mark8ly/milestones/3`), whose charter is: *"Bugs affecting correctness, data integrity or money. Defects with evidence, not hardening."*

Two of its ten were closed on 2026-08-28 (#395, #318). Everything below is what the previous session learned doing them. Read it before starting — several items will cost you hours if rediscovered the hard way.

---

## The work

| # | title | notes from prior session |
|---|---|---|
| #399 | `ApplyTrialRamp` re-inflates already-consumed budget on re-run despite claiming idempotency | money; the guarantee is documented but false |
| #398 | Merchant appeal text overflows `subscription_arbitrage_audit.mismatch_reason varchar(100)` | |
| #397 | Blocked-downgrade audit row is written then rolled back with the transaction it runs in | **adjacent to #318's work** — audit that doesn't audit; user flagged this as costing them today |
| #396 | billing/tax/revalidation cron deadlocks (cross-connection lock wait) — **has never run successfully** | user flagged as costing them today; needs lock-ordering investigation before a fix |
| #372 | `signature.go`: the encodeURIComponent divergence is five characters wider than the doc says, and no vector covers them | security; `signature.go` is the platform-admin HMAC, exercised by the conformance CronJob |
| #369 | Erasure free-text carve-out covers only the purge row, not the tenant's other operator audit rows | compliance; **adjacent to #318 and #259** |
| #350 | `notifications.recipient_user_id` is documented as a GIP UID; it is the customer profile id | data-model; likely a doc-vs-code correctness question — establish which is wrong before changing either |
| #259 | GDPR erasure requests are collected and never processed | largest; building a processor, not fixing a defect. The unprocessed rows already surface through `internal/inbox/erasure.go`'s provider |

**Suggested order.** #397 and #369 while the audit emitter is fresh (both are audit-record correctness, in code just hardened by #318). #396 next — it is the one with a user-stated cost that nobody has diagnosed. #259 last, or as its own effort; it is a feature.

---

## Environment and conventions — verified, not guessed

**Repo:** `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`. Go service is `services/marketplace-api`. Siblings that matter: `../tesserix-home` (console/platform-api), `../tesserix-k8s` (charts), `../design-system` (the `admin-conformance` suite).

**Use a git worktree, always.** Convention here is `.claude/worktrees/<name>` — Go tooling ignores dot-directories so `go build ./...` at the repo root cannot see it. On 2026-08-28 a second stream checked out a different branch in the main checkout **mid-task**, and a subagent's audit silently ran against the wrong branch. A worktree removes that class of problem entirely:

```
git worktree add .claude/worktrees/<name> <branch>
```

**Go commands** — run from `services/marketplace-api`, never path-scoped (the root package holds a schema-version guard that silently does not run otherwise), always `-count=1`:

```
go build ./... ; go vet ./... ; go vet -tags=integration ./... ; go test ./... -count=1
```

`go vet -tags=integration ./...` is the **only** command that compiles build-tagged files. Include it.

**Integration tests:**
- `//go:build integration`, gated on `TEST_DATABASE_URL` — **never** `TEST_DB_DSN`; files using that skip silently (#317).
- Run with `-p 1` (the packages share one Postgres; parallel runs exhaust connections and present as data pollution).
- Verified DSN: `postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` — container `dev-postgres-1`, reachable at both the LAN IP and 127.0.0.1. Use the LAN IP.
- The database is **shared**. Use transaction-scoped fixtures (`pkg/testdb`'s `NewTx` rolls back in `t.Cleanup`) or clean up in a defer.

**Commits:** conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.

**Staging:** use explicit paths (`git add <path>`). Never `git add -A` — the previous session did so while a subagent held the working tree and swept its in-progress work into an unrelated commit.

---

## THE TRAP THAT CAUGHT US TWICE

**Integration tests skip silently without `TEST_DATABASE_URL`, and a skip reads as a pass.**

`./internal/whitelabel/lifecycle` takes **1.4s skipping** versus **3.2s actually running**. A subagent reported that the repo's known-failure packages "all pass now" — they did not; its run had skipped every one. That same run was its evidence that a fix worked.

So: **any claim about an integration test must name the DSN it ran with.** Require your subagents to state explicitly whether their runs were unit-only. Treat "the suite is green" as meaningless until you know what actually executed.

---

## Known pre-existing failures — the list is partly stale, verify before trusting

Repeatedly quoted to subagents as "not yours to fix". Current understanding:

- `internal/billing/trial/subscribe_integration_test.go` — 19 tests (#317)
- `internal/subscription/planchange` integration
- `internal/whitelabel` integration nil-pointer panic — **FIXED by #318**, remove it from the list
- `TestIntegration_ProductService_UpdateAggregate_OptionValueInUseRejected` — fails `variant_matrix_mismatch`; **not on the official list**, verified pre-existing on 2026-08-28, makes `go test -tags=integration ./internal/product/...` exit non-zero. Deserves its own issue.
- `internal/handlers/platformadmin` integration — 2 failures, both `relation "inbox_action_idempotency" does not exist`. A missing table on the shared test DB, i.e. a migration gap in the test environment, not a code defect.

**Do not accept "pre-existing" without evidence.** When a failure appears next to your change, check it at a pre-change commit — a throwaway worktree at the base commit plus one test run takes two minutes:

```
git worktree add /private/tmp/base <pre-fix-sha> --detach
```

That is how the previous session confirmed `variant_matrix_mismatch` was not a regression, after a subagent's stated evidence turned out not to establish it (its "baseline" already contained the fix).

---

## How to work these issues

**Verify the issue before planning from it.** Four of six issues closed on 2026-08-28 were understated or misdescribed, and in every case checking beat reasoning:

- #393 said IDR was mispriced. The code was correct; only a comment was wrong.
- #413 said one row was wrong. The whole table was, plus a second bug class quoting ~3% of price to unpriced markets.
- #395 named `Variant`. `Product` had the identical declaration, and three sites were already working around the leak.
- #318 described a constructor crash. The same crash existed one layer down in the repository, and again via an unvalidated `Logger`.

**Audit before changing anything with silent blast radius.** #395's plan spent a whole task producing *no code* — just enumerating every query that touched the model — because the one-line fix would make GORM filter every query on it, and a query that needed to see deleted rows would have stopped seeing them with no compile error. That audit is what made the change safe to make confidently.

**Mutation-test every guard.** This was the single highest-value practice. A green suite proves nothing; the question is whether it stays green with the fix deleted. On #318 it three times decided the outcome:

- moving a nil check after `go e.worker()` failed the test (`expected: 0, actual: 2`) — proving the test caught ordering, not just the error
- a rewritten assertion was re-verified to still catch that mutant
- deleting the nil-`db` guard caused **zero** test failures, exposing that it was unpinned

If you add a guard, delete it and watch a test fail. If nothing fails, the test is decoration.

**Beware your own enumerations.** The previous session made this error twice, and both times a subagent confirmed it back:

- grepped `\.DeletedAt` requiring "variant" on the same line — could never match `v.DeletedAt`
- enumerated `^func (e *Emitter)` in `emitter.go` alone — the package had **eleven** methods across four files, and the one that mattered most was in a file never opened

**A subagent agreeing with your finding is not independent verification of it.** State findings as "verified by X" so a reviewer can check the method, not just the conclusion.

---

## Process that worked

Subagent-driven execution (`superpowers:subagent-driven-development`), one plan per issue in `docs/superpowers/plans/`, a fresh implementer per task, and a scoped review after each.

What made the reviews earn their cost:

- **Scope the review to the user-visible property, not the brief.** "Does a removed variant still reach a customer" caught a test that skipped deleted variants and would have passed either way. "Does the diff match the brief" would have passed it.
- **Name the load-bearing question explicitly** in the review prompt, and tell the reviewer which claims you have already verified so it spends its budget elsewhere.
- **Tell implementers to escalate rather than coerce.** On #318 one hit a contradiction between the audit and the compiler, refused to paper over it, did not commit a red build, and escalated — which is how the eleven-methods error surfaced.
- **Regenerate task briefs after any plan edit.** Inserting tasks mid-plan leaves stale briefs off by one; one subagent correctly followed the plan over its own brief and flagged it.

**One failure mode to watch:** a fix subagent completed its work across nine files, then stalled without committing or writing a report — its final output was *"I'll stop issuing filler commands and wait for the monitor notification"*. Uncommitted work with no report is indistinguishable from a task that never ran. If a subagent returns something that is not a status, check the worktree before re-dispatching.

---

## Context you may need

- **Concurrent streams exist.** Another session worked #414 in this repo on 2026-08-28. Check `git worktree list` and the current branch before assuming the checkout is yours.
- **#420** was filed and is open: `internal/wishlist/repository.go:66-67` computes a product's min/max price from `product_variants` with no `deleted_at` filter, so a removed cheap variant still sets the customer-visible "from" price. Sibling of #395; raw SQL, so #395's model change does not reach it.
- **Outstanding elsewhere:** tesserix-home#407 (federation registry — mark8ly's shipped endpoints are invisible to the console until `FEDERATION_MARK8LY_ENDPOINTS` is set) and the #305 note about the console inheriting four lists rather than one table.
- **`docs/admin-conformance.md`** documents the conformance declaration, the closed nine-id vocabulary, and why seven mounted mark8ly reads cannot be declared. Worth reading before touching anything on the platform-admin surface.

---

## Start here

1. Read the issue. Then verify its central claim at the source before planning from it.
2. Create a worktree and a branch.
3. Write a plan under `docs/superpowers/plans/`, with an explicit Global Constraints section and a findings section recording what you verified and how.
4. Execute subagent-driven, reviewing after each task.
5. Mutation-test every guard you add.
6. Open a PR that says what was actually wrong, not what the issue said was wrong.
