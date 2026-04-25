package marketplaceapi

import (
	"regexp"
	"strconv"
	"testing"
)

// TestExpectedSchemaVersionMatchesHighestMigration guards landmine #7:
// adding a 0000NN_*.sql file without bumping ExpectedSchemaVersion
// crashloops every marketplace-api pod after deploy. This test fails
// at CI time so the bump is never forgotten again.
func TestExpectedSchemaVersionMatchesHighestMigration(t *testing.T) {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}

	versionRE := regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)
	var highest uint
	for _, e := range entries {
		m := versionRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			t.Fatalf("parse migration %q: %v", e.Name(), err)
		}
		if uint(n) > highest {
			highest = uint(n)
		}
	}

	if highest == 0 {
		t.Fatal("no .up.sql migrations found in embedded FS")
	}
	if ExpectedSchemaVersion != highest {
		t.Fatalf("ExpectedSchemaVersion = %d, but highest migration on disk is %d — bump the constant in migrations.go to match", ExpectedSchemaVersion, highest)
	}
}
