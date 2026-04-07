# Database migrations

> Audience: anyone adding, rolling back, or troubleshooting a schema
> change. For the *why* behind the migration strategy (no AutoMigrate,
> two-binary pattern, `AssertVersion` on boot) read
> [`docs/planning/05-migrations-strategy.md`](./planning/05-migrations-strategy.md).

## The rules

1. **No GORM AutoMigrate anywhere.** Every schema change is a pair of
   files under `services/<svc>/migrations/`.
2. **Server refuses to start on schema mismatch.** `cmd/server/main.go`
   calls `migrate.AssertVersion(platformapi.ExpectedSchemaVersion)`
   before opening a single route. A stale binary can never touch a
   newer DB, a newer binary can never touch a stale DB.
3. **Every Up has a Down.** The Down must be a true inverse — enforced
   by the migration integration test in
   `services/platform-api/pkg/migrate/migrate_integration_test.go`
   which exercises the up/down/up cycle on every commit.
4. **Migrations are backwards-compatible.** The new binary runs against
   the old schema during rollout; the old binary runs against the new
   schema after a partial rollback. This rules out destructive column
   changes in a single migration — see the multi-step pattern below.
5. **Every migration is idempotent under re-run.** `migrate.Up()` is
   called by an init container on every `make dev`. If a migration
   fails halfway the DB is marked dirty and manual intervention is
   required — dirty state is a loud failure, not a silent one.

## Where files live

```
services/platform-api/
└── migrations/
    ├── 0001_init.up.sql
    ├── 0001_init.down.sql
    ├── 0002_create_locations.up.sql
    ├── 0002_create_locations.down.sql
    ├── 0003_create_tenants.up.sql
    ├── 0003_create_tenants.down.sql
    ├── 0004_create_onboarding.up.sql
    ├── 0004_create_onboarding.down.sql
    ├── 0005_create_verification.up.sql
    ├── 0005_create_verification.down.sql
    ├── 0006_create_outbox.up.sql
    └── 0006_create_outbox.down.sql
```

- **Naming:** four-digit version prefix, snake_case slug, `.up.sql` /
  `.down.sql` suffix. Enforced by `make migrate-new`.
- **Embedding:** all files are embedded at build time via
  `//go:embed migrations/*.sql` in
  `services/platform-api/migrations.go`. The binary is self-contained
  — there's no file-system lookup at runtime.
- **Version constant:** `services/platform-api/migrations.go` exports
  `ExpectedSchemaVersion`. **Bump this number in the same commit that
  adds the migration.** CI will fail otherwise.

## Adding a migration

```bash
# 1. Scaffold the pair.
make migrate-new SERVICE=platform-api NAME=add_tenant_billing_email

# Two empty files appear:
#   services/platform-api/migrations/0007_add_tenant_billing_email.up.sql
#   services/platform-api/migrations/0007_add_tenant_billing_email.down.sql

# 2. Write the Up and the Down. Both must be valid SQL on their own
#    and the Down must undo exactly what the Up did.

# 3. Bump ExpectedSchemaVersion in services/platform-api/migrations.go
#    from 6 to 7.

# 4. Apply locally.
make migrate-up SERVICE=platform-api
make migrate-version SERVICE=platform-api   # should print 7

# 5. Run the migration cycle test to catch Down breakage early.
cd services/platform-api
go test -tags integration ./pkg/migrate/...

# 6. Run the relevant integration tests so any tables you touched
#    still pass under the new schema.
make test-int
```

## Common migration recipes

### Add a nullable column

Safe in a single migration. New code reads/writes the column; old code
ignores it.

```sql
-- 0007_add_tenant_billing_email.up.sql
ALTER TABLE tenants
  ADD COLUMN billing_email varchar(320);

-- 0007_add_tenant_billing_email.down.sql
ALTER TABLE tenants
  DROP COLUMN billing_email;
```

### Add a NOT NULL column

Never do this in one step against a live table. Use the three-commit
dance:

1. **Commit A** — add the column as nullable with a default. Deploy.
2. **Commit B** — backfill existing rows (via a migration script or a
   one-off job). Deploy.
3. **Commit C** — set the column NOT NULL and drop the default. Deploy.

Each commit is independently deployable and rollback-safe.

### Rename a column

Two steps, never one.

1. **Commit A** — add the new column, copy the old one's value in the
   migration, keep both in sync in the application code.
2. **Commit B** — drop the old column once every reader/writer has
   shipped.

### Drop a column

One step, but only after you've audited every service for reads and
writes and confirmed no deployed binary references it. Commit the code
removal first, wait for the deploy, then drop.

### Index creation on a large table

Use `CREATE INDEX CONCURRENTLY` in production-sized databases so
writes aren't blocked. Concurrent index creation can't run inside a
transaction — the migration framework detects this pragma and opts
out of wrapping the file in a tx automatically.

```sql
-- 0008_idx_tenants_slug.up.sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tenants_slug
  ON tenants (slug);

-- 0008_idx_tenants_slug.down.sql
DROP INDEX IF EXISTS idx_tenants_slug;
```

## Rolling back

```bash
# Roll back exactly one migration.
make migrate-down SERVICE=platform-api

# Check where you are.
make migrate-version SERVICE=platform-api
```

For multi-step rollbacks, run `migrate-down` repeatedly — the Makefile
target rolls back one version at a time so you have a chance to stop.

**Rollback in production:**

1. Deploy the previous application version first. Never roll back the
   schema before the code that depends on it.
2. Run `migrate-down` against production using the rollback job
   container (the plan for production infra still lives in
   `tesserix-infra/`).
3. Validate that the previous application version still starts.
4. File a postmortem.

## Testing a migration against a Cloud SQL clone

Production-sized data hides bugs that a small dev DB misses —
`CREATE INDEX` on 10M rows takes minutes, not milliseconds, and
`CONCURRENTLY` is the only way to do it without an outage. Before any
production-destined migration touches a large table:

1. **Clone the production database.** In GCP:
   ```bash
   gcloud sql backups create --instance=mark8ly-prod
   gcloud sql instances clone mark8ly-prod mark8ly-migrate-test \
     --point-in-time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
   ```
2. **Point a local platform-api at the clone.** Set
   `DATABASE_URL=postgres://.../mark8ly-migrate-test?sslmode=require`
   in `.env` and start the migrate binary only (not the server) so
   nothing writes in parallel.
3. **Run the migration with timing.**
   ```bash
   go run ./cmd/migrate up
   ```
   Measure wall-clock, lock wait, and any replication lag on the
   clone's replicas.
4. **Run the Down immediately** and measure the same numbers. If the
   Down is slower than the Up by more than 2×, stop and redesign.
5. **Delete the clone** when you're done — it charges by the hour.

This isn't a regular developer workflow — it's a production-readiness
gate for migrations that can't be validated against a 100-row dev DB.

## The integration test

`services/platform-api/pkg/migrate/migrate_integration_test.go` is the
CI-friendly version of the production dance. On every run it:

1. Creates a uniquely-named throwaway database on the same Postgres
   instance as `TEST_DATABASE_URL`.
2. Runs `Up` — asserts version equals `ExpectedSchemaVersion`.
3. Runs `Up` again — asserts it's idempotent and version is unchanged.
4. Runs `Down` all the way to version 0.
5. Runs `Up` again — asserts the restore path works.
6. Drops the throwaway database.

Run it locally with:

```bash
make dev                              # for TEST_DATABASE_URL to resolve
cd services/platform-api
go test -tags integration ./pkg/migrate/...
```

If this test ever fails, a migration is non-idempotent, has a broken
Down, or the embedded migrationsFS is missing files. Fix the migration
before shipping.

## Troubleshooting

**`schema is dirty at version N — manual intervention required`**
A previous migration failed halfway. Inspect the schema_migrations
table:

```sql
SELECT * FROM schema_migrations;
-- → { version: N, dirty: true }
```

Fix the schema by hand (or restore from a backup), then mark the
version clean:

```sql
UPDATE schema_migrations SET dirty = false WHERE version = N;
```

Then re-run `migrate up`. Never deploy to production after a dirty
state without a postmortem.

**`no migration found for version N`**
The embedded migrationsFS doesn't contain file `000N_*.up.sql`. You
either forgot to commit the file or bumped `ExpectedSchemaVersion` too
aggressively. Diff `git status` and check `services/<svc>/migrations.go`
embed directive.

**`AssertVersion: expected N, got M` on boot**
The server binary was built against a newer migration set than the DB
has applied. Run `make migrate-up SERVICE=<svc>` to catch up, or roll
back the binary to match the DB.
