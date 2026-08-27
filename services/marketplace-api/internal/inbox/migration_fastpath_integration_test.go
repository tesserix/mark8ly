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

func seedFastPath(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO migration_fast_path_reviews
			(id, tenant_id, store_id, evidence_type, evidence_url, prior_platform, status, created_at)
		VALUES (?, ?, ?, 'platform_screenshot', 'https://example.com/e.png', 'shopify', ?, ?)`,
		id, tenantID, storeID, status, createdAt,
	).Error)
	return id
}

func TestMigrationFastPathProvider_OnlyPending(t *testing.T) {
	db := testdb.NewTx(t)
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	now := time.Now().UTC()
	seedFastPath(t, db, tenantID, storeID, "approved", now.Add(-time.Hour))
	wanted := seedFastPath(t, db, tenantID, storeID, "pending", now.Add(-5*time.Hour))

	p := inbox.NewMigrationFastPathProvider(db)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, wanted.String(), items[0].ID)
	require.Equal(t, inbox.KindMigrationFastPath, items[0].Kind)
	require.Equal(t, "shopify", items[0].Subtitle)
}
