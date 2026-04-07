# Mark8ly

Multi-tenant commerce platform — clean rebuild focused on the onboarding
slice. This repo is the result of Phases A–G of the plan in
[`docs/planning/06-onboarding-phase-plan.md`](./docs/planning/06-onboarding-phase-plan.md):
a working marketing site, a single-page onboarding form with magic-link
verification, a small Go control plane, a GIP + OpenFGA security
boundary, and an end-to-end test suite that proves it all hangs
together.

> **Status:** v0.1.0 — onboarding slice complete. See
> [`docs/planning/`](./docs/planning) for the architectural decisions and
> the why behind every constraint.

## Layout

```
mark8ly/
├── apps/
│   └── onboarding/        Next.js 16 marketing site + single-page onboarding form
├── services/
│   ├── platform-api/      Go control plane — locations, tenants, sessions, verification, outbox
│   └── auth-bff/          Go security boundary — GIP token verification + encrypted session cookies
├── packages/
│   ├── ui/                Shared React compositions (primitives stay in @tesserix/web)
│   ├── eslint-config/
│   └── typescript-config/
├── infra/
│   ├── dev/               docker-compose.yml + seed scripts + load-secrets.sh
│   └── openfga/           Authorization model (model.fga)
├── docs/
│   ├── architecture.md    One-page system overview (read this second)
│   ├── migrations.md      How to add, roll back, and test migrations
│   └── planning/          Phase plans, ADRs, auth bug postmortem
├── go.work                Go workspace spans services/*
├── turbo.json             Turborepo task pipelines (build / test / lint / check-types)
├── Makefile               Entry point for everything: dev, test, migrate, e2e
└── package.json           npm workspaces (apps/*, packages/*)
```

## Quick start

Bring up the full stack in one command:

```bash
# 1. Install JS deps (npm workspaces, Go uses go.work automatically)
npm install

# 2. Pull local secrets and bring up the local stack
#    — Postgres, OpenFGA, Firebase Auth emulator, platform-api,
#      auth-bff, onboarding.
make dev

# 3. Marketing site + onboarding form
open http://localhost:4201
```

`make dev` runs `dev-secrets` first to populate `infra/dev/.env.local`
from GCP Secret Manager (`gcloud auth application-default login` if this
is your first time). If you don't have GCP access, drop a hand-written
`.env.local` into `infra/dev/` with the GIP project fields and
`make dev` will pick it up.

**Exit check:** once everything is green, visit
`http://localhost:4201/onboarding`, submit the form with a throwaway
email, grab the verification link from the platform-api container logs
(the `LogSender` prints the full email body), paste it into the browser,
and you should land on `/welcome`.

## Common commands

```bash
make help                                # list every target with descriptions
make dev                                 # full local stack
make dev-down                            # stop the stack
make dev-clean                           # stop and wipe Postgres data
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

## Testing

Three layers, each independent:

1. **Unit tests** — `make test-unit`. Pure Go and TS tests, no external
   services. Fast; run on every save.
2. **Integration tests** — `make test-int`. Go tests gated by the
   `integration` build tag; they assume `make dev` is up and the shared
   Postgres is reachable. Covers the tenant tuple-write race
   ([auth-bug #2](./docs/planning/auth-bugs.md)), outbox recovery,
   migration up/down/idempotency, and verification token replay.
3. **End-to-end tests** — `make e2e`. Playwright against the running
   stack. Covers the golden path, slug collision, invalid-token error
   UI, form validation, and the browser-close-and-resume scenario.

See [`docs/planning/08-testing-strategy.md`](./docs/planning/08-testing-strategy.md)
for the why behind each layer.

## Architecture summary

- **Two Go services** — `platform-api` owns the domain (locations,
  tenants, onboarding sessions, verification, outbox); `auth-bff` is the
  security boundary for every token that touches GIP. Notification and
  storage are inlined into `platform-api` during this slice and will
  split out only if a second consumer appears.
- **One frontend** — `apps/onboarding` is the marketing site and the
  single-page form, ported from the legacy 5-step wizard. Next.js 16 +
  React 19 + `@tesserix/web` design system.
- **Magic-link verification** — no OTP, no password. Form submit sends a
  clickable link; clicking it completes tenant provisioning and mints
  the session cookie in a single round-trip.
- **GIP + OpenFGA from day one** — both run locally via docker-compose
  (Firebase Auth emulator, bundled `openfga` container). The verifier
  explicitly compares the `firebase.tenant` claim to prevent cross-pool
  login ([auth-bug #5](./docs/planning/auth-bugs.md)).
- **Outbox pattern** — tenant row + FGA write tuple commit in the same
  DB transaction via an outbox table. A drainer ships tuples to OpenFGA
  after the commit; the auth-bff does check-with-retry to close the
  tuple-visibility race window.
- **Schema safety** — golang-migrate with two binaries per service
  (`cmd/server`, `cmd/migrate`). `cmd/server` refuses to start on schema
  mismatch via `AssertVersion`. No GORM AutoMigrate anywhere.

One-page deep dive: [`docs/architecture.md`](./docs/architecture.md).

## Environment files

Every service has a `.env.example` listing every variable it reads. Copy
it to `.env` (or let `make dev` inject values via docker-compose) and
fill in secrets.

```
apps/onboarding/.env.example        # Next.js — browser + server actions
services/platform-api/.env.example  # Go control plane
services/auth-bff/.env.example      # Go security boundary
```

`make dev-secrets` populates the GIP/OAuth bits from GCP Secret Manager
so you never check them into the repo.

## Further reading

Read in this order for a green-field contributor:

1. [`README.md`](./README.md) — you're here
2. [`docs/architecture.md`](./docs/architecture.md) — one-page diagram + why
3. [`docs/migrations.md`](./docs/migrations.md) — how to add and test schema changes
4. [`docs/planning/00-overview.md`](./docs/planning/00-overview.md) — the kill-list
5. [`docs/planning/06-onboarding-phase-plan.md`](./docs/planning/06-onboarding-phase-plan.md) — what shipped in each phase
6. [`docs/planning/auth-bugs.md`](./docs/planning/auth-bugs.md) — the bugs the rebuild exists to fix
