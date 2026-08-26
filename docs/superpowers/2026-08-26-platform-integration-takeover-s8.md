# Takeover prompt — Platform integration v1 (session 8)

Paste everything below into a new session.

> Supersedes `2026-08-26-platform-integration-takeover-s7.md`. **Delete s7 rather than leaving it** —
> the same instruction s7 gave about s6, for the same reason: these documents are written to be
> trusted wholesale, so a stale one is worse than none.
>
> It also supersedes the unmerged local branch `docs/s7-refresh`, whose content is folded in here.
> **That branch can be deleted unpushed.**

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, Go + Next.js; `services/marketplace-api`
is where nearly all of this lives).

Read first: `docs/superpowers/2026-08-23-platform-integration-handoff.md` (traps, conventions,
environment) · the latest comment on **#260** · then **#348** and the spec named below.

## READ THIS FIRST — you are not alone in this checkout

**Another session is working in this repository right now.** When this document was written the
shared checkout was on `feat/admin-conformance-ci`, doing **#290** (wiring
`@tesserix/admin-conformance` into CI) — an issue s7 listed as blocked, so that blocker has cleared.
`main` also advanced with **#380** (lifecycle reason codes) without this session's involvement.

**Before you touch `services/marketplace-api`, check what branch the checkout is on and whether it
is dirty.** If someone else is mid-work there, do not switch branches under them. Use a worktree:

```
git worktree add /tmp/<your-workspace> <your-branch>
```

There is already one at `/tmp/m8-348a` for the branch below. Two long-lived worktrees also exist
under `.claude/worktrees/` and are unrelated — leave them alone.

## Your task: #348 piece A — the email send log

`#348` as filed is four subsystems. It is decomposed in the spec; **piece A is the send log and the
transport decorator**, and it is the only piece designed so far.

**Spec: `docs/superpowers/specs/2026-08-26-email-send-log-design.md`** — written, corrected and
committed on the branch below. Read it before anything else; it records four decisions already made
with the user and the reasoning for each, including two that contradict the issue.

**State: spec DONE and approved. Implementation plan NOT written.** Your next step is
`superpowers:writing-plans`, then `superpowers:subagent-driven-development`.

**Branch `feat/348a-email-send-log`, LOCAL ONLY, never pushed.** Head is `3e2b7719`. Its worktree is
at `/tmp/m8-348a`.

### The four decisions already made — do not relitigate

1. **Correlation is our own `send_id`, not the provider's.** The decorator mints a uuid, uses it as
   the row's primary key, and injects it into `Message.CustomArgs`; both providers echo it back
   (SendGrid as `custom_args`, Resend as tags). No change to `Sender.Send`, no call sites touched.
2. **Write before the send, update after, and never block delivery on the log write.** A crash
   mid-send leaves a row at `sending` — distinguishable from "never attempted", exactly like the
   outbox `pending` state from #336. If the log write fails, log loudly and send anyway.
3. **No `subject` column.** #348 asks for one; the spec declines. Subject lines are interpolated
   customer content, and three prior endpoints deliberately excluded exactly that (`message` from
   #332, `description` from #329, `payload` from #331). `kind` answers "which email" and is more
   queryable.
4. **No `provider` column either**, and this one was discovered while designing. `FallbackSender`
   tries SendGrid then Resend, but the decorator wraps the whole chain and sees one `Send` returning
   one `error` — it cannot know which provider accepted the mail without the interface change
   decision 1 rejects. Recording the configured primary would lie precisely during a fallback, which
   is when the answer matters. Piece B fills it in, since provider events identify themselves.

### What piece A is NOT

Attribution needs **no** work. Exactly five files construct an `email.Message`
(`ticket`, `giftcard`, `orderdoc`, `shipping/labelmailer`, `campaign/email_dispatcher`) and **all
five already set `kind`**. An earlier draft of the spec claimed seven mailers were unattributed; that
was wrong and the spec records why. Piece A is a table plus a decorator, touching no mailer.

## #381 — arguably more urgent than #348, and filed during it

**Dunning notices, the whole trial-reminder cadence, payment-action reminders, win-back and the
trial-billed confirmation have never been sent.** `email.Client` (`internal/email/client.go:33`) has
exactly one implementation — `NoOpClient`, which logs and returns nil — wired at
`cmd/marketplace-api/main.go:1599`, `:1764` and `:1879`.

Two details make it worse than a missing adapter:

- **`DunningEmailsSentTotal` increments *after* the no-op returns** (`dunning_emails.go:127-130`), so
  dashboards show dunning working. Same family as the dead outbox gauge fixed in #375 — but that one
  read zero, and this one reads a plausible non-zero, which is worse.
- **The recipient argument is a store UUID, not an email address** (`dunning_emails.go:115-120`,
  awaiting a `StoreSubscription.email` column). Wiring a real adapter tomorrow would send to a UUID.

A merchant whose card fails gets no day-5 notice, no day-7 notice, and no payment-action reminder,
and proceeds toward suspension in silence. **Consider whether this outranks #348 before starting.**

## State

**Milestone: 8 open, 20 closed.** Open: #348, #333, #330, #319, #290, #281(a), #280, #278.

Blocked outside this repo: **#278** (console decision) · **#280** (rationale void) · **#281(a)**
(depends on #280) · **#330** (Otto decision) · **#333** (capability vocabulary — #364 shipped the
mechanism, the console owns the names). **#290 is in flight in another session.**

Genuinely actionable: **#348** (in progress, this document) and **#319** (OpenBao credentials —
opens with a transport decision: direct via the service's own Kubernetes SA token, or a machine API
`secret-service` does not yet have, since every route there sits behind `RequireSession` + CSRF).

Plus **#381**, outside the milestone, above.

### Unpushed local branches — decide their fate

| branch | state |
|---|---|
| `feat/348a-email-send-log` | **local only**, head `3e2b7719`, the spec + its correction. Your working branch. |
| `docs/s7-refresh` | **local only**, head `222bdf70`. Superseded by this document — **delete it**. |

`docs/s7-takeover` and `feat/331-admin-outbox` are done: merged as `e8fc6dd7` and `1e28e44c`.
Note `git merge-base --is-ancestor` reports them UNMERGED, because this repo **squash-merges** — the
branch tip is never an ancestor of `main`. Check the PR state, not the ancestry.

## What shipped in session 7

- **#336 → PR #373 (`0fc2763f`)** — the outbox publisher no longer records a dropped event as
  published; three derived states; `/admin/health` counts `errored` separately and degrades on it.
  **Verified in production**, full loop including recovery — see the comment on #336.
- **#374/#375/#376 → PR #377 (`11b5f330`)** — the last poison-pill causes and the dead outbox
  metrics.
- **#331 → PR #379 (`1e28e44c`)** — `GET /admin/outbox`, the cross-tenant read.

## Traps

Carried from s7 (still live): read back every `gh` write (16) · `Closes #a, #b, #c` closes only `#a`
(17, and it recurred after being written down) · agents' reported identifiers can be wrong-but-
plausible (18) · a `-run`-scoped test run hides everything it does not name (19) · `exit=0` does not
distinguish PASS from SKIP (20) · deleting a metric can break a CI check in another repo (21) · a
validation pre-check can *move* an abort rather than remove it (22).

New:

- **Trap 23 — two integration suites against one database corrupt each other, and the diff still
  looks authoritative.** The branch-vs-baseline comparison runs the full suite twice against the same
  dev Postgres. A verification agent and the controller each started one; both results were
  discarded, every `go test` process killed, the database confirmed clean, and one run redone. Check
  before starting: `ps aux | grep "go test -tags=integration"`.
- **Trap 24 — a subagent that backgrounds a long job cannot wake itself to collect it.** One parked
  twice reporting "waiting for it to finish" and ending its turn; its runs were fine. Drive long
  verification from the controller, and before nudging, check the job is genuinely alive
  (`ps -p <pid>`) rather than assuming it is wedged.
- **Trap 25 — grep the TYPE, not the method name.** "Who uses this interface?" answered with
  `grep '.Send(ctx'` produced a confident, wrong answer that reached an approved spec: it matched
  Slack, a campaign dispatcher and an unrelated template client. `grep 'email.Message{'` — the type
  actually constructed — gave the real set of five. A method name is shared by every interface that
  happens to use it.
- **Trap 26 — the working tree can move under you.** Another session switched branches and advanced
  `main` mid-task. Nothing was lost, but a `docs/...` path silently stopped existing. If a file you
  just wrote is "not found", check `git branch --show-current` before concluding anything else.

## Environment

- LAN IP `192.168.1.110`, never `localhost`. `-p 1` on full integration runs.
  `go vet -tags=integration ./...` is the only thing that compiles build-tagged files.
  `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable`
- **The dev Postgres is the container `dev-postgres-1`** (binding `0.0.0.0:5432`). After a Docker
  restart it can be left `Exited` while other projects' postgres containers come up healthy on other
  ports — `5432` refused while `docker ps` looks fine is the tell. Fix: `docker start dev-postgres-1`.
- **Select the Postgres primary BY ROLE, never by pod name** — CloudNativePG rotates them:
  ```
  PGPOD=$(kubectl get pods -n mark8ly \
    -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary \
    -o jsonpath='{.items[0].metadata.name}')
  ```
  `psql` is not installed locally; `docker run --rm postgres:15 psql …` works.
- The editor's LSP may report `go.work requires go >= 1.26.6 (running go 1.26.5)`. The CLI toolchain
  is 1.26.6 and matches. Ignore it; it is not a regression.
- Deploys: CI → ghcr → Kargo (polls every 5 min) → ArgoCD, images as `main-<sha7>`. Check the
  specific container's tag, not a substring of a concatenation.
- One GKE cluster and it is production.

## Pre-existing test failures — not yours, do not fix, do not let them mask yours

Measured at `e8fc6dd7`: **22 packages / 191 tests failing.** PRs #377 and #379 were each verified
against that baseline in both directions and added zero. **Never call a failure pre-existing without
diffing against a throwaway worktree of the base commit, in both directions.**

## Process

Per piece: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development`.

**The whole-branch review keeps finding what task-scoped reviews cannot.** On #373 it found the
requeue-contract defect; on #377 it found a fourth poison-pill shape; on #379 it found that nothing
proved the read was cross-tenant. None lived in a single task's diff. Budget for one on the most
capable model, every branch.

A recurring theme worth naming: across three branches, most findings were not wrong code but
**documentation promising more than the code enforced** — an untested `CASE` precedence, a "one
instant" invariant that was not held, a comment claiming a defect class was closed when one shape
remained. When you write a comment asserting a property, add the test that makes it true.

Every dispatch prompt must carry an explicit prohibition on pushing, opening PRs, merging and
deploying unless that is the task. Verify what an agent reports. Ask before merging anything.
