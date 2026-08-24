//go:build integration

package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedNotification inserts one notifications row via GORM. The table has NO
// foreign keys (only notification_preferences.store_id references stores),
// so no parent store row is needed. NOT NULL columns are tenant_id,
// store_id, type, title — see migrations/000016_notifications.up.sql.
func seedNotification(t *testing.T, db *gorm.DB, n notification.Notification) notification.Notification {
	t.Helper()
	if n.Title == "" {
		n.Title = "seeded title"
	}
	if n.Type == "" {
		n.Type = notification.TypeNewOrder
	}
	require.NoError(t, db.Create(&n).Error)
	return n
}

// The whole point of the method: two notifications under two different
// stores in two different tenants must both come back from one unfiltered
// call. A single-store fixture would pass against ListByStore too, and
// prove nothing about this method.
func TestListPlatform_SpansStoresAndTenants(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantA, StoreID: storeA, Title: "Alpha title"})
	seedNotification(t, db, notification.Notification{TenantID: tenantB, StoreID: storeB, Title: "Beta title"})

	got, err := repo.ListPlatform(context.Background(), db, notification.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	titles := map[string]bool{}
	for _, n := range got.Notifications {
		titles[n.Title] = true
	}
	require.True(t, titles["Alpha title"], "notification from tenant A / store A must appear")
	require.True(t, titles["Beta title"], "notification from tenant B / store B must appear")
}

// store_id and tenant_id NARROW; neither is a required scope. Both
// directions are asserted, because a filter that always applies and a
// filter that never applies both pass a one-sided test.
func TestListPlatform_TenantAndStoreNarrowRatherThanScope(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantA, StoreID: storeA, Title: "Alpha title"})
	seedNotification(t, db, notification.Notification{TenantID: tenantB, StoreID: storeB, Title: "Beta title"})

	all, err := repo.ListPlatform(context.Background(), db, notification.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total, "unset filters must return every store")

	byStore, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{StoreID: &storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byStore.Total)
	require.Equal(t, "Alpha title", byStore.Notifications[0].Title)

	byTenant, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{TenantID: &tenantB, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byTenant.Total)
	require.Equal(t, "Beta title", byTenant.Notifications[0].Title)
}

// Audience discriminates on recipient_user_id IS NULL. BOTH kinds of row
// are seeded: with only one kind present, a filter that always applies and
// one that never applies give the same answer.
func TestListPlatform_AudienceDiscriminatesOnRecipient(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	uid := "gip-uid-customer-1"
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Store row"})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Customer row", RecipientUserID: &uid})

	store, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: notification.AudienceStore, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), store.Total)
	require.Equal(t, "Store row", store.Notifications[0].Title)

	customer, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: notification.AudienceCustomer, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), customer.Total)
	require.Equal(t, "Customer row", customer.Notifications[0].Title)

	// An unrecognised audience is ignored, not an error, and must not
	// silently behave like one of the two real values.
	both, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Audience: "nonsense", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), both.Total)
}

// read filters on is_read. Both a read and an unread row are seeded, for
// the same reason as the audience test above.
func TestListPlatform_ReadFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Unread row", IsRead: false})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Read row", IsRead: true})

	yes, no := true, false

	readOnly, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Read: &yes, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), readOnly.Total)
	require.Equal(t, "Read row", readOnly.Notifications[0].Title)

	unreadOnly, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Read: &no, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), unreadOnly.Total)
	require.Equal(t, "Unread row", unreadOnly.Notifications[0].Title)
}

// type filters on the type column, with a second type present so a
// no-op WHERE cannot pass.
func TestListPlatform_TypeFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Order row", Type: notification.TypeNewOrder})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Stock row", Type: notification.TypeLowStock})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Type: string(notification.TypeLowStock), Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "Stock row", got.Notifications[0].Title)
}

// recipient_user_id is an exact match, with a SECOND customer row under a
// different uid so a query that ignores the filter cannot pass.
func TestListPlatform_RecipientUserIDFilter(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	uidA, uidB := "gip-uid-aaa", "gip-uid-bbb"
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "For A", RecipientUserID: &uidA})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "For B", RecipientUserID: &uidB})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{RecipientUserID: uidB, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "For B", got.Notifications[0].Title)
}

// The from/to window is inclusive on both ends, and the fixture sits on the
// exact boundary instant — the value where a `>` implementation and a `>=`
// implementation disagree. "Close to the edge" is not the edge.
func TestListPlatform_FromToBoundaryIsInclusive(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	boundary := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seedNotification(t, db, notification.Notification{
		TenantID: tenantID, StoreID: storeID, Title: "On the boundary", CreatedAt: boundary,
	})
	seedNotification(t, db, notification.Notification{
		TenantID: tenantID, StoreID: storeID, Title: "Ten days earlier", CreatedAt: boundary.AddDate(0, 0, -10),
	})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{From: &boundary, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total, "a row created exactly at `from` must be included")
	require.Equal(t, "On the boundary", got.Notifications[0].Title)

	gotTo, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{To: &boundary, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), gotTo.Total, "a row created exactly at `to` must be included")
}

// Limit clamps at MaxPlatformPageSize rather than refusing, and an unset
// limit takes the default. Asserted on the returned row count with more
// rows present than the clamp would allow is impractical here; instead
// assert the SQL-visible effect: a limit of 1 returns 1 of 2 rows, and
// Total still reports 2 so the console can page.
func TestListPlatform_LimitAppliesAndTotalIgnoresIt(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	tenantID, storeID := uuid.New(), uuid.New()
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "First"})
	seedNotification(t, db, notification.Notification{TenantID: tenantID, StoreID: storeID, Title: "Second"})

	got, err := repo.ListPlatform(context.Background(), db,
		notification.PlatformListFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, got.Notifications, 1, "limit must bound the page")
	require.Equal(t, int64(2), got.Total, "total must count every match, not the page")
}

// The existing store-scoped List is fail-safe and must stay that way. If a
// future change makes a zero StoreID mean "all stores", this test fails —
// which is its entire purpose.
func TestListByStore_ZeroStoreIDStillMatchesNothing(t *testing.T) {
	db := testdb.NewDB(t, "notifications")
	repo := notification.NewRepository()

	seedNotification(t, db, notification.Notification{
		TenantID: uuid.New(), StoreID: uuid.New(), Title: "Should not leak",
	})

	got, err := repo.ListByStore(context.Background(), db, notification.ListFilter{})
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total, "a zero StoreID must match nothing, never everything")
	require.Empty(t, got.Notifications)
}
