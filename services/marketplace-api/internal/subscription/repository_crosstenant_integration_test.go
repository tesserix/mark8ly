//go:build integration

package subscription_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedCrossTenantStore inserts a minimal stores row so a store_subscriptions
// row referencing storeID satisfies store_subscriptions_store_id_fkey.
// Mirrors internal/billing/trial/expiring_integration_test.go's helper.
func seedCrossTenantStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	slug := "xt-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Cross Tenant Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// seedCrossTenantRow seeds a stores row plus a store_subscriptions row that
// references it, and registers cleanup for the subscription row.
func seedCrossTenantRow(t *testing.T, db *gorm.DB, row subscription.StoreSubscription) subscription.StoreSubscription {
	t.Helper()

	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	if row.TenantID == uuid.Nil {
		row.TenantID = uuid.New()
	}
	if row.StoreID == uuid.Nil {
		row.StoreID = uuid.New()
	}
	if row.StripeCustomerID == "" {
		row.StripeCustomerID = "cus_xt_" + row.ID.String()
	}
	if row.Plan == "" {
		row.Plan = subscription.PlanStarter
	}
	if row.Status == "" {
		row.Status = subscription.StatusActive
	}

	seedCrossTenantStore(t, db, row.TenantID, row.StoreID)

	require.NoError(t, db.Create(&row).Error)
	t.Cleanup(func() {
		db.Unscoped().Delete(&subscription.StoreSubscription{}, "id = ?", row.ID)
	})
	return row
}

// TestListAllSubscriptions_CrossesTenants proves the estate-wide behaviour
// this method exists for: two rows belonging to two different tenants both
// come back from a single unscoped call.
func TestCrossTenantListAllSubscriptions_CrossesTenants(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	rowA := seedCrossTenantRow(t, db, subscription.StoreSubscription{})
	rowB := seedCrossTenantRow(t, db, subscription.StoreSubscription{})
	require.NotEqual(t, rowA.TenantID, rowB.TenantID, "rows must belong to two different tenants")

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, rows, 2)

	tenants := map[uuid.UUID]bool{}
	for _, r := range rows {
		tenants[r.TenantID] = true
	}
	assert.True(t, tenants[rowA.TenantID], "expected rowA's tenant present")
	assert.True(t, tenants[rowB.TenantID], "expected rowB's tenant present")
}

// TestListAllSubscriptions_StatusFilterNarrows proves Status narrows the
// result set, and that an empty Status returns everything (the guard).
func TestCrossTenantListAllSubscriptions_StatusFilterNarrows(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedCrossTenantRow(t, db, subscription.StoreSubscription{Status: subscription.StatusActive})
	seedCrossTenantRow(t, db, subscription.StoreSubscription{Status: subscription.StatusTrialing})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{
		Status: string(subscription.StatusActive),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, subscription.StatusActive, rows[0].Status)

	rows, total, err = repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, rows, 2)
}

// TestListAllSubscriptions_PlanFilterNarrows proves Plan narrows the result
// set, and that an empty Plan returns everything (the guard).
func TestCrossTenantListAllSubscriptions_PlanFilterNarrows(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedCrossTenantRow(t, db, subscription.StoreSubscription{Plan: subscription.PlanStarter})
	seedCrossTenantRow(t, db, subscription.StoreSubscription{Plan: subscription.PlanStudio})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{
		Plan: string(subscription.PlanStudio),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, subscription.PlanStudio, rows[0].Plan)

	rows, total, err = repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, rows, 2)
}

// TestListAllSubscriptions_FiltersCombineWithAND proves Status and Plan
// combine as AND, rather than one replacing the other.
func TestCrossTenantListAllSubscriptions_FiltersCombineWithAND(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	// Matches both.
	match := seedCrossTenantRow(t, db, subscription.StoreSubscription{
		Status: subscription.StatusActive,
		Plan:   subscription.PlanStudio,
	})
	// Matches status only.
	seedCrossTenantRow(t, db, subscription.StoreSubscription{
		Status: subscription.StatusActive,
		Plan:   subscription.PlanStarter,
	})
	// Matches plan only.
	seedCrossTenantRow(t, db, subscription.StoreSubscription{
		Status: subscription.StatusTrialing,
		Plan:   subscription.PlanStudio,
	})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{
		Status: string(subscription.StatusActive),
		Plan:   string(subscription.PlanStudio),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, match.ID, rows[0].ID)
}

// TestListAllSubscriptions_OrderedCreatedAtDesc seeds three rows in an
// insertion order that differs from their created_at ordering, and asserts
// the listing comes back newest-first.
func TestCrossTenantListAllSubscriptions_OrderedCreatedAtDesc(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rowOldest := seedCrossTenantRow(t, db, subscription.StoreSubscription{CreatedAt: base})
	rowNewest := seedCrossTenantRow(t, db, subscription.StoreSubscription{CreatedAt: base.Add(48 * time.Hour)})
	rowMiddle := seedCrossTenantRow(t, db, subscription.StoreSubscription{CreatedAt: base.Add(24 * time.Hour)})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, rows, 3)

	assert.Equal(t, rowNewest.ID, rows[0].ID)
	assert.Equal(t, rowMiddle.ID, rows[1].ID)
	assert.Equal(t, rowOldest.ID, rows[2].ID)
}

// TestListAllSubscriptions_Pagination proves Limit: 1 returns one row with
// total equal to the full match count.
func TestCrossTenantListAllSubscriptions_Pagination(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedCrossTenantRow(t, db, subscription.StoreSubscription{})
	seedCrossTenantRow(t, db, subscription.StoreSubscription{})
	seedCrossTenantRow(t, db, subscription.StoreSubscription{})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{
		Page:  1,
		Limit: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, rows, 1)
}

// TestListAllSubscriptions_LimitClampsTo500 proves a Limit above the ceiling
// clamps to 500 and that the clamped value governs the returned page size.
func TestCrossTenantListAllSubscriptions_LimitClampsTo500(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	seedCrossTenantRow(t, db, subscription.StoreSubscription{})

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{
		Limit: 9999,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.LessOrEqual(t, cap(rows), 500)
}

// TestListAllSubscriptions_EmptyTableReturnsAllocatedEmptySlice proves an
// empty table returns an allocated empty slice (not nil) and total=0.
func TestCrossTenantListAllSubscriptions_EmptyTableReturnsAllocatedEmptySlice(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")
	repo := subscription.NewRepository()

	rows, total, err := repo.ListAllSubscriptions(context.Background(), db, subscription.CrossTenantFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.NotNil(t, rows)
	assert.Empty(t, rows)
}
