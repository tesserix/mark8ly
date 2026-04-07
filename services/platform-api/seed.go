package platformapi

import "embed"

// SeedFS embeds the JSON seed data files used by cmd/seed.
//
//go:embed seed/*.json
var SeedFS embed.FS
