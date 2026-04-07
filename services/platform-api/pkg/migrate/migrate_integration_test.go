//go:build integration

package migrate_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/pkg/migrate"
)

// TestIntegration_MigrationsApplyCleanly is the Phase G migration regression
// test. It exercises the up/down/up cycle against a fresh, throwaway
// Postgres database so it never collides with the shared platform_api DB
// used by the other integration tests.
//
// Why a throwaway DB instead of TEST_DATABASE_URL directly:
//   - Running `migrate.Down()` against the shared DB would wipe every
//     other integration test's data.
//   - The test must guarantee a "starts at version 0" precondition that
//     the shared DB violates by definition (it's already migrated).
//
// The test creates a uniquely-named DB on the same Postgres instance,
// runs the cycle there, and drops it on cleanup. Both successful and
// failed test runs leave the shared DB untouched.
//
// What's covered:
//  1. Up applies every migration; AssertVersion(latest) succeeds.
//  2. Up is idempotent — second invocation is a no-op, version unchanged.
//  3. Down rolls everything back to version 0.
//  4. Up again restores to latest.
//
// If this ever fails, a migration is non-idempotent, has a broken Down,
// or the embedded migrationsFS is missing files.
func TestIntegration_MigrationsApplyCleanly(t *testing.T) {
	baseDSN := os.Getenv("TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping migration integration test. Run `make dev` first.")
	}

	dbName := uniqueDBName(t)
	createDB(t, baseDSN, dbName)
	t.Cleanup(func() {
		dropDB(t, baseDSN, dbName)
	})

	dsn := dsnWithDatabase(t, baseDSN, dbName)
	mig, err := migrate.New(platformapi.MigrationsFS, "migrations", dsn)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	expected := platformapi.ExpectedSchemaVersion

	// 1. Initial Up — applies every migration.
	if err := mig.Up(); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := mig.AssertVersion(expected); err != nil {
		t.Fatalf("AssertVersion after first Up: %v", err)
	}

	// 2. Up again — must be a no-op (idempotent).
	if err := mig.Up(); err != nil {
		t.Fatalf("second Up should be idempotent, got: %v", err)
	}
	if err := mig.AssertVersion(expected); err != nil {
		t.Fatalf("AssertVersion after idempotent Up: %v", err)
	}

	// 3. Down to zero — every Down migration must work. Migrator.Down(n)
	// takes a step count; rolling back `expected` steps lands us at v0.
	if err := mig.Down(int(expected)); err != nil {
		t.Fatalf("Down: %v", err)
	}
	v, dirty, err := mig.Version()
	if err == nil {
		// version is non-zero — Down didn't go all the way to 0
		t.Fatalf("expected version 0 after Down, got version=%d dirty=%v", v, dirty)
	}
	// migrate.ErrNilVersion is the expected error after a clean Down to 0.
	if !strings.Contains(err.Error(), "no migration") && !strings.Contains(err.Error(), "nil version") {
		t.Fatalf("Version after Down: unexpected error %v", err)
	}

	// 4. Up after Down — restores to latest. This is the rollback-then-reapply
	// path that catches any migration whose Down isn't a true inverse of Up.
	if err := mig.Up(); err != nil {
		t.Fatalf("Up after Down: %v", err)
	}
	if err := mig.AssertVersion(expected); err != nil {
		t.Fatalf("AssertVersion after Up-after-Down: %v", err)
	}
}

// uniqueDBName returns a Postgres-safe identifier guaranteed to differ
// from any concurrent test run. Lowercase only — Postgres folds unquoted
// identifiers to lowercase and we want predictable behavior.
func uniqueDBName(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "platform_api_migrate_test_" + hex.EncodeToString(buf)
}

// createDB connects to the maintenance DB and creates a fresh, empty
// database we can run migrations against without disturbing the shared
// platform_api DB used by other integration tests.
func createDB(t *testing.T, baseDSN, name string) {
	t.Helper()
	db, err := sql.Open("postgres", dsnWithDatabase(t, baseDSN, "postgres"))
	if err != nil {
		t.Fatalf("createDB open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf(`CREATE DATABASE %q`, name)); err != nil {
		t.Fatalf("createDB exec: %v", err)
	}
}

// dropDB removes the throwaway DB. Best-effort: failures are logged but
// don't fail the test (the test's main assertions already passed; a
// leaked DB is an annoyance, not a regression).
func dropDB(t *testing.T, baseDSN, name string) {
	t.Helper()
	db, err := sql.Open("postgres", dsnWithDatabase(t, baseDSN, "postgres"))
	if err != nil {
		t.Logf("dropDB open: %v", err)
		return
	}
	defer db.Close()
	// FORCE drops active connections (Postgres 13+).
	if _, err := db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name)); err != nil {
		t.Logf("dropDB exec: %v", err)
	}
}

// dsnWithDatabase rewrites a Postgres URL's path to point at a different
// database, preserving query params (sslmode, etc.).
func dsnWithDatabase(t *testing.T, dsn, dbName string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsnWithDatabase parse: %v", err)
	}
	u.Path = "/" + dbName
	return u.String()
}
