//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// insertOperatorRow seeds one store-less operator audit row at a given age.
func insertOperatorRow(t *testing.T, db *gorm.DB, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'operator', 'tenant.purged', 'tenant', 'success', 'warning', ?)`,
		id, uuid.New(), createdAt,
	).Error)
	return id
}

// THE BOUNDARY, ON THE BOUNDARY. Seven years minus a second survives;
// seven years plus a second is deleted. "Close to the edge" is not the edge.
func TestOperatorPrune_SevenYearBoundary(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(-audit.OperatorRetentionYears, 0, 0)

	survives := insertOperatorRow(t, db, cutoff.Add(time.Second))
	deleted := insertOperatorRow(t, db, cutoff.Add(-time.Second))

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 0)
	stats, err := cron.Run(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.OperatorRowsDeleted, int64(1))

	require.Equal(t, int64(1), countRows(t, db, survives),
		"a row one second inside seven years must survive")
	require.Equal(t, int64(0), countRows(t, db, deleted),
		"a row one second past seven years must be deleted")
}

// THE NEGATIVE GUARD. #311 says store-less rows are never pruned; #365
// narrows that for actor_type='operator' ONLY. A store-less row of any other
// actor_type must still survive, however old — otherwise the narrowing is
// wider than it was written to be.
func TestOperatorPrune_LeavesNonOperatorStoreLessRows(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	ancient := now.AddDate(-20, 0, 0)

	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, created_at)
		 VALUES (?, ?, NULL, 'system', 'tenant.something', 'tenant', 'success', 'info', ?)`,
		id, uuid.New(), ancient,
	).Error)

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 0)
	_, err := cron.Run(ctx)
	require.NoError(t, err)

	require.Equal(t, int64(1), countRows(t, db, id),
		"only actor_type='operator' is pruned by the #365 path; #311 still covers the rest")
}

// The batch loop must terminate and delete everything eligible, not just one
// batch's worth.
func TestOperatorPrune_DeletesBeyondOneBatch(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	ctx := context.Background()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(-audit.OperatorRetentionYears, 0, -1)
	for i := 0; i < 5; i++ {
		insertOperatorRow(t, db, old)
	}

	cron := audit.NewPruneCron(db, nil, func() time.Time { return now }, 2) // batchSize 2
	stats, err := cron.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(5), stats.OperatorRowsDeleted,
		"the loop must continue past the first batch")
}

func countRows(t *testing.T, db *gorm.DB, id uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM audit_logs WHERE id = ?`, id).Scan(&n).Error)
	return n
}
