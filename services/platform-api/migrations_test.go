package platformapi

import (
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

// TestExpectedSchemaVersionMatchesHighestMigration guards the failure mode
// where a migration is added but ExpectedSchemaVersion is not bumped.
//
// cmd/server calls AssertVersion, which requires EXACT equality, while the
// deployment's initContainer runs `migrate up` on every pod start. So a
// forgotten bump does not fail at build or review time — it fails in prod,
// as a crashloop: the initContainer advances the DB to the new version and
// the server then refuses to boot against it.
//
// pkg/migrate's integration test already covers this, but it needs
// TEST_DATABASE_URL and is therefore SKIPPED in CI — which is exactly how a
// missing bump reached main once. This test needs no database, so it runs
// on every `go test ./...`.
func TestExpectedSchemaVersionMatchesHighestMigration(t *testing.T) {
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	var highest uint
	seen := map[uint]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Errorf("migration %q does not match NNNN_name.up.sql", name)
			continue
		}
		n, err := strconv.ParseUint(prefix, 10, 64)
		if err != nil {
			t.Errorf("migration %q has a non-numeric version prefix %q", name, prefix)
			continue
		}
		v := uint(n)
		if seen[v] {
			t.Errorf("duplicate migration version %d", v)
		}
		seen[v] = true
		if v > highest {
			highest = v
		}
	}

	if highest == 0 {
		t.Fatal("no .up.sql migrations found — embed or path is wrong")
	}
	if ExpectedSchemaVersion != highest {
		t.Errorf("ExpectedSchemaVersion = %d, but the highest migration is %d.\n"+
			"Bump ExpectedSchemaVersion in migrations.go to %d — otherwise the "+
			"migrate initContainer advances the DB past what cmd/server accepts "+
			"and platform-api crashloops on rollout.",
			ExpectedSchemaVersion, highest, highest)
	}

	// Every up must have a matching down, or a rollback strands the schema.
	for v := uint(1); v <= highest; v++ {
		if !seen[v] {
			t.Errorf("migration version %d is missing — versions must be contiguous", v)
		}
	}
}

// Each .up.sql must have a sibling .down.sql.
func TestEveryMigrationHasADownFile(t *testing.T) {
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	downs := map[string]bool{}
	var ups []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasSuffix(name, ".down.sql"):
			downs[strings.TrimSuffix(name, ".down.sql")] = true
		case strings.HasSuffix(name, ".up.sql"):
			ups = append(ups, strings.TrimSuffix(name, ".up.sql"))
		}
	}

	for _, u := range ups {
		if !downs[u] {
			t.Errorf("migration %q has no matching %s.down.sql", u+".up.sql", u)
		}
	}
}
