package authbff

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ExpectedSchemaVersion is the version cmd/server requires on startup.
const ExpectedSchemaVersion uint = 3
