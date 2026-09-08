# Onboarding e2e baseline against a real backend stack (mark8ly#858, Task 1)

Measured 2026-09-09. This is the first time the onboarding Playwright suite has been
run against a real backend stack (Postgres + OpenFGA + platform-api) in this repo's
history. It establishes `ONBOARDING_PASS_COUNT`, the canary number later tasks (3, 4,
5) build on.

## Environment

- Node: **v24.20.0** locally (CI currently runs Node 22 — tracked separately in
  issue #857; not changed or worked around here).
- `docker compose` v5.3.1 (supports the `!reset` YAML tag).
- Stack brought up via `infra/dev/docker-compose.yml` +
  `infra/dev/docker-compose.override.yml` (untracked, per-developer — see below),
  bypassing `dev-secrets`/`load-secrets.sh` per the task brief (GIP secret loader is
  dead code post-GIP-removal; Task 2 fixes it).
- Services started: `postgres`, `openfga-migrate`, `openfga`, `openfga-seed`,
  `platform-api-migrate`, `platform-api-seed`, `platform-api`. Zitadel, auth-bff, and
  every other service were **not** started — deliberately absent at this stage
  (Zitadel arrives in Stage 3 per issue #858).
- `apps/onboarding` built with `next build` and started with `next start --port 4201`,
  pointed at `PLATFORM_API_URL=http://localhost:8086`.
- Playwright run: `BASE_URL=http://localhost:4201 API_URL=http://localhost:8086 npx
  playwright test --reporter=list`. Run **without** `CI` set, so
  `playwright.config.ts`'s `retries: process.env.CI ? 2 : 0` evaluates to 0 — this is
  raw first-attempt behaviour, not retried.
- `workers: 1`, `fullyParallel: false` (config's own note: "golden-path tests share
  Postgres state") — so results are deterministic, not a parallelism artifact.

## Two environment bugs found and worked around locally (not fixed)

Bringing the stack up hit two pre-existing bugs in `infra/dev/docker-compose.yml`,
neither related to Task 1's actual measurement goal. Both were worked around inside
the untracked `docker-compose.override.yml` so the measurement could proceed; neither
was fixed in tracked files, per the task's "bypass, don't fix" instruction. Both are
findings for whoever picks up the compose file next:

1. **Stale build context.** `platform-api-migrate`, `platform-api-seed`, and
   `platform-api` all set `build.context: ../../services/platform-api`, but
   `services/platform-api/Dockerfile` says (in its own header comment) "Build context
   MUST be the monorepo root (mark8ly#720)" and `COPY`s both
   `services/platform-api/` and `packages/platformauth/` — paths that don't exist
   under a `services/platform-api`-scoped context. Build failed with `"/services/
   platform-api": not found` until the override repointed `context` to `../..` with
   an explicit `dockerfile:`.
2. **Missing ENTRYPOINT on the `migrate` stage.** The `migrate` Dockerfile stage sets
   `CMD ["/migrate"]` with no `ENTRYPOINT`. `docker-compose.yml`'s
   `platform-api-migrate` service sets `command: ["up"]`, which — with no entrypoint —
   replaces the CMD outright, so the container tried to exec `up` as a binary
   (`exec: "up": executable file not found in $PATH`). Worked around locally with
   `entrypoint: ["/migrate"]` in the override so `command: ["up"]` is passed as an
   argument instead.

Both bugs are visible in
`/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/infra/dev/docker-compose.override.yml`
(untracked, gitignored, left in place for the controller to inspect). Neither
`docker-compose.yml` nor any Dockerfile was modified.

## Stack reachability (Step 2 checks, all passed)

```
$ curl -fsS http://localhost:8086/health
{"status":"ok"}

$ curl -fsS http://localhost:8089/stores | jq -r '.stores[] | select(.name=="mark8ly-platform") | .id'
01M20XXKWD1JDA8CSTYS7KWQKF

$ curl -fsS "http://localhost:8086/api/v1/locations/countries" | jq '.data | length'
20
```

`apps/onboarding` build succeeded; `next start` served `/onboarding` with HTTP 200.

## Result

Run three times total, all under the same conditions (CI unset so
`retries: 0`, same `BASE_URL=http://localhost:4201 API_URL=http://localhost:8086`,
same running stack throughout, no code touched between runs). Every run produced
a full raw transcript (`/tmp/onboarding-e2e.txt`, `/tmp/onboarding-e2e-run2.txt`,
`/tmp/onboarding-e2e-run3.txt` respectively). Diffing the "N failed" / "N passed"
block across all three transcript files byte-for-byte confirms they are
identical — same 10 failing specs in the same order, same summary line, in every
run:

- **run 1**: `10 failed` / `6 passed (3.5m)`
- **run 2**: `10 failed` / `6 passed (3.5m)`
- **run 3** (added after code review flagged that run 2's evidence wasn't
  reproduced in this document): `10 failed` / `6 passed (3.5m)`

Verbatim failing-list + summary, common to all three runs (`diff` shows zero
difference between run 1, run 2, and run 3's blocks):

```
10 failed
  [chromium] › tests/e2e/cross-device-verify.spec.ts:24:5 › magic link works in a fresh browser context (cross-device)
  [chromium] › tests/e2e/form-validation.spec.ts:26:7 › onboarding form validation › rejects a slug that is too short
  [chromium] › tests/e2e/golden-path.spec.ts:16:5 › golden path: landing → form → magic link → welcome
  [chromium] › tests/e2e/resume.spec.ts:26:5 › survives full browser close between submit and verify
  [chromium] › tests/e2e/slug-collision.spec.ts:20:5 › rejects a slug that another tenant has already claimed
  [chromium] › tests/e2e/tax-id-migration.spec.ts:68:7 › migration fast-path (§5.1.1) › migration radio defaults to 'new store'
  [chromium] › tests/e2e/tax-id-migration.spec.ts:82:7 › migration fast-path (§5.1.1) › switching to 'migrating' reveals the evidence panel
  [chromium] › tests/e2e/tax-id-migration.spec.ts:100:7 › migration fast-path (§5.1.1) › switching back to 'new store' hides the evidence panel
  [chromium] › tests/e2e/tax-id-migration.spec.ts:118:7 › migration fast-path (§5.1.1) › submitting 'migrating' without evidence shows the required error
  [chromium] › tests/e2e/tax-id-migration.spec.ts:154:7 › migration fast-path (§5.1.1) › filling WHOIS URL clears the migration required error
6 passed (3.5m)
```

No run disagreed with any other — no per-spec delta to report.

**`ONBOARDING_PASS_COUNT = 6`** — the number of tests that passed, taken verbatim
from Playwright's summary line, confirmed identical across three separately
logged runs (not two, and not from memory).

Note: five specs walk the magic-link flow via `GET
/api/v1/test/verification/latest` (mounted because `ENV: dev` on platform-api, so
`ENV != "prod"`), not a mailbox — that endpoint worked correctly in this run (the
suite did reach the token and proceed to `/onboarding/set-password`, not an empty-
token failure). The failure for those specs is one step later: completing
`set-password` requires Zitadel, which is absent at this stage.

## Per-spec table

| Spec file | Test | Result | First error line | Cause (one clause) |
|---|---|---|---|---|
| cross-device-verify.spec.ts | magic link works in a fresh browser context (cross-device) | fail | `Received string: "http://localhost:4201/onboarding/set-password?session=..."` (expected `/\/welcome/`) | needs Zitadel to complete set-password → welcome |
| form-validation.spec.ts | rejects an obviously invalid email | pass | — | — |
| form-validation.spec.ts | rejects a slug that is too short | fail | `Expected: disabled` / `Received: enabled` on the submit button | UI bug — submit button is not gated on the short-slug case (independent of backend) |
| golden-path.spec.ts | golden path: landing → form → magic link → welcome | fail | `Received string: "http://localhost:4201/onboarding/set-password?session=..."` (expected `/\/welcome/`) | needs Zitadel to complete set-password → welcome |
| invalid-token.spec.ts | invalid magic link token surfaces an error and never navigates to welcome | pass | — | — |
| resume.spec.ts | survives full browser close between submit and verify | fail | `Received string: "http://localhost:4201/onboarding/set-password?session=..."` (expected `/\/welcome/`) | needs Zitadel to complete set-password → welcome |
| slug-collision.spec.ts | rejects a slug that another tenant has already claimed | fail | `Received string: "http://localhost:4201/onboarding/set-password?session=..."` (expected `/\/welcome/`) | needs Zitadel to complete set-password → welcome |
| tax-id-migration.spec.ts | renders with fallback help text when no country is selected | pass | — | — |
| tax-id-migration.spec.ts | shows GB-specific help text when country is United Kingdom | pass | — | — |
| tax-id-migration.spec.ts | shows IN-specific help text when country is India | pass | — | — |
| tax-id-migration.spec.ts | shows US-specific help text when country is United States | pass | — | — |
| tax-id-migration.spec.ts | migration radio defaults to 'new store' | fail | `Locator: getByRole('radio', { name: /this is a new store/i })` / `Error: element(s) not found` | UI bug — radio not present/rendered as expected (independent of backend) |
| tax-id-migration.spec.ts | switching to 'migrating' reveals the evidence panel | fail | `Error: locator.click: Test timeout of 30000ms exceeded.` on the "migrating" radio | UI bug — same radio issue as above, click target never appears |
| tax-id-migration.spec.ts | switching back to 'new store' hides the evidence panel | fail | `Error: locator.click: Test timeout of 30000ms exceeded.` on the "migrating" radio | UI bug — same radio issue |
| tax-id-migration.spec.ts | submitting 'migrating' without evidence shows the required error | fail | `Error: locator.click: Test timeout of 30000ms exceeded.` on the "migrating" radio | UI bug — same radio issue |
| tax-id-migration.spec.ts | filling WHOIS URL clears the migration required error | fail | `Error: locator.click: Test timeout of 30000ms exceeded.` on the "migrating" radio | UI bug — same radio issue |

No `skip` results occurred — all 16 tests ran to a pass/fail outcome.

## Specs that need Zitadel (Task 5 / Stage 3 consumes this)

These four specs all fail at the same point — successfully reaching
`/onboarding/set-password?session=...` via the magic-link/test-verification path, then
failing to progress to `/welcome` because completing password setup requires Zitadel:

- `tests/e2e/cross-device-verify.spec.ts` — "magic link works in a fresh browser context (cross-device)"
- `tests/e2e/golden-path.spec.ts` — "golden path: landing → form → magic link → welcome"
- `tests/e2e/resume.spec.ts` — "survives full browser close between submit and verify"
- `tests/e2e/slug-collision.spec.ts` — "rejects a slug that another tenant has already claimed"

## Specs that fail for a different reason (not Zitadel — a UI finding, not fixed here)

- `tests/e2e/form-validation.spec.ts` — "rejects a slug that is too short": submit
  button is enabled when the spec expects it disabled for a too-short slug.
- `tests/e2e/tax-id-migration.spec.ts` — all 5 "migration fast-path (§5.1.1)" tests:
  the "this is a new store" / "I'm migrating from an existing store" radio group is
  not found/interactable on `/onboarding` in this configuration. Independent of the
  backend/Zitadel gap — the 4 tax-ID-only tests in the same file (country-specific
  help text) pass, so the page itself loads; only the migration radio group fails.
  Not investigated further — out of scope for this measurement task.

## Stack state after this task

Left running for the controller to inspect, per instruction:

```
NAME                 SERVICE        STATUS
dev-openfga-1        openfga        Up
dev-platform-api-1   platform-api   Up
dev-postgres-1       postgres       Up (healthy)
```

`apps/onboarding` is running via `next start --port 4201` in the background
(PID captured in the shell that started it; log at `/tmp/onboarding-server.log`).
