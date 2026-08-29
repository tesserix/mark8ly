// Package testdb provides a per-test database connection that auto-rolls-back
// on test cleanup. Used by integration tests across the platform-api service.
//
// Pattern: each test gets a *gorm.DB inside a transaction. When the test ends,
// `t.Cleanup` rolls the transaction back, leaving the DB exactly as it was.
// This is much faster than spinning up a fresh container per test, and works
// for ~95% of integration tests. Tests that need real commits (e.g. testing
// LISTEN/NOTIFY or multi-connection behavior) should use a different approach.
//
// Required: TEST_DATABASE_URL must point at a real Postgres with the migrations
// already applied. In CI and `make test-int`, this is the docker-compose
// `platform_api` database. Locally, run `make dev` first.
package testdb

import (
	"os"
	"slices"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewTx returns a *gorm.DB inside an open transaction. The transaction is
// rolled back automatically when the test completes (success or failure).
//
// Use this for tests where the assertion can be made through the same
// connection as the writes (which is the common case). Don't use this for
// outbox-style tests where a background drainer reads through a separate
// connection — uncommitted rows aren't visible across connections.
//
// If TEST_DATABASE_URL is unset, the test is skipped — integration tests
// must opt in by exporting the env var.
func NewTx(t *testing.T) *gorm.DB {
	t.Helper()

	db := openOrSkip(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("testdb: begin: %v", tx.Error)
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil {
			t.Logf("testdb: rollback: %v", err)
		}
	})
	return tx
}

// NewDB returns a regular *gorm.DB (NOT in a transaction) and TRUNCATEs the
// given tables on test cleanup so the next test starts with a clean state.
//
// Use this for tests where you need real commits — for example outbox tests
// where a background drainer reads in a separate connection. Truncation
// is per-test, not global, so tests don't have to worry about ordering.
//
// Pass tables in dependency order (children before parents). The truncate
// uses CASCADE so referential integrity is preserved.
func NewDB(t *testing.T, tablesToCleanup ...string) *gorm.DB {
	t.Helper()

	db := openOrSkip(t)

	// Truncate up-front too, in case a previous test crashed without cleanup.
	truncate(t, db, tablesToCleanup)

	t.Cleanup(func() {
		truncate(t, db, tablesToCleanup)
		dropOrphanStoreSequences(t, db, tablesToCleanup)
	})
	return db
}

// dropOrphanStoreSequences reclaims per-store sequences whose store no longer
// exists, but only when this fixture truncated `stores` — which is precisely
// the operation that orphans them.
//
// SeedStore drops the sequences it created, but it is not the only source:
// nineteen test files INSERT INTO stores directly and handler tests create
// stores over HTTP, so a per-fixture drop cannot reach them all and the next
// fixture added leaks again with nothing failing. A sweep here is self-healing
// and needs no discipline from whoever writes the next test (#436).
//
// Why this matters: migration 000004's AFTER INSERT trigger creates two
// sequences per store, TRUNCATE does not touch catalog objects, and nothing
// ever dropped them — the dev database reached 27,990, at which point pg_dump
// failed with "out of shared memory" because it locks every relation in one
// transaction. One suite pass alone put 4,152 back.
//
// Best-effort by design: a sweep failure must never fail a test that passed.
func dropOrphanStoreSequences(t *testing.T, db *gorm.DB, tablesToCleanup []string) {
	t.Helper()
	if !slices.Contains(tablesToCleanup, "stores") {
		return
	}

	// LIMIT bounds the work so this stays cheap once the catalog is healthy;
	// a large backlog drains over successive runs rather than stalling one.
	var names []string
	if err := db.Raw(`
		SELECT c.relname
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind = 'S'
		   AND n.nspname = 'public'
		   AND (c.relname LIKE 'mk\_seq\_order\_%' OR c.relname LIKE 'mk\_seq\_return\_%')
		   AND NOT EXISTS (
		       SELECT 1 FROM stores s
		        WHERE s.id::text = translate(
		              regexp_replace(c.relname, '^mk_seq_(order|return)_', ''), '_', '-'))
		 LIMIT 500`).Scan(&names).Error; err != nil {
		t.Logf("testdb: sweep orphan store sequences: %v", err)
		return
	}

	for _, name := range names {
		// Identifier, not a bind value. Safe to interpolate: every name came
		// from pg_class and matched the mk_seq_(order|return)_ pattern above,
		// so the charset is [a-z0-9_] only.
		if err := db.Exec(`DROP SEQUENCE IF EXISTS ` + name).Error; err != nil {
			t.Logf("testdb: drop orphan sequence %s: %v", name, err)
		}
	}
}

func openOrSkip(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test. Run `make dev` then `make test-int`.")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testdb: open: %v", err)
	}
	return db
}

func truncate(t *testing.T, db *gorm.DB, tables []string) {
	t.Helper()
	if len(tables) == 0 {
		return
	}
	// Build a single TRUNCATE ... CASCADE statement so the order is handled
	// by Postgres rather than the caller.
	stmt := "TRUNCATE TABLE "
	for i, table := range tables {
		if i > 0 {
			stmt += ", "
		}
		stmt += table
	}
	stmt += " CASCADE"
	if err := db.Exec(stmt).Error; err != nil {
		t.Fatalf("testdb: truncate: %v", err)
	}
}
