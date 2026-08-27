# Takeover prompt — Platform integration (session 10)

Paste everything below the line into a new session.

> Supersedes `2026-08-27-platform-integration-takeover-s9.md` on `feat/348a-email-send-log`.
> **Delete s9 rather than leaving it** — the same instruction s9 gave about s8, for the same reason:
> these are written to be trusted wholesale, so a stale one is worse than none.

---

You are taking over work in `tesserix/mark8ly` (repo root
`/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, Go + Next.js; nearly all of this lives in
`services/marketplace-api`, with `services/platform-api` in the same repo).

Read first: `docs/superpowers/2026-08-23-platform-integration-handoff.md` (traps 1-15, conventions,
environment). Then this document. Then the issue you pick.

## THE DEPLOY PIPELINE — both faults are now fixed, read them as case studies

Session 9 found two faults and fixed neither. Session 11 fixed both, and CORRECTED THE DIAGNOSIS OF
EACH ONE — in the second case the prescribed action would have destroyed a working check. Read both
before you trust any other diagnosis in this document; they are the two best worked examples here of
a failure that reports nothing.

**Outstanding:** `tesserix/tesserix-workflows` PR #6 is MERGED as `92e48afa`. mark8ly still pins
`@v2.2.0`, which is content-identical but is NOT an ancestor of that repo's `main` — the squash
orphaned it. Cut `v2.2.1` at `92e48afa` and repin mark8ly (`ci.yml`, `mobile-admin-build.yml`, and
`RELEASE_REF` in `.github/scripts/test-ci-contract.py` — the contract test fails if you miss it).

### 1. Cache-hit builds published a stale `created`, and Kargo silently stopped deploying — FIXED

> **Corrected and fixed in session 11.** The session-9 diagnosis below the line was directionally
> right and wrong in one detail that mattered. Left in place so the correction is legible.

**What was actually wrong.** BuildKit inherits the image config's `created` field from cache. On a
full cache-hit build the exported config keeps the ORIGINAL timestamp, so a commit that changes no
code for a given image — a base-image repin, a dependency bump, the shape of nearly all security
patching — publishes a NEW DIGEST ADVERTISING AN OLD BUILD TIME.

Reproduced locally in three builds:

| build | cache | `created` |
|---|---|---|
| a (cold) | — | `2026-08-27T16:06:02Z` |
| c (full cache hit, 2 min later) | hit | `2026-08-27T16:06:02Z` — inherited, stale |
| b (full cache hit, `SOURCE_DATE_EPOCH=1700000000`) | hit | `2023-11-14T22:13:20Z` — honored |

**Session 9 said the baked timestamp "disagreed with reality". The sharper fact: for `auth-bff` and
`platform-api` FIVE DISTINCT DIGESTS all reported the identical `created` of
`2026-08-26T05:40:42Z`.** It was not a wrong ordering, it was a TIE, and Kargo's `NewestBuild` broke
it lexically — which is how `main-ffda99c` outranked `main-58344a9`. That distinction is why
"refresh the warehouse" could never have worked: discovery was correct, and so was the ranking, given
the input it was handed.

**No Kargo-side strategy could fix this.** Tags are `main-<sha>`, so `Lexical` and `SemVer` are
meaningless, and `Digest` needs a mutable tag CI does not push. The fix had to be in the build.

**The fix.** `SOURCE_DATE_EPOCH` set to the commit's COMMITTER DATE — identical across a rerun of the
same commit, strictly increasing along `main`. It overrides the inherited value on a cache hit, and
two builds differing only in `SOURCE_DATE_EPOCH` produce IDENTICAL LAYER DIGESTS, so it costs no
cache hits and no extra push bandwidth (verified, buildx v0.35).

It lives in the SHARED workflow, `tesserix/tesserix-workflows` →
`.github/workflows/container-release.yml`, released as `v2.2.0` with a contract test
(`test_published_images_carry_a_build_time_that_moves`) that fails against the pre-fix workflow.
mark8ly pins it in `3d0831d3`.

**Trap: the warehouse config is in a THIRD repo.** Not `tesserix-k8s`, as session 9 guessed. The
`services` Warehouse is `tesserix/kargo-manifests` → `projects/mark8ly/warehouses/services.yaml`,
served by the ArgoCD app `kargo-project-mark8ly`. Find it from the cluster, not by grepping repos:

```bash
kubectl get application kargo-project-mark8ly -n argocd -o jsonpath='{.spec.source}'
```

**Trap: `tesserix-workflows` tags are immutable.** A ruleset on `refs/tags/v*` blocks BOTH `deletion`
and `update`. I pushed `v2.2.0` before confirming `main` was reachable, then could not retract it —
and `main` requires linear history AND a PR AND an approving review, so only a SQUASH merge is
possible, which orphans the tag. Cut the tag only AFTER the squash lands.

<details>
<summary>Original session-9 text, superseded</summary>

Every subscription uses `imageSelectionStrategy: NewestBuild`, which ranks by the OCI `created`
timestamp baked into the image manifest rather than registry push time. For `mark8ly-otto`,
`mark8ly-auth-bff` and `mark8ly-platform-api` — the only services in the `58344a9` merge with no code
changes — the baked timestamp disagreed with reality, so the new build never ranked first. Refreshing
the warehouse did not help. Likely fix: `Digest`, or `SemVer`/`Lexical` with a tag constraint.

</details>

### 2. The admin-conformance CronJob is NOT an orphan — it found a real bug

> **Session 9 got this wrong and the wrong action was destructive.** It said the CronJob was an orphan
> of `d52b5cd2` (#383) and told you to remove it from the manifests. Do not.

The manifest's own header says the opposite of "orphan"
(`tesserix-k8s` → `charts/apps/mark8ly-marketplace-api-admin/templates/admin-conformance-cronjob.yaml`):
#383 dropped the GitHub Actions job because Cloudflare serves a bot-challenge page to GitHub-hosted
runner IPs, making every endpoint return `<!DOCTYPE html>` instead of JSON. The CronJob is the
DELIBERATE IN-CLUSTER REPLACEMENT. It reaches the ClusterIP directly with no edge in the path, and it
mounts the existing Kubernetes HMAC secret instead of a GitHub copy nothing would rotate.

**It was failing because it was right.** Read the pod logs before touching the manifest:

```
3 failing checks
  entities/tenants  §8.9  data[0] (id "78137b54…") has no label; §8.9 requires it on every row
```

`/admin/entities/tenants` served `id, name, owner_email, status, created_at` and no `label`. §8.9
requires a non-empty `id` AND `label` on every entity row; an empty label renders as a blank line the
operator cannot click. Fixed in `c701f82f` — `label` with a `name → owner_email → id` fallback so an
unnamed tenant still yields something non-empty.

**The proof it is a working detector, not an orphan:** `kora-api-admin-conformance` is the same job on
the same schedule in the same cluster, and it COMPLETES. mark8ly was simply the non-conformant product.

**The lesson worth more than the fix.** A red tile on a check you did not write reads as "the check is
broken". Here, deleting it would have destroyed the only thing that noticed a contract violation
already shipping to the console. Read the failing output BEFORE concluding a failing check is stale.

## READ THIS FIRST — three things that will cost you an hour each

**1. You cannot create pull requests.** `gh pr create` fails with
`GraphQL: Unauthorized: As an Enterprise Managed User, you cannot access this content (createPullRequest)`.
This is diagnosed; do not re-diagnose it. General `gh` writes DO work — issues, comments, edits,
closes, labels, milestones all succeeded repeatedly this session. Only PR creation is refused, at the
account level. **Do not retry more than twice.** Either push the branch and hand the user a
`compare/main...<branch>?expand=1` URL with the body in a file, or merge locally and push — both were
done this session and both are acceptable.

**2. When a PR is opened by hand, it will have an EMPTY body.** Four of five PRs today. Restoring one
after merge is routine now: `gh pr view <n> --json body --jq '.body|length'`, then
`gh pr edit <n> --body-file`, then read it back. **A local merge sidesteps this entirely** — no PR
body, no lost `Fixes #` keyword. That is a real argument for merging locally when the record matters.

**3. `pg_constraint` with a `contype` filter will lie to you.** This cost four separate defects in one
plan. The complete query, with no filter at all:

```sql
select contype, conname, pg_get_constraintdef(oid)
from pg_constraint where conrelid='<table>'::regclass;
```

`contype in ('c','f','u')` misses `x` (EXCLUDE). `pg_indexes` misses EXCLUDE too — it shows as an
ordinary non-unique index. I widened my query twice and still missed a category. **Before writing any
test that INSERTs into a table, check columns AND CHECK constraints AND unique indexes AND foreign
keys AND exclusion constraints.**

## State of the world

**`main` is `3d0831d3`. CI is green. Production schema is 106, not dirty.**

**Deploy was 5/8; `3d0831d3` is the first build carrying the timestamp fix.** That commit changes no
code in `auth-bff`, `platform-api` or `otto`, so all three are cache-hit builds — the exact shape that
produced the stale timestamps. If they still lag after one 5-minute Kargo discovery interval, the
diagnosis in section 1 is wrong and should be reopened, not worked around.

**`GET /admin/inbox` is merged but NOT MOUNTED.** `internal/inbox` and the handler are on `main`
(`04ef8c3d`), `Deps.Inbox` is nil, so the route 404s. **Wiring it in `cmd/marketplace-api/main.go` is
what closes #280.** That was deliberate: keeping it unwired is why six Important defects found in the
whole-branch review were cheap to fix rather than a production incident.

**A digest you pin can be deleted beneath you.** This broke `main` this session. I dispatched
`weekly-rebuild.yml` in `tesserix/base-docker-images` to clear an openssl CVE; it published new
versions; registry retention then evicted the older `base-go-builder` / `base-distroless-static`
digests that mark8ly still pinned, and all four Go image builds failed with `not found`. Fixed by
repinning to `20260827` (`58344a9e`) and closing four stale Dependabot PRs (#385-#388). The evidence
is on `tesserix/base-docker-images#23`.

**Production still has 3 stores and ZERO `store_subscriptions`.** Unchanged from s9. Check this before
treating any billing issue as urgent.

## The invariant everything rests on — do not break it

**`email.Client.Send` returns `nil` if and only if a provider accepted the message.** Every
`*_sent_total` counter's honesty depends on it. `ErrUndeliverable` is produced only by
`ValidateRecipient`, before any network I/O. If you touch `internal/email`, re-read
`template_client.go`'s contract comment and keep it true. Validation lives in the client, never in the
crons; test doubles must honour the real client's contract.

**New this session, and the same shape:** a provider in `internal/inbox` must **return** its error,
never swallow it and return an empty slice. The aggregator distinguishes "this queue is empty" from
"we could not reach this queue" solely by whether the provider errored. `onboarding_test.go` pins it.

## What shipped in session 9-10

- **#391 (`30c3fdff`)** — unstuck `main` and a day-old deploy. openssl CVE-2026-14456 in the Node
  bases; nothing in mark8ly was misconfigured, the bases were last built before Alpine published the
  fix.
- **#403 (`a0cca46f`)** — integration fixture repair. **187 → 25 failing tests, 162 fixed, 0
  regressions**, `make test-int` from 4 packages to 20. Closed #316, #317.
- **`04ef8c3d`** — `GET /admin/inbox` (#280). Five providers, aggregator, handler. 28 tests. Route
  unmounted.
- **`58344a9e`** — repinned Go bases after the digest eviction.

## Issues: 60 open, 14 filed this session

**Ten production defects, all found by relighting the dark test suite** — #394-#402 plus a comment on
the pre-existing #318. In severity order:

| Issue | Why it matters |
|---|---|
| **#394** | `page.Published` / `category.IsActive` are non-pointer `bool` with `gorm:"default:true"`. GORM omits zero-valued fields carrying a `default:` tag, so `false` **can never be persisted** — unpublished pages are served to customers. |
| **#396** | The tax revalidation cron deadlocks and **has never run successfully**. It also poisons every package that runs after it under `-p 1`. |
| **#395** | `Variant.DeletedAt` is `*time.Time` not `gorm.DeletedAt` — soft-deleted variants leak through all five `Preload("Variants")` sites. |
| **#397** | A blocked downgrade's audit row is written then rolled back with its own transaction. |
| **#398** | Merchant appeal text overflows `varchar(100)`; appeals are not recorded. |
| **#399** | `ApplyTrialRamp` re-inflates already-consumed budget. |
| **#400** | `OptionValueInUse` unreachable via one path, reachable via another. |
| **#401** | `testdb.NewDB` truncates only its named tables — cross-package pollution. |
| **#402** | `Product.VendorID` is `*string` against a NOT NULL column with no FK, so the type guards nothing. |
| **#404, #405** | Break-glass and outbox **write** operations, split out of #260 — each had been half-satisfied by a read-only endpoint. |
| **#406** | `platform-api`'s `ListSessions` hardcodes `ORDER BY created_at DESC`; the inbox consumer needs the oldest rows. |

**Milestones.** `Platform integration v1`: 5 open (#278, #280, #281, #333, #348). #333 is
`blocked:console` — the capability vocabulary is unsettled and the codebase says so at
`platformadmin/middleware.go:36-51`. **`Billing & subscriptions — parked for console`: 7 open**, all
`blocked:console` — product and subscription setup moves to the console as single source of truth
driving Stripe. Do not start those.

**Every open issue now carries an `area:*` label and a type.** 28 were unlabelled before.

## What to pick next

1. **The two pipeline bugs above.** Both diagnosed, both silent-failure classes.
2. **Wire `Deps.Inbox`** in `main.go` — closes #280, makes 28 tests worth of code reachable. Read
   #406 first; the ordering gap becomes operator-visible the moment the route mounts.
3. **#394** — the only customer-visible defect in the set, and the fix is small. Audit every other
   model with the same `bool` + `default:` shape; nothing guarantees these two are the only ones.
4. **#281** — unblocked, and **only part (a) remains**. Part (b) turned out to be already done and
   shipped; `main.go:2148` mounts the route and names #281 as the reason. Its route shape changed to
   `POST /admin/inbox/{kind}/{id}/actions/{actionId}` — recorded on the issue.

## Rulings I made — disagree freely

- **Merged twice directly to `main`** rather than waiting on PRs, because PR creation is blocked and
  the user asked for merges. Cost if wrong: two changes bypassed PR review.
- **Left #280 open** after merging its implementation. Its acceptance criteria describe endpoint
  behaviour and the route is unmounted; closing it would put a checkmark on behaviour nobody has
  exercised.
- ~~**Did not fix the two pipeline bugs.**~~ Session 11 judged this wrong and fixed both. A silent
  deploy failure does outrank tidiness about scope, and the second "bug" was a working check whose
  prescribed fix was to delete it — the cost of deferring was not zero, it was a wrong instruction
  left standing in a document written to be trusted wholesale.
- **Pushed a release tag before confirming the branch could land** (session 11, `v2.2.0` on
  `tesserix-workflows`). A ruleset made it undeletable and the repo only allows squash merges, so the
  tag is permanently orphaned. Cost: cosmetic. Lesson: tag AFTER the merge, never before.
- **`Count` for `onboarding_stalled` saturates at 500** rather than being accurate. A total that is
  bounded and explicable beats one that grows as an operator pages forward.
- **Erasure requests get no derived `due_at`** despite GDPR's 30-day window, because the table has no
  due column and deriving a statutory deadline in a read endpoint invents policy in the wrong place.
- **`actions` are derived from item state, not capability**, following #287's precedent of declining
  to invent capability names. This keeps the vocabulary blocker costing one issue (#333), not two.

## Traps

Carried and still live: read back every `gh` write (16) · `Closes #a, #b, #c` closes only `#a` (17) ·
agents' reported identifiers can be wrong-but-plausible (18) · a `-run`-scoped run hides everything it
does not name (19) · `exit=0` does not distinguish PASS from SKIP (20) · a validation pre-check can
*move* an abort rather than remove it (22) · two integration suites against one database corrupt each
other (23) · a subagent that backgrounds a long job cannot wake itself (24) · the working tree can move
under you (26) · a baseline run against an older commit is contaminated once the shared DB moves ahead
(27) · `comm` against a deleted baseline prints nothing and looks like success (28) · **never push or
commit while a subagent is running (29)** · `gofmt` breaks when you insert a comment into an aligned
struct (31) · a test can be silently disarmed by a refactor (33) · `testdb.NewDB` SKIPS when the
database is unreachable (34).

New, all earned this session:

- **Trap 35 — a FAILED image-publish job does not mean the image was not published.**
  `container-release.yml` pushes first and scans second. Check the registry before saying a bad build
  "did not ship". See `tesserix/tesserix-workflows#5`.
- **Trap 36 — `git merge-base --is-ancestor` reports every squash-merged branch as unmerged.** Wrong
  test for "safe to delete", and it fails in the *safe* direction so it is easy to trust. Correct
  test: PR is `MERGED`, remote tip equals `headRefOid`, and `git diff origin/<branch> origin/main --
  <files it touched>` is empty.
- **Trap 37 — Kargo promotes the newest image each app *has***, which can be an older, gate-failing
  commit. A green gate does not mean every workload is on the green commit.
- **Trap 38 — a red CI gate can be rooted three repos away.** When CI fails on an OS-package CVE the
  question is "when was the base last built", not "what did we change".
- **Trap 39 — a declared `workflow_dispatch` input may be wired to nothing.**
  `weekly-rebuild.yml` takes an `images:` subset and ignores it; the matrix is hard-coded.
- **Trap 40 — macOS friction.** `timeout` does not exist; zsh globs `custom-columns=…[0]…` unless the
  whole `-o` argument is quoted; `sed -i` needs the empty `''`.
- **Trap 41 — a pinned digest is not permanent.** Registry retention can delete it beneath you, so
  triggering a base rebuild is a change to *every* consumer of that base, not only the ones currently
  red. Four Dependabot PRs were telling me this and I classified them as unrelated noise.
- **Trap 42 — a full `./...` integration run reports fiction.** Cross-package pollution invents
  failures that do not reproduce; 7 of 19 "failing" packages passed on a clean database. **TRUNCATE
  every table between packages or do not believe the number.** (#401.)
- **Trap 43 — `NewestBuild` ranks by the OCI `created` timestamp, not registry push time.** A
  cache-hit build inherits `created` from cache, so it can TIE with older images and lose the
  lexical tie-break, and simply never deploy, with no error anywhere. Fixed at the source in
  `tesserix-workflows` `container-release.yml` (`92e48afa` on `main`) via `SOURCE_DATE_EPOCH`;
  see section 1 for the tag caveat.

- **Trap 44 — a failing check you did not write is not therefore a stale check.** The
  admin-conformance CronJob was documented as an orphan to delete. It was the deliberate replacement
  for a dropped workflow, and it was red because it had found a real contract violation. READ THE
  FAILING OUTPUT before concluding a check is obsolete. The identical job on a sibling product was
  passing the whole time, which is the cheapest possible disproof and took one `kubectl get jobs`.

- **Trap 45 — tag AFTER the merge, never before.** `tesserix-workflows` has a ruleset on
  `refs/tags/v*` blocking `deletion` AND `update`, `main` requires linear history plus a PR plus an
  approving review, and the repo only allows SQUASH merges. A tag pushed at a branch head is
  therefore permanently orphaned by its own merge. Also: `enforce_admins` is on, so `gh pr merge
  --admin` does NOT bypass `require_last_push_approval` — the merge needs admin enforcement lifted
  and restored around it.

## Process that worked, and is worth repeating

**The whole-branch review keeps finding what task-scoped review cannot, and it is not close.** On the
fixture branch, 13 task reviews came back clean and the branch-wide pass found two Importants. On the
inbox branch, seven task reviews came back clean and the branch-wide pass returned **do not merge** —
six Importants, four of them in one 82-line file, findable only by reading `platform-api` code that
was not in the diff. Budget for one on the most capable model, every branch. It has never once been
wasted.

**Require regression tests to fail before the fix, and demand the failing output.** A test written
after a fix, which has never once failed, proves only that it compiles. Two real bugs this session
were pinned this way.

**Most findings were defects in my plans, not the implementations.** On the inbox plan the count was
seven to zero. Every one was caught by an implementer or reviewer speaking up rather than working
around it silently. **Write plans expecting this, verify every subagent claim, and read reviews
critically** — one review classified a missing tenant filter as a "non-blocking observation" because
the remote API cannot filter by tenant; the *response* carried the tenant, so the filter silently did
not filter. Escalating it found a real bug.

**Fix the class, not the instance.** I escalated a missing `TenantID` filter and fixed only that
field; `Filter.Status` had the identical defect in four providers and I did not look. The
whole-branch reviewer found it and said "I'm applying your own precedent, not a new standard."

**Never `git add -A` while a subagent is live.** I did it twice today — hours apart, having recorded
the lesson in between — and swept implementer files into commits labelled `docs:` both times. Stage
explicit paths. A lesson in a ledger does not change behaviour; a habit does.

Per piece: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development`. Every dispatch prompt must
carry an explicit prohibition on pushing, opening PRs, merging and deploying — **and on dispatching
subagents**, which I omitted once and got a duplicate review seat I could not vouch for.

## Environment

- LAN IP `192.168.1.110`, never `localhost`. `-p 1` on integration runs.
- **Use an isolated container, never the shared dev DB** — another session uses `dev-postgres-1`:
  ```bash
  docker run -d --name m8-db -e POSTGRES_USER=dev -e POSTGRES_PASSWORD=dev \
    -e POSTGRES_DB=marketplace_db -p 55435:5432 postgres:15
  docker exec m8-db psql -U dev -d marketplace_db -c 'CREATE EXTENSION IF NOT EXISTS pgcrypto;'
  cd services/marketplace-api && DATABASE_URL='postgres://dev:dev@192.168.1.110:55435/marketplace_db?sslmode=disable' go run ./cmd/migrate up
  ```
  `pgcrypto` must be enabled manually — migration 058 uses `gen_random_bytes`. Verify schema **106**,
  `dirty=f`. Ports 55432-55434 were used this session; pick a free one.
- **Never run `./internal/billing/tax/revalidation/...`** — it deadlocks (#396) and hangs for the full
  test timeout, and under `-p 1` it poisons every package after it.
- Migrations: `DATABASE_URL=... go run ./cmd/migrate up` from `services/marketplace-api`. No
  `-database` flag. Bookkeeping table is **`marketplace_db_schema_migrations`**. Next free number is
  **000107**; bumping one means bumping `ExpectedSchemaVersion`, which a test guards.
- Production Postgres: select the primary BY ROLE, never by pod name —
  `kubectl get pods -n mark8ly -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary -o jsonpath='{.items[0].metadata.name}'`
- The editor LSP may report `go.work requires go >= 1.26.6 (running go 1.26.5)`. The CLI toolchain is
  1.26.6 and matches. Ignore it.
- One GKE cluster and it is production.

## Pre-existing test failures

**`make test-int` (20 packages) is GREEN** — verified three times, twice consecutively on an
already-dirty database. That is the number to trust and the target CI runs.

A full `./...` run is **not** a usable instrument — see Trap 42 and #401. Measured with TRUNCATE
between every package: **25 failing tests across 12 packages** at `a0cca46f`, down from 187/22. All 25
are triaged in `docs/superpowers/plans/2026-08-27-integration-fixture-drift-tail.md`, and most are
product defects rather than fixture drift.
