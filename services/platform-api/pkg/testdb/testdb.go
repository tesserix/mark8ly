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
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewTx returns a *gorm.DB inside an open transaction. The transaction is
// rolled back automatically when the test completes (success or failure).
//
// If TEST_DATABASE_URL is unset, the test is skipped — integration tests
// must opt in by exporting the env var.
func NewTx(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test. Run `make dev` then export TEST_DATABASE_URL.")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testdb: open: %v", err)
	}

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
