# Architecture — one page

> Audience: a new contributor who has cloned the repo, skimmed the root
> README, and now wants to know where things live and why. If you want
> the design trade-offs behind every choice, read `docs/planning/` in
> numerical order after this.

## System diagram

```
                               ┌──────────────────────────────┐
                               │      Browser / Email         │
                               └──────────────┬───────────────┘
                                              │ HTTPS
                        ┌─────────────────────┼─────────────────────┐
                        │                     │                     │
                        ▼                     ▼                     ▼
         ┌─────────────────────┐    ┌──────────────────┐   ┌────────────────┐
         │  apps/onboarding    │    │   auth-bff       │   │  platform-api  │
         │  Next.js 16 + RSC   │◀──▶│   Go / Gin       │◀─▶│  Go / Gin      │
         │                     │    │                  │   │                │
         │  • marketing pages  │    │  • GIP verifier  │   │  • locations   │
         │  • /onboarding form │    │  • session cookie│   │  • tenants     │
         │  • /onboarding/     │    │  • auto-login    │   │  • onboarding  │
         │     verify (magic)  │    │    + FGA retry   │   │  • verification│
         │  • server actions   │    └────────┬─────────┘   │  • outbox      │
         │    call both APIs   │             │             │  • notification│
         └─────────┬───────────┘             │             │     (inlined)  │
                   │                         │             └────┬───────┬───┘
                   │ id_token (GIP)          │                  │       │
                   ▼                         ▼                  ▼       ▼
         ┌────────────────────┐   ┌─────────────────────┐   ┌────────┐ ┌─────────┐
         │ Firebase Auth      │   │      OpenFGA        │   │Postgres│ │SendGrid │
         │ (emulator in dev)  │   │  (own Postgres DB)  │   │platform│ │ (LogSend│
         │                    │   │                     │   │  _api  │ │  in dev)│
         │ per-product pools: │   │ stores:             │   │ auth_  │ └─────────┘
         │  • platform        │   │  • mark8ly-platform │   │  bff   │
         │  • mp-internal     │   │                     │   └────────┘
         │  • mp-customer     │   │ tuples:             │
         └────────────────────┘   │  user:uid + tenant  │
                                  └─────────────────────┘
```

## Services

### `services/platform-api` — Go control plane

Single Go process that owns every tenant-scoped domain in the
onboarding slice. Domains:

- **location** — countries, states, cities, currencies, timezones.
  Smallest domain; established the patterns every other one follows.
- **tenant** — CRUD, slug uniqueness, internal lookup.
- **onboarding** — session lifecycle, draft save, completion handler
  (the one that implements the outbox pattern, see below).
- **verification** — email magic-link send + verify. Stores the
  SHA-256 hash of each token, not the plaintext. The plaintext is
  captured by an optional `TokenRecorder` in non-prod so the e2e
  suite can bypass the inbox.
- **outbox** — single drainer process that ships pending rows to
  OpenFGA. Lives in the same binary as the API; not a separate worker.
- **notification** — SendGrid wrapper. Inlined, not a separate service.
- **test** — non-prod-only e2e helper endpoint. Gated at wire time in
  `cmd/server/main.go`; the package does not exist on the hot path in
  production.

Two binaries live under `cmd/`:

```
services/platform-api/
├── cmd/
│   ├── server/main.go     # starts the API; calls migrate.AssertVersion on boot
│   ├── migrate/main.go    # up / down / version / new
│   └── seed/main.go       # idempotent reference data (locations etc.)
├── internal/              # one directory per domain, handler/service/repository layering
├── migrations/            # 0001_init.up.sql, 0001_init.down.sql, …
└── pkg/                   # cross-domain utilities (db, httpserver, logger, errors, migrate, testdb)
```

### `services/auth-bff` — Go security boundary

Verifies Firebase / GIP ID tokens, mints encrypted session cookies,
and handles the post-onboarding auto-login handshake with the
platform-api outbox. Single binary, no database of its own (the only
state is the user session, encoded in the cookie). The auto-login
handler does a **check-with-retry** against OpenFGA to close the
tuple-visibility race described in
[`docs/planning/auth-bugs.md`](./planning/auth-bugs.md) (bug #2).

### `apps/onboarding` — Next.js marketing + form

Everything a prospective merchant sees before they're a merchant. The
`/onboarding` route is the single-page form that replaces the legacy
5-step wizard. Server actions talk directly to `platform-api` for
session create + verification send, and to `auth-bff` for the
post-verify session mint. The onboarding form state lives in a zustand
store persisted to `sessionStorage`, so the magic-link target
(`/onboarding/verify`) can rehydrate even when the user clicks the link
from a new tab an hour later.

### `@tesserix/web` — shared design system (external)

Published separately; consumed here as an npm dep. Provides the
primitive components (`Button`, `Input`, `Select`, `Card`, …). The
`packages/ui` workspace in this repo is reserved for project-specific
compositions that live above the primitives.

## Data flow — golden onboarding path

```
 1. User  → apps/onboarding /onboarding
    • Form mounted, fetches locations/currencies/timezones from platform-api

 2. Submit → apps/onboarding server action
    • Client: Firebase signUp (email, password-less) → { uid, refreshToken }
    • Server action: POST /api/v1/onboarding/sessions → session_id
    • Server action: POST /api/v1/onboarding/sessions/:id/verification/send
      └─ platform-api generates token, stores SHA-256, sends email via LogSender/SendGrid

 3. User → /onboarding/check-inbox   (form state persisted in sessionStorage)

 4. User clicks magic link → /onboarding/verify?token=...
    • VerifyMagicLink waits for zustand to finish rehydrating
    • Client refreshes the GIP id_token using the persisted refresh token
    • Server action: verifyAndLogin
      ├─ POST /api/v1/onboarding/verify-token   → platform-api marks session verified
      ├─ POST /api/v1/onboarding/sessions/:id/complete → in one DB tx:
      │    INSERT tenant, INSERT membership, INSERT outbox row, COMMIT
      └─ POST /auth/auto-login on auth-bff
         ├─ Verify GIP id_token (pool comparison fixes auth-bug #5)
         ├─ Check membership in OpenFGA — retry if NOT_FOUND
         │  (closes the tuple-visibility race, auth-bug #2)
         └─ Mint encrypted session cookie
    • Client: router.push('/welcome')

 5. Background: platform-api outbox drainer
    • Polls outbox rows with status='pending'
    • Writes the FGA tuple
    • Marks row 'completed'
```

## The outbox pattern

This is the single most load-bearing pattern in the codebase. Every
tenant-scoped write that must be mirrored to OpenFGA goes through it.

```
    ┌──────────────────┐
    │ Complete handler │
    └────────┬─────────┘
             │
             ▼
 ┌─────────────────────────────┐        ┌────────────────────┐
 │  BEGIN DB transaction       │        │  Outbox drainer    │
 │    INSERT INTO tenants      │        │  (tick every 500ms)│
 │    INSERT INTO memberships  │        │                    │
 │    INSERT INTO outbox (kind,│        │  SELECT … WHERE    │
 │                       payload,│──────▶│    status=pending  │
 │                       status)│        │                    │
 │  COMMIT                     │        │  For each row:     │
 └─────────────────────────────┘        │    call handler    │
                                         │    mark completed  │
                                         │    or retry        │
                                         └──────────┬─────────┘
                                                    │
                                                    ▼
                                            ┌───────────────┐
                                            │    OpenFGA    │
                                            │  Write tuple  │
                                            └───────────────┘
```

Why the outbox exists: if the code wrote the tenant row and the FGA
tuple in two separate network calls, a crash between them would leave
the system with a tenant that the user can't access. By making the
outbox row part of the same DB transaction as the tenant, we guarantee
that every committed tenant has a pending FGA write, and the drainer
gives us at-least-once delivery.

The auth-bff auto-login handler does a short retry loop on FGA
`NOT_FOUND` to absorb the visible window between COMMIT and drainer
tick. In practice that window is <500ms but the retry makes it
invisible.

## The test helper bypass (non-prod only)

`internal/test/` exposes `GET /api/v1/test/verification/latest?email=...`
that returns the most recent plaintext magic-link token for an email.
Wired up in `cmd/server/main.go` only when `cfg.Env != "prod"`. Used by
the Playwright suite to walk the golden path without a real inbox. The
handler and the in-memory `TokenRecorder` do not exist on the hot path
in production — the recorder field on `verification.Service` is nil,
every call is a no-op, and the route is not registered.

## Local development topology

```
make dev → docker compose -f infra/dev/docker-compose.yml up

postgres           :5432   platform_api, auth_bff, openfga databases
firebase-emulator  :9099   GIP identity emulator
openfga            :8089   http :8090 grpc
openfga-migrate    init    runs openfga migrate then exits
fga-seed           init    creates the mark8ly-platform store + model
gip-seed           init    creates platform / mp-internal / mp-customer tenants
platform-api       :8086   migrations run via a separate init container
auth-bff           :8080
apps/onboarding    :4201   Next.js dev server (npm workspace)
```

Everything is wired so that a fresh clone + `make dev` brings the
entire dependency graph up in one command. The `postgres-init.sh`
script creates the three databases the first time the volume is empty;
every service owns its own database and runs its own migrations in
its own init container.

## Where the tests live

- **Go unit tests** — next to the code under test
  (`internal/<domain>/service_test.go`). Pure, no DB.
- **Go integration tests** — `//go:build integration` tag, use the
  `pkg/testdb` fixture to get a real Postgres connection:
  - `internal/onboarding/completion_integration_test.go` — outbox atomicity
  - `internal/outbox/outbox_integration_test.go` — drainer recovery
  - `internal/tenant/repository_integration_test.go` — slug uniqueness
  - `pkg/migrate/migrate_integration_test.go` — up/down/idempotency against a throwaway DB
- **Auth-bff tests** — `internal/gip/verifier_test.go` and
  `internal/autologin/service_test.go` cover the GIP pool isolation
  regression (auth-bug #5).
- **Playwright e2e** — `apps/onboarding/tests/e2e/`. Six specs covering
  the golden path, form validation, slug collision, invalid token,
  browser-close-and-resume.

## Shipping & carrier integrations

The Delhivery integration lives across storefront checkout, admin
shipping settings, the per-tenant GCP Secret Manager backing store,
a background tracking CronJob, and a public push-webhook endpoint.
End-to-end architecture, sequence diagrams, schema, and error
classification are in their own doc:

- [`delhivery-integration.md`](./delhivery-integration.md) — master doc
- [`delhivery-pickup.md`](./delhivery-pickup.md) — pickup-scheduling detail
- [`delhivery-webhook.md`](./delhivery-webhook.md) — push-webhook detail

Adding a new carrier is a one-file extension: implement
`shipping.Carrier` (+ optionally `PickupScheduler`, `WarehouseSyncer`,
`LabelFetcher`), register it in `shipping.NewCarrier`, and the
per-tenant secret store auto-namespaces the credentials by provider.

## Payment gateway integrations

Payment providers sit behind `payment.Gateway`, resolved per (store,
provider) from `payment_gateway_configs`. Checkout, the refund saga,
webhook receipt and admin config all run through that one interface, so
adding a gateway does not touch the money paths.

Which providers a store may configure is data, not code:
`supported_countries.payment_providers` gates both the admin write path
and the storefront picker, **and its order is the preference order** —
the first entry is pre-selected at checkout.

- [`cashfree-webhook.md`](./cashfree-webhook.md) — Cashfree webhook setup
  runbook (endpoint, event subscriptions, signature scheme)
- [`testing-payments.md`](./testing-payments.md) — test store, per-provider
  test-mode setup and instruments

Two optional capability interfaces let a provider opt into behaviour the
base interface can't express, via type assertion at the call site rather
than provider branching:

- `CheckoutGateway` — hosts its own payment page (Stripe). Gift-card
  purchase requires this.
- `OrderStatusGateway` — has no client-side signature, so confirmation
  polls the provider instead (Cashfree).

## Gateway JWT gate (istio-ingress)

The cluster's Istio ingress carries an `AuthorizationPolicy` named
`require-customer-auth` (namespace `istio-ingress`, repo `tesserix-k8s`) that
denies any request without a valid JWT to a fixed list of path prefixes,
including `/api/v1/admin/*`, `/api/v1/staff/*`, `/api/v1/analytics/*`,
`/api/v1/reports/*`, `/api/v1/orders/*`, `/api/v1/cart/*`,
`/api/v1/payments/*`, and others. This runs at the mesh, before any
`marketplace-api` handler sees the request.

Any surface that authenticates by something other than a JWT — e.g.
`marketplace-api`'s platform-console admin surface
(`internal/handlers/platformadmin/`), which is HMAC-signed — must be mounted
outside those prefixes, or the mesh rejects it with `403` and body
`RBAC: access denied` before the app-level auth even runs. This is why that
surface is mounted at `/api/v1/platform/...` rather than `/api/v1/admin/...`
(see `internal/handlers/platformadmin/routes.go`).

The failure only reproduces against the deployed cluster — Istio isn't part
of local dev or CI, so a caller signing correctly against a JWT-gated prefix
sees a `403` that looks like an auth bug on their end. If you hit a `403`
with `RBAC: access denied` from `marketplace-api`, check whether the route
is inside one of `require-customer-auth`'s prefixes before debugging the
request signature — the policy itself lives in `tesserix-k8s`, not this
repo.

## What's deliberately NOT here (yet)

This slice ends at the welcome page. Everything below is next-phase
work and is intentionally absent from the current codebase:

- **marketplace-api** — separate Go service for products, orders,
  inventory, payments. Deferred until after the onboarding slice is
  shipped in prod.
- **admin** and **storefront** Next.js apps. Deferred.
- **payment processing** — stubbed in dev. No Stripe wiring yet.
- **notification-service** / **document-service** as separate
  microservices — inlined into platform-api, will split only if a
  second consumer forces the issue.
- **Production infra** — no Terraform, no GKE, no ArgoCD in this repo.
  The cloud deploy lives in `tesserix-infra/` which is not pulled in
  here.
- **CI/CD pipeline** — Makefile targets and test harness exist, but no
  GitHub Actions workflow. Adding one is a trivial follow-up; the
  guarantee is that `make test-unit && make test-int && make e2e` must
  pass on a clean clone.
