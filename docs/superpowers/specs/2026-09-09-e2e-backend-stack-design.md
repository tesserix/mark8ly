# Running the e2e suite against a real backend stack

Design for [#858](https://github.com/tesserix/mark8ly/issues/858). Split from
[#834](https://github.com/tesserix/mark8ly/issues/834), which delivered the
triage and the route manifest but deliberately left the backend problem alone.

**Status:** design approved in chat 2026-09-09; not yet planned or implemented.

## Problem

Of 35 Playwright specs presented as the enforced tier, exactly one runs in CI
(`apps/admin/tests/e2e/pricing.spec.ts`, via `.github/workflows/e2e-runnable.yml`).
The other 34 need a backend, so they sit under `tests/e2e/` reading as coverage
and providing none.

The goal is to stand up a real stack in CI and run them against it — not to
mock more. #834's second problem was that a spec mocking the API it tests
passes forever after that API is deleted, and adding mocks would reproduce it.

## What exploration changed

Five findings reshaped this design away from "build a CI harness from scratch".

**1. Most of the stack already exists.** `infra/dev/docker-compose.yml` already
brings up postgres (multi-DB: `platform_api`, `auth_bff`, `openfga`,
`marketplace_db`), OpenFGA v1.8.4 with a migrate step and a seed step that loads
`infra/openfga/model.fga`, plus `platform-api` (migrate + seed + server, :8086),
`auth-bff` (migrate + server, :8087→8080) and `marketplace-api`
(migrate + server, `MODE=both`, :8091). That is Tiers B and C substantially
complete. The work is to extend this file, not to write a new one.

**2. The specs need OpenFGA directly, not just transitively.**
`apps/admin/tests/e2e/helpers.ts` reads `OPENFGA_URL`, looks up a store named
`mark8ly-platform`, and writes role tuples through `/stores/{id}/write` to
obtain non-owner roles — because there is no invite-teammate UI to drive. Any
harness that omits OpenFGA fails these specs at the helper, not at the assertion.

**3. `make dev` is broken today, and fixing it is on this path anyway.**
`infra/dev/load-secrets.sh` pulls only `GIP_*` and `NEXT_PUBLIC_GIP_*` values,
and `make dev` depends on `dev-secrets`. GIP was fully removed on 2026-09-08, so
those secrets feed nothing. The compose header still says "auth-bff talks to
real Google Identity Platform", which is now false. Adding Zitadel to this stack
repairs local dev and builds the CI harness in the same change — they become the
same artifact rather than two things that drift.

**4. Email verification needs no mail catcher, but does need `ENV != "prod"`.**
Five of the seven onboarding specs walk a magic-link flow. They do not read a
mailbox — `apps/onboarding/tests/e2e/helpers.ts:45` polls
`GET /api/v1/test/verification/latest?email=...`, a test-only endpoint served by
`services/platform-api/internal/test/handler.go` and mounted only when `ENV` is
not `"prod"`. So no SMTP or mail-catcher service is required, but the harness
must set `ENV=dev` (or any non-`prod` value) on platform-api or five specs fail
at the helper with no obvious cause.

**5. The Next.js client id is build-time, so bootstrap ordering is forced.**
`NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID` and `NEXT_PUBLIC_ZITADEL_ISSUER` are
inlined at `next build`. Zitadel mints the client id during bootstrap. CI must
therefore bootstrap Zitadel _before_ building any Next app. This is a hard
ordering constraint, not a preference, and it is the single most likely thing to
be got wrong by someone reordering steps for speed.

## The dependency tiers

Measured by which specs import auth helpers (`completeOnboarding`, `signIn`,
`ADMIN_URL`):

| tier       | additionally needs                | specs unlocked                  |
| ---------- | --------------------------------- | ------------------------------- |
| runs today | nothing                           | 1 (`pricing`)                   |
| B          | postgres + OpenFGA + platform-api | up to 7 onboarding + probes     |
| C          | + marketplace-api                 | storefront `home`, admin probes |
| D          | + Zitadel + auth-bff              | **18 of 26 admin specs**        |

Tier D holds most of the value and all of the risk.

> **Addendum (measured 2026-09-09).** This table was derived by reading
> imports and is wrong about Tier C -- see the Stage 2 addendum below. What
> was measured against real stacks:
>
> | tier       | specs it actually unlocks                               |
> | ---------- | ------------------------------------------------------- |
> | runs today | 1 (`pricing`, 5 tests)                                  |
> | B          | 3 onboarding specs (7 passed, 5 skipped) -- **shipped** |
> | C          | **0**                                                   |
> | D          | **19** -- 13 admin + 2 storefront + 4 onboarding        |
>
> The denominator changed too: 12 of the 35 were operator scripts, so there
> are **23 real specs**, of which 4 run. Tier D is not "most of the value",
> it is all of the remaining value.

## Approach

**Extend `infra/dev/docker-compose.yml` with a Zitadel service, and drive that
same stack from a new workflow.** One stack definition, two consumers (a
developer running `make dev`, and CI). A CI-only stack would drift from the dev
one and neither would be trustworthy.

### Zitadel bootstrap

A fresh Zitadel needs an org, a project, a confidential OIDC app, a login-client
machine user with a PAT, and a test user — none of which exist in a blank
instance.

**Chosen: a small CI-owned bootstrap script (~150 lines), written after reading
`tesserix-k8s/charts/apps/zitadel-bootstrap/files/bootstrap.py`.**

That file is a 1048-line reconciler that already creates orgs, projects, OIDC
apps, roles, machine users and PATs against a live instance, and its header
records that every JSON shape is pinned to what was _observed_, not to what the
docs claim. We port its handling for the specific calls we make, and skip the
rest (SMTP, branding, assets, IDP templates) as irrelevant to CI.

Rejected alternatives:

- _Vendor `bootstrap.py` wholesale._ Cross-repo coupling for a file that is
  ~85% irrelevant here, and it reconciles a long-lived instance rather than
  initialising a fresh one.
- _Commit a pre-bootstrapped Zitadel database dump._ Fastest and fully
  deterministic — fixed client ids remove the ordering constraint entirely — but
  it is an opaque blob that drifts silently from the Zitadel version. That is
  exactly the "fixture diverges from reality" failure this issue exists to
  prevent, so the speed is not worth it.

Zitadel's `FirstInstance` configuration creates the initial org and a machine
user whose PAT is written to a known path at startup; the bootstrap script then
authenticates with that PAT for everything else.

### Two scars to encode, from prior sessions

- **A test user needs an explicit project grant.** With `projectRoleCheck=true`,
  a user without a grant gets a 403 at _finalize_ — after a successful password
  check. A bootstrap that skips the grant produces a spec failure that reads as
  broken auth rather than as missing setup.
- **Zitadel v2 protojson flattens oneofs.** A wrapped oneof returns 200 and
  silently does the wrong thing, so the bootstrap must assert on the _success_
  path rather than trusting the status code.

## Staging

Land in three independently-green stages. This is ordering, not scope
reduction — the destination is still Tier D.

**Stage 1 (Tier B).** Add a CI workflow that brings up the existing compose
stack minus Zitadel, builds and starts the onboarding app with real
`PLATFORM_API_URL`, and runs the onboarding specs — including the magic-link
ones, via the test-only verification endpoint above. Retire `load-secrets.sh`'s
GIP block so `make dev` works again.

> **Addendum (superseded by measurement):** the magic-link claim above did not
> hold up — those specs need Zitadel to reach `/welcome`, so they are excluded
> from Stage 1 and enabled later alongside the other auth-dependent specs in
> Stage 3. See `docs/superpowers/plans/2026-09-09-e2e-baseline.md` for what was
> actually measured against the running stack.

**Stage 2 (Tier C).** Add marketplace-api to the CI path and the storefront and
admin apps. Unlocks the storefront and probe specs.

> **Addendum (superseded by measurement, 2026-09-09):** Tier C unlocks
> **nothing**, and the tier table above overstates it in the same way the
> Stage 1 magic-link claim did. Measured against a running Tier C stack:
>
> - Both storefront specs (`home`, `auth-isolation`) walk
>   `set-password -> /welcome`, so they are **Tier D**, not Tier C.
> - Of the four admin specs that reach `ADMIN_URL` without an auth helper,
>   only `pricing` passes (5 tests, and it already ran). `image-audit`,
>   `products-sync` and `delivery-timeline` all fail at `/login`.
> - The 8 admin specs that import no auth helper at all are **not tests**:
>   six contain zero `expect()` calls, and all of them attach to a
>   pre-existing tenant with caller-supplied credentials.
>
> So Stage 2 was retired as a no-op. What shipped under its name instead was
> an honesty pass: fix two compose defects that meant marketplace-api had
> **never started** in the dev stack, and move 12 operator scripts out of
> `tests/e2e/`. The real remaining work is entirely Tier D.
>
> **The dividing line that replaced the tier table** -- a spec belongs in
> `tests/e2e/` if it builds its own tenant (`completeOnboarding()`); it is an
> operator script if it attaches to an existing one (`TENANT_NAME` plus
> caller credentials). That partitions all 26 original admin specs with no
> judgement calls left over, and it is what `tests/operator/README.md` now
> records.

**Stage 3 (Tier D).** Add Zitadel plus the bootstrap script, wire auth-bff, feed
the minted client id into the Next builds, and enable the 18 auth-dependent
specs.

Each stage ends with its spec set actually passing in CI, not merely wired.

## Verification

The canary discipline already established in this repo is the requirement, not a
nicety. Two existing precedents, both of which exist because a green run once
proved nothing:

- The `integration` job in `.github/workflows/ci.yml` runs one named test with
  `-v` and greps for `--- PASS:`, failing at line 160 if it did not run —
  because an integration test with no `TEST_DATABASE_URL` skips and the package
  still prints `ok`.
- `.github/workflows/e2e-runnable.yml` asserts an exact passed-test count,
  because a spec whose tests all skip also exits 0.

Every stage here asserts an exact expected pass count for the specs it enables.
A stage that cannot state that number is not done.

## Cost and triggering

The stack is postgres + OpenFGA + Zitadel + three Go services + up to three
Next builds, and a cold Next 16 admin build alone routinely exceeds three
minutes. Running this on every PR would dominate CI time.

**Trigger on push to `main`, a nightly schedule, and `workflow_dispatch`** —
not on `pull_request`. The fast gates (unit, `tsc`, `next build`, the pricing
canary, the route manifest) stay on PRs and keep the tight loop tight; this
suite catches integration drift on a slower cadence. Revisit if it starts
catching things late.

## Open questions

- **Which Zitadel version.** `tesserix-k8s` runs v4.15.3 against CloudNativePG.
  CI should pin the same major, but v4's `FirstInstance` config shape must be
  confirmed against the running image rather than the docs.
- **Whether `MODE=both` is faithful.** The dev compose runs marketplace-api as
  one process with `MODE=both`; production runs separate admin and storefront
  deployments, and migrations run only on the admin one. A defect that depends
  on that split will not reproduce under `MODE=both`.
- **Test data.** Several admin specs assume products, orders and a storefront
  handle exist. Whether specs seed their own fixtures or a shared seed step does
  is unresolved, and shared mutable fixtures across 18 specs invite the
  ordering-dependence that made the old suite untrustworthy.
