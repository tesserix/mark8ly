# Takeover prompt — Platform integration (session 9)

> **Updated 2026-08-27, late session 9.** `main` went red and the deploy sat stuck for a day;
> that is fixed and everything through #391 is live. The sections below marked *(updated)* were
> wrong before this revision — re-read them rather than trusting a remembered version.

Paste everything below the line into a new session.

> Supersedes `2026-08-26-platform-integration-takeover-s8.md`. **Delete s8 rather than leaving it**
> — the same instruction s8 gave about s7, for the same reason: these are written to be trusted
> wholesale, so a stale one is worse than none.

---

You are taking over work in `tesserix/mark8ly` (repo root
`/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, Go + Next.js; nearly all of this lives in
`services/marketplace-api`).

Read first: `docs/superpowers/2026-08-23-platform-integration-handoff.md` (traps 1-15, conventions,
environment). Then this document. Then the issue you pick.

## READ THIS FIRST — three things that will cost you an hour each

**1. You cannot create pull requests.** `gh pr create` fails with
`GraphQL: Unauthorized: As an Enterprise Managed User, you cannot access this content (createPullRequest)`.
This was diagnosed properly, so do not re-diagnose it: general writes DO work (an issue-comment probe
succeeded), the active account is `mahesh-sangawar` with `repo` scope and `admin: true` on the repo,
`viewer` resolves correctly, and both GraphQL and REST are refused identically. It worked once
(PR #384) and then stopped. **Do not retry more than twice.** Push the branch, hand the user a
`compare/main...<branch>?expand=1` URL and the body in a file, and ask them to open it.

**2. When the user opens the PR by hand, it will have an EMPTY body.** *(updated)* This is not a
one-off — it has now happened on **three of the last four PRs**: #390 (no `Fixes #<issue>` keyword,
so the issue did not auto-close), #389 (found empty a day later, during this session), and #391
(17 characters — a truncated fragment of the title). All three have been restored.

Treat it as certain, not likely. **Always check `gh pr view <n> --json body --jq '.body|length'`
after they open it**, restore with `gh pr edit <n> --body-file`, read the length back, and close the
issue yourself with a comment if the keyword never landed. Write the body to a file *before* handing
over the compare URL, so restoring is one command.

**3. Another session shares this checkout, and the shared dev database is ahead of old commits.**
Before touching `services/marketplace-api`, check the branch and whether the tree is dirty. Use a
worktree: `git worktree add /tmp/<name> -b <branch> origin/main`. Two long-lived worktrees under
`.claude/worktrees/` are unrelated — leave them alone.

## What shipped in session 8-9

Three PRs, all merged, all verified on `main`:

- **#384 (`1dc04a87`) — #381: billing email actually delivers.** `email.Client` had exactly one
  implementation, a no-op returning `nil`, so no merchant had ever received a dunning notice, trial
  reminder, payment-action reminder, win-back promo or trial-billed confirmation — while Prometheus
  counters reported success. Now: a real `templateClient` over the existing SendGrid→Resend chain,
  `store_subscriptions.email` (migration 104) written by the `customer.updated` webhook plus
  `cmd/backfill-email`, eleven templates on the shared loader, and `billing_email_sends` (migration
  105) giving dunning and win-back the claim-first idempotency they lacked.
- **#389 (`394ac0c3`) — three follow-up defects.** Claims are released on `ErrUndeliverable` only;
  `payment_action_reminders.subscription_id` renamed to `store_id` (migration 106); the provider HTTP
  call moved out of the advisory-locked transaction into a new `internal/postcommit` package.
- **#390 (`04aade5f`) — #360: the 150-day hard-delete sweep.** Nine tables were swept by a `store_id`
  they do not have, aborting the transaction, so the GDPR hard-delete had never completed for any
  store. They are now reached through their parent FK.
- **#391 (`30c3fdff`) — unstick `main` and the deploy.** *(new)* Not a product change: nine `FROM`
  lines in `apps/{admin,storefront,onboarding}/Dockerfile`, repinning the Node bases to the
  `20260827` rebuild for openssl `3.5.8-r0` (CVE-2026-14456). See "State of the world" for why this
  was blocking #389 and #390 from ever reaching production.

## The invariant everything rests on — do not break it

**`email.Client.Send` returns `nil` if and only if a provider accepted the message.** Every
`*_sent_total` counter's honesty depends on it. `ErrUndeliverable` is produced only by
`ValidateRecipient`, before any network I/O. If you touch `internal/email`, re-read
`template_client.go`'s contract comment and keep it true.

Two rules that follow from it, both established by review and both previously violated and reverted:

- **Validation lives in the client, never in the crons.** Callers only classify the error they get
  back. A cron that calls `ValidateRecipient` itself also silently weakens the test that was supposed
  to prove the production path works.
- **Test doubles must honour the real client's contract.** A stub `email.Client` whose `Send` does not
  call `ValidateRecipient` first will let a cron pass a test the real client would fail.

## Decisions already made — do not relitigate

1. **Claims release on `ErrUndeliverable` only.** Address problems are recoverable (the backfill or a
   `customer.updated` webhook may supply one later). Transport failures keep the claim burned:
   at-most-once is deliberate, because a duplicate billing email is worse than a missed one.
2. **The trial-billed claim happens at drain time, not inside the transaction.** This reverses an
   earlier decision, deliberately. Keeping it inside was correct only while the email could already
   have left on a rollback; once the send became deferred and dropped on rollback, a surviving claim
   only suppressed the retry that should deliver. Duplicate protection now rests on the constant
   `"first_charge"` period key. **There is a 24-line comment in `handlers.go` explaining this — read it
   before "fixing" the claim's position.**
3. **Post-commit work runs on `context.WithoutCancel`** with a 45s per-unit bound. Without it, a commit
   at T+29.5s plus a client cancel at T+30s lost the send outright and permanently.
4. **The nine store_id-less tables are deleted explicitly via their parent, not left to FK cascade.**
   `sweepTable` emits a per-table audit event (`rows_deleted`, severity Critical) that the code calls
   the compliance trail; cascade deletions are not observed by the sweeper, so nine tables would lose
   their deletion evidence in a GDPR path.
5. **`payment_action_reminders` was renamed, not re-keyed.** Changing the inserted value would have
   stranded every existing claim and re-sent reminders merchants had already received.

## State of the world

**Production has 3 stores and ZERO `store_subscriptions`.** This matters more than it sounds: no
merchant is on a billing lifecycle, so nothing sends, nothing is skipped, no idempotency slot is
burned, and `cmd/backfill-email` is currently a no-op. Check this before treating any billing issue as
urgent — I twice warned the user about deploy sequencing before measuring, and the blast radius was nil.

**Deployment is automatic and CURRENT.** *(updated — this section said the opposite and was wrong.)*
CI → ghcr → Kargo (polls ~5 min) → ArgoCD, images tagged `main-<sha7>`. Every workload is on
`main-30c3fdf` and production schema is **106**, not dirty. #384, #389, #390 and #391 are all live.

It got there the hard way, and the shape is worth knowing because it will recur:

- `main` was **red** from 2026-08-26 to 2026-08-27. Kargo will not promote a red commit, so #389 and
  #390 sat unshipped for a day while the doc you are reading claimed they were merely "behind".
- The cause was three repos away. Trivy failed the three Next.js image publishes on CVE-2026-14456
  (openssl `libssl3`/`libcrypto3` `3.5.7-r0` → `3.5.8-r0`). Upstream `node:22-alpine3.24` had **not**
  moved and still ships `3.5.7-r0`; Alpine `v3.24/main` did have `3.5.8-r0`; the base Dockerfiles in
  `tesserix/base-docker-images` already run `apk upgrade`, so a **rebuild** was the entire fix — but
  the bases were last built 2026-08-22, and that job only runs Saturdays 03:00 UTC.
- Fixed by dispatching `weekly-rebuild.yml` manually (it has a fire-now `workflow_dispatch`), then
  repinning the digests here. **If a frontend image fails Trivy on an OS package, check the base
  build date first** — `gh api /orgs/tesserix/packages/container/base-node-runtime-22/versions`.
  Repinning alone cannot help when you already pin the newest base that exists.

Migration **106 must ship with its code** — `ExpectedSchemaVersion` is strict equality, so old
replicas error `column "subscription_id" does not exist` during a rollout and an old pod that
restarts refuses to boot. Roll back both or neither. (This held: 106 applied cleanly when #390
finally rolled.)

**Before the first 09:05 UTC cron after a real merchant exists:** confirm `SENDGRID_API_KEY` or
`RESEND_API_KEY` is set. Without one the client now reports `skipped{reason="no_provider"}` rather than
false success — correct, but nothing is delivered, which is the exact silent failure #381 was about.

**#390 armed a destructive cron that had never successfully run.** A store at `pending_hard_delete` +
150 days is now irreversibly destroyed. Intended, but one-way. *(updated)* It was **not** actually
armed until 2026-08-27, because #390 did not reach production until #391 unblocked the deploy — an
earlier revision of this document asserted it was armed on merge. It is armed now.

## What to pick next

49 issues open. Milestone `platform-integration` has 7: **#348, #333, #330, #313, #281, #280, #278**.
Of those only **#348** is actionable — the rest are blocked on decisions owned outside this repo
(console shape, Otto routing, capability vocabulary, the rationale void), and #313 is itself a decision.

**My recommendation: #316 + #317, the integration-fixture drift.** The baseline is **187 failing tests**,
which taxes every single change: you cannot tell whether you broke something without running the full
suite twice and diffing both directions. #317 is the store-FK / `stores` NOT NULL drift across 17
packages — it produced the `SQLSTATE 23503` failures I had to re-prove pre-existing on three separate
branches. #316 is `products.vendor_id` NOT NULL breaking 54 tests; it bit the #360 test too. Fixing
these ships no merchant-facing behaviour but makes the repo cheaply verifiable for everyone after.

**Strong alternative: the deletion/retention theme.** #360 turned out to be one instance of a pattern,
and two independent "delete matched nothing" bugs were found in one day. Also open: **#259** (GDPR
erasure requests collected and never processed), **#369** (erasure carve-out covers only the purge row),
**#361** (teardown orphans staff FGA tuples and GIP identities). Given the hit rate, auditing the whole
deletion path is likely to surface more.

**#348 piece A** is specced and approved but observes a transport that currently sends nothing. Its
spec is on the branch this document lives on: `docs/superpowers/specs/2026-08-26-email-send-log-design.md`.
It records four decisions including two that contradict the issue (no `subject` column, no `provider`
column) — read it rather than re-deriving.

**#371/#366** — the production Stripe billing key is still in test mode. A launch blocker, but a
human decision.

## Loose ends

| branch | state |
|---|---|
| `feat/348a-email-send-log` | **pushed, unmerged, deliberate.** Holds the #348 spec and this document. Keep until #348 is picked up. Current `main` is merged into it, so the doc sits against today's code. |
| ~~`fix/381-followup-defects`~~ | merged as #389, **deleted from the remote** 2026-08-27. |
| ~~`fix/360-harddelete-sweep`~~ | merged as #390, **deleted from the remote** 2026-08-27. |
| ~~`fix/base-node-openssl-cve-2026-14456`~~ | merged as #391, **deleted from the remote** 2026-08-27. |

**Two issues filed against other repos, both open, neither in this repo's milestones:**

- **`tesserix/tesserix-workflows#5` — `container-release.yml` publishes before it scans.** A red
  Trivy gate reddens the job but the image is already in ghcr under `main-<sha7>` and is fully
  promotable. This is not theoretical: storefront and onboarding ran the CVE-2026-14456 images in
  production for ~2 minutes during promotion. `cosign sign` sits after the scan, so a failed scan
  does leave the image unsigned — but the cluster's Kyverno has no `verifyImages` policy, so nothing
  consumes that signal. Affects every repo on `container-release.yml@v2.1.0`.
- **`tesserix/base-docker-images#23` — the drift workflow cannot open its bump PR.**
  `GitHub Actions is not permitted to create or approve pull requests`;
  `can_approve_pull_request_reviews: false` at **both** org and repo level. Failing daily since
  2026-08-25. It did *not* cause the outage above (upstream's node digest never moved), but it is the
  only automated path that moves a pinned upstream digest forward, and it is dead. The same issue
  records a second defect: `weekly-rebuild.yml` declares a `workflow_dispatch` input `images:` that
  is never referenced, so a dispatch asking for a subset silently rebuilds and republishes all ten.

`gh`'s active account was switched from `Mahesh-Sangawar_civica` to `mahesh-sangawar` during PR
diagnosis, at the user's confirmation. Leave it.

## Traps

Carried from earlier sessions and still live: read back every `gh` write (16) · `Closes #a, #b, #c`
closes only `#a` (17) · agents' reported identifiers can be wrong-but-plausible (18) · a `-run`-scoped
run hides everything it does not name (19) · `exit=0` does not distinguish PASS from SKIP (20) ·
deleting a metric can break a CI check in another repo (21) · a validation pre-check can *move* an
abort rather than remove it (22) · two integration suites against one database corrupt each other (23)
· a subagent that backgrounds a long job cannot wake itself (24) · grep the TYPE, not the method name
(25) · the working tree can move under you (26).

New, all earned the hard way:

- **Trap 27 — a baseline run against an older commit is contaminated once the shared dev DB has moved
  ahead.** Base code at schema 105 against a 106 database produced 14 phantom "fixed by this branch"
  tests. Only *new-in-branch* is trustworthy, because contamination can only ADD failures to base.
  Never report the "fixed" column without checking the error text.
- **Trap 28 — `comm` against a deleted baseline file prints nothing and looks like success.** I cleaned
  up `/tmp` and then "verified" a diff against a file that no longer existed. Always
  `[ -s baseline ] || re-measure` before believing an empty diff.
- **Trap 29 — never push while a subagent is running.** Its experiment may have the tree deliberately
  broken. On #360 the tree had `WHERE store_id = ?` *removed* at the moment a push was requested; a
  push would have published a sweep that deletes across store boundaries. Check
  `git status` and grep for the invariant before any push.
- **Trap 30 — deliberately-broken destructive SQL against the shared dev DB can HANG, not fail.** Do
  counterfactuals on an ephemeral container (`docker run --rm postgres:15`, apply migrations, point
  `TEST_DATABASE_URL` at it). It needs `pgcrypto` enabled manually — migration 058 uses
  `gen_random_bytes`.
- **Trap 31 — inserting a comment into the middle of an aligned Go struct breaks `gofmt`.** It
  re-aligns the preceding fields, and the review will not catch it if you told the reviewer not to run
  tooling. Run `gofmt -l .` yourself before every commit.
- **Trap 32 — a constraint you impose can become wrong when the code changes underneath it.** I
  required the trial-billed claim to stay inside the transaction; once the send moved out, that same
  constraint caused permanent email loss. When a design premise changes, re-check the constraints that
  were derived from it.
- **Trap 33 — a test can be silently disarmed by a refactor.** Moving the send behind a collector meant
  `_NotResentAfterRollback` began exercising the inline fallback, so it would have passed even with the
  bug reintroduced. After any refactor, ask which tests changed *paths*, not just which still pass.
- **Trap 34 — `testdb.NewDB` SKIPS when the database is unreachable.** A clean-looking run may have run
  nothing. Confirm `--- PASS` by name.

Added late in session 9, while unsticking the deploy:

- **Trap 35 — a FAILED image-publish job does not mean the image was not published.**
  `container-release.yml` pushes first and scans second (the step is literally named "Scan
  *published* digest"). I reported three publishes as "failed" and implied nothing shipped; all three
  images were in ghcr the whole time, and two of them later ran in production. Before saying a bad
  build "did not ship", check the registry:
  `gh api /orgs/tesserix/packages/container/<name>/versions`. See `tesserix-workflows#5`.
- **Trap 36 — `git merge-base --is-ancestor` reports every squash-merged branch as unmerged.** It is
  the wrong test for "is this branch safe to delete", and it fails in the *safe* direction, which is
  why it is easy to trust. The right test: the PR is `MERGED`, the remote tip equals the PR's
  `headRefOid` (nothing pushed after the merge), and
  `git diff origin/<branch> origin/main -- <files it touched>` is empty.
- **Trap 37 — Kargo promotes the newest image each app *has*, which can be an older, gate-failing
  commit.** When the gate went green on `30c3fdff`, storefront and onboarding were promoted to
  `main-04aade5` — the commit whose scan had failed — and only reached `30c3fdf` on the next cycle.
  A green gate does not mean every workload is on the green commit. Check pod images, not just
  deployment specs, and re-check a minute later.
- **Trap 38 — a red CI gate can be a deploy blocker rooted three repos away.** mark8ly → its base
  digests → `base-docker-images` → its weekly cron's schedule. Nothing in this repo was
  misconfigured. When CI fails on an OS-package CVE, the question is not "what did we change" but
  "when was the base last built".
- **Trap 39 — a declared `workflow_dispatch` input may be wired to nothing.** `weekly-rebuild.yml`
  takes an `images:` subset and ignores it; the matrix is hard-coded. It reported success while doing
  something broader than asked. Grep for `inputs.<name>` before believing a knob.
- **Trap 40 — macOS shell friction that costs minutes.** `timeout` does not exist (use
  `--request-timeout` on kubectl, or a poll loop); zsh globs `custom-columns=…[0]…` unless you quote
  the whole `-o` argument; `sed -i` needs the empty `''` argument.

## Process that worked, and is worth repeating

**The whole-branch review keeps finding what task-scoped review cannot.** Every one of 13 task reviews
on #384 came back clean; the branch-wide pass then found two Criticals, both cross-task interactions
(a backfill silently resetting win-back timing; a confirmation double-sending on rollback). Same on
#389. Budget for one on the most capable model, every branch.

**Most findings were defects in the plan, not the implementation.** The wrong migrate invocation, test
fixtures violating an FK I had not checked for, a wall-clock idempotency key, a hardcoded store name,
`%w: %v` dropping an error chain, and two of the final review's three must-fixes. Write plans expecting
this, and verify every subagent claim — several reports overstated what they had actually run.

**Prove, do not assert.** The retention bug ("tenant purge has never deleted these rows") and the
cross-store isolation guarantee were both established by observing a real failing run against the
pre-fix code. Reviewers flagged both when the evidence was only an argument. Demand the failing output.

Per piece: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` → `superpowers:writing-plans`
→ `superpowers:subagent-driven-development`. Every dispatch prompt must carry an explicit prohibition
on pushing, opening PRs, merging and deploying. Ask before merging anything.

## Environment

- LAN IP `192.168.1.110`, never `localhost`. `-p 1` on integration runs. Dev DB is container
  `dev-postgres-1`; if `5432` is refused while `docker ps` looks fine, `docker start dev-postgres-1`.
  `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- Migrations: `DATABASE_URL=... go run ./cmd/migrate up` from `services/marketplace-api`. The runner
  reads the env var and has **no** `-database` flag. `make migrate-up SERVICE=marketplace-api` from the
  repo root also works but reads `DATABASE_URL` only, not `TEST_DATABASE_URL`. The bookkeeping table is
  **`marketplace_db_schema_migrations`**, not `schema_migrations`. Next free number is **000107**;
  bumping one means bumping `ExpectedSchemaVersion`, which a test guards.
- Production Postgres: select the primary BY ROLE, never by pod name — CloudNativePG rotates them.
  `kubectl get pods -n mark8ly -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary -o jsonpath='{.items[0].metadata.name}'`
  then `kubectl exec -n mark8ly <pod> -- psql -U postgres -d mark8ly_marketplace_api -tAc "..."`.
  Databases are `mark8ly_marketplace_api`, `mark8ly_platform_api`, `mark8ly_openfga`.
- The editor LSP may report `go.work requires go >= 1.26.6 (running go 1.26.5)`. The CLI toolchain is
  1.26.6 and matches. Ignore it.
- One GKE cluster and it is production.

## Pre-existing test failures — not yours, do not fix, do not let them mask yours

**187 unique failures at `394ac0c3`**, measured both directions. #384 and #389 each added zero and
between them fixed four. *(updated)* The baseline still stands at `30c3fdff`: #390 touched only
`internal/harddelete` and its tests, and #391 touched only three Dockerfiles, so neither can have
moved it — but the dev DB has since been migrated to 106, so re-read Trap 27 before diffing against
anything older. **Never call a failure pre-existing without diffing against a throwaway
worktree of the base commit, in both directions** — and read Trap 27 first, because the diff lies if
the dev DB has moved ahead of the base commit's schema.
