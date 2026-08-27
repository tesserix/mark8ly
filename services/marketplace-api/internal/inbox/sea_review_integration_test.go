//go:build integration

package inbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedSEARow(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, queuedAt, slaDueAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`
		INSERT INTO sea_manual_review_queue
			(id, tenant_id, store_id, country, tax_id, business_name, queue_reason, status, sla_due_at, queued_at)
		VALUES (?, ?, ?, 'MY', 'MY123456789', 'Acme Pte Ltd', 'format_unrecognised', ?, ?, ?)`,
		id, tenantID, storeID, status, slaDueAt, queuedAt,
	).Error
	require.NoError(t, err)
	return id
}

func TestSEAProvider_BreachedSLAIsCriticalAndCarriesDueAt(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	// Queued 8 days ago against a 5-business-day SLA that expired yesterday.
	id := seedSEARow(t, db, tenantID, storeID, "pending", now.Add(-8*24*time.Hour), now.Add(-24*time.Hour))

	p := inbox.NewSEAReviewProvider(db, func() time.Time { return now })
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)

	got := items[0]
	require.Equal(t, id.String(), got.ID)
	require.Equal(t, inbox.KindSEAManualReview, got.Kind)
	require.Equal(t, "Acme Pte Ltd", got.Title)
	require.Equal(t, inbox.SeverityCritical, got.Severity, "a breached SLA must read critical")
	require.NotNil(t, got.DueAt, "due_at is required on every sea_manual_review item")
	require.WithinDuration(t, now.Add(-24*time.Hour), *got.DueAt, time.Second)
	require.Equal(t, []inbox.Action{
		{ID: "approve", Label: "Approve", Destructive: false},
		{ID: "reject", Label: "Reject", Destructive: true},
	}, got.Actions)
}

func TestSEAProvider_ResolvedRowsAreAbsentNotReturnedAsResolved(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID := uuid.New()
	// sea_manual_review_queue has a UNIQUE(tenant_id, store_id, country)
	// constraint, so each row needs its own store under the same tenant.
	storeApproved, storeRejected, storeInReview := uuid.New(), uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeApproved)
	testdb.SeedStore(t, db, tenantID, storeRejected)
	testdb.SeedStore(t, db, tenantID, storeInReview)

	now := time.Now().UTC()
	seedSEARow(t, db, tenantID, storeApproved, "approved", now.Add(-time.Hour), now.Add(time.Hour))
	seedSEARow(t, db, tenantID, storeRejected, "rejected", now.Add(-time.Hour), now.Add(time.Hour))
	wanted := seedSEARow(t, db, tenantID, storeInReview, "in_review", now.Add(-time.Hour), now.Add(time.Hour))

	p := inbox.NewSEAReviewProvider(db, func() time.Time { return now })
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1, "only pending and in_review are waiting on a human")
	require.Equal(t, wanted.String(), items[0].ID)

	n, err := p.Count(context.Background(), inbox.Filter{})
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "Count must answer the same filter as List")
}
