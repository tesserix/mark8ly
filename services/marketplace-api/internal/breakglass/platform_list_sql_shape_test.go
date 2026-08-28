package breakglass_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// dryRunSQLCapture is a minimal gorm logger.Interface that records every
// SQL statement traced through it. Same pattern as sqlCapture in
// platform_list_integration_test.go (which needs a live Postgres and the
// `integration` build tag to run, so it never executes in CI — see
// TestListPlatform_PageQueryNeverSelectsStarOrCredentialColumns below).
// Named distinctly from its integration-test counterpart so both compile
// together without a symbol clash under `-tags=integration`.
type dryRunSQLCapture struct {
	queries *[]string
}

func (c *dryRunSQLCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *dryRunSQLCapture) Info(context.Context, string, ...interface{})  {}
func (c *dryRunSQLCapture) Warn(context.Context, string, ...interface{})  {}
func (c *dryRunSQLCapture) Error(context.Context, string, ...interface{}) {}
func (c *dryRunSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	*c.queries = append(*c.queries, sql)
}

func findDryRunPageQuery(t *testing.T, queries []string) string {
	t.Helper()
	for _, q := range queries {
		if strings.Contains(q, "lockout_expires_at") {
			return q
		}
	}
	t.Fatalf("no captured query looked like the row-fetch select; captured: %v", queries)
	return ""
}

// TestListPlatform_PageQueryNeverSelectsStarOrCredentialColumns is the
// CI-visible twin of
// TestIntegration_ListPlatform_SelectsExplicitColumnsNeverCredentialFields
// (platform_list_integration_test.go): that test guards the security
// property of this whole endpoint — that the generated page query can
// never reach secret_path, password_hash, or totp_secret_ref — but it sits
// behind `//go:build integration` plus a live TEST_DATABASE_URL, which the
// PR pipeline does not provide, so it never runs on a PR.
//
// This test asserts the identical four things against the identical query
// ListPlatform builds, without a database: gorm.Session{DryRun: true} on a
// Postgres dialector backed by an unconnected sql.DB builds the exact SQL
// string a real run would send, but never opens a connection (DryRun skips
// execution entirely — see gorm's callbacks/query.go — and
// DisableAutomaticPing skips gorm.Open's connectivity check). A PR that
// regresses the Select to `SELECT *` now goes red without Postgres.
func TestListPlatform_PageQueryNeverSelectsStarOrCredentialColumns(t *testing.T) {
	base, err := gorm.Open(postgres.Open("postgres://dry-run:unused@127.0.0.1:1/dry-run?sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	require.NoError(t, err, "gorm.Open must not dial — DryRun below never executes a query")

	var captured []string
	dryDB := base.Session(&gorm.Session{
		DryRun: true,
		Logger: &dryRunSQLCapture{queries: &captured},
	})

	// ListPlatform's page fetch goes through gorm's Scan(), which builds
	// on Rows() — and Rows() does not support DryRun (gorm returns
	// ErrDryRunModeUnsupported for it once it reaches the point where it
	// would otherwise execute). That error is expected and harmless here:
	// by the time it's produced, BuildQuerySQL has already run and the
	// full SQL text has already reached dryRunSQLCapture.Trace below, which
	// is the only thing this test needs — it never reads res.Accounts.
	tid := uuid.New()
	_, _ = breakglass.ListPlatform(context.Background(), dryDB,
		breakglass.PlatformListFilter{TenantID: &tid}, time.Now())

	pageQuery := findDryRunPageQuery(t, captured)
	t.Logf("captured page query: %s", pageQuery)

	for _, forbidden := range []string{"*", "secret_path", "password_hash", "totp_secret_ref"} {
		require.NotContains(t, pageQuery, forbidden,
			"the row-fetch query must never be able to reach %q", forbidden)
	}
}
