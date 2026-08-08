package authbff

import (
	"strconv"
	"strings"
	"testing"
)

// cmd/server asserts the live schema equals ExpectedSchemaVersion exactly,
// so a migration added without bumping the constant boots into a panic.
func TestExpectedSchemaVersionMatchesHighestMigration(t *testing.T) {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	var highest uint
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		num, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Fatalf("migration %q is not in <version>_<name>.up.sql form", name)
		}
		v, err := strconv.ParseUint(num, 10, 32)
		if err != nil {
			t.Fatalf("migration %q has a non-numeric version: %v", name, err)
		}
		if uint(v) > highest {
			highest = uint(v)
		}
	}

	if highest == 0 {
		t.Fatal("no .up.sql migrations found — the embed pattern is broken")
	}
	if ExpectedSchemaVersion != highest {
		t.Errorf("ExpectedSchemaVersion = %d, highest migration = %d", ExpectedSchemaVersion, highest)
	}
}

// Every up needs a down or the rollback path is a lie.
func TestEveryMigrationHasADownFile(t *testing.T) {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		seen[e.Name()] = true
	}
	for name := range seen {
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		down := strings.TrimSuffix(name, ".up.sql") + ".down.sql"
		if !seen[down] {
			t.Errorf("migration %q has no matching %q", name, down)
		}
	}
}
