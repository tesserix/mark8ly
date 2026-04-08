# Onboarding-Only First Slice — Phase Plan

## Why scope down to onboarding only

The original instinct was "rewrite everything." Refined to: **port one full
vertical slice first, then re-evaluate.** Onboarding is the right slice
because:

- Smallest of the three frontend apps (~213 TS/TSX files)
- Most isolated — fewest backend service dependencies
- Where the auth/authz bugs live, so fixing it has immediate value
- Establishes every convention the other apps will follow
- Shippable on its own — if work stops after this slice, you still have a
  working onboarding flow on a clean codebase

## Scope

**In:**
- One frontend: `apps/onboarding`
- One backend monolith: `services/platform-api` (only what onboarding needs)
- One auth service: `services/auth-bff` (only what onboarding needs)
- Real GIP (via Firebase Auth Emulator in dev) and real OpenFGA in dev
- golang-migrate + two-binary pattern (server + migrate)
- Local docker-compose for everything
- Playwright e2e for the full onboarding flow
- Just enough shared packages to support the above (only `ui`,
  `eslint-config`, `typescript-config`)

**Out (for now):**
- admin, storefront
- marketplace-api
- notification, document, payment as separate services (inlined or stubbed)
- Production infra (Terraform, K8s, ArgoCD)
- CI beyond basic lint+test+build
- Codegen tooling (hand-write types in this phase)

## Backend surface needed

**`platform-api` exposes:**
- Onboarding sessions: create, get, update, list events, draft save / heartbeat /
  browser-close, draft fetch by sessionId
- Verification: send email OTP, verify token, token info, method lookup, resend
- Tenants: lookup by id (internal), create-on-completion
- Locations: countries / states / cities lookup
- Tenant routing: subdomain reservation, custom domain check (stub OK for v1)
- Test helpers: verify-email bypass for e2e (gated by env flag)

Roughly **6 internal domains**: `onboarding`, `verification`, `tenant`,
`location`, `tenantrouter`, `test`.

**`auth-bff` exposes:**
- TOTP setup initiate
- TOTP setup confirm
- Auto-login after onboarding completion
- Session cookie issue / validate (bare minimum — full MFA / passkey can wait)

**Inlined into platform-api (not separate services in this phase):**
- `notification` (SendGrid SDK calls inline)
- `document` (GCS signed-URL helpers inline)

**Stubbed in dev:**
- `tenant-router-service` Cloudflare DNS provisioning (real DNS happens at
  deploy time, not in local dev)

**Deferred entirely:**
- `payment` — onboarding can collect payment for paid plans. Port the UI,
  stub the API to always return success in dev.

## Phase plan

### Phase A — Foundations (3–4 days)

**Goal:** `make dev` brings up the empty stack, `make test` and `make e2e`
run successfully against empty placeholders.

- Turborepo workspaces wired (root `package.json`, `turbo.json` updates)
- `go.work` with `services/platform-api` and `services/auth-bff`
- Empty `platform-api` with health endpoint, reads config, connects to
  Postgres, **calls `migrate.AssertVersion` on startup**
- Empty `auth-bff` with health endpoint, same pattern
- `cmd/migrate` binary scaffolded for both services
- `cmd/seed` binary scaffolded for platform-api
- Empty `apps/onboarding` (delete the scaffolded `web` and `docs` from
  create-turbo)
- `infra/dev/docker-compose.yml` with:
  - postgres
  - openfga + openfga-migrate init
  - firebase-auth-emulator
  - fga-seed (creates store, loads model from `infra/openfga/model.fga`)
  - gip-seed (creates GIP tenant pools via emulator REST API)
  - platform-api-migrate init container
  - platform-api-seed init container
  - platform-api
  - auth-bff-migrate init container
  - auth-bff
  - onboarding (Next.js)
- `infra/openfga/model.fga` — minimal model (`user`, `tenant` with `member`
  and `owner`)
- `Makefile`: `dev`, `test`, `e2e`, `lint`, `build`, `clean`, `migrate-up`,
  `migrate-down`, `migrate-new`, `migrate-version`, `seed`
- Root `README.md` with one-command setup
- Basic GitHub Actions: lint + test on PR (no deploy)

**Exit criterion:** clone repo → `make dev` → all containers healthy →
`make e2e` runs an empty Playwright suite that passes. **No business code
yet.**

### Phase B — Diagnose old auth bugs (1 day)

**Goal:** write down exactly what the bugs were, before porting code that
might preserve them.

Read the old code carefully:
- `auth-bff/internal/gip/*` — pool selection, issuer URL, verifier caching
- `auth-bff/internal/session/*` — cookie domain, SameSite, Secure, expiry,
  encryption key, payload size
- The onboarding `auto-login` route in the backup
- `marketplace-admin/middleware.ts` — tenant validation cache, fail-open
  behavior, negative-result caching
- OpenFGA tuple write timing in old `tenant-service` onboarding completion
  handler — is it inside the DB tx? After? What happens on failure?

Write findings into `docs/auth-bugs.md` — one section per bug, with file:line
references and suspected root cause.

**Exit criterion:** a written list of likely bugs with locations. Doesn't
have to be perfect — just enough that during the port, the bugs are caught
instead of recreated.

### Phase C — Port platform-api skeleton + location domain (1–2 days)

Location is the smallest, dumbest, most-isolated domain. Perfect for
establishing patterns.

- `pkg/httpserver/`: Gin setup, request ID middleware, structured logging,
  error handler, recovery
- `pkg/db/`: GORM init with retry
- `pkg/config/`: env loading via godotenv + struct binding
- `pkg/logger/`: slog (modern call)
- `pkg/errors/`: typed errors + HTTP mapping
- `pkg/migrate/`: golang-migrate wrapper (Up, Down, Version, AssertVersion)
- `internal/location/`: models, repository, service, handler — port from old
  `location-service`
- `migrations/0001_create_locations.up.sql` — countries, states, cities
- `migrations/0001_create_locations.down.sql`
- `seed/locations.json` — countries from a static JSON file
- `cmd/seed` reads `seed/locations.json` and inserts with
  `ON CONFLICT DO NOTHING`
- Unit tests for the location service
- Wire into `cmd/server/main.go`

**Exit criterion:**
`curl localhost:8080/api/v1/locations/countries` returns a list. Tests green.

### Phase D — Verification + tenant + onboarding domains (4–6 days)

Now the real work. These three are tightly coupled.

- `internal/verification/`: email OTP send + verify, token storage, expiry
- `internal/tenant/`: tenant CRUD, slug uniqueness, internal lookup endpoint
- `internal/onboarding/`: session lifecycle, draft save/heartbeat, events,
  completion handler
- `internal/storage/`: GCS signed URL generation + validation (replaces
  document-service)
- `internal/notification/`: SendGrid wrapper for welcome email (inlined,
  not a separate service)
- `internal/authz/`: thin OpenFGA SDK wrapper
  (`WriteMembership`, `CheckMembership` — two methods)
- `internal/outbox/`: failed FGA writes table + drainer
- `internal/test/`: e2e helpers (verify-email bypass), gated by `ENV=dev|test`
- Migrations for each domain
- Unit tests per domain
- One integration test that exercises the full session: create → save draft
  → submit verification → complete

**The onboarding completion handler implements the outbox pattern:**
1. Begin DB tx
2. Insert tenant row
3. Insert membership row
4. Insert outbox row for the FGA write
5. Commit DB tx
6. Best-effort: write the FGA tuple immediately
7. If success, mark outbox row completed
8. Background drainer (in-process for now) processes pending outbox rows

**Discipline:** for each domain, copy the old handler/service/repo into the
new structure, fix imports, run tests. Don't refactor while porting.
Refactor passes happen after the test passes.

**Exit criterion:** integration test creates a tenant via the onboarding
flow end-to-end against a real Postgres + real OpenFGA in docker-compose.

### Phase E — Port auth-bff (3–4 days)

Only the parts onboarding needs.

- `pkg/` setup mirroring platform-api
- `internal/gip/`: real OIDC client. In dev, points to
  `http://firebase-auth-emulator:9099`. In prod, real GIP.
- **Multi-tenant aware** — the issuer/audience changes per GIP tenant pool
  (`platform`, `mp-internal`, `mp-customer`). This is the bit most likely to
  have been wrong in the old code; pay attention.
- `internal/session/`: encrypted cookie sessions, applying the
  cookie-domain / SameSite / Secure fixes from Phase B
- `internal/totp/`: TOTP setup initiate + confirm
- `internal/autologin/`: post-onboarding session mint. **Before issuing the
  session, do an FGA `Check` with retry-on-not-found** (up to ~2 seconds).
  This closes the tuple-write race window.
- `internal/handlers/`: HTTP layer
- Unit tests
- Wire into `cmd/server/main.go`

**Exit criterion:** integration test logs a user into auth-bff, gets a session
cookie, validates it on a protected endpoint, and the auto-login regression
test passes (onboard → immediately auto-login → succeed).

### Phase F — Port the onboarding Next.js app (4–6 days)

Port the frontend route by route. Order matters — start with entry pages and
work through the wizard.

- `app/layout.tsx`, providers (theme, analytics)
- `lib/api/`: typed client for platform-api (hand-written, not generated)
- `lib/auth/`: auth-bff client
- `lib/content/`: Drizzle schema for the marketing CMS DB (port schema and
  migrations)
- Marketing/static pages (home, blog, guides, help center) — port as-is
- Onboarding wizard pages — port step by step, each step gets a Playwright spec
- API route proxies — replace the old proxy-everything pattern with **direct
  calls to platform-api from server components and route handlers**, using
  the typed client. Keep proxies only where server-side secrets injection is
  required.
- `middleware.ts` — port tenant/auth resolution, applying cookie/domain fixes
  from Phase B

**Discipline:** copy old components/pages first, get them rendering, then
clean up. Don't rewrite UI from scratch.

**Exit criterion:** onboarding app boots, every page renders, every step
submits successfully against the local platform-api.

### Phase G — Playwright e2e suite (2–3 days)

The deliverable that proves everything works:

- **Golden path:** visit landing → start onboarding → fill business info →
  verify email (via test helper) → set up TOTP → complete → auto-login →
  land on welcome page
- **Tuple-write race regression:** onboard → immediately auto-login →
  confirm authenticated. If this ever flakes, the bug is back.
- **Outbox recovery:** onboard → kill platform-api before outbox drains →
  restart → confirm tuple eventually appears.
- **GIP pool isolation:** login with wrong tenant pool → confirm 401.
- Variations: invalid email, expired OTP, slug collision, duplicate email,
  browser-close-and-resume.
- Migration tests: drop dev DB → run `make dev` → confirm migrations apply
  cleanly. Run migrate up twice → confirm idempotent. Up → down → up →
  confirm down migrations work.
- Run the suite in CI against docker-compose.
- Run it locally with `make e2e`.

**Exit criterion:** `make e2e` is green on a clean clone, in CI, and locally.

### Phase H — Polish + docs (1–2 days)

- README explaining the architecture, how to run, how to test
- One-page architecture doc with the diagram
- `.env.example` files documenting every env var
- `docs/migrations.md` — how to add migrations, the backwards-compat rule,
  rollback, testing against a Cloud SQL clone
- Cleanup of any TODOs introduced during porting
- Tag a v0.1.0

**Exit criterion:** a new contributor (or future-you in 3 months) can clone,
read the README, and have it running in 10 minutes.

## Updated timeline

| Phase | Estimate |
|---|---|
| A: Foundations | 3–4 days |
| B: Diagnose auth bugs | 1 day |
| C: Location domain + pkg | 1–2 days |
| D: Verification + tenant + onboarding domains | 4–6 days |
| E: auth-bff | 3–4 days |
| F: onboarding Next.js | 4–6 days |
| G: Playwright e2e | 2–3 days |
| H: Polish + docs | 1–2 days |

**Total: ~3.5 weeks of focused work. Realistic: 4–5 weeks with buffer.**

## Discipline rules for this slice

These are the things most likely to derail the timeline if violated:

1. **Behavioral parity first.** No "improvements" to API contracts during the
   port. The new code must produce identical responses for identical inputs
   as the old code.
2. **No premature shared packages.** Resist the urge to extract while writing.
   Wait for the second consumer.
3. **Copy-then-clean, not rewrite-from-memory.** Copy old code into the new
   structure, make it compile, run tests, *then* clean.
4. **Don't port admin/storefront stuff "while you're in there."** Even if it
   would only take 10 minutes. It compounds.
5. **Don't add features not on the kill-list.** If it's not in the scope
   above, it doesn't get built.
6. **Diagnose auth bugs before porting auth code.** Phase B exists for a
   reason.
7. **`mark8ly_backup/` stays around as read-only reference** for the duration.
   Don't delete it until cutover.

## What this slice does and doesn't deliver

**Delivers:**
- A working, tested, debuggable onboarding flow
- A small clean platform-api you understand top to bottom
- A small clean auth-bff with the auth bugs actually fixed
- A local dev environment that takes one command
- A test suite that catches regressions
- Established patterns for porting admin and storefront later
- The migrations pattern proven in production-grade form
- The GIP + OpenFGA integration debugged and stable

**Does not deliver:**
- A working admin or storefront
- Production deployment of the new system
- Migration of real data from the old system
- Any decision about the admin/storefront restructure (revisited after
  this slice)

## Phases beyond the v0.1 onboarding slice

Phases A–H shipped the kill-list slice. Everything below was added
after v0.1 was tagged, when the re-evaluation point in the planning doc
asked "what's next?" and we picked admin as the answer. The ordering
mirrors the same discipline: smallest-useful-thing first, then
re-evaluate.

### Phase I — Admin workspace scaffold *(shipped)*

`apps/admin` Next.js workspace at `localhost:4202`. Middleware-based
session-cookie gate, `AdminShell` chrome with sidebar + topbar, stub
routes for /products /orders /customers /settings, /login redirect
target, /logout cookie clear. No real auth yet — just cookie-presence.

### Phase J — Real session validation + cross-app auth handoff *(shipped)*

`auth-bff GET /auth/session` and `POST /auth/logout`. Admin middleware
calls auth-bff on every request, fail-closed on invalid/tampered/
unreachable. Resolved session forwarded to pages via request headers
(`x-session-user-id`, `x-session-email`, `x-session-tenant-id`). Admin
dashboard reads them and calls `platform-api /internal/tenants/:id`
to render the real tenant name. Cross-app Playwright e2e — anonymous
admin visit → bounce → onboarding → magic link → admin dashboard with
real tenant → logout → bounce again.

### Phase K — Admin chrome port from legacy *(shipped)*

`@tesserix/web` Sidebar primitives. Collapsible nav groups (Analytics,
Catalog, Orders, Customers, Marketing, Settings, Support) ported from
`mark8ly_backup/apps/admin/app/(tenant)/layout.tsx` with the same icon
set. Topbar gets a UserMenu dropdown showing the merchant's email
with Profile / Settings / Sign out. ComingSoon placeholder restyled
to use the admin token set (`bg-card`, `border-border`, etc) instead
of the warm-editorial palette.

### Phase L — Cross-tab/cross-device magic link *(shipped)*

The verify page used to read GIP credentials from per-tab
sessionStorage, breaking when the user clicked the magic link in a
different tab/device. Phase L moves the credentials into the
persisted onboarding session draft (existing JSONB column, no
migration). New `verifyAndLoginByToken` server action takes ONLY the
token, fetches the draft from platform-api, refreshes the GIP
id_token, completes onboarding, mints the cookie, all server-side.
Welcome page gains an "Open admin dashboard →" CTA. New Playwright
test opens the magic link in a fresh browser context (no shared
storage) — proves cross-device works.

### Phase M — Returning-user sign-in + post-verify credential picker *(shipped)*

Closes the loop for users who already have a tenant AND restructures
the signup flow so the credential (password OR Google) is collected
AFTER the email is verified, not before.

The shipped UX is:

1. **Onboarding form** at `/onboarding` collects email + business
   name + slug + country + currency. No password, no Google button.
   Submit creates the session, persists the business draft, sends a
   magic link.
2. **Magic link click** lands on `/onboarding/verify?token=…`. The
   `verifyToken` server action marks the session verified server-side
   and the page redirects to `/onboarding/set-password?session=…`.
3. **Set-password page** is a server-component shell that fetches
   the session from platform-api, asserts it's verified, and renders
   the `SetPasswordForm` with the verified email + business name.
   The form offers two paths to a credential:
   - Email + password (min 8 chars) → `signUp(email, password)` via
     Identity Toolkit `accounts:signUp`.
   - **Continue with Google** via `getGoogleCredential` (gsi/client
     popup) → `signInWithGoogle` (Identity Toolkit
     `accounts:signInWithIdp`). Defense-in-depth check: the Google
     credential's email must match the session's verified email, so
     a stray Google account can't hijack a session.
   Both paths produce a fresh GIP id_token + uid + refreshToken and
   call the `completeOnboarding` server action, which reads the
   draft, calls platform-api `complete()` (the session is already
   verified, no bypass needed), calls auth-bff `/auth/auto-login`,
   and forwards the session cookie. Lands on `/welcome`.
4. **Returning users** get their own `/login` page on the **admin app**
   (port 4202). It hosts a real client form: email + password + a
   "Continue with Google" button. Both paths produce a GIP id_token,
   handed to admin's `signIn` server action which looks up the
   workspace tenant by GIP UID via platform-api
   `GET /api/v1/tenants/by-owner?uid=…`, then calls
   `/auth/auto-login`. Lands on `/dashboard`.
5. **Admin middleware** redirects unauth users to admin's own `/login`
   instead of bouncing across origins to the marketing site.

New backend surface (kept):

- `tenant.GetByOwnerUserID` repo + service + handler + tests
- `GET /api/v1/tenants/by-owner?uid=…`

Removed (dead code from the earlier iteration that did
verify-google-on-signup before this restructure):

- `onboarding.GoogleVerifier` Identity Toolkit `accounts:lookup` client
- `onboarding.VerifyGoogleAndMark` service method + `verify-google`
  handler/route
- `GIP_API_KEY` / `GIP_TENANT_ID` config on platform-api
- `submitOnboardingWithGoogle` server action and the onboarding
  `signInWithPassword` / `refreshIdToken` GIP REST helpers
- The onboarding `/login` stub (returning users go to the admin
  origin's `/login` directly)

New env vars:

- onboarding app: `NEXT_PUBLIC_GOOGLE_CLIENT_ID` for the set-password
  page's GSI button.
- admin app: same `NEXT_PUBLIC_GOOGLE_CLIENT_ID`. The `lib/gip` and
  `lib/auth/auth-bff` helpers are duplicated in both apps for now —
  small enough that a shared package isn't worth the build-graph cost.

Why the post-verify credential picker matters: a Google user never
sees the magic-link email but the email is still verified before the
tenant is created (via the magic-link round-trip). One funnel, two
credential paths, no special-case backend.

### Phase N — Tenant settings (general) page *(shipped)*

First real admin feature on top of the chrome. Proves the
auth/session/tenant-context plumbing holds when a page actually reads
and writes tenant data, and surfaces every onboarding-form field
back to the merchant so "what I typed when I signed up" is visible
and (where safe) editable.

What landed:

1. **Admin page** `apps/admin/app/settings/general/page.tsx` — server
   component pulling the tenant row from `getServerSessionContext()`
   (which already hits platform-api `GET /internal/tenants/:id`).
   Graceful fallback if the fetch fails. `/settings` redirects here.
2. **Update endpoint** on platform-api —
   `PATCH /internal/tenants/:id` (mounted under `/internal`, not
   `/api/v1`, matching the existing tenant-read pattern: trusted
   in-cluster callers only). Accepts a partial JSON body; Phase N
   ships with only `name` editable. Pointer fields in `UpdateInput`
   keep "unset" vs "empty" distinguishable, so adding more fields
   later is additive.
3. **Repository `UpdateEditable`** — `map[string]any` patch against
   GORM `Updates()`, auto-bumps `updated_at`, translates unique
   violations to `tenant_update_conflict`, returns `tenant_not_found`
   on zero rows affected.
4. **Service `Update`** — trims whitespace, rejects empty names,
   caps at 200 chars (matches the DB `VARCHAR(200)`), rejects
   empty patches with `empty_update`. 6 new service tests cover the
   happy path and every rejection branch.
5. **Server action** `updateGeneralSettings` in
   `apps/admin/app/settings/general/actions.ts` — reads tenant id
   from middleware-forwarded session headers (never the browser),
   re-validates server-side, calls `updateTenant` on the platform-api
   client, then `revalidatePath("/", "layout")` so the AdminShell
   top-bar tenant name refreshes everywhere on next nav.
6. **Form** `apps/admin/components/settings/GeneralSettingsForm.tsx`
   — client component. Surfaces **all** onboarding fields:
   - **Editable**: store name.
   - **Read-only**: slug (as a `{slug}.mark8ly.com` URL preview),
     owner email, country code, currency code, timezone. Each gets
     a "contact support to change" hint explaining why it's locked.
   Simple `useState` + `useTransition` + `router.refresh()` — no
   RHF/Zod yet because there is only one editable field; we upgrade
   when the second lands. Error and success banners wired to the
   server action's discriminated-union return type.
7. **E2E** `apps/admin/tests/e2e/settings-general.spec.ts` — two
   specs. Happy path: onboard a merchant, sign in, navigate to
   `/settings/general`, assert every onboarding field is visible
   with the value the merchant typed, edit the name, save, reload,
   assert the new name comes back from the DB. Negative path:
   assert whitespace-only name is rejected.

Deliberate scope cuts from the original spec:

- **No `logo_url` or `contact_email` fields.** The original plan
  listed them but the `tenants` schema has neither column. Adding
  two columns (plus a GCS upload flow for the logo) would have
  quadrupled the slice. Reuse the fields that already exist, ship,
  re-evaluate.
- **No currency / timezone / country editing.** Currency change has
  billing implications, country affects tax, timezone editing
  needs a searchable picker for ~400 entries to be usable. All
  three surface read-only with a support hint; each is a sensible
  follow-up slice.
- **No RBAC guard on the PATCH yet.** Phase N enforces "one owner
  per tenant" implicitly (a UID owns at most one tenant, and the
  server action uses the session tenant id unconditionally). Real
  FGA `Check()` lands in Phase O and retrofits onto this endpoint.
- **No generic settings layout with a side sub-nav.** Single page
  flat until a second settings page justifies the refactor.

### Phase O — Roles + RBAC *(shipped)*

Extended the OpenFGA model so tenants can have more than one human
and so the admin app can hide/deny things based on role. Unblocks
the future "invite teammate" flow and retrofits a real FGA
`Check()` onto Phase N's PATCH.

What landed:

1. **OpenFGA DSL committed to the repo.** `infra/openfga/model.fga`
   now has four directly-assignable roles (`owner`, `admin`,
   `staff`, `viewer`) and three derived permissions:
   - `member` = `owner ∪ admin ∪ staff ∪ viewer`
   - `can_view_settings` = same as `member`
   - `can_edit_settings` = `owner ∪ admin`
   The `infra/dev/seed/fga-init.sh` hand-encoded JSON was updated
   to match — the DSL and JSON are now the two-file source of
   truth. Re-running `docker compose up openfga-seed` writes a new
   authorization model version; OpenFGA supports model versioning,
   so existing tuples keep working and new Check/Write calls use
   the latest version.
2. **`member` is now derived, not directly-assignable.** The old
   FGA handler wrote both an `owner` AND a bare `member` tuple on
   onboarding completion. Under the new model `member` is a union,
   so writing a bare `member` tuple is invalid. The handler
   (`services/platform-api/internal/onboarding/fga_handler.go`)
   now only calls `WriteOwnership` — the derived `member` relation
   resolves transitively to the owner tuple, so auth-bff's
   `CheckMembership` retry loop still works without any auth-bff
   change.
3. **`authz.Client` expanded.** Added `WriteRole(role)`, generic
   `Check(relation)`, and `GetRole()` (iterates the four direct
   role relations in priority order and returns the highest).
   `WriteMembership` removed — it was only called by the onboarding
   FGA handler, which no longer needs it. `CheckMembership` is
   retained as a convenience wrapper over `Check(ctx, uid,
   "member", tid)` because auth-bff autologin uses the same
   semantic name. New `FakeClient` in `fake.go` keeps the derivation
   rules hand-maintained (six unit tests in `fake_test.go` lock
   down the DSL ↔ fake parity).
4. **`GET /internal/tenants/:id/me?uid=…`** — new endpoint on the
   tenant handler. Admin BFF calls this once per authenticated
   request to discover the caller's role. Returns
   `{ data: { role: "owner"|"admin"|"staff"|"viewer" } }` or
   `404 no_role` if the user has no role at all. Degrades to
   `role=owner` if the fga client is nil (dev without OpenFGA)
   so local iteration isn't blocked.
5. **`PATCH /internal/tenants/:id` retrofitted.** The handler now
   runs `fga.Check(uid, "can_edit_settings", tenantID)` before
   calling `svc.Update`. The `uid` is passed in the request body
   (not a header) because Gin/middleware edge cases on internal
   routes with empty header values were causing silent auth
   bypasses in early drafts. Returns `403 forbidden` on denied.
   Same nil-fga dev fallback as `/me`.
6. **Admin middleware forwards the role.** `apps/admin/middleware.ts`
   fetches the role from platform-api alongside the auth-bff
   session validation and forwards it as an `x-session-role`
   request header. Fails closed — if the user has a valid session
   but no role (deleted tuple, FGA outage), the request redirects
   to `/login` rather than rendering an admin page with no
   authorization context.
7. **`getServerSessionContext()` surfaces the role**, with
   `canEditSettings(role)` / `canViewSettings(role)` helpers that
   mirror the DSL. UI gating ONLY — server action re-checks.
8. **Settings page + form honour the role.** Non-editors see a
   read-only amber banner in the header (`Read-only: your role
   (viewer) can view settings but cannot edit them`), the name
   input is disabled, and the Save / Reset buttons are removed
   entirely so no amount of React DevTools fiddling re-enables
   the submit path. The server action re-runs `canEditSettings`
   before calling platform-api — fails fast with a clean 403
   message instead of a network error.
9. **AdminShell role badge.** Top bar shows a small uppercase
   pill with the current role (`owner` / `admin` / `staff` /
   `viewer`). Marked with `data-testid="role-badge"` for the
   e2e suite. The shell accepts `role` as an optional prop so
   stub pages that haven't been updated yet still compile.
10. **E2E coverage.**
    - Phase N's `settings-general.spec.ts` still green under
      Phase O wiring: the onboarded owner can edit the name,
      save, reload, and the PATCH now goes through the FGA
      Check end-to-end.
    - New `settings-role-gate.spec.ts`: onboards a merchant,
      grabs their uid from auth-bff `/auth/session`, writes
      an atomic `{delete owner, write viewer}` tuple pair
      against the live OpenFGA store, reloads `/settings/general`
      and asserts the role badge says "viewer", the read-only
      banner is visible, the name input is disabled, and the
      Save / Reset buttons have count 0.
    - New helper functions: `fetchPlatformStoreId` and
      `writeFgaTuples` in `apps/admin/tests/e2e/helpers.ts` —
      the minimum glue needed for future phase tests to seed
      FGA state without an invite-teammate UI.
    - Full admin suite: **6/6 passing**.

Deliberate scope cuts:

- **No invite-teammate UI.** The DSL supports admin/staff/viewer
  tuples and the write path (`WriteRole`) is there, but there's
  no page to invite a second user yet. When that lands it wires
  into the existing infrastructure with zero schema changes.
- **Role is not cached in the session cookie.** The plan called
  this out as a possibility; we chose to re-fetch from
  platform-api on every request instead. Avoids coupling
  auth-bff redeploys to model changes. ~10ms added latency per
  authenticated nav; move to an LRU or cookie cache when it
  shows up as a real problem.
- **Nav sub-items are not role-filtered.** All four roles can
  `can_view_settings`, so hiding the Settings section would be
  wrong. Future role-gated sub-nav (e.g. Payments for admin only)
  lands alongside the backing feature phase.
- **`CheckMembership` retained as a convenience method** — technically
  `Check(..., "member", ...)` would have been enough, but auth-bff
  and a bunch of comments use the "membership" name. Less churn
  to keep the wrapper.

**Legacy reference (`../mark8ly_backup`):** the old stack never
committed an FGA DSL file — the authorization model lived only
in the running OpenFGA store. What the code tells us:

- DB-level membership roles existed as string constants in
  `services/tenant-service/internal/models/models.go:538-544`:
  `owner`, `admin`, `manager`, `member`, `viewer`. These drove a
  `tenant_memberships` table, not FGA.
- FGA was only used for **one** platform-level tuple:
  `user:<idpUserID>` `owner` `tenant:<tenantID>`, written from
  `services/tenant-service/internal/services/onboarding_completion.go:505`
  via `platformFGA.Grant(...)` with a 3-attempt retry. No other
  roles (admin/manager/member/viewer) were ever written to the
  platform store.
- Separate store-level and vendor-level roles lived in the
  `staff` service (`services/staff/internal/services/fga_tuple_writer.go`)
  via `GrantStoreRole` / `GrantVendorRole`. Out of scope for Phase O
  — those belong to a future staff/marketplace slice.
- The go-shared authz client already exposes everything we need:
  `Can`, `Grant`, `Revoke`, `ListObjects`, `GrantRelation`,
  `WriteModel` (`packages/go-shared/authz/client.go:147-246`).
  Port or depend on this verbatim rather than reinventing.

Implications for Phase O:

1. **Drop `manager` and `member` from the initial model.** The old
   stack defined them but never wrote tuples for them. Start with
   `owner` / `admin` / `staff` / `viewer` and add more only when a
   real feature needs them.
2. **Commit the FGA DSL to the repo this time.** Put the model at
   `services/platform-api/internal/authz/model.fga` (or
   `infra/openfga/platform-model.fga`) with a tiny apply script
   that calls `WriteAuthorizationModel`. The legacy drift between
   "what the code writes" and "what the store accepts" came from
   having no checked-in source of truth.
3. **Reuse the legacy write path pattern.** The onboarding-complete
   tuple write in Phase O item 2 should copy the retry-with-backoff
   shape from `onboarding_completion.go:499-516`: 3 attempts,
   500ms → 5s, and on final failure mark the tenant `failed` and
   abort onboarding. Do not silently swallow.
4. **One store, not per-product.** The legacy code distinguished
   "platform store" (tenant membership) from a marketplace store
   (product-level). Phase O only touches the platform store;
   marketplace-store roles are a later phase when products ship.

### Phase P — Multi-tenant membership + invite teammate *(shipped)*

Phase O shipped the role infrastructure but no way for a second
human to enter the tenant. Phase P closes that loop AND fixes the
implicit "one tenant per UID" assumption that leaks through the
whole stack today (auth-bff session, `tenant.GetByOwnerUserID`, the
sign-in server action, admin middleware).

Phase P does NOT introduce the store abstraction. A tenant is still
the thing that owns a storefront URL via `tenant.slug`. Multi-store
is a dedicated later slice (Phase Q) because the migration is big
and tangling it with invites produces a monster commit.

**P.1 — Multi-tenant plumbing + tenant switcher**

1. **`tenant.ListMemberTenants(uid)`** replaces the implicit "owner
   lookup gives you the single workspace tenant" path. Implementation:
   FGA `ListObjects("user:<uid>", "member", "tenant")` → enrich from
   the `tenants` table with name/slug/role. Returns
   `[]{tenant_id, name, slug, role}`. `GetByOwnerUserID` stays as a
   thin wrapper for the sign-up auto-login path (tenant-just-created,
   uid must own exactly one).
2. **`GET /api/v1/users/me/tenants`** on platform-api. Public
   endpoint (called from the admin BFF with the uid from the
   session cookie). Powers the switcher dropdown.
3. **`POST /auth/switch-tenant {tenant_id}`** on auth-bff.
   Verifies the user has an FGA role on the target tenant via
   `CheckMembership`, mints a new session cookie with
   `tenant_id=<target>`. Keeps uid and email unchanged.
4. **`/login` → multi-tenant sign-in**. Phase M's `signIn` server
   action currently calls `GetByOwnerUserID`. Updated flow: after
   GIP id_token is minted, call `ListMemberTenants(uid)`.
   - 0 tenants: "no store found for this account" error (unchanged).
   - 1 tenant: auto-login against that tenant, land on `/dashboard`
     (unchanged UX).
   - 2+ tenants: auto-login against the first tenant, redirect to
     `/pick-tenant?returnUrl=/dashboard`. Minimal page listing the
     user's tenants as cards; clicking one calls `/auth/switch-tenant`
     then redirects.
5. **Tenant switcher in AdminShell**. New dropdown in the top bar
   next to the role badge. Shows current tenant name, lists others,
   clicking calls `/auth/switch-tenant` and reloads. Single-tenant
   users see a non-interactive label (no dropdown) so the UI stays
   quiet for the solo-founder default.
6. **Admin middleware unchanged in shape** — still reads a single
   `x-session-tenant-id` header — but the tenant id is now whatever
   the last switch put there. `/settings/general` etc continue to
   operate on the session's current tenant with no new plumbing.

**P.2 — Invitations**

1. **Migration**: new `invitations` table on platform-api with
   `id, tenant_id, email, role, token_hash, expires_at, status,
   invited_by_user_id, created_at, accepted_at`. No new tables
   for tracking accepted members — that's what FGA is for. The
   row only tracks pending/expired/revoked state.
2. **New FGA permission**: `can_invite_members = owner or admin`
   added to `model.fga` + `fga-init.sh` JSON. Same gate as
   `can_edit_settings` for Phase P; may tighten in a future slice
   if owner-only invites become a paid feature.
3. **`internal/invitation/` domain** on platform-api:
   - `POST /internal/tenants/:id/invitations` — body
     `{uid, email, role}`, FGA-Checked for `can_invite_members`.
     Generates a 32-byte random token, stores sha256 hash, sends a
     magic-link email via the existing `notification.Sender`.
     Rejects invites where `email` is already a member of the tenant.
     Rejects `role=owner` (owner is founder-only, can only transfer
     via a dedicated flow that doesn't exist yet).
   - `GET /api/v1/invitations/verify?token=...` — public. Hashes the
     token, looks up the row, returns `{tenant_name, role, expired,
     invited_email}` for the accept page to render without auth.
   - `POST /api/v1/invitations/accept` — public. Body `{token, uid}`.
     Verifies the GIP token server-side against the invited email
     (strict email match: GIP `email_verified=true` AND
     `email == invited_email`), writes `fga.WriteRole(uid, role, tid)`,
     marks invitation accepted, returns
     `{tenant_id}` so the admin BFF can auto-switch the session.
   - `GET /internal/tenants/:id/invitations` — lists pending invites
     for the team page.
   - `DELETE /internal/tenants/:id/invitations/:inv_id` — revoke.
4. **Admin `/settings/team` page**:
   - Sub-page of Settings (sub-nav item added to AdminShell's
     navigation array). FGA-gated by `can_view_settings`.
   - Header "Team"
   - Hardcoded first row: the tenant owner (from
     `tenant.owner_email` + role=owner). Members list is scoped
     to pending invitations for Phase P — a real "all current
     members" list needs either an OpenFGA `ListUsers` call
     (supported in 1.x but new) or a parallel Postgres staff
     table, both deferred.
   - Table of pending invitations: email, role, invited date,
     "Revoke" button (only visible to `can_invite_members` roles).
   - "Invite teammate" button (only visible to
     `can_invite_members` roles) → modal with email + role
     dropdown (admin/staff/viewer — owner excluded).
   - Server actions `inviteTeammate({email, role})` and
     `revokeInvitation({id})`. Both re-check role server-side.
5. **Public `/accept-invite?token=...` page** on admin:
   - New public prefix in admin middleware (alongside `/login`,
     `/logout`).
   - Server component fetches invitation via `verify` endpoint,
     renders "You're invited to join {store_name} as {role}".
   - Two paths to an authenticated session:
     - **Continue with Google** via the existing Phase M gsi/client
       helper. Popup → id_token → submit accept.
     - **Email + password** form. Split on whether the GIP email
       already exists:
       - Already exists (invited user has a Mark8ly account):
         `signInWithPassword` flow reusing the Phase M helper.
       - New account: `signUp(email, password)` then accept.
     The page picks the right sub-flow by calling
     `accounts:lookup` on Identity Toolkit for the invited email.
   - Strict email match enforced server-side in `accept` — the GIP
     token's verified email must equal the invited email or the
     request is rejected with "this invite was sent to X, please
     sign in with that account".
   - On success: accept endpoint returns the tenant_id, the admin
     `/accept-invite/actions.ts` calls the same `/auth/switch-tenant`
     path P.1 built, the user lands on `/dashboard` of the new
     tenant.
6. **Invitation email template** via `notification.Sender`. Subject:
   "You've been invited to join {store_name} on Mark8ly". Link:
   `https://{tenant_slug}-admin.mark8ly.com/accept-invite?token=...`.
7. **Dev test helper** — extend the existing
   `test/verification/latest` pattern with a similar
   `test/invitations/latest?email=...` endpoint so the e2e suite
   can bypass the inbox. Only mounted when `cfg.Env != "prod"`.
8. **E2E specs**:
   - **Invite → accept happy path**: onboard A as owner → navigate
     to `/settings/team` → open invite modal → fill B's email +
     role=viewer → submit → fetch token via test helper → open
     second browser context → `/accept-invite?token=…` → choose
     password path → sign up → land on /dashboard of A's tenant
     with role=viewer → navigate to `/settings/general` → assert
     the Phase O read-only banner is visible.
   - **Multi-tenant switcher**: onboard A as owner of tenant-A →
     onboard C as owner of tenant-C with a fresh email → A invites
     C's email as staff on tenant-A → C accepts → C's session
     auto-switches to tenant-A → C opens the tenant switcher →
     switches back to tenant-C → assert /settings/general is
     editable again (C is still owner of tenant-C).
   - **Strict email mismatch**: invite B@example.com, B tries to
     accept with a different Google account → expect an error
     and no FGA tuple written.
   - **Revoke**: A invites B, then revokes before B accepts →
     B's accept attempt fails with "invitation_revoked".

**Out of scope for Phase P** (land in later slices):
- Member listing beyond pending invites (Phase Q or team-management
  phase, needs OpenFGA `ListUsers` or a staff table)
- Promote/demote UI for existing members
- Remove member / kick out
- Transfer ownership
- Audit log of membership changes
- Store-level invites (Phase R — see below)
- Paywall on seat count

**Dependencies**: Phase O ships first (done). No migration ordering
with Phase Q — Phase P adds one table, Phase Q adds another.

### Phase Q — Store model + store switcher *(shipped — switcher deferred to Q.2)*

**What landed:**

- **Migration 0007 0008** (`create_stores.up.sql`) creates the
  `stores` table, backfills one "Main Store" per existing tenant
  copying slug/currency/timezone/country, and drops those columns
  (plus their FK constraints) from `tenants`. Destructive one-shot
  for dev; prod would need a two-phase deploy.
- **`internal/store/`** — new domain package (models, repo,
  service, handler) mirroring the pre-Phase-Q tenant code for
  slug lookup, slug-available, list-by-tenant, and editable
  update. Routes: `GET /api/v1/stores/slug-available`, `GET /internal/stores/:id`, `PATCH /internal/stores/:id`, `GET /internal/tenants/:id/stores`.
- **`tenant.Tenant`** now carries only `id, name, owner_user_id,
  owner_email, status, created_at, updated_at`. `GetBySlug`,
  `SlugExists`, `IsSlugAvailable`, and `validateSlug` moved to
  the store package along with their unit tests. `Membership`
  lost its slug field — the Phase P tenant switcher shows tenant
  name only; URL identity is store-scoped.
- **Onboarding `Complete()`** now creates tenant + default store
  in the same transaction. The merchant's "business name" is
  written into both `tenant.name` (company label) and
  `store.name` (public-facing, Phase Q ships it editable). The
  slug, currency, timezone, country all land on the store. The
  bug-fix transaction (Phase D) still covers everything: tenant
  row + store row + onboarding session completion + FGA outbox
  event commit atomically.
- **FGA store type** added to `model.fga` and the seed JSON.
  Store has direct relations (`owner/manager/staff/viewer`),
  inherited relations (`tenant_admin = admin from parent`,
  `tenant_owner = owner from parent`), and derived permissions
  (`can_edit_store_settings`, `can_manage_catalog`,
  `can_view_store`, `member`). Tenant owners/admins
  automatically get every store-level permission via
  `from parent` — the common single-founder case needs zero
  store-level tuples.
- **`authz.Client.WriteStoreParent`** writes
  `tenant:<tid> parent store:<sid>` tuples. Called by the
  onboarding outbox drainer alongside the existing owner tuple
  write. Idempotent.
- **`authz.Client.CheckObject`** — new method that takes an
  explicit object type so callers can check store-scoped
  permissions without the old `Check`'s hardcoded `tenant:`
  prefix. `Check` is kept as a convenience wrapper around
  `CheckObject(…, "tenant", …)`. The `FakeClient` implements
  store resolution by walking the `storeParents` map to the
  tenant, mirroring the DSL's `from parent` semantics without
  parsing the DSL.
- **`PATCH /internal/stores/:id`** is FGA-gated on
  `can_edit_store_settings` against the store object. Tenant
  owners/admins pass automatically via inheritance; staff/viewer
  don't. Same "uid in body" pattern the tenant handler has used
  since Phase O.
- **Invitation service** now looks up the tenant's default store
  slug (first store by created_at) when building the invitation
  accept URL and when rendering the `VerifyResult.tenant_slug`
  field. A tenant with zero stores falls back to an empty slug —
  shouldn't happen post-Phase-Q.
- **Config-driven URL templates** wired consistently: onboarding
  welcome + invitation accept links all use
  `ADMIN_BASE_URL_TEMPLATE` / `STOREFRONT_BASE_URL_TEMPLATE` from
  `pkg/config` (either flat hosts for dev or `%s`-templated
  per-slug hosts for prod).
- **Admin `/settings/general`** now edits the CURRENT store, not
  the tenant. `GeneralSettingsForm` takes `store: Store`
  alongside `tenant: Tenant` and displays store-owned fields
  (name, slug, country, currency, timezone) + the one tenant-
  owned field (owner_email). Server action
  `updateGeneralSettings` resolves the current store via
  `listStoresByTenant` (first = default) then calls the new
  `updateStore` client.
- **`getServerSessionContext`** fetches `stores` + `currentStore`
  in parallel with tenant and memberships; every shell-using
  page gets them for free.
- **Admin `platform-api.ts` client** — new `Store` type,
  `listStoresByTenant`, `fetchStore`, `updateStore`. Old
  `updateTenant` removed (no tenant-level edit path anymore).
- **Onboarding wizard** — slug-availability check re-pointed at
  `/api/v1/stores/slug-available` (was `/tenants/slug-available`).

**E2E coverage after Phase Q**: full admin suite passes
(**7/7 green**) against the new data model without changes to
any existing spec beyond helper touch-ups (invite-teammate
updates for the radix Select in TeamSettings, nothing Phase Q
specific).

**Deliberate scope cuts still open:**

- **Store switcher UI is not shipped** (Phase Q.2). Onboarding
  creates exactly one store per tenant, so there's nothing to
  switch between yet. The switcher dropdown will slot next to
  the tenant switcher in AdminShell when an "Add store" flow
  lands.
- **`/settings/stores` index + Add store** deferred to Q.2 for
  the same reason — no value without a second-store creation
  flow.
- **`current_store_id` in the session cookie** deferred. Phase Q
  derives "current store" as the first-created store per request
  on the server. Moving to a cookie-cached id is a straightforward
  auth-bff change that can happen when multi-store tenants exist.
- **Store-level role grants** stay deferred to Phase R —
  infrastructure is ready (DSL + WriteRole supports `store:`
  objects), UI is not.

### Phase Q (original plan — superseded above, kept for reference)

(4–5 days)

The biggest schema change since Phase D. Introduces the concept that
a tenant can run multiple storefronts. Today a tenant IS a store:
`tenant.slug` is the storefront URL. Phase Q separates them, backfills
a default store per existing tenant, and introduces a second-level
switcher in the admin.

Data model:

1. **New `stores` table**:
   `id, tenant_id (FK), slug, name, currency_code, timezone,
   country_code, logo_url, status, created_at, updated_at`.
   Same slug rules as the current tenant slug (3-63 chars,
   lowercase + hyphens, no edge hyphens). Unique across the whole
   table — storefront URLs must be globally unique.
2. **Migration moves columns off tenants**: `slug`, `currency_code`,
   `timezone`, `country_code` move from `tenants` to `stores`. The
   migration creates a default store per existing tenant, named
   "Main Store", copying those fields over, then drops them from
   `tenants`. Zero-downtime strategy: two-phase migration — Phase
   Q.1 adds the stores table + default rows + keeps the tenant
   columns nullable; Phase Q.2 drops the tenant columns after the
   code migration is verified in prod. For dev, both phases run
   together.
3. **`tenants` keeps**: `id`, `name` (company name, e.g.
   "Acme Retail Holdings"), `owner_user_id`, `owner_email`,
   `status`, `created_at`, `updated_at`. Becomes thinner and more
   meaningful — a tenant is the company, not the store.
4. **Onboarding update**: onboarding creates tenant + one default
   store in the same transaction. The onboarding form's "business
   name" maps to `tenant.name` AND `store.name` (same value) and
   the "slug" field goes to the new store row. UX unchanged.
5. **Tenant-by-owner sign-in flow** still works because
   `tenants.owner_user_id` stays put. The first store under a
   newly created tenant is the user's landing store.

FGA model:

6. **New `store` type** in `model.fga`:
   ```
   type store
     relations
       define parent: [tenant]
       define owner:   [user]
       define manager: [user]
       define staff:   [user]
       define viewer:  [user]
       define tenant_admin: admin from parent
       define tenant_owner: owner from parent
       define can_edit_store_settings:
           owner or manager or tenant_admin or tenant_owner
       define can_manage_catalog:
           owner or manager or staff or tenant_admin or tenant_owner
       define can_view_store:
           owner or manager or staff or viewer or member from parent
   ```
   Tenant-level admins/owners automatically inherit store permissions
   via `from parent`, so the common case (solo founder) needs zero
   per-store tuple writes. The migration writes `store:<id> parent
   tenant:<id>` for every backfilled store.

Admin rewrite:

7. **Session adds `current_store_id`**. auth-bff `switch-tenant`
   clears it; new `POST /auth/switch-store {store_id}` sets it.
   Middleware forwards both `x-session-tenant-id` and
   `x-session-store-id` into the request headers.
8. **`/settings/general` becomes store-scoped**. Renamed to
   `/settings/stores/[store_id]/general`. The `[store_id]` defaults
   to the session's `current_store_id`. Existing URL
   `/settings/general` redirects to the current store's page.
9. **`/settings/stores` index page** — lists all stores under the
   current tenant, with "Add store" CTA for `can_edit_settings`
   roles. First iteration of "create a second store".
10. **AdminShell: two-level switcher**. Top bar gets a combined
    "Tenant › Store" breadcrumb-style control. Clicking tenant
    opens the tenant dropdown (from Phase P), clicking store
    opens a store dropdown scoped to the current tenant.

E2E:

11. Onboarded merchant lands on their default store. Create a
    second store. Switch between them. Edit settings on store 2,
    switch back to store 1, assert store 1's settings weren't
    touched.

**Deliberate scope cuts**:
- No per-store currency/country change UI in Q — it ships in Q with
  the store settings form, same as Phase N's tenant version, but
  still read-only (no billing/tax reshuffle on currency change).
- No store-level invites — that's Phase R.
- No "clone store" / "duplicate catalog" — out of scope.

### Phase R — Store-level invites (1–2 days)

Extends Phase P's invite infrastructure to accept an optional
`store_id` and write store-level FGA tuples.

1. **Invitation row** gains `store_id` nullable column. If set, the
   invite grants a store-level role; if null, it grants a
   tenant-level role like Phase P.
2. **Invite modal** on `/settings/team` adds a "scope" toggle:
   "Tenant-wide" (Phase P behaviour) or "Specific store" (new —
   shows a store picker).
3. **Accept endpoint** writes to `fga.WriteRole` with either
   `tenant:<id>` or `store:<id>` as the object based on the
   invitation row.
4. **`/settings/team` grows a "Store-level teammates" section**
   grouped by store.
5. E2E: tenant-level admin invites a staff member to store 2 only;
   staff member accepts; asserts they see store 2 but NOT store 1
   in their store switcher.

**Dependencies**: Phase Q must land first.

### Phase S+ — Still TBD

Unchanged from before, just pushed back:

- **Products service + admin products UI** — first real new backend
  service. Pairs with products page port. ~3–5 days.
- **Marketplace / vendors** — when products exist, add a `vendor`
  type under `store` with the same parent-inheritance pattern.
  Products belong to vendors, vendors belong to stores, stores
  belong to tenants. Founder's store is vendor 0 / default.
- **Orders service + dashboard metrics** — ~2–3 days.
- **Storefront** — separate slice, customer-facing site.
- **Production deploy** — Terraform / Cloudflare Worker / Cloud SQL
  / GKE. Multi-day.
- **Data migration from legacy** — snapshot + cutover.

The guiding principle is unchanged from the original kill-list: pick
the smallest useful slice, ship it, re-evaluate. Don't bulk-port
admin pages whose backing services don't exist yet.
