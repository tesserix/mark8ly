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

### Phase M — Returning-user sign-in *(planned)*

Closes the loop for users who already have a tenant. (1) Add a
password field to the onboarding form so the merchant has a credential
on file. (2) Switch the client signUp call from email-only to
`createUserWithEmailAndPassword`. (3) Replace the marketing /login
stub with a real form: email + password fields and a "Continue with
Google" button. (4) Both paths produce a GIP id_token and POST it to
auth-bff `/auth/auto-login` — same endpoint onboarding uses, zero
new backend code. (5) Successful sign-in sets the session cookie and
redirects to admin. New e2e: existing user signs in via password →
admin dashboard renders. ~3–5 hours, frontend only.

### Phases N+ — TBD

Open at the time of writing. Likely candidates:

- **Tenant settings (general) page** — store name, logo, contact email,
  currency. Real feature page using endpoints platform-api already
  has. ~half day. The first Phase that proves we can ship a real
  admin feature on top of the chrome.
- **Roles + RBAC** — extend the OpenFGA model with `admin` / `staff` /
  `viewer` relations, write the `owner` tuple on onboarding completion,
  add role-aware checks in admin middleware, hide nav items based on
  role. Discussed in detail when the user asked about role pickup.
  Probably 1–2 days.
- **Products service + admin products UI** — first real new backend
  service. Domain CRUD, image URLs, single stock count. Pairs with
  the admin products page port. ~3–5 days.
- **Orders service + dashboard metrics** — read-only orders list, real
  metric tiles on the dashboard. Pairs with the admin orders page
  port. ~2–3 days.
- **Storefront** — separate slice after admin v0.1 → v0.2 lands.
  Customer-facing site. Bigger than admin because it's its own
  app + a different audience.
- **Production deploy** — Terraform / Cloudflare Worker / Cloud SQL /
  GKE. Probably its own multi-day slice once admin v0.1 is in
  customers' hands.
- **Data migration from legacy** — real tenants, real users, real
  orders. Snapshot strategy + cutover plan.

The guiding principle is unchanged from the original kill-list: pick
the smallest useful slice, ship it, re-evaluate. Don't bulk-port
admin pages whose backing services don't exist yet.
