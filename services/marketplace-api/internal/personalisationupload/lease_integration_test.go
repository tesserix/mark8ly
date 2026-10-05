//go:build integration

// Internal package: ExtendLease is on the unexported gormRepository and
// the thing worth asserting is what it does to expires_at in Postgres,
// which no fake can tell us.
package personalisationupload

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #966: carts outlive uploads, and that is the bug.
//
// An upload is swept 72 hours after it is created. A cart line sits in
// localStorage indefinitely. So a shopper who takes four days to decide
// comes back to a cart that renders perfectly, presses Pay, and is told
// their image is gone. Looking at the preview is the only evidence this
// service gets that a cart is still live, so a preview read renews the
// lease.

func leaseTestDB(t *testing.T) *gorm.DB {
	return testdb.NewDB(t,
		"personalisation_uploads",
		"product_personalisation_fields",
		"products",
		"stores",
	)
}

// seedUpload builds the whole chain an upload needs.
//
// Built out rather than stubbed with random uuids because
// personalisation_uploads.field_id references
// product_personalisation_fields, which itself has a COMPOSITE foreign
// key on (product_id, store_id) -> products (id, store_id). A random uuid
// aborts the transaction and poisons the rest of the suite.
func seedUpload(t *testing.T, db *gorm.DB, expiresAt time.Time, state string) (id, cartToken string) {
	t.Helper()
	storeID, tenantID := uuid.New(), uuid.New()
	slug := "lease-" + strings.ReplaceAll(storeID.String(), "-", "")[:16]
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status,
		                     storefront_customer_portal_secret)
		 VALUES (?, ?, ?, 'Lease Store', 'IE', 'EUR', 'Europe/Dublin', 'active',
		         encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug).Error)

	productID, fieldID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, handle, title, status, vendor_id)
		 VALUES (?, ?, ?, ?, 'Figurine', 'draft', ?)`,
		productID, tenantID, storeID, "lease-p-"+productID.String()[:8], uuid.New()).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO product_personalisation_fields
		   (id, tenant_id, store_id, product_id, key, label, kind, required, position, max_images, min_px)
		 VALUES (?, ?, ?, ?, 'photo', 'Your photo', 'image', true, 0, 1, 600)`,
		fieldID, tenantID, storeID, productID).Error)

	uploadID, cart := uuid.New(), uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO personalisation_uploads
		   (id, tenant_id, store_id, product_id, field_id, cart_token,
		    storage_key_original, content_hash, content_type, size_bytes,
		    original_filename, state, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'image/jpeg', 1024, 'nana.jpg', ?, ?)`,
		uploadID, tenantID, storeID, productID, fieldID, cart,
		"buyer-uploads/"+tenantID.String()+"/"+cart.String()+"/up.jpg",
		uuid.NewString(), state, expiresAt).Error)

	return uploadID.String(), cart.String()
}

func expiryOf(t *testing.T, db *gorm.DB, id string) time.Time {
	t.Helper()
	var out time.Time
	require.NoError(t, db.Raw(
		`SELECT expires_at FROM personalisation_uploads WHERE id = ?`, id).Scan(&out).Error)
	return out
}

func TestExtendLease_PushesExpiryOutByTheFullTTL(t *testing.T) {
	db := leaseTestDB(t)
	repo := NewRepository(db)

	// An upload with an hour left: the shopper has been deciding for 71
	// hours and is about to lose their photo.
	nearlyGone := time.Now().Add(1 * time.Hour)
	id, cart := seedUpload(t, db, nearlyGone, StateVerified)

	require.NoError(t, repo.ExtendLease(context.Background(), id, cart, TTL))

	got := expiryOf(t, db, id)
	require.True(t, got.After(time.Now().Add(TTL-time.Hour)),
		"expiry %s was not renewed to roughly now()+72h", got)
	require.True(t, got.After(nearlyGone.Add(time.Hour)),
		"expiry must move forward, not sideways")
}

func TestExtendLease_RefusesAnotherCartsUpload(t *testing.T) {
	// The cart token is the only authorisation on this surface. If a
	// renewal ignored it, one shopper could keep another's upload alive
	// — minor on its own, and the same hole that would let them read it.
	db := leaseTestDB(t)
	repo := NewRepository(db)

	original := time.Now().Add(2 * time.Hour)
	id, _ := seedUpload(t, db, original, StateVerified)

	require.NoError(t, repo.ExtendLease(context.Background(), id, uuid.NewString(), TTL),
		"a miss is not an error — the row may simply have been swept")

	got := expiryOf(t, db, id)
	require.WithinDuration(t, original, got, time.Second,
		"another cart's token must not renew this upload")
}

func TestExtendLease_LeavesAClaimedUploadAlone(t *testing.T) {
	// An order owns a claimed upload and the sweeper already ignores it.
	// Moving its expiry would imply the lease still means something.
	db := leaseTestDB(t)
	repo := NewRepository(db)

	original := time.Now().Add(2 * time.Hour)
	id, cart := seedUpload(t, db, original, StateClaimed)

	require.NoError(t, repo.ExtendLease(context.Background(), id, cart, TTL))

	got := expiryOf(t, db, id)
	require.WithinDuration(t, original, got, time.Second,
		"a claimed upload's expiry is not the lease any more")
}

func TestExtendLease_MissingRowIsNotAnError(t *testing.T) {
	// The row can be swept between the read that found it and this write.
	// The caller's job — handing the buyer a preview URL — does not depend
	// on the renewal landing.
	db := leaseTestDB(t)
	repo := NewRepository(db)
	require.NoError(t, repo.ExtendLease(
		context.Background(), uuid.NewString(), uuid.NewString(), TTL))
}

// The renewal has to beat the sweeper, which is the whole point: an
// upload renewed at hour 71 must not be collected at hour 72.
func TestExtendLease_SavesAnUploadFromTheSweeper(t *testing.T) {
	db := leaseTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	doomed := time.Now().Add(-1 * time.Minute) // already expired
	id, cart := seedUpload(t, db, doomed, StateVerified)

	require.NoError(t, repo.ExtendLease(ctx, id, cart, TTL))

	claimed, err := repo.ClaimExpired(ctx, time.Now(), 100)
	require.NoError(t, err)
	for _, u := range claimed {
		require.NotEqual(t, id, u.ID,
			"a renewed upload was still swept — the renewal did not take")
	}

	var alive int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM personalisation_uploads WHERE id = ?`, id).Scan(&alive).Error)
	require.Equal(t, int64(1), alive)
}
