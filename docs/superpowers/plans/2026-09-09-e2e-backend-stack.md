# E2E Backend Stack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run mark8ly's Playwright e2e specs against a real backend stack in CI, starting with the onboarding suite, so the specs stop reading as coverage they do not provide.

**Architecture:** Extend the existing `infra/dev/docker-compose.yml` stack rather than building a CI-only harness, so a developer running `make dev` and CI exercise the same definition. Stage 1 covers Tier B (postgres + OpenFGA + platform-api + the onboarding app). Stages 2 and 3 are outlined but deliberately not step-planned yet — see "Why stages 2 and 3 are not detailed".

**Tech Stack:** docker compose, postgres 15, OpenFGA v1.8.4, Go 1.26 (platform-api), Next.js 16 (onboarding), Playwright, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-09-e2e-backend-stack-design.md`

## Global Constraints

- Node **22** in CI today, **24** in production containers. Do not "fix" this here — it is tracked in #857 and changing it mid-plan conflates two investigations.
- `ENV` must never be `"prod"` on platform-api in this stack. `services/platform-api/internal/test/handler.go` mounts `GET /api/v1/test/verification/latest` only when `ENV != "prod"`, and five onboarding specs poll it for the magic-link token.
- Every CI step that runs tests must assert an **exact expected pass count**. Exit status alone is insufficient: a spec whose tests all skip exits 0. This is the failure mode #834 exists to prevent, and both `.github/workflows/ci.yml` (grep for `--- PASS:`) and `.github/workflows/e2e-runnable.yml` (grep for `N passed (`) already encode it.
- Do not add `page.route()` mocks to any spec. The point of this work is to remove mocking, not relocate it.
- OpenFGA store name is `mark8ly-platform` — hardcoded in four Go files and in `apps/admin/tests/e2e/helpers.ts:27`. Do not rename it.
- Commit messages are single-line, conventional-commit prefixed, and carry no trailers of any kind.

## Why stages 2 and 3 are not detailed

Stage 3 (Zitadel) is the bulk of the work and the least knowable from reading. Prior sessions record that Zitadel's v2 API shapes are pinned to what was *observed* against a live instance rather than what the docs describe, that a wrapped oneof returns 200 while silently doing the wrong thing, and that IDPs are invisible to v1 search endpoints. Writing bite-sized steps for a Zitadel bootstrap before a container has been booted would produce confident fiction, and a defect in a brief survives every downstream review.

Stage 1 makes the harness real. Stages 2 and 3 get planned against a stack that exists.

---

### Task 1: Measure what the onboarding suite actually does against a real stack

The suite has never been run against this stack in this repo's history, so the set of specs that pass — and therefore the canary number every later step depends on — is unknown. Establish it by running it, exactly as `e2e-runnable.yml` established its subset.

This task produces a **measurement recorded in the repo**, not product code.

**Files:**
- Create: `docs/superpowers/plans/2026-09-09-e2e-baseline.md`
- Create (untracked, per-developer): `infra/dev/docker-compose.override.yml`
- Modify: `.gitignore` (add the override path)

**Interfaces:**
- Consumes: nothing.
- Produces: `ONBOARDING_PASS_COUNT` (integer) and a per-spec pass/fail/skip table that Tasks 3 and 4 consume.

- [ ] **Step 1: Bring the stack up without the dead secret loader**

`make dev` depends on `dev-secrets`, which runs `infra/dev/load-secrets.sh` — a script that pulls only `GIP_*` and `NEXT_PUBLIC_GIP_*` values from GCP Secret Manager. GIP was removed on 2026-09-08, so those feed nothing. Bypass it for this measurement rather than fixing it yet (Task 2 fixes it):

**Host port 5432 is very likely already taken** by a local postgres — it was
on the machine this plan was written on. The compose file publishes
`5432:5432`, so the stack will fail to bind. Nothing in this task needs
postgres from the host (platform-api reaches it container-internally at
`postgres:5432`), so drop the publish rather than stopping anyone's database.
Create `infra/dev/docker-compose.override.yml`, which compose picks up
automatically. It is **not** currently gitignored (verified: `git check-ignore`
does not match it), so add `infra/dev/docker-compose.override.yml` to
`.gitignore` in the same step — it is a per-developer file and committing it
would publish one machine's port layout to everyone:

```yaml
# Local-only: do not publish postgres on the host. 5432 is commonly taken,
# and no service in the Tier-B stack is reached from the host on that port.
services:
  postgres:
    ports: !reset []
```

Then:

```bash
cd infra/dev
touch .env.local            # compose declares env_file, so the file must exist
docker compose up -d postgres openfga-migrate openfga openfga-seed \
                     platform-api-migrate platform-api-seed platform-api
docker compose ps
```

Expected: `platform-api` running and `openfga-seed` exited 0. If compose still
reports a port conflict, `lsof -nP -iTCP:5432 -sTCP:LISTEN` names the holder —
do **not** kill it, it may belong to another session working in this checkout.

Ports 8086 (platform-api), 8087, 8089/8090 (OpenFGA), 8091 and 4201 were all
free when this plan was written; only 5432 collided.

- [ ] **Step 2: Prove the backend is genuinely reachable, not merely running**

```bash
curl -fsS http://localhost:8086/health && echo
curl -fsS http://localhost:8089/stores | jq -r '.stores[] | select(.name=="mark8ly-platform") | .id'
curl -fsS "http://localhost:8086/api/v1/locations/countries" | jq '.data | length'
```

Expected: a health response, a non-empty ULID for the store, and a country count greater than zero. A zero country count means `platform-api-seed` did not run — do not proceed, because the `/onboarding` route renders from this data and every spec will time out with a misleading error.

- [ ] **Step 3: Build and start the onboarding app against the stack**

The onboarding Playwright config has no `webServer` and assumes the app is already up on :4201. Build separately so a build failure is its own log line rather than a spawn timeout:

```bash
cd apps/onboarding
PLATFORM_API_URL=http://localhost:8086 npm run build
PLATFORM_API_URL=http://localhost:8086 PORT=4201 npm run start &
sleep 5 && curl -fsS -o /dev/null -w '%{http_code}\n' http://localhost:4201/onboarding
```

Expected: `200`. A 500 here means the server component's `locations` fetch failed — recheck Step 2 before going further.

- [ ] **Step 4: Run the onboarding suite and record the real result**

```bash
cd apps/onboarding
BASE_URL=http://localhost:4201 API_URL=http://localhost:8086 \
  npx playwright test --reporter=list 2>&1 | tee /tmp/onboarding-e2e.txt
tail -5 /tmp/onboarding-e2e.txt
```

Do not treat any outcome as failure of this task. A spec that fails because it needs Zitadel is a **finding**, and it is the finding this task exists to produce.

- [ ] **Step 5: Write the baseline document**

Create `docs/superpowers/plans/2026-09-09-e2e-baseline.md` containing:

- the exact summary line (e.g. `9 passed, 6 failed (1.2m)`);
- a table with one row per spec file: `pass` / `fail` / `skip`, and for each failure the **first** error line and a one-clause cause;
- the integer `ONBOARDING_PASS_COUNT`, defined as the number of tests that passed with the stack in this exact configuration;
- an explicit list of which specs need Zitadel, which Task 5's Stage 3 outline consumes.

State counts as measured. Do not round, estimate, or write "should be".

- [ ] **Step 6: Commit**

```bash
git add docs/superpowers/plans/2026-09-09-e2e-baseline.md .gitignore
git commit -m "docs(e2e): record the measured onboarding suite baseline against a real stack (#858)"
```

`infra/dev/docker-compose.override.yml` must NOT be committed — confirm with
`git status --short infra/dev/` that it shows as ignored, not staged.

---

### Task 2: Make `make dev` work again without GCP

`make dev` is broken: it depends on `dev-secrets`, which pulls GIP secrets that nothing reads. This blocks both a developer and CI. Fix it so the Tier-B subset comes up with no GCP access at all.

**Files:**
- Create: `apps/onboarding/tests/unit/dev-stack-config.spec.ts`
- Modify: `infra/dev/docker-compose.yml` (build context and migrate/seed invocation — see Step 0)
- Modify: `infra/dev/load-secrets.sh`
- Modify: `apps/onboarding/playwright.config.ts:1-14` (stale header comment)
- Modify: `Makefile:16-20`
- Modify: `infra/dev/.env.local.example`
- Modify: `infra/dev/docker-compose.yml:1-18` (the stale header comment)

**Interfaces:**
- Consumes: the Task 1 finding that the Tier-B services boot with no secrets.
- Produces: `make dev-min`, a target that brings up postgres + OpenFGA + platform-api with no GCP dependency.

- [ ] **Step 0: Fix the two tracked compose defects Task 1 uncovered**

Task 1 could only bring the stack up by patching an *untracked* override, so
the tracked file still cannot build `platform-api`. Task 3's CI job runs
`make dev-min` against the tracked file, so without this it fails every time.
Both defects were verified directly against the Dockerfile:

1. **Wrong build context.** `services/platform-api/Dockerfile:23-30` copies
   `services/platform-api/` and `packages/platformauth/` — paths relative to
   the **repo root** — but all three `platform-api*` services set
   `context: ../../services/platform-api`. Change each to:

```yaml
    build:
      context: ../..
      dockerfile: services/platform-api/Dockerfile
      target: migrate     # (or: seed / server, per service)
```

2. **`command:` replaces the binary.** The `migrate` and `seed` stages declare
   `CMD ["/migrate"]` / `CMD ["/seed"]` with no `ENTRYPOINT`
   (`services/platform-api/Dockerfile:53,57`). Compose's `command: ["up"]`
   therefore *replaces* `/migrate` rather than passing it an argument, so the
   container tries to execute `up`. Change the migrate service to:

```yaml
    command: ["/migrate", "up"]
```

Add a comment above it recording why, so nobody "simplifies" it back:

```yaml
    # Full argv: the migrate stage sets CMD with no ENTRYPOINT, so a bare
    # ["up"] replaces the binary instead of being passed to it.
```

Verify with a clean build before moving on:

```bash
cd infra/dev && docker compose build platform-api-migrate platform-api-seed platform-api
```

Expected: all three build. If you still need an override file to get the
stack up after this step, the fix is incomplete — say so rather than
re-adding the workaround.

- [ ] **Step 1: Write the failing test**

The repo guards workflow and config invariants with tests (see `apps/admin/tests/unit/lib/copy/pricing-copy.test.ts` for the established shape). Add `apps/onboarding/tests/unit/dev-stack-config.spec.ts`:

```typescript
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

const root = join(__dirname, "../../../..");

// GIP was removed on 2026-09-08 (#GIP removal, 21/21). A secret loader that
// still pulls GIP_* values makes `make dev` depend on GCP for credentials
// nothing reads, which is why the local stack was unusable.
test("the dev secret loader no longer pulls GIP values", () => {
  const sh = readFileSync(join(root, "infra/dev/load-secrets.sh"), "utf8");
  expect(sh).not.toMatch(/GIP_/);
  expect(sh).not.toMatch(/NEXT_PUBLIC_GIP_/);
});

// The compose header told developers auth-bff talks to Google Identity
// Platform. It does not, and a stale orientation comment is worse than none.
test("the compose header does not claim a GIP dependency", () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  expect(yml).not.toMatch(/Google Identity Platform/);
});

// The onboarding Playwright config still lists a "firebase auth emulator"
// among the services it assumes are up. There has never been one since GIP
// was removed, and it sends anyone debugging a failure looking for a
// container that does not exist. NOTE: do not assert this against
// docker-compose.yml — it contains zero occurrences of "firebase", so the
// assertion would pass vacuously and guard nothing.
test("the onboarding playwright config does not reference a firebase emulator", () => {
  const cfg = readFileSync(join(root, "apps/onboarding/playwright.config.ts"), "utf8");
  expect(cfg).not.toMatch(/firebase/i);
});

// The Tier-B subset must come up with no GCP access, so the target that
// brings it up must not depend on the secret-pulling target.
test("dev-min does not depend on dev-secrets", () => {
  const mk = readFileSync(join(root, "Makefile"), "utf8");
  const line = mk.split("\n").find((l) => l.startsWith("dev-min:"));
  expect(line, "Makefile has no dev-min target").toBeTruthy();
  expect(line).not.toMatch(/dev-secrets/);
});
```

- [ ] **Step 2: Run it to confirm it fails**

```bash
cd apps/onboarding && npx playwright test --config=playwright.unit.config.ts tests/unit/dev-stack-config.spec.ts
```

Expected: **four** failures — `GIP_` present in the loader, `Google Identity
Platform` present in the compose header, `firebase` present in the onboarding
Playwright config, and no `dev-min` target in the Makefile.

- [ ] **Step 3: Strip the GIP block from the loader**

Remove every `GIP_*` and `NEXT_PUBLIC_GIP_*` fetch and the lines that write them to `.env.local`. Keep the non-GIP values (`SESSION_ENCRYPT_KEY`, `OAUTH_CLIENT_*`, `PLATFORM_API_URL`, `AUTH_BFF_URL`, `NEXT_PUBLIC_SITE_URL`, `NEXT_PUBLIC_MARKETING_URL`). Above the remaining fetches add:

```bash
# GIP was removed on 2026-09-08; its secrets are not fetched here any more.
# Zitadel credentials are NOT pulled either — the Tier-B stack runs without
# an identity provider, and Stage 3 stands up a disposable Zitadel in-stack
# rather than pointing developers at the shared live instance (#858).
```

- [ ] **Step 4: Add the `dev-min` target**

In the `Makefile`, beside the existing `dev` target:

```makefile
dev-min: ## Bring up the Tier-B stack (postgres, OpenFGA, platform-api) with no GCP access
	$(COMPOSE) up -d postgres openfga-migrate openfga openfga-seed \
	                platform-api-migrate platform-api-seed platform-api
```

Add `dev-min` to the `.PHONY` list on line 10.

- [ ] **Step 5: Correct the compose header**

Replace the `What's NOT running here:` block's GIP paragraph with:

```
#   - Any identity provider — the Tier-B stack runs without one. Specs that
#     need sign-in are enabled in Stage 3 (#858), which adds a disposable
#     Zitadel to this file. Do not point this stack at the shared live
#     instance at auth.tesserix.app.
```

Also update `.env.local.example` to list the keys the loader still writes, so the file stops describing two keys when the stack reads more.

Then fix `apps/onboarding/playwright.config.ts:7`, which still lists a
"firebase auth emulator" among the services it assumes are running. Replace
that service list with: `Postgres, OpenFGA, platform-api, and the onboarding
Next.js server on :4201`. Leave the rest of the comment — its point about not
spawning the stack from Playwright is still correct and is why this plan
builds and starts the app in separate CI steps.

- [ ] **Step 6: Run the tests to confirm they pass**

```bash
cd apps/onboarding && npx playwright test --config=playwright.unit.config.ts tests/unit/dev-stack-config.spec.ts
```

Expected: 4 passed.

- [ ] **Step 7: Verify the stack still comes up from clean**

```bash
cd infra/dev && docker compose down -v && cd ../.. && make dev-min
sleep 20 && curl -fsS http://localhost:8086/health && echo OK
```

Expected: `OK`. This must be run from a wiped volume — a stack that only works with warm state is not a working stack.

- [ ] **Step 8: Commit**

```bash
git add infra/dev/load-secrets.sh infra/dev/docker-compose.yml infra/dev/.env.local.example Makefile apps/onboarding/tests/unit/dev-stack-config.spec.ts
git commit -m "fix(dev): drop the dead GIP secret loader and add a GCP-free dev-min target (#858)"
```

---

### Task 3: Add the CI workflow that runs the onboarding suite against the stack

**Files:**
- Create: `.github/workflows/e2e-onboarding.yml`
- Test: the workflow itself, verified by dispatching it

**Interfaces:**
- Consumes: `ONBOARDING_PASS_COUNT` from Task 1's baseline document; `make dev-min` from Task 2.
- Produces: a green required-ish job proving the onboarding suite runs against real services.

- [ ] **Step 1: Write the workflow**

Substitute the real integer from the baseline document for `<N>` — do not invent it.

```yaml
name: E2E (onboarding, real stack)
env:
  FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: "true"

# =============================================================================
# mark8ly#858 — Stage 1 of running the e2e suite against a real backend.
#
# Unlike e2e-runnable.yml, which runs the one spec needing no backend at all,
# this job stands up postgres, OpenFGA and platform-api from the SAME
# infra/dev/docker-compose.yml a developer uses, so the two cannot drift.
#
# NOT triggered on pull_request: the stack plus a cold Next 16 build costs
# several minutes, and the fast gates (unit, tsc, next build, the pricing
# canary, the route manifest) already run there. This catches integration
# drift on a slower cadence.
# =============================================================================
on:
  push:
    branches: [main]
    paths:
      - "apps/onboarding/**"
      - "services/platform-api/**"
      - "infra/dev/**"
      - "infra/openfga/**"
      - ".github/workflows/e2e-onboarding.yml"
  schedule:
    - cron: "0 16 * * *"   # 02:00 AEST
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  onboarding:
    name: onboarding suite vs real platform-api
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
      - uses: actions/setup-node@a0853c24544627f65ddf259abe73b1d18a591444 # v5
        with:
          # Matches ci.yml's Next.js job. Production containers run 24; that
          # divergence is tracked in #857 and must not be changed here.
          node-version: "22"

      - name: Bring up the Tier-B stack
        # No compose override here: on a clean runner 5432 is free, and the
        # override that a developer needs locally is deliberately not
        # committed. If this step ever fails on a port bind, the runner image
        # has started shipping a postgres and this needs the same treatment.
        run: |
          touch infra/dev/.env.local
          make dev-min

      - name: Prove the backend is reachable, not merely running
        # A stack that is "up" but unseeded renders /onboarding as a 500 and
        # every spec then fails on a timeout that names nothing useful.
        run: |
          for i in $(seq 1 60); do
            curl -fsS http://localhost:8086/health >/dev/null 2>&1 && break
            sleep 2
          done
          curl -fsS http://localhost:8086/health >/dev/null \
            || { echo "::error::platform-api never became healthy"; docker compose -f infra/dev/docker-compose.yml logs platform-api; exit 1; }
          countries=$(curl -fsS http://localhost:8086/api/v1/locations/countries | jq '.data | length')
          [ "$countries" -gt 0 ] \
            || { echo "::error::locations seed is empty ($countries) — platform-api-seed did not run"; exit 1; }
          store=$(curl -fsS http://localhost:8089/stores | jq -r '.stores[] | select(.name=="mark8ly-platform") | .id')
          [ -n "$store" ] \
            || { echo "::error::openfga store mark8ly-platform missing — openfga-seed did not run"; exit 1; }

      - name: Install dependencies
        run: npm ci --legacy-peer-deps

      - name: Install Playwright Chromium
        working-directory: apps/onboarding
        run: npx playwright install --with-deps chromium

      - name: Build onboarding
        working-directory: apps/onboarding
        # Its own step, not playwright webServer: a cold Next 16 build
        # routinely exceeds three minutes and would red the job on the build
        # clock rather than on anything a spec asserts.
        env:
          PLATFORM_API_URL: http://localhost:8086
        run: npm run build

      - name: Start onboarding
        working-directory: apps/onboarding
        env:
          PLATFORM_API_URL: http://localhost:8086
          PORT: "4201"
        run: |
          npm run start &
          for i in $(seq 1 30); do
            code=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:4201/onboarding || true)
            [ "$code" = "200" ] && exit 0
            sleep 2
          done
          echo "::error::/onboarding never returned 200 (last: ${code:-none})"
          exit 1

      - name: Run the onboarding suite and verify the exact pass count
        working-directory: apps/onboarding
        env:
          BASE_URL: http://localhost:4201
          API_URL: http://localhost:8086
        # THREE gates, as in e2e-runnable.yml. None is sufficient alone:
        #  - playwright's exit status;
        #  - exactly <N> PASSED, because an all-skipped run also exits 0 —
        #    the mark8ly#834 failure this job exists to prevent;
        #  - no "N failed"/"N timed out", so a correct pass count can never
        #    coexist with a failure.
        # Patterns are not anchored to start-of-line: the reporter emits ANSI
        # cursor escapes ahead of the summary. [^0-9] stops "<N> passed"
        # matching a larger count.
        run: |
          set -o pipefail
          status=0
          out=$(CI=true npx playwright test --reporter=line 2>&1) || status=$?
          printf '%s\n' "$out"
          fail=0
          if [ "$status" -ne 0 ]; then
            echo "::error::playwright exited $status"
            fail=1
          fi
          grep -qE '(^|[^0-9])<N> passed' <<<"$out" || {
            echo "::error::did not see exactly <N> passed — the onboarding suite is not actually executing and passing"
            fail=1
          }
          if grep -qE '(^|[^0-9])[0-9]+ (failed|timed out)' <<<"$out"; then
            echo "::error::a failed or timed-out test coexists with the expected pass count"
            fail=1
          fi
          exit $fail

      - name: Dump service logs on failure
        if: failure()
        run: docker compose -f infra/dev/docker-compose.yml logs --no-color --tail=200

      - name: Upload failure artifacts
        if: failure()
        uses: actions/upload-artifact@330a01c490aca151604b8cf639adc76d48f6c5d4 # v5
        with:
          name: onboarding-e2e-failure
          path: apps/onboarding/test-results/
          if-no-files-found: ignore
          retention-days: 7
```

- [ ] **Step 2: Confirm the pass-count placeholder is gone**

```bash
grep -n '<N>' .github/workflows/e2e-onboarding.yml
```

Expected: no output. If `<N>` remains, the workflow will never pass — substitute the measured integer from `docs/superpowers/plans/2026-09-09-e2e-baseline.md`.

- [ ] **Step 3: Commit and push the branch**

```bash
git add .github/workflows/e2e-onboarding.yml
git commit -m "ci(e2e): run the onboarding suite against a real platform-api stack (#858)"
git push -u origin HEAD
```

- [ ] **Step 4: Dispatch the workflow and confirm it is green**

```bash
gh workflow run e2e-onboarding.yml --ref "$(git branch --show-current)"
sleep 30 && gh run list --workflow=e2e-onboarding.yml --limit 1
```

Watch it to completion. A workflow that has never been dispatched is not verified — that is the whole subject of #834.

- [ ] **Step 5: Prove the canary actually bites**

Temporarily change the expected count to `<N+1>`, dispatch again, and confirm the job goes **red** with the "did not see exactly" error. Then revert. A canary that has only ever been observed passing has not been tested.

```bash
git commit -am "test(ci): temporarily break the onboarding canary to prove it fails closed"
gh workflow run e2e-onboarding.yml --ref "$(git branch --show-current)"
# after confirming red:
git revert --no-edit HEAD
git push
```

---

### Task 4: Open the follow-up issues Task 1 uncovered

**Files:** none — GitHub only.

**Interfaces:**
- Consumes: the "specs that need Zitadel" list from Task 1's baseline document.

- [ ] **Step 1: File one issue per spec that failed for a reason unrelated to Zitadel**

Task 1 will likely surface specs that fail for their own reasons (a changed selector, a dead route, a renamed field). Those are real defects found by running tests that had never run — the point of this work. Each gets its own issue citing the exact first error line from the baseline document.

Write bodies correctly the first time: `gh issue edit` cannot update a body in these repos, so a wrong reference cannot be corrected afterwards. Run `gh issue create` as a direct command, never from inside a shell script.

- [ ] **Step 2: Comment the measured baseline on #858**

Post the per-spec table and `ONBOARDING_PASS_COUNT` so the numbers live on the issue, not only in a plan file.

---

### Stage 2 outline (Tier C) — to be step-planned after Stage 1 lands

Add `marketplace-api` (already defined in the compose file, `MODE=both`, :8091) and the storefront and admin apps to the CI path. Unlocks `apps/storefront/tests/e2e/home.spec.ts`, `auth-isolation.spec.ts`, and the admin probe specs that do not sign in.

Open question to resolve first: production runs marketplace-api as **separate** admin and storefront deployments with migrations on the admin one only, while the compose stack runs one process with `MODE=both`. Decide whether Stage 2 mirrors the split or accepts the divergence, and record which.

### Stage 3 outline (Tier D) — to be step-planned after Stage 2 lands

Add a Zitadel service to the compose file plus a bootstrap script (~150 lines, written after reading `tesserix-k8s/charts/apps/zitadel-bootstrap/files/bootstrap.py`), wire `auth-bff`, and feed the minted client id into the Next builds. Unlocks the 18 auth-dependent admin specs.

Non-negotiables established in the spec:

- Bootstrap **must** run before `next build` — `NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID` is inlined at build time.
- The test user **must** get an explicit project grant, or `projectRoleCheck=true` yields a 403 at finalize *after* a successful password check, which reads as broken auth rather than missing setup.
- Assert on the **success** path of every bootstrap call: Zitadel v2 protojson flattens oneofs, and a wrapped oneof returns 200 while silently doing the wrong thing.
- Never point this stack at the shared live instance at `auth.tesserix.app`.
