# Mark8ly

[![CI](https://github.com/tesserix/mark8ly/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/tesserix/mark8ly/actions/workflows/ci.yml)
[![Dashboard validation](https://github.com/tesserix/mark8ly/actions/workflows/dashboard-validation.yml/badge.svg?branch=main)](https://github.com/tesserix/mark8ly/actions/workflows/dashboard-validation.yml)
[![Logging smoke](https://github.com/tesserix/mark8ly/actions/workflows/logging-smoke.yml/badge.svg?branch=main)](https://github.com/tesserix/mark8ly/actions/workflows/logging-smoke.yml)
[![PII leak guard](https://github.com/tesserix/mark8ly/actions/workflows/pii-leak-guard.yml/badge.svg?branch=main)](https://github.com/tesserix/mark8ly/actions/workflows/pii-leak-guard.yml)

Multi-tenant commerce platform — the consolidated monorepo carrying
every mark8ly frontend app, Go service, and shared package. Each tenant
gets `{slug}-admin.mark8ly.com` (admin) and `{slug}.mark8ly.com`
(storefront); onboarding happens via a magic-link flow at
`mark8ly.com/onboarding`.

> **Status:** production on `mark8ly.com`. The CI workflow gates every
> merge with `npm audit`, `govulncheck` (per Go service), and a Trivy
> scan; security failures email the distribution list
> (see [Security](#security)).

---

## Repo layout

```
mark8ly/
├── apps/
│   ├── admin/               Next.js 16 tenant admin dashboard (port 4202)
│   ├── storefront/          Next.js 16 customer storefront (port 4203)
│   ├── onboarding/          Next.js 16 marketing + onboarding wizard (port 4201)
│   ├── mobile-admin/        (stub) future Expo admin
│   └── storefront-mobile/   (stub) future Expo storefront
├── services/
│   ├── auth-bff/            Go — HttpOnly session cookies, CSRF, WS tickets
│   ├── platform-api/        Go — tenants, users, onboarding, settings, FGA bootstrap
│   ├── marketplace-api/     Go — catalog, orders, checkout, billing, white-label app creds
│   ├── otto/                Go — real-time support chat (MongoDB + WebSocket)
│   ├── products/            Go — product catalog
│   ├── categories/          Go — category taxonomy
│   ├── customers/           Go — customer profiles
│   ├── coupons/             Go — discount codes
│   ├── payment/             Go — Stripe + Razorpay gateways
│   └── tax/                 Go — tax engine
├── packages/
│   ├── ui/                  Shared React compositions
│   ├── otto-widget/         Reusable support-chat widget + admin inbox
│   ├── eslint-config/
│   └── typescript-config/
├── infra/                   docker-compose dev stack, OpenFGA model, seed scripts
├── docs/                    Architecture, migrations, planning, postmortems
└── .github/workflows/       CI + security + base-image-refresh
```

Every Go service owns its own `go.mod` (no Go workspace, keeps services
independently releasable). Frontends share a single `package.json` via
npm workspaces.

---

## Quick start

```bash
# 1. Install JS deps (npm workspaces)
npm install --legacy-peer-deps

# 2. Pull local secrets + bring up the local stack
#    Postgres, OpenFGA, Firebase Auth emulator, platform-api,
#    auth-bff, marketplace-api, onboarding, admin, storefront.
make dev

# 3. Visit
open http://localhost:4201    # onboarding
open http://localhost:4202    # admin (once a tenant exists)
open http://localhost:4203    # storefront
```

`make dev` shells out to `make dev-secrets` first, which populates
`infra/dev/.env.local` from GCP Secret Manager. Run
`gcloud auth application-default login` once if this is your first
time. Without GCP access, drop a hand-written `.env.local` in
`infra/dev/` with your GIP/OpenFGA values and `make dev` will use it.

## Common commands

```bash
make help                                # every target with a one-line description
make dev                                 # full local stack
make dev-down                            # stop the stack
make dev-clean                           # stop + wipe Postgres volume
make dev-logs                            # tail container logs

make build                               # turbo build every workspace
make test                                # go + ts unit tests
make test-int                            # go integration tests (needs make dev running)
make lint                                # every linter
make check-types                         # tsc --noEmit across TS workspaces
make e2e                                 # Playwright suite against make dev

make migrate-up SERVICE=platform-api     # apply pending migrations
make migrate-down SERVICE=platform-api   # roll back one migration
make migrate-version SERVICE=platform-api
make migrate-new SERVICE=platform-api NAME=add_tenants
make seed SERVICE=platform-api

make go-build                            # go build every Go service
make go-tidy                             # go mod tidy every Go service
```

---

## Architecture (one page)

- **Hostname-based tenant isolation** — Next.js middleware resolves
  subdomain → tenant via `platform-api`, then pins `x-tenant-id` on
  every downstream request. That header always overrides JWT claims to
  avoid cross-tenant smear.
- **BFF auth** — `auth-bff` owns the HttpOnly session cookie; the
  admin/storefront never see a raw JWT. `/bff/login` → Keycloak →
  callback → session.
- **OpenFGA** — role chain is `owner` → `admin` → `manager` → `staff`
  → `viewer`. Derived permissions (`can_manage_settings`,
  `can_read_settings`, …) come from the model. Staff bootstrap writes
  FGA tuples in the same DB transaction as the tenant row, via an
  outbox.
- **Istio wildcard routing** — `*-admin.mark8ly.com` → admin,
  `*.mark8ly.com` → storefront. Custom domains are provisioned by
  `tenant-router-service` (clones the wildcard VS templates).
- **Pub/Sub events** — every write emits a domain event on
  `mp-<domain>-events`; `notification-hub` fans them out as in-app
  notifications and email/SMS.
- **Knative + ArgoCD** — every service/app ships as a Knative ksvc in
  the `marketplace` namespace; Helm charts live in `tesserix-k8s`.

Deep dive in [`docs/architecture.md`](./docs/architecture.md).

---

## Security

CI runs a reusable security workflow per change:

| Check            | Scope                                                            |
|------------------|------------------------------------------------------------------|
| `npm audit`      | HIGH+ on the production workspaces (`admin`, `storefront`, `onboarding`, `@repo/ui`, `@repo/otto-widget`). |
| `govulncheck`    | Per Go service (`platform-api`, `auth-bff`, `marketplace-api`, `otto`). Reachability-narrowed CVEs. |
| `Trivy` (image)  | CRITICAL + HIGH on every published image, `ignore-unfixed: true`. |

All three fail the build on finding. On failure, the
`notify-security-failure` job in
[`.github/workflows/reusable-security.yml`](./.github/workflows/reusable-security.yml)
emails the distribution list (`samyak.rout@gmail.com`,
`mahesh.sangawar@gmail.com`, `unidevidp@gmail.com`) via Gmail SMTP.
Requires `SMTP_USERNAME` and `SMTP_PASSWORD` repo secrets.

`.github/workflows/base-image-refresh.yml` listens for the weekly
`tesserix-base-images-updated` event from
[`tesserix/base-docker-images`](https://github.com/tesserix/base-docker-images)
and dispatches `ci.yml` on `main`, so base-image CVE fixes propagate
without a manual trigger.

---

## Deploy

- **Images** — `ghcr.io/tesserix/mark8ly-<app|service>` built by
  `ci.yml` on every push to `main`. Base images are
  `ghcr.io/tesserix/base-*:latest` (weekly Trivy-gated rebuilds).
- **Knative** — `kubectl patch ksvc <name> -n marketplace` with a
  fresh timestamp annotation rolls the service after a successful
  build.
- **ArgoCD** — Helm values, External Secrets, VirtualServices, and the
  DB schema bootstrap cronjob all live in `tesserix-k8s`. ArgoCD
  auto-syncs with `prune: true` + `selfHeal: true`. Never
  `kubectl apply` directly.

## Knative ksvc names

| App / service     | Image                         | ksvc                     |
|-------------------|-------------------------------|--------------------------|
| admin             | `mark8ly-admin`               | `marketplace-admin`      |
| storefront        | `mark8ly-storefront`          | `mp-storefront`          |
| onboarding        | `mark8ly-onboarding`          | `marketplace-onboarding` |
| auth-bff          | `mark8ly-auth-bff`            | `mp-auth-bff`            |
| platform-api      | `mark8ly-platform-api`        | `mp-platform-api`        |
| marketplace-api   | `mark8ly-marketplace-api`     | `mp-marketplace-api`     |
| otto              | `mark8ly-otto`                | `mp-otto`                |
| products          | `mark8ly-products`            | `mp-products`            |
| categories        | `mark8ly-categories`          | `mp-categories`          |
| customers         | `mark8ly-customers`           | `mp-customers`           |
| coupons           | `mark8ly-coupons`             | `mp-coupons`             |
| payment           | `mark8ly-payment`             | `mp-payment`             |
| tax               | `mark8ly-tax`                 | `mp-tax`                 |

---

## Required repo secrets

| Secret              | Used by                                              |
|---------------------|------------------------------------------------------|
| `PKG_READ_TOKEN`    | Installing `@tesserix/web` from GHCR in every Next app |
| `GO_PRIVATE_TOKEN`  | `GOPRIVATE=github.com/tesserix/*` for `go-shared`    |
| `SMTP_USERNAME`     | `reusable-security.yml` notify job                   |
| `SMTP_PASSWORD`     | `reusable-security.yml` notify job (Gmail app password) |

WIF provider + CI service account are repo variables.

---

## Environment files

Every app and service ships a `.env.example` listing every variable it
reads. `make dev-secrets` injects values from GCP Secret Manager into
the compose stack so secrets never land in the repo.

```
apps/admin/.env.example
apps/storefront/.env.example
apps/onboarding/.env.example
services/platform-api/.env.example
services/auth-bff/.env.example
services/marketplace-api/.env.example
# (and one per service)
```

---

## Further reading

1. [`docs/architecture.md`](./docs/architecture.md) — one-page
   diagram + why
2. [`docs/migrations.md`](./docs/migrations.md) — adding and testing
   schema changes
3. [`docs/planning/`](./docs/planning) — phase plans, ADRs,
   postmortems
