package platformapi

import "embed"

// MigrationsFS embeds all SQL migration files.
// Both cmd/server (for AssertVersion) and cmd/migrate (for Up/Down) read from here.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ExpectedSchemaVersion is the version cmd/server requires on startup.
// Bump this when you add a new migration. cmd/server refuses to start if the
// DB is at any other version.
const ExpectedSchemaVersion uint = 1
