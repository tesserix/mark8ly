//go:build integration

package tenantpurge_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/blobreap"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

const purgeBucket = "mark8ly-test-media"

func objURL(key string) string {
	return "https://storage.googleapis.com/" + purgeBucket + "/" + key
}

// seedProductWithMedia creates a product in storeID carrying one media row
// that names `key` in all three of its object columns, the way a real
// upload does.
func seedProductWithMedia(t *testing.T, db *gorm.DB, tenantID, storeID, key string) {
	t.Helper()
	productID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id)
		 VALUES (?, ?, ?, ?, 'P', 'draft', ?)`,
		productID, tenantID, storeID, "p-"+productID[:8], uuid.NewString()).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO product_media (id, product_id, url, storage_key, gcs_path_original, media_type, position)
		 VALUES (?, ?, ?, ?, ?, 'image', 0)`,
		uuid.NewString(), productID, objURL(key), key, key).Error)
}

// TestIntegration_Purge_DestroysTheObjectsItsRowsPointedAt is the whole
// point of #961: before it, the rows went and the images stayed in a
// public bucket forever.
func TestIntegration_Purge_DestroysTheObjectsItsRowsPointedAt(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	tenantID := uuid.NewString()
	storeID := seedStore(t, db, tenantID)

	key := "tenants/" + tenantID + "/products/media/abc/photo.jpg"
	seedProductWithMedia(t, db, tenantID, storeID, key)

	fake := media.NewFakeUploader()
	fake.Register(media.Attrs{StorageKey: key, Size: 10, ContentType: "image/jpeg"})

	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, tenantID, []string{storeID},
		blobreap.New(db, fake, purgeBucket, nil))
	require.NoError(t, err)

	require.Equal(t, 1, rep.Blobs.Deleted, "the object must go with its rows")
	require.Equal(t, 1, fake.Deleted(key))

	// The three columns naming one object must not be counted as three.
	require.Zero(t, rep.Blobs.SkippedStillReferenced)
	require.Zero(t, rep.Blobs.Failed)
}

// The reason blobreap exists rather than reusing the erasure helper.
//
// product_media.storage_key is content-addressed and copy-to-store points
// rows in SEVERAL stores at one object. Purging one tenant must not blank
// the image in a store that is still trading.
func TestIntegration_Purge_LeavesAnObjectAnotherTenantStillUses(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)

	shared := "tenants/shared/products/media/dup/hero.jpg"

	doomedTenant := uuid.NewString()
	doomedStore := seedStore(t, db, doomedTenant)
	seedProductWithMedia(t, db, doomedTenant, doomedStore, shared)

	survivorTenant := uuid.NewString()
	survivorStore := seedStore(t, db, survivorTenant)
	seedProductWithMedia(t, db, survivorTenant, survivorStore, shared)

	fake := media.NewFakeUploader()
	fake.Register(media.Attrs{StorageKey: shared, Size: 10, ContentType: "image/jpeg"})

	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, doomedTenant, []string{doomedStore},
		blobreap.New(db, fake, purgeBucket, nil))
	require.NoError(t, err)

	require.Zero(t, rep.Blobs.Deleted)
	require.Equal(t, 1, rep.Blobs.SkippedStillReferenced)
	require.Zero(t, fake.Deleted(shared),
		"the surviving tenant's product image must still exist")

	// And the survivor's row is untouched.
	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM product_media m JOIN products p ON p.id = m.product_id
		  WHERE p.tenant_id = ?`, survivorTenant).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestIntegration_Purge_WithNoReaperStillPurgesRowsAndSaysSo(t *testing.T) {
	db := testdb.NewDB(t, domainTablesToCleanup...)
	tenantID := uuid.NewString()
	storeID := seedStore(t, db, tenantID)
	key := "tenants/" + tenantID + "/products/media/x/p.jpg"
	seedProductWithMedia(t, db, tenantID, storeID, key)

	rep, err := tenantpurge.PurgeWithReaper(context.Background(), db, tenantID, []string{storeID}, nil)
	require.NoError(t, err)

	require.Positive(t, rep.TotalRows, "row purge must not depend on object reaping")
	require.Positive(t, rep.Blobs.SkippedNotOurs,
		"a deployment with no bucket must report skips, not a clean zero")
	require.Zero(t, rep.Blobs.Deleted)
}
