# marketplace-api

The marketplace runtime service. Consolidates what used to be ~20 `mp-*`
microservices into a single Go binary with per-domain internal packages.

Slice 1 scope: products, categories, and media. See
`docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` for
the authoritative design and `docs/superpowers/plans/` for the milestone
implementation plans.

## Local development

Run the dev stack from the repo root:

```bash
docker compose -f infra/dev/docker-compose.yml up -d postgres marketplace-api-migrate marketplace-api
```

(Or whatever `make dev` target wires these up once added to the Makefile.)

This brings up Postgres, OpenFGA, auth-bff, platform-api, and marketplace-api
via docker-compose. marketplace-api listens on `http://localhost:8091`.

Verify:

```bash
curl http://localhost:8091/health   # {"status":"ok"}
curl http://localhost:8091/ready    # {"status":"ok"}
```

Note: port 8091 was chosen because 8086 / 8087 / 8088 / 8089 / 8090 are
already taken by the rest of the dev stack. The `HTTPPort` default in
`pkg/config/config.go` is 8087 — that's the value used when no env var is
set (e.g. running the compiled binary directly outside compose); the
compose file overrides it to 8091 explicitly.

## Binaries

- `cmd/marketplace-api` — the HTTP server. Reads `DATABASE_URL`, `MODE`,
  `HTTP_PORT`, `ENV` from env.
- `cmd/migrate` — golang-migrate CLI. Supports `up`, `down N`, `version`.

## MODE

`MODE` selects which Gin engine(s) the binary constructs on startup:

| MODE         | Admin engine | Storefront engine |
|--------------|--------------|-------------------|
| `admin`      | yes          | no                |
| `storefront` | no           | yes               |
| `both`       | yes          | yes               |

`both` is the default and is used in local dev. In the dev/prod cluster,
two Knative Services deploy the same image with `MODE=admin` and
`MODE=storefront` respectively. See
`docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md` §14.8.

## Scaffolding duplication

`pkg/config`, `pkg/db`, `pkg/logger`, `pkg/httpserver`, `pkg/migrate`, and
`pkg/testdb` are copy-pasted from `services/platform-api/pkg/`. This is
deliberate: inter-service compile-time coupling between microservice
runtimes is explicitly forbidden by the architecture decision.

`pkg/httpserver` is the one exception — it is adapted (not byte-copied)
because marketplace-api's `New` takes a `mode.Mode` argument and returns
per-engine fields, while platform-api's `New` is single-engine.

When a third service emerges that needs the same scaffolding, extract
these packages into a shared `pkg/go-shared` Go module. Until then,
tolerate the duplication and keep the copies in sync manually if either
platform-api or marketplace-api needs a scaffolding change.

## Tests

```bash
go test ./...                                    # unit tests only
TEST_DATABASE_URL=postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable \
  go test -tags=integration ./...                # integration tests (when they exist)
```

`pkg/testdb` provides a per-test transaction-rollback helper consumed by
integration tests starting in M3.

## Database

- DB name: `marketplace_db` (on the shared dev Postgres instance)
- User: `dev` (dev only) / `marketplace_user` (prod)
- Migrations tracking table: `marketplace_db_schema_migrations`
- Slice 1 schema lands in M2 (not yet landed as of M1 completion)

## Migrations — scaffold placeholder

M1 ships a **no-op scaffold migration** at `migrations/000001_init.up.sql`
(and matching `.down.sql`). It exists only to satisfy `golang-migrate`'s
`iofs` source, which refuses an empty migration set. The migration runs a
`SELECT 1` and does nothing. `ExpectedSchemaVersion = 1` in `migrations.go`
matches this state.

**When M2 lands the real products schema, it must REPLACE `000001_init.*`
entirely** — delete both files, add `0001_products_initial.up.sql` /
`0001_products_initial.down.sql` (or whatever final naming the M2 plan
chooses), and keep `ExpectedSchemaVersion = 1`. The scaffold migration is
ephemeral and was never intended to ship to production. The `.keep.sql`
file in the migrations directory is a leftover from Task 2 of the M1 plan
and is also safe to delete alongside the scaffold migration in M2.

## Related

- Spec: `docs/superpowers/specs/2026-04-09-products-feature-slice-1-design.md`
- M1 plan: `docs/superpowers/plans/2026-04-09-products-m1-marketplace-api-scaffold.md`
- Sister service (pattern reference): `services/platform-api/`
