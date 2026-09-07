package breakglass_test

import (
	"context"
	"errors"
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

// orderedTraceLogger is a minimal gorm logger.Interface — the same pattern
// as dryRunSQLCapture in platform_list_sql_shape_test.go — except it
// appends onto a slice SHARED with an orderedSecretClient below, so a
// single test can observe the interleaving of "wrote to the secret store"
// and "built the DB INSERT" without ever opening a real connection to
// either backend.
type orderedTraceLogger struct {
	order *[]string
}

func (l *orderedTraceLogger) LogMode(logger.LogLevel) logger.Interface      { return l }
func (l *orderedTraceLogger) Info(context.Context, string, ...interface{})  {}
func (l *orderedTraceLogger) Warn(context.Context, string, ...interface{})  {}
func (l *orderedTraceLogger) Error(context.Context, string, ...interface{}) {}
func (l *orderedTraceLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	// GetByTenant's existence check also traces a SELECT before Create's
	// INSERT ever runs — only the INSERT is the DB-row write this test
	// cares about ordering against the secret write.
	if strings.Contains(strings.ToUpper(sql), "INSERT INTO") {
		*l.order = append(*l.order, "db_insert")
	}
}

// orderedSecretClient is a breakglass.SecretClient spy recording its call
// into the same shared order slice as orderedTraceLogger, and able to fail
// on command to exercise Provision's error path.
type orderedSecretClient struct {
	order    *[]string
	failWith error
}

func (c *orderedSecretClient) AddVersion(_ context.Context, _ string, _ []byte) error {
	*c.order = append(*c.order, "secret_write")
	return c.failWith
}

func (c *orderedSecretClient) AccessLatest(context.Context, string) ([]byte, error) {
	return nil, errors.New("orderedSecretClient: AccessLatest not implemented")
}

// newDryRunRepo builds a *breakglass.Repository over a Postgres dialector
// that never dials (DisableAutomaticPing) and a DryRun session (skips
// executing the built statement) — the same no-database technique
// TestListPlatform_PageQueryNeverSelectsStarOrCredentialColumns uses, so
// this test runs in the ordinary `go test ./...` pass with no
// TEST_DATABASE_URL and no `integration` build tag.
func newDryRunRepo(t *testing.T, order *[]string) *breakglass.Repository {
	t.Helper()
	base, err := gorm.Open(postgres.Open("postgres://dry-run:unused@127.0.0.1:1/dry-run?sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	require.NoError(t, err, "gorm.Open must not dial — DryRun below never executes a query")

	dryDB := base.Session(&gorm.Session{
		DryRun: true,
		// Repository.Create wraps every insert in gorm's default
		// transaction (callbacks/transaction.go BeginTransaction), and
		// unlike the query/create callbacks themselves, BeginTransaction
		// has no DryRun check at all — it unconditionally calls db.Begin(),
		// which would try to dial the unreachable 127.0.0.1:1 DSN above.
		// SkipDefaultTransaction avoids that entirely, matching the same
		// combination gorm's own internal dry-run helper uses (see
		// gorm.io/gorm/gorm.go's ToSQL: `Session(&Session{DryRun: true,
		// SkipDefaultTransaction: true})`).
		SkipDefaultTransaction: true,
		Logger:                 &orderedTraceLogger{order: order},
	})

	// DryRun's query callback (gorm.io/gorm/callbacks.Query) deliberately
	// never executes, so it never reaches the RaiseErrorOnNotFound check
	// that would normally turn a missing row into gorm.ErrRecordNotFound
	// (see gorm.io/gorm/scan.go) — every First()/Find() under DryRun
	// returns a nil error with a zero-value Dest instead. Bootstrapper.
	// Provision relies on GetByTenant translating "no row yet" into
	// breakglass.ErrNotFound to know it's safe to proceed, so this test
	// forces that outcome itself via gorm's public callback hook, without
	// touching production code: every dry-run SELECT this repository
	// issues is a lookup for a tenant that doesn't exist yet.
	err = dryDB.Callback().Query().After("gorm:query").Register("test:force_not_found", func(tx *gorm.DB) {
		if tx.Error == nil {
			tx.Error = gorm.ErrRecordNotFound
		}
	})
	require.NoError(t, err)

	return breakglass.NewRepository(dryDB)
}

// TestBootstrapper_Provision_WritesSecretBeforeDBRow is the mutation-check
// for the invariant secret_manager.go documents on SecretManager.Upsert and
// bootstrap.go restates on Provision: the secret write must land before the
// DB row, so a store failure never leaves a password_hash referencing a
// non-existent blob. Flipping Provision's two calls should turn this red;
// it was run that way by hand while writing this test, then restored.
func TestBootstrapper_Provision_WritesSecretBeforeDBRow(t *testing.T) {
	var order []string
	repo := newDryRunRepo(t, &order)
	secrets := breakglass.NewSecretManager(&orderedSecretClient{order: &order})
	b := breakglass.NewBootstrapper(repo, secrets, "test-project")

	// The DB insert is a DryRun no-op (Create's Trace call still fires —
	// see orderedTraceLogger — but nothing is sent over the wire), so
	// Provision runs to completion with no error.
	err := b.Provision(context.Background(), uuid.New())
	require.NoError(t, err)

	require.Equal(t, []string{"secret_write", "db_insert"}, order,
		"break-glass must write the secret before inserting the DB row")
}

// TestBootstrapper_Provision_SecretFailureNeverReachesDB proves the other
// half of the same invariant: when the secret write fails, Provision must
// return before ever building the DB insert, so no order slice entry named
// "db_insert" appears at all.
func TestBootstrapper_Provision_SecretFailureNeverReachesDB(t *testing.T) {
	var order []string
	repo := newDryRunRepo(t, &order)
	boom := errors.New("boom: secret backend unreachable")
	secrets := breakglass.NewSecretManager(&orderedSecretClient{order: &order, failWith: boom})
	b := breakglass.NewBootstrapper(repo, secrets, "test-project")

	err := b.Provision(context.Background(), uuid.New())
	require.Error(t, err)
	require.ErrorIs(t, err, breakglass.ErrSecretManagerFailed)

	require.Equal(t, []string{"secret_write"}, order,
		"a failed secret write must short-circuit Provision before any DB insert is attempted")
}
