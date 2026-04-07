# Monorepo Structure

## The decision: one polyglot Turborepo

All code — Go services, Next.js apps, shared TS packages, infra, tooling —
lives in a single repository (`mark8ly/`) managed by Turborepo with a
`go.work` workspace for the Go side.

The "reusability across products" argument that justified separate repos in
the original Tesserix design is dead: Mark8ly is the only product. Optimize
for the actual case.

## Why monorepo

- **One source of truth for types.** Backend defines models; frontends consume
  them. Today: hand-written TS types in `lib/types/`, drift constantly.
  In-monorepo: generate TS from Go structs (or share via OpenAPI), zero drift.
- **Atomic changes.** Renaming a tenant field today = 1 PR per repo, hope they
  merge in the right order, hope nothing breaks. In-monorepo: 1 PR, 1 CI run,
  1 merge.
- **Local e2e becomes trivial.** `make dev` (or `turbo dev`) starts every
  service + every frontend together. No 4 terminal tabs, no version skew, no
  "did you `git pull` in the other repo."
- **Turborepo handles polyglot.** It doesn't care that one workspace is Go and
  the rest are TS. Add a `package.json` shim with build/test scripts that
  shell out to `go build`/`go test`, and Turborepo caches it like any other
  workspace.
- **One CI pipeline.** Lint + test + build runs once with smart caching;
  unchanged workspaces don't rebuild.

## Layout

```
mark8ly/
├── apps/                            ← Next.js applications
│   ├── onboarding/                  Port 4201
│   ├── admin/                       Port 3001 (later)
│   └── storefront/                  Port 3200 (later)
│
├── services/                        ← Go services
│   ├── platform-api/                Control plane monolith
│   │   ├── cmd/
│   │   │   ├── server/main.go       API binary
│   │   │   ├── migrate/main.go      Migration runner binary
│   │   │   └── seed/main.go         Seed data loader binary
│   │   ├── internal/
│   │   │   ├── onboarding/          Domain: handler + service + repo + models
│   │   │   ├── verification/
│   │   │   ├── tenant/
│   │   │   ├── tenantrouter/
│   │   │   ├── location/
│   │   │   ├── audit/
│   │   │   ├── settings/
│   │   │   ├── storage/             Replaces document-service (GCS wrapper)
│   │   │   ├── notification/        Replaces notification-service (inlined initially)
│   │   │   ├── authz/               Thin OpenFGA SDK wrapper
│   │   │   ├── outbox/              Failed FGA writes table + drainer
│   │   │   └── test/                E2E helpers, env-gated
│   │   ├── pkg/                     Reusable within this service (and copy-pasteable to others until extracted)
│   │   │   ├── httpserver/
│   │   │   ├── db/
│   │   │   ├── logger/
│   │   │   ├── config/
│   │   │   ├── errors/
│   │   │   └── migrate/
│   │   ├── migrations/              SQL files (golang-migrate format)
│   │   ├── seed/                    JSON/YAML seed data files
│   │   ├── go.mod
│   │   ├── package.json             Turborepo shim
│   │   └── Dockerfile               Multi-stage: server + migrate targets
│   │
│   ├── marketplace-api/             (later)
│   │
│   ├── auth-bff/                    Security boundary service
│   │   ├── cmd/
│   │   │   ├── server/main.go
│   │   │   └── migrate/main.go
│   │   ├── internal/
│   │   │   ├── gip/                 OIDC client (real GIP / Firebase emulator)
│   │   │   ├── session/             Encrypted cookie sessions
│   │   │   ├── totp/
│   │   │   ├── autologin/           Post-onboarding session mint
│   │   │   └── handlers/
│   │   ├── pkg/                     (mirrors platform-api/pkg until extracted)
│   │   ├── migrations/
│   │   ├── go.mod
│   │   ├── package.json             Turborepo shim
│   │   └── Dockerfile
│   │
│   ├── notification/                (later — currently inlined into platform-api)
│   └── payment/                     (later)
│
├── packages/                        ← Shared TypeScript
│   ├── ui/                          Shared React components (grow as needed)
│   ├── eslint-config/               Already scaffolded by create-turbo
│   ├── typescript-config/           Already scaffolded by create-turbo
│   └── (extract more here ONLY when 2nd consumer appears)
│
├── infra/
│   ├── dev/                         Local development (docker-compose)
│   │   ├── docker-compose.yml
│   │   ├── firebase.json            Firebase Auth Emulator config
│   │   ├── .firebaserc
│   │   ├── seed/
│   │   │   ├── init.sh              Orchestrates store + tenant creation
│   │   │   ├── create-gip-tenants.sh
│   │   │   └── create-fga-store.sh
│   │   └── env/                     .env files for local dev
│   ├── openfga/
│   │   ├── model.fga                Authorization model DSL
│   │   └── README.md
│   └── (terraform, k8s, argocd come in a later phase)
│
├── tools/
│   └── (codegen, scripts — added when needed)
│
├── docs/
│   └── planning/                    These planning docs
│
├── go.work                          Go workspace (services/* + future shared)
├── turbo.json                       Turborepo config
├── package.json                     Root workspace
├── Makefile                         dev / test / e2e / lint / build / migrate
└── README.md
```

## Shared packages strategy

**Default position: do not extract until there's a second consumer.**

Premature shared packages are worse than duplication. They lock you into an
abstraction before you understand the variation. Two copies of 50 lines of
config-loading code is fine. One leaky shared package that 5 services depend
on is not.

**Extract when:**
- A second service/app actually needs the same code
- AND the duplication has stabilized (you're not still iterating on the shape)
- AND extracting it would actually save more code than the package overhead

**Examples for the onboarding-only phase:**

| Package | Status | When to extract |
|---|---|---|
| `packages/ui` | Empty shell | When `apps/admin` or `apps/storefront` lands and shares an **app-level composition** with onboarding. **UI primitives stay in `@tesserix/web`** (the existing design system) — `packages/ui` is for shared composed components like `<TenantSwitcher>`, `<EmptyState>`, `<DataTable>`, not buttons/inputs/dialogs. |
| `packages/api-client` | Don't create | Onboarding's API client lives in `apps/onboarding/lib/api/`. Extract when admin/storefront need it. |
| `packages/auth` | Don't create | Same — only one consumer. |
| `packages/domain-types` | Don't create | Hand-write types in `apps/onboarding/lib/types/`. Codegen in a later phase. |
| `packages/tenant` | Don't create | Onboarding doesn't have tenant middleware (it creates tenants). Admin will. |
| `packages/gcp` | Don't create | Wrap GCP SDKs locally in `apps/onboarding/lib/` and `services/platform-api/internal/storage/`. |
| `packages/observability` | Don't create | Logger lives in each service's `pkg/logger`. Frontends use their own analytics wiring. |
| `go-shared` (shared Go module) | Don't create | Copy the small slices each service needs into its own `pkg/`. Hoist when ≥3 services have the same code. |

The mantra: **wait for the second consumer**. Extracting at consumer #1 is
guesswork. Extracting at consumer #2 is informed.

## Polyglot integration

Each Go service has a tiny `package.json` so Turborepo can orchestrate it:

```json
{
  "name": "@mark8ly/platform-api",
  "scripts": {
    "build": "go build ./...",
    "test": "go test ./...",
    "lint": "golangci-lint run",
    "dev": "go run ./cmd/server"
  }
}
```

`turbo.json` defines task pipelines that work across both:

```json
{
  "tasks": {
    "build": { "dependsOn": ["^build"], "outputs": ["dist/**", "bin/**"] },
    "test":  { "dependsOn": ["build"] },
    "lint":  {},
    "dev":   { "cache": false, "persistent": true }
  }
}
```

`go.work` ties Go services together for cross-service Go imports without
publishing:

```
go 1.26

use (
    ./services/platform-api
    ./services/auth-bff
)
```

## What's deliberately external

- **OpenFGA** — managed image, deployed but not built. Lives in `infra/`,
  not `services/`.
- **Postgres** — same. Container in dev, Cloud SQL in prod.
- **Firebase Auth Emulator** — dev only, replaces GIP locally.
- **Old `tesserix-infra/` Terraform** — pulled in only when the infra-rewrite
  phase starts. Until then, the local dev environment is the only environment
  that matters.
