package marketplaceapi

import "embed"

// MigrationsFS embeds all SQL migration files.
// Both cmd/marketplace-api (for AssertVersion on startup) and cmd/migrate
// (for Up/Down/Version) read from here.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ExpectedSchemaVersion is the migration version the current code was
// written against. cmd/marketplace-api refuses to start if the database's
// migration state doesn't match. M1 runs against version 0 (empty schema,
// only the marketplace_db_schema_migrations table exists). Bump this with
// every migration.
const ExpectedSchemaVersion uint = 0
