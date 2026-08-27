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

func seedErasure(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, requestedAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	// customer_erasure_requests has UNIQUE (store_id, customer_email), so the
	// email must be distinct per row within a store. Derive it from the row id.
	email := "buyer-" + id.String()[:8] + "@example.com"
	require.NoError(t, db.Exec(`
		INSERT INTO customer_erasure_requests
			(id, tenant_id, store_id, customer_email, requested_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, tenantID, storeID, email, requestedAt, status,
	).Error)
	return id
}

func TestErasureProvider_OnlyPendingAndNoDueDate(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedErasure(t, db, tenantID, storeID, "processed", now.Add(-48*time.Hour))
	seedErasure(t, db, tenantID, storeID, "rejected", now.Add(-48*time.Hour))
	wanted := seedErasure(t, db, tenantID, storeID, "pending", now.Add(-72*time.Hour))

	p := inbox.NewErasureProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindErasureRequest, items[0].Kind)
	require.Contains(t, items[0].Title, "@example.com", "title is the customer email")
	require.Nil(t, items[0].DueAt,
		"no derived GDPR deadline — the table has no due column and this endpoint does not invent policy")
	require.Equal(t, inbox.SeverityNormal, items[0].Severity)
	require.WithinDuration(t, now.Add(-72*time.Hour), items[0].WaitingSince, time.Second)
}

func TestErasureProvider_FilterStatus(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedErasure(t, db, tenantID, storeID, "pending", now.Add(-72*time.Hour))

	p := inbox.NewErasureProvider(db)

	items, err := p.List(context.Background(), inbox.Filter{Limit: 10, Status: "bogus"})
	require.NoError(t, err)
	require.Empty(t, items, "a non-matching status must short-circuit to empty, not query")

	n, err := p.Count(context.Background(), inbox.Filter{Status: "bogus"})
	require.NoError(t, err)
	require.Zero(t, n)

	items, err = p.List(context.Background(), inbox.Filter{Limit: 10, Status: "pending"})
	require.NoError(t, err)
	require.Len(t, items, 1, "the matching status must behave exactly as an empty status")

	n, err = p.Count(context.Background(), inbox.Filter{Status: "pending"})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
