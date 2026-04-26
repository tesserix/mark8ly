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
// migration state doesn't match. M1 scaffold: version 1 is the no-op init
// migration that seeds the migrations tracking table. Bump this with every
// real migration added.
const ExpectedSchemaVersion uint = 82
