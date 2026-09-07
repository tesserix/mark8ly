//go:build integration

package platformauth_test

// A small, self-contained test-database helper for this module's own
// integration test (nonce_integration_test.go).
//
// marketplace-api has a richer pkg/testdb with fixtures for its own domain
// tables (stores, vendors, ...). This package cannot import it: doing so
// would make platformauth's test binary depend on the marketplace-api
// module, which already depends on platformauth via go.work's replace —
// a real import cycle at the module level, not just an inconvenience. This
// file exists so platformauth stays a leaf module; keep it minimal rather
// than growing it into a second testdb.

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestDB opens a *gorm.DB against TEST_DATABASE_URL and truncates
// platform_request_nonces on cleanup so tests start and end clean. It skips
// the test (never fails it) when TEST_DATABASE_URL is unset — integration
// tests opt in by exporting the env var, matching marketplace-api's
// pkg/testdb convention.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("newTestDB: open: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("newTestDB: pool handle: %v", err)
	}
	// Bounded, not large: this test holds at most two connections at once
	// (TestClaimIsSafeUnderConcurrency uses one per goroutine).
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(1)

	t.Cleanup(func() {
		if err := db.Exec("TRUNCATE TABLE platform_request_nonces").Error; err != nil {
			t.Logf("newTestDB: truncate platform_request_nonces: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Logf("newTestDB: close pool: %v", err)
		}
	})
	return db
}
