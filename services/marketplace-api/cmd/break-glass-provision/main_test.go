package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	adminhandlers "github.com/mark8ly/marketplace-api/internal/handlers/admin"
)

// discardLogger is a *slog.Logger that writes nowhere — provisionTenant
// logs progress, but no test here asserts on log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- DryRun repository harness -------------------------------------------
//
// Same no-database technique as internal/breakglass/bootstrap_test.go's
// newDryRunRepo (gorm DryRun + SkipDefaultTransaction, driven over a
// Postgres dialector that never dials because DryRun never executes a
// query), duplicated here rather than exported from internal/breakglass
// because that package intentionally keeps its test helpers unexported to
// internal/breakglass_test. This harness only needs to observe ordering
// ("db_insert" landed relative to the secret write and the FGA write), not
// any SQL text, so it does not need bootstrap_test.go's insertSQL capture.
type orderTraceLogger struct {
	order *[]string
}

func (l *orderTraceLogger) LogMode(logger.LogLevel) logger.Interface      { return l }
func (l *orderTraceLogger) Info(context.Context, string, ...interface{})  {}
func (l *orderTraceLogger) Warn(context.Context, string, ...interface{})  {}
func (l *orderTraceLogger) Error(context.Context, string, ...interface{}) {}
func (l *orderTraceLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	if strings.Contains(strings.ToUpper(sql), "INSERT INTO") {
		*l.order = append(*l.order, "db_insert")
	}
}

// newDryRunBootstrapper builds a *breakglass.Bootstrapper over a DryRun
// gorm session (never dials a real Postgres) and a
// breakglass.FakeSecretClient, both instrumented to append into the
// shared order slice, so a test can observe "secret_write" and
// "db_insert" landing in order relative to a later "fga_write" — all
// without a live database, OpenBao, or OpenFGA.
//
//   - forceNotFound=true models "no account exists yet for this tenant":
//     Provision proceeds through the secret write and the DB insert.
//   - forceNotFound=false models "an account already exists" (what a
//     second CLI run for the same tenant sees): GetByTenant's DryRun
//     SELECT returns a nil error with no forced not-found, so Provision
//     short-circuits to ErrAlreadyProvisioned before either the secret
//     write or the DB insert.
//
// failSecret, when true, makes the secret write return an error
// (exercising Provision's secret-before-DB short-circuit on failure).
func newDryRunBootstrapper(t *testing.T, order *[]string, forceNotFound, failSecret bool) *breakglass.Bootstrapper {
	t.Helper()

	base, err := gorm.Open(postgres.Open("postgres://dry-run:unused@127.0.0.1:1/dry-run?sslmode=disable"), &gorm.Config{
		DisableAutomaticPing: true,
	})
	require.NoError(t, err, "gorm.Open must not dial — DryRun below never executes a query")

	dryDB := base.Session(&gorm.Session{
		DryRun:                 true,
		SkipDefaultTransaction: true,
		Logger:                 &orderTraceLogger{order: order},
	})

	if forceNotFound {
		err = dryDB.Callback().Query().After("gorm:query").Register("test:force_not_found", func(tx *gorm.DB) {
			if tx.Error == nil {
				tx.Error = gorm.ErrRecordNotFound
			}
		})
		require.NoError(t, err)
	}

	repo := breakglass.NewRepository(dryDB)
	secrets := breakglass.NewSecretManager(&orderedFakeSecretClient{
		order:    order,
		failWith: failSecretErr(failSecret),
	})
	return breakglass.NewBootstrapper(repo, secrets)
}

func failSecretErr(fail bool) error {
	if !fail {
		return nil
	}
	return errors.New("boom: secret backend unreachable")
}

// orderedFakeSecretClient is a breakglass.SecretClient spy — same pattern
// as bootstrap_test.go's orderedSecretClient — recording into the shared
// order slice this file's tests assert on.
type orderedFakeSecretClient struct {
	order    *[]string
	failWith error
}

func (c *orderedFakeSecretClient) AddVersion(_ context.Context, _ string, _ []byte) error {
	*c.order = append(*c.order, "secret_write")
	return c.failWith
}

func (c *orderedFakeSecretClient) AccessLatest(context.Context, string) ([]byte, error) {
	return nil, errors.New("orderedFakeSecretClient: AccessLatest not implemented")
}

// orderedFakeAuthzClient wraps authz.FakeClient, appending "fga_write"
// (or "fga_write_fail") into the shared order slice on WriteRole so tests
// can assert it lands strictly after "secret_write" and "db_insert".
type orderedFakeAuthzClient struct {
	*authz.FakeClient
	order    *[]string
	failWith error
}

func newOrderedFakeAuthzClient(order *[]string) *orderedFakeAuthzClient {
	return &orderedFakeAuthzClient{FakeClient: authz.NewFakeClient(), order: order}
}

func (c *orderedFakeAuthzClient) WriteRole(ctx context.Context, userID string, role authz.Role, tenantID string) error {
	if c.failWith != nil {
		*c.order = append(*c.order, "fga_write_fail")
		return c.failWith
	}
	*c.order = append(*c.order, "fga_write")
	return c.FakeClient.WriteRole(ctx, userID, role, tenantID)
}

// --- tests -----------------------------------------------------------------

// TestProvisionTenant_RunsSecretThenDBThenTuple is the ordering assertion
// the brief calls out explicitly: the full sequence runs secret -> DB ->
// tuple, observed via the shared order slice across three independent
// fakes (secret client, DryRun DB, FGA client) — never a live backend.
func TestProvisionTenant_RunsSecretThenDBThenTuple(t *testing.T) {
	var order []string
	bootstrapper := newDryRunBootstrapper(t, &order, true, false)
	fga := newOrderedFakeAuthzClient(&order)

	tenantID := uuid.New()
	err := provisionTenant(context.Background(), discardLogger(), bootstrapper, fga, tenantID)
	require.NoError(t, err)

	require.Equal(t, []string{"secret_write", "db_insert", "fga_write"}, order,
		"break-glass provisioning must write the secret, then the DB row, then the FGA tuple, in that order")
}

// TestProvisionTenant_TupleIsExactlyBreakGlassAdminOnTenant asserts the
// tuple written is user:<BreakGlassUserID(tenant)> admin tenant:<tenant> —
// the UUIDv5 value BreakGlassUserID itself produces, not a hand-copied
// literal, so a future change to breakGlassNamespace or the UUIDv5 inputs
// in break_glass_login.go is caught here too.
func TestProvisionTenant_TupleIsExactlyBreakGlassAdminOnTenant(t *testing.T) {
	var order []string
	bootstrapper := newDryRunBootstrapper(t, &order, true, false)
	fga := newOrderedFakeAuthzClient(&order)

	tenantID := uuid.New()
	err := provisionTenant(context.Background(), discardLogger(), bootstrapper, fga, tenantID)
	require.NoError(t, err)

	wantPrincipal := adminhandlers.BreakGlassUserID(tenantID).String()
	role, err := fga.GetRole(context.Background(), wantPrincipal, tenantID.String())
	require.NoError(t, err)
	require.Equal(t, authz.RoleAdmin, role,
		"the break-glass principal must hold exactly the admin role on the tenant")

	// Negative check: nothing was written under the raw tenant ID as a
	// principal.
	otherRole, err := fga.GetRole(context.Background(), tenantID.String(), tenantID.String())
	require.NoError(t, err)
	require.Empty(t, otherRole, "the tenant ID itself must never be used as the FGA principal")
}

// TestProvisionTenant_SecretFailureStopsBeforeDBAndTuple exercises the
// first failure point: a secret-store failure must stop before both the
// DB insert and the FGA write.
func TestProvisionTenant_SecretFailureStopsBeforeDBAndTuple(t *testing.T) {
	var order []string
	bootstrapper := newDryRunBootstrapper(t, &order, true, true)
	fga := newOrderedFakeAuthzClient(&order)

	tenantID := uuid.New()
	err := provisionTenant(context.Background(), discardLogger(), bootstrapper, fga, tenantID)
	require.Error(t, err)
	require.ErrorIs(t, err, breakglass.ErrSecretManagerFailed)

	require.Equal(t, []string{"secret_write"}, order,
		"a failed secret write must stop before the DB insert and before the FGA write")
}

// TestProvisionTenant_TupleFailureAfterProvisionRunsNoFurtherSteps
// exercises the second failure point: bootstrapper.Provision succeeds
// (secret + DB both land) but the FGA write fails. The error must name
// the tenant and instruct re-running the command — the exact recovery
// contract this command's package doc promises, so this test also pins
// that message.
func TestProvisionTenant_TupleFailureAfterProvisionRunsNoFurtherSteps(t *testing.T) {
	var order []string
	bootstrapper := newDryRunBootstrapper(t, &order, true, false)
	fga := newOrderedFakeAuthzClient(&order)
	fga.failWith = errors.New("boom: fga unreachable")

	tenantID := uuid.New()
	err := provisionTenant(context.Background(), discardLogger(), bootstrapper, fga, tenantID)
	require.Error(t, err)

	require.Equal(t, []string{"secret_write", "db_insert", "fga_write_fail"}, order,
		"secret + DB must both have run before the FGA write is attempted")

	require.Contains(t, err.Error(), "HAS a secret and DB row",
		"the failure message must tell the operator the account is half-provisioned")
	require.Contains(t, err.Error(), tenantID.String(),
		"the failure message must name the tenant to re-run for")
	require.Contains(t, err.Error(), "break-glass-provision -tenant",
		"the failure message must name the exact command to re-run")
}

// TestProvisionTenant_AlreadyProvisioned_SkipsSecretAndDBButGrantsTuple
// covers Provision's own idempotency contract (ErrAlreadyProvisioned) as
// seen from provisionTenant: when the account already exists, neither the
// secret store nor the DB is touched again, and this is NOT treated as a
// fatal error — the FGA grant still runs (and, by construction, is itself
// idempotent — see the next test).
func TestProvisionTenant_AlreadyProvisioned_SkipsSecretAndDBButGrantsTuple(t *testing.T) {
	var order []string
	// forceNotFound=false: GetByTenant's DryRun SELECT succeeds with no
	// forced error, so Provision sees "row already exists" and returns
	// ErrAlreadyProvisioned before ever calling AddVersion or Create.
	bootstrapper := newDryRunBootstrapper(t, &order, false, false)
	fga := newOrderedFakeAuthzClient(&order)

	tenantID := uuid.New()
	err := provisionTenant(context.Background(), discardLogger(), bootstrapper, fga, tenantID)
	require.NoError(t, err, "an already-provisioned tenant must not be a fatal error")

	require.Equal(t, []string{"fga_write"}, order,
		"an already-provisioned tenant must skip the secret write and the DB insert, and still grant the FGA tuple")
}

// TestProvisionTenant_ReRunConverges is the whole-command idempotency
// proof: calling provisionTenant twice for the same tenant — once as if
// nothing exists yet, once as if the account already exists (the two
// DryRun bootstrapper modes above) — against the SAME real, stateful
// authz.FakeClient leaves the FGA tuple exactly once, at exactly
// RoleAdmin: the second run neither errors, nor duplicates, nor escalates
// the grant.
func TestProvisionTenant_ReRunConverges(t *testing.T) {
	fga := authz.NewFakeClient()
	tenantID := uuid.New()
	wantPrincipal := adminhandlers.BreakGlassUserID(tenantID).String()

	// First run: "nothing provisioned yet".
	var order1 []string
	firstBootstrapper := newDryRunBootstrapper(t, &order1, true, false)
	err := provisionTenant(context.Background(), discardLogger(), firstBootstrapper, fga, tenantID)
	require.NoError(t, err)
	require.Equal(t, []string{"secret_write", "db_insert"}, order1)

	role, err := fga.GetRole(context.Background(), wantPrincipal, tenantID.String())
	require.NoError(t, err)
	require.Equal(t, authz.RoleAdmin, role)

	// Second run: "already provisioned" (what re-running the CLI for the
	// same tenant sees against the real DB/OpenBao in production).
	var order2 []string
	secondBootstrapper := newDryRunBootstrapper(t, &order2, false, false)
	err = provisionTenant(context.Background(), discardLogger(), secondBootstrapper, fga, tenantID)
	require.NoError(t, err, "re-running provisionTenant for an already-provisioned tenant must converge, not error")
	require.Empty(t, order2, "the second run must not touch the secret store or the DB")

	role, err = fga.GetRole(context.Background(), wantPrincipal, tenantID.String())
	require.NoError(t, err)
	require.Equal(t, authz.RoleAdmin, role, "re-running must not change the granted role")
}
