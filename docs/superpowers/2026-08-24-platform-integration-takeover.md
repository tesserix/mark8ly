# Takeover prompt — Platform integration v1 (session 2)

Paste everything below into a new session.

---

You are taking over the **Platform integration v1** milestone in `tesserix/mark8ly`
(repo root `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly`, a multi-service
Go + Next.js workspace).

## Read these first, in this order

1. **`docs/superpowers/2026-08-23-platform-integration-handoff.md`** — the working
   document. Traps, conventions, environment, what each endpoint inherits. Everything
   below assumes you have read it, especially **trap 7**.
2. The umbrella comment on **#260** — what every new endpoint inherits.
3. `docs/superpowers/specs/2026-08-22-platform-admin-surface-design.md` — the foundation.

## Where things stand

**Nine endpoints delivered and live:** #274, #275, #276, #277, #279, #282, #283,
#284, #285.

**Remaining:** #289 (health — the last read), then the writes #281, #286, #287, and
#288 (purge) last because it is irreversible. #278, #280 and #290 are blocked — see
their issues. #319 (OpenBao) is grouped in and goes last of all.

**Start with #289.** Before designing it, check what "dependency health" can honestly
mean here: the surface talks to platform-api through three clients, and an endpoint
that reports on dependencies it never actually exercises would be the purest possible
instance of trap 7.

## The one thing to internalise

The handoff doc calls it trap 7, and it cost more than everything else in this
milestone combined: **a claim is not a guarantee — including your own.**

Eleven instances so far. The three worst were not weak tests but wrong conclusions
about the code:

- `/admin/kpis` counted expiring trials on `current_period_end` because that column
  *looked* right. The rule actually lives in `expiry_cron.go`. The counter was
  **structurally incapable of returning non-zero** and shipped "verified".
- A spec claimed mark8ly holds no prices, on the strength of finding `PriceIDFor`
  and stopping. `internal/billing/pricing/catalog.go` holds them in minor units.
- "There are eight subscription statuses." There are ten. The first eight of a const
  block were read and the conclusion drawn.

All three passed review, passed mutation testing, and in one case passed a production
check. **Mutation testing proves a test constrains the code; it cannot prove the code
asks the right question.** Only comparing a value's rule against whatever else in the
system enforces it can do that — the cron, the merchant-facing endpoint, the DB CHECK
constraint.

And concluding that something **does not exist** requires a search, not a single
lookup. Two of the three above were one `grep` from being caught.

Apply this to your own prose too. Three of the eleven were comments asserting
properties the code lacked — twice in a row on the same line, the second time in the
"fix" for the first.

## Process that works

Per endpoint: `superpowers:brainstorming` → spec in `docs/superpowers/specs/` →
`superpowers:writing-plans` → `superpowers:subagent-driven-development` (fresh
subagent per task, review between, final whole-branch review).

What actually earns its cost:

- **Reviews that mutate rather than read.** Every real finding this session came from
  a mutation. Reading-only reviews have found approximately nothing.
- **Telling the reviewer where the fixture might be unable to discriminate.** "Check
  whether the chosen values can distinguish the two tables" found more than "review
  this diff" ever did.
- **Instructing implementers to stop and ask rather than encode a guess.** One did
  exactly that on the status question and was right on facts the controller had wrong.
- **Verifying implementer claims about the tree.** One committed onto the wrong parent
  and rebuilt a prior task's work into its own commit; a push rejection was the only
  thing that caught it before a merge that would have shipped a stale CI result.

Two recurring failures to watch for in reviewers:

- Downgrading a real finding because "the plan mandated it". A plan is not authority;
  rule on those yourself and record the ruling.
- Reporting a mutation "failed as expected" without saying *which* test failed. Ask
  for the test name and the failure text.

## Verification discipline

`store_subscriptions` is **empty** in production (4 tenants, 4 stores; subscriptions
need an explicit merchant call with a Stripe customer, and none has been made). The
billing endpoints therefore correctly serve `[]`.

So when you verify a deploy, **separate the checks that carry information from the
ones that merely mean "no data reached this code"**: status codes, validation
rejections and clamps exercise real paths; an empty `200` proves nothing about row
shaping. Say which is which. An earlier session reported a structurally-zero counter
as verified precisely because a `0` was accepted as evidence.

Deploys are Kargo-gated: CI → ghcr → Warehouse → Freight → Promotion → rollout.
Expect 10–20 minutes *from freight appearing*, and gate verification on **every**
service the change touches — a new caller against an old callee produces failures that
look like bugs.

## Housekeeping to decide

- **Direct pushes to `main` bypass a branch protection rule** ("Changes must be made
  through a pull request"); admin permissions allow it silently. Two docs commits went
  this way. Ask whether docs should route through PRs like code.
- **Dependabot reports 2 high-severity vulnerabilities** on the default branch.
  Unrelated to this milestone, unaddressed.
- Local toolchain drift: vet/LSP report `go.work requires go >= 1.26.6 (running
  1.26.5)`. Harmless to tests, noisy in diagnostics.

## Follow-ups this milestone produced

**#322** (dead onboarding statuses and a package doc claiming a gc that does not
exist), **#323** (no test covers `main.go` wiring — five instances, three distinct
failure modes including a runtime panic), **#326** (a hardcoded 90-day trial length
sent to **Stripe** as `trial_end`), **#328** (closed — the pricing-catalog
correction), plus the pre-existing **#311**, **#312**, **#316/#317**, **#318**.

---

Start by reading the handoff doc and #260's latest comment, then pick up #289 and run
the brainstorming → spec → plan → subagent-driven flow. Ask before merging anything.
