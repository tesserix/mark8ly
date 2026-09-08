# 48 e2e specs, no workflow, and three populations wearing one name (#834)

## What #834 gets wrong, measured on `origin/main`

| #834 says | Measured |
|---|---|
| 42 specs | **48** — admin 37, onboarding **7**, storefront 4. Onboarding is absent from the issue entirely. |
| "they mock the backend they test" | **5 of 48** call `page.route`. The other 43 do not. |
| `arbitrage.spec.ts` as the exemplar | Deleted in #829/#704. The example no longer exists. |

The generalisation from one spec is the issue's central error. "Mocked e2e" is not
the problem here; it describes a tenth of the suite.

## What is actually in `tests/e2e/`

Three populations with different purposes, one directory, one `playwright test`
entry point:

**A. Local-stack integration — 20 specs.** Reach `completeOnboarding`,
`onboardStore`, `fetchMagicLinkToken`, or a `localhost:{4201,4202,8086,8089}`
helper. These are real tests. They need Postgres, platform-api, auth-bff,
marketplace-api, OpenFGA and three Next servers.

**B. Opt-in operator scripts — 8 specs**, each behind an env flag: `FULL_FLOW`
(admin-setup, admin-verify, full-flow, storefront-journey), `ADMIN_AUDIT` and
`STOREFRONT_AUDIT` (the two remote-audits), `SEED_IMAGES`
(seed-product-images), `STOREFRONT_VISUAL_TEST_SLUG` (layout-blocks). Five of
them still default to a **production** host. They seed data, take screenshots
and audit a live system. They are tools, not tests, and no CI run should ever
execute one.

**C. Backend-free — 3 specs.** `admin/pricing`, `onboarding/form-validation`,
`onboarding/tax-id-migration`. Assert client-side state only.

The remainder are ungated specs that need a live backend and, with `?? ""`
defaults, fail on an empty `baseURL` rather than doing anything.

**One piece of good news, verified:** all five prod-defaulting specs are behind
an opt-in flag, so a bare `playwright test` cannot reach production. This became
true only with #844; before it, `admin-setup` also carried real credentials.

## Dead specs — 9, each verified

- **`a11y-audit`, `cancellation`, `plan-change`, `pro-app`, `tax-id`** navigate
  to `/admin/settings/billing` and `/admin/stores/${id}/...`. **That URL space
  does not exist.** `apps/admin/app/(admin)` is a route *group* — the
  parentheses are excluded from the path — and `app/api/admin/...` is an API
  route, not a page. They also mock `**/api/v1/admin/...` while the app calls
  `/api/admin/...`, so the interception matches nothing either. They pass
  because their assertions are `getByText(/upgrade/i).or(...)`-shaped.
- **`settings-themes`** expects an `h1` of "Themes & branding"; the page renders
  "Branding".
- **`storefront-final`** navigates to a product handle hardcoded from one past
  run.
- **`account-shot`** reads `.audit/customer-state.json` and never writes it.
- **`storefront/remote-audit`** visits `/orders`, which has only `[id]/`.

This is #834's stated failure mode — a spec passing while what it tests is gone
— live in nine files, and in six of them by mocking an endpoint that never
existed at that path.

## Tasks

### 1. Separate the three populations
- [ ] Move the 8 opt-in scripts out of `tests/e2e/` to `tests/operator/`, with a
      README stating they target live systems, require credentials, and are
      never run by CI.
- [ ] Delete the 5 specs whose URL space does not exist. They cannot be
      repaired into anything meaningful: the pages, the endpoints and the mock
      prefixes are all wrong, so "fixing" them is writing new specs. Record on
      #834 which billing flows thereby have no e2e coverage, so the gap is
      stated rather than silently inherited.
- [ ] Fix the 4 cheaply-repairable dead specs (`settings-themes` heading,
      `storefront-final` handle, `account-shot` state path,
      `storefront/remote-audit` route).
- **Done when** `tests/e2e/` holds only specs that are either CI-runnable or
  local-stack tests, and nothing in it targets a production default.

### 2. Run the runnable subset in CI, and prove it ran
- [ ] `playwright.ci.config.ts` per app, with a `webServer` block building and
      serving the app. No config currently declares one, which is the mechanical
      reason nothing runs.
- [ ] A standalone `e2e-ci.yml`, **not** a job in `ci.yml`: that caller is at
      exactly **181/181** capped meaningful lines, enforced by
      `.github/scripts/test-ci-contract.py`, and the cap exists so growth
      argues for itself. `dashboard-validation.yml`, `pii-leak-guard.yml` and
      `logging-smoke.yml` establish the standalone pattern.
      Path filters must include `apps/**`, not just the workflow and its config
      — a filter that only watches itself is the #834 failure in a new costume.
- [ ] **A canary asserting the expected spec count actually executed.** Copy the
      precedent in `ci.yml`'s integration job, which greps for `--- PASS:`
      because "an integration test with no TEST_DATABASE_URL SKIPS and the
      package still prints ok — a green suite here would otherwise prove
      nothing." Playwright exits 1 on "No tests found" (verified), but a spec
      whose tests all skip exits 0 having asserted nothing.
- [ ] Assert CI never sets `FULL_FLOW`, `ADMIN_AUDIT`, `STOREFRONT_AUDIT` or
      `SEED_IMAGES`.
- [ ] Add the `test:e2e` script `apps/storefront/package.json` lacks — its 4
      specs currently cannot be invoked at all.
- **Done when** 3 specs run on every PR touching `apps/**`, and deleting one
  turns the job red rather than shrinking a number nobody reads.

### 3. Route manifest — the instrument for the #826 class
- [ ] A Go test in `cmd/marketplace-api` that builds the real engine and
      reconciles `engine.Routes()` against a committed `route-manifest.json`,
      **bidirectionally**: mounted-but-undeclared and declared-but-unmounted are
      distinct failures.
- [ ] Reuse `route_parity_test.go`'s `fillHandlers`, which defeats the
      `if deps.X != nil` conditional registration that makes many routes
      invisible to a source grep.
- [ ] Runs in the existing `Go (marketplace-api)` job. **No `ci.yml` change**,
      so no cap argument.
- **Done when** deleting a handler fails the build, and the manifest diff shows
  `- POST /api/v1/admin/.../arbitrage-appeal` as a review signal on its own.

**Do not write a source-literal extractor.** This estate has already built this
instrument twice — `internal/handlers/admin/route_parity_test.go` and
`internal/handlers/platformadmin/conformance_declaration_test.go` — and
`route_parity_test.go:36-42` explicitly rejects source-parsing and hand-kept
lists as an anti-pattern that "already bit this codebase once". Gin's real route
table is the honest source. The one OpenAPI spec in the repo
(`internal/handlers/storefront/openapi.go`) is hand-written, storefront-read-only
and says in its own header that it is deliberately not derived from the code —
it is not a candidate.

### 4. Check the frontend and the e2e mocks against the manifest
- [ ] ~144 of ~145 marketplace-api paths referenced by `apps/admin` are literal
      and extractable; the single dynamic one is an otto catch-all, not
      marketplace-api. Resolve the base variable to classify which service a
      path belongs to — `mediaUploadClient.ts` writes
      `${MARKETPLACE_API_URL}/api/admin/...` with no `/api/v1`, and that is
      correct, because the base resolves to `""` in the browser and the request
      lands on a Next proxy. Classifying by path prefix produces a false
      positive there.
- [ ] Follow `proxyAdminApi` and the three `*Url()` suffix helpers one hop.
- [ ] **Cover `page.route()` globs in the same check.** That is where #834
      actually hid, it is the cheapest win, and it goes red on six real files
      the moment it is switched on.
- **Done when** a path the backend does not serve fails CI, whether it is called
  by the app or mocked by a spec.

## Known limits, stated rather than discovered later

- **Method drift is invisible** to a path-only check: the method sits in an
  adjacent object literal or the `apiClient.get/post` selector. Dropping
  `PATCH /coupons/:id` while keeping `GET` would pass. An AST pass would catch
  it; regex will not.
- **Shape drift is invisible entirely.** #826's `arbitrage_flag` was a *field*.
  A route check catches the deleted endpoint, never a deleted response field.
  That needs Zod schemas reconciled against Go DTO tags — a separate and much
  harder instrument, and it should be its own issue rather than a silent gap in
  this one.
- **Population A stays unrun.** 20 local-stack specs still need a stack CI
  cannot stand up; there is no `docker-compose` or stack script in the repo, and
  25 admin specs authenticate for real through Zitadel. This plan does not fix
  that — it stops the suite misrepresenting itself, and makes the drift the
  stack would have caught detectable without one. Standing up the stack is a
  separate decision with an auth story attached.

## Decisions this needs before task 1

1. **Where do the 8 operator scripts live**, and do they stay Playwright?
   `tests/operator/` keeps the tooling; `scripts/` says more clearly that they
   are not tests.
2. **Delete or repair the 5 dead billing specs?** Recommend delete: their pages,
   endpoints and mock prefixes are all wrong, so repair is authorship. But it
   removes the only nominal e2e coverage of cancellation, plan change, pro-app
   and tax-id.
3. **Scope.** Tasks 1+2 make the repo honest and are ~a day. Tasks 3+4 build the
   instrument that would have caught #826. Both halves are worth doing; the
   question is whether they ship together or 1+2 first.
