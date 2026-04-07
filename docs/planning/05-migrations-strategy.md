# Database Migrations Strategy

## The problem

The current Go services use **GORM `AutoMigrate`** for schema management. This
is broken in the existing setup (it doesn't run reliably, or runs
inconsistently). The instinct is to "fix AutoMigrate." The right call is to
**delete every AutoMigrate call** and replace with versioned SQL migrations.

## Why GORM AutoMigrate is the wrong tool

- **Not deterministic.** Same code + same DB can produce different schemas
  depending on struct tags, GORM version, dialect quirks, and column order.
  Two pods starting at the same time can race.
- **Doesn't version anything.** No record of what ran when. No rollback. No
  way to know if a fresh DB matches a long-running DB.
- **Can't do data migrations.** Renaming a column = drop + add = data loss.
  Backfilling, splitting tables, changing types — none of it works.
- **Silently skips destructive changes.** Removing a field from a struct
  leaves the column. Drift between code and schema you can't see.
- **Doesn't run reliably in Knative scale-to-zero.** First request triggers
  cold start → triggers AutoMigrate → times out the request → fails the
  readiness probe → kills the pod → retries the cold start. **This is
  probably exactly what's biting the current setup.**
- **No production team runs AutoMigrate happily.** It's a dev convenience
  that turns into a prod liability.

**Verdict:** delete every `AutoMigrate` call. Replace with versioned SQL
migrations. Add a lint rule or pre-commit hook so it can't come back.

## The tool: golang-migrate

Already used in the existing codebase per the project CLAUDE.md
("Migrations: golang-migrate for Go services, Drizzle for marketplace-onboarding").
Stick with it. The problem isn't the tool — it's that AutoMigrate is *also*
in the codebase and the two are fighting.

Alternatives considered:
- **goose** — also fine, smaller community.
- **atlas** — newer, declarative + diffing, overkill for this phase.
- **Custom bootstrap script** — don't. You'll reinvent half a migration
  tool badly.

For Drizzle (in `apps/onboarding`'s CMS DB), keep using `drizzle-kit`. Same
principle: SQL migrations, no auto-sync.

## The pattern: two binaries from one repo

Each Go service produces **two binaries** from a single source tree:

```
services/platform-api/
├── cmd/
│   ├── server/                  ← The API binary
│   │   └── main.go
│   └── migrate/                 ← The migration runner binary
│       └── main.go
├── pkg/
│   └── migrate/
│       └── migrate.go           ← Library both binaries can use
├── migrations/
│   ├── 0001_init.up.sql
│   ├── 0001_init.down.sql
│   ├── 0002_add_onboarding.up.sql
│   ├── 0002_add_onboarding.down.sql
│   └── ...
└── Dockerfile                    ← Multi-stage: server + migrate targets
```

### The two binaries

**`cmd/server`** — the API.
- Starts fast.
- Does **not** run migrations.
- On startup, calls `migrate.AssertVersion(db, ExpectedSchemaVersion)`.
- **Refuses to start** if the schema is at the wrong version. Loud, fast,
  obvious failure: `"schema version 5 expected, found 3 — run migrations"`.
- This is the safety net that makes the whole pattern work.

**`cmd/migrate`** — a tiny binary whose only job is to run `migrate up`
(or `down`, `version`, `force`) against the DB. Embeds the SQL migrations
via `//go:embed`. Built into its own Docker image (separate target in the
same Dockerfile).

### The shared library

`pkg/migrate/migrate.go`:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up runs all pending migrations.
func Up(ctx context.Context, dbURL string) error { ... }

// Down rolls back N migrations.
func Down(ctx context.Context, dbURL string, n int) error { ... }

// Version returns the current schema version.
func Version(ctx context.Context, dbURL string) (uint, bool, error) { ... }

// AssertVersion fails if the DB is not at the expected version.
func AssertVersion(ctx context.Context, dbURL string, expected uint) error { ... }
```

`ExpectedSchemaVersion` is a constant in the binary, bumped manually when a
migration is added. Or computed at build time from `len(migrationFiles)` —
even better.

## Three execution contexts, one binary

The same `cmd/migrate` binary runs in three places:

### 1. Local dev (docker-compose)

The migrate binary runs as an **init container** before the API container:

```yaml
services:
  platform-api-migrate:
    build:
      context: ../../services/platform-api
      target: migrate                              # multi-stage Dockerfile target
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://...
    restart: "no"                                  # run once, exit

  platform-api:
    build:
      context: ../../services/platform-api
      target: server
    depends_on:
      platform-api-migrate:
        condition: service_completed_successfully  # ← key
      postgres:
        condition: service_healthy
```

`service_completed_successfully` means **the API container won't start until
the migrate container exits 0**. Cold start = run migrations → exit → start
app. Repeat `make dev` = migrations are no-ops (already applied) → exit
immediately → start app. Idempotent and fast.

### 2. CI (deploy workflow)

The migrate binary runs as a CI step against the target DB via Cloud SQL
Auth Proxy, **before** the app deploy step. Migration failure fails the
deploy. The app is never deployed against an unmigrated DB.

### 3. Kubernetes (ArgoCD sync hook)

Belt-and-braces. A Helm hook / ArgoCD sync wave runs the migrate Job before
the Deployment rollout. Even if CI was bypassed (manual deploy, emergency
hotfix), the cluster won't roll out an incompatible schema.

**All three are used.** Local dev is the most important — devs need
migrations to "just work." CI catches problems before they reach the cluster.
The K8s sync hook is the safety net for the case where CI was skipped.

## Why this pattern works

- **Migrations always run before the app starts.** Locally, in CI, in K8s.
  No exceptions.
- **The app refuses to start against the wrong schema.** Loud, obvious, fast
  failure instead of mysterious runtime errors.
- **No race conditions** between pods (migration is gated to a single runner;
  app pods wait for the version to be correct).
- **No Knative cold-start surprises** because migrations don't run in the
  request path.
- **Rollbacks possible** because every up has a down.
- **Local dev is one command.** `make dev` does the right thing.
- **Schema is documented and reviewable** because every change is a SQL file
  in a PR.
- **GORM stays in its lane** as a query/ORM tool, not a schema manager.
- **Easy to run migrations manually** when needed (CLI binary, takes a DB URL).

## The Dockerfile

Multi-stage, two targets:

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /out/server ./cmd/server
RUN go build -o /out/migrate ./cmd/migrate

FROM alpine:3.19 AS server
COPY --from=build /out/server /server
ENTRYPOINT ["/server"]

FROM alpine:3.19 AS migrate
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]
```

One Dockerfile, two targets. CI builds both:
`tesserix/platform-api:tag` and `tesserix/platform-api-migrate:tag`.
Same source, same migrations embedded, **no version skew**.

## Migration authoring rules

A few rules that prevent 90% of migration pain:

1. **SQL migrations only.** Not Go migrations. SQL is reviewable, runnable
   manually, portable, and doesn't depend on the app code.
2. **Never edit a committed migration.** Always add a new one. The committed
   file is what ran in prod.
3. **Always write the `.down.sql`.** Even if you never run it, writing it
   forces you to think about reversibility.
4. **One concern per migration.** Don't bundle "add table + add column to
   other table + insert seed data" in one file. Separate them.
5. **Migrations must be backwards-compatible with the previous app version.**
   The deploy order is: migrate first, then app. The old app must still work
   against the new schema for the duration of the rollout. This means:
   - Adding a column? Make it nullable or have a default. Don't `NOT NULL`
     without a default in the same migration.
   - Removing a column? Two-phase: deploy app that doesn't read the column,
     then in a later release, drop the column.
   - Renaming a column? Three-phase: add new column, dual-write, switch
     reads, drop old column.
   - This discipline prevents "deploys break the previous version" outages.
6. **Idempotent where possible.** `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX
   IF NOT EXISTS`. Helps with manual recovery.
7. **Test data migrations on a copy of prod.** Easy because the migrate
   binary is self-contained — point it at a Cloud SQL clone.
8. **Lock long migrations behind a flag or run them out-of-band.** A
   migration that takes 30 minutes shouldn't block a deploy. Deploy the app
   first (tolerant of both schemas), then run the migration manually with the
   migrate binary.

## How GORM fits in

GORM is fine as a **query builder and ORM**. The mistake is using it for
schema management.

New rules:
- **GORM models are read/write.** They define what the app reads and writes.
- **SQL migrations are the source of truth.** They define what the database
  actually is.
- **Never call `db.AutoMigrate()`.** Anywhere. Grep the codebase, delete every
  call. Add a lint rule that fails if `AutoMigrate` reappears.
- **GORM tag-based schema is documentation, not enforcement.** Struct tags
  help GORM map columns; they do not create columns. A column exists because
  a SQL migration added it.
- **Write the migration first, then write the GORM struct to match.** The
  migration is the contract; the struct conforms.
- **Optional CI safety net:** `pkg/migrate` exposes
  `AssertSchemaMatchesModels(db, models...)` that uses GORM's introspection
  to compare a fresh-migrated DB against the model definitions. Run in CI.
  Catches "I added a field to the struct but forgot the migration."

## Seeds (separate concern, related question)

You'll quickly want **seed data** — countries list, default settings, dev users.
**Don't put seeds in migrations.**

- Seeds change with environments (dev seeds ≠ prod seeds).
- Seeds shouldn't be versioned the same way schema is.
- Seeds often need to be re-runnable; migrations are not.

**Pattern:** a separate `cmd/seed/main.go` binary that reads JSON/YAML files
from `seed/` and inserts them. Run separately from migrations:

```
make migrate-up SERVICE=platform-api
make seed SERVICE=platform-api ENV=dev
```

In docker-compose, the seed runs as another init container after migrate,
before the app. In CI, seed only in dev/staging environments — never prod.

For onboarding, you'll need at least: countries (~250 rows), states/provinces
for major countries, top cities. Load from
`services/platform-api/seed/locations.json`. The seed binary reads it and
inserts with `ON CONFLICT DO NOTHING` for idempotency.

## Makefile integration

```makefile
migrate-up:
	cd services/$(SERVICE) && go run ./cmd/migrate up

migrate-down:
	cd services/$(SERVICE) && go run ./cmd/migrate down 1

migrate-version:
	cd services/$(SERVICE) && go run ./cmd/migrate version

migrate-new:
	cd services/$(SERVICE) && \
	  N=$$(printf "%04d" $$(( $$(ls migrations/*.up.sql 2>/dev/null | wc -l) + 1 ))); \
	  touch migrations/$${N}_$(NAME).up.sql migrations/$${N}_$(NAME).down.sql

seed:
	cd services/$(SERVICE) && go run ./cmd/seed --env=$(ENV)
```

Usage:
```
make migrate-up SERVICE=platform-api
make migrate-new SERVICE=platform-api NAME=add_onboarding
make seed SERVICE=platform-api ENV=dev
```

## What's enforced, what's convention

**Enforced (by code):**
- `cmd/server` calls `AssertVersion` and fails on mismatch.
- Migrations are embedded in the binary (`//go:embed`).
- docker-compose uses `service_completed_successfully` to gate the app.

**Convention (by discipline + review + lint):**
- No `AutoMigrate` calls (lint rule).
- Backwards-compatible migration authoring.
- `.down.sql` written for every `.up.sql`.
- Seeds outside migrations.
