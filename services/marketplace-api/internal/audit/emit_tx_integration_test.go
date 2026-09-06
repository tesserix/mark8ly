//go:build integration

package audit_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// newRealEmitter wires the shipped gormRepository against a real handle.
// The unit tests in emit_tx_test.go use a double, which can prove which
// handle EmitTx passed but not what Postgres does with it — and
// transactional visibility is the only thing these tests are about.
func newRealEmitter(t *testing.T, db *gorm.DB) *audit.Emitter {
	t.Helper()
	e, err := audit.NewEmitter(audit.EmitterConfig{
		DB:     db,
		Repo:   audit.NewRepository(),
		Logger: slog.Default(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { e.Stop(context.Background()) })
	return e
}

// countAuditRowsForTenant counts audit_logs rows for one tenant through the given handle.
// Reading through `tx` sees uncommitted work; reading through `db` does not —
// that difference is the assertion in both tests below.
func countAuditRowsForTenant(t *testing.T, h *gorm.DB, tenantID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, h.Model(&audit.Entry{}).Where("tenant_id = ?", tenantID).Count(&n).Error)
	return n
}

// TestEmitTx_RowIsAbsentAfterTheCallersTransactionRollsBack is the reason
// EmitTx exists. Emit and EmitSync both write on the Emitter's own handle, so
// their row commits independently of whatever the caller was doing: roll the
// business change back and the audit row claiming it happened survives.
//
// The in-transaction count is not decoration — without it an EmitTx that
// wrote nothing at all would satisfy the post-rollback assertion perfectly.
// The pair together says: the row was really written, on this transaction,
// and died with it.
//
// NewDB rather than NewTx: this test manages its own transaction, and NewTx
// hands back a handle that is already inside one.
func TestEmitTx_RowIsAbsentAfterTheCallersTransactionRollsBack(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	e := newRealEmitter(t, db)
	tenantID := uuid.New()

	tx := db.Begin()
	require.NoError(t, tx.Error)

	require.NoError(t, e.EmitTx(context.Background(), tx, nil, audit.Event{
		Action: "tenant.discount_applied", ResourceType: "subscription",
		TenantID: tenantID, Metadata: map[string]any{"coupon_id": "co_123"},
	}))

	require.EqualValues(t, 1, countAuditRowsForTenant(t, tx, tenantID),
		"the row must be visible inside the caller's transaction")

	require.NoError(t, tx.Rollback().Error)

	require.EqualValues(t, 0, countAuditRowsForTenant(t, db, tenantID),
		"the audit row must roll back with the change it describes")
}

// TestEmitTx_RowSurvivesTheCallersCommit is the positive control for the
// test above: rolling back is only meaningful if committing keeps the row.
func TestEmitTx_RowSurvivesTheCallersCommit(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	e := newRealEmitter(t, db)
	tenantID := uuid.New()

	tx := db.Begin()
	require.NoError(t, tx.Error)

	require.NoError(t, e.EmitTx(context.Background(), tx, nil, audit.Event{
		Action: "tenant.discount_applied", ResourceType: "subscription", TenantID: tenantID,
	}))
	require.NoError(t, tx.Commit().Error)

	require.EqualValues(t, 1, countAuditRowsForTenant(t, db, tenantID),
		"a committed transaction must leave its audit row behind")
}
