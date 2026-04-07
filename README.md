# Mark8ly

Multi-tenant marketplace platform — clean rebuild.

> **Status:** Phase A (Foundations). Onboarding-only first slice in progress.
> See [`docs/planning/`](./docs/planning) for the full plan.

## Layout

```
mark8ly/
├── apps/
│   └── onboarding/        Next.js 16 — onboarding wizard + marketing pages
├── services/
│   ├── platform-api/      Go monolith — control plane
│   └── auth-bff/          Go service — security boundary (GIP + sessions)
├── packages/
│   ├── ui/                Shared UI compositions (primitives stay in @tesserix/web)
│   ├── eslint-config/
│   └── typescript-config/
├── infra/
│   ├── dev/               docker-compose.yml + seed scripts
│   └── openfga/           Authorization model (model.fga)
├── docs/planning/         Architecture decisions, phase plans, open questions
├── go.work                Go workspace (services/*)
├── turbo.json             Turborepo task pipelines
├── Makefile               One-command dev / test / migrate / e2e
└── package.json           npm workspaces (apps/*, packages/*, services/*)
```

## Quick start

```bash
# 1. Install JS deps
npm install

# 2. Bring up the local stack (postgres, openfga, firebase emulator,
#    platform-api, auth-bff, onboarding)
make dev

# 3. Open the onboarding app
open http://localhost:4201
```

## Common commands

```bash
make help                                # list all targets
make dev                                 # full local stack
make dev-down                            # stop the stack
make dev-clean                           # stop + wipe Postgres data
make build                               # turbo build everything
make test                                # turbo test everything
make lint                                # turbo lint everything
make e2e                                 # Playwright e2e (Phase G+)

make migrate-up SERVICE=platform-api     # apply pending migrations
make migrate-new SERVICE=platform-api NAME=add_tenants
make migrate-version SERVICE=platform-api
make seed SERVICE=platform-api

make go-build                            # go build every Go service
make go-tidy                             # go mod tidy every Go service
```

## Architecture summary

- **6 services total** (down from ~30): `platform-api`, `marketplace-api`,
  `auth-bff`, `notification`, `payment`, plus managed `openfga`.
- **3 frontend apps**: `onboarding`, `admin`, `storefront` — all Next.js 16
  + React 19 + `@tesserix/web` design system.
- **Migrations**: golang-migrate, two-binary pattern (`cmd/server` +
  `cmd/migrate`). No GORM AutoMigrate anywhere.
- **Auth**: Google Identity Platform (Firebase Auth Emulator in dev) +
  OpenFGA, both real from day one.
- **Local-first**: full stack runs in docker-compose. No GKE required for dev.

For the *why* behind every decision, read [`docs/planning/`](./docs/planning)
in numerical order.

## Phase A exit criterion

Clone the repo → `npm install` → `make dev` → all containers healthy →
`http://localhost:4201` renders the placeholder onboarding page.

When that works on a clean machine, Phase A is done and Phase B (diagnose
auth bugs in `mark8ly_backup/`) begins.
