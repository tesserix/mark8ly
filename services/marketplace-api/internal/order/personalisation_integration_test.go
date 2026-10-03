//go:build integration

package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// The merchant-read side of #968. What these tests are actually defending:
//
//  1. a store id in the URL cannot be used to read another merchant's
//     artwork, which is the only thing standing between two tenants and
//     each other's customers' photographs; and
//  2. the ORIGINAL and the PREVIEW stay distinct all the way out of the
//     database. They are two nullable text columns one line apart, and
//     conflating them ships a blurry figurine to someone who paid for a
//     sharp one — a bug that is invisible in code review and obvious only
//     to the customer holding the product.

func newPersonalisationDB(t *testing.T) *gorm.DB {
	return testdb.NewDB(t,
		"order_item_personalisations",
		"order_items",
		"order_addresses",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
}

// seedOrderWithPersonalisation creates one order carrying one image answer
// and one text answer, and returns the order id plus the two answer ids.
func seedOrderWithPersonalisation(
	t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID, seq int64,
) (orderID, imageID, textID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	res, err := svc.Create(ctx, order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "TST",
		OrderNumberSeq: seq,
		IdempotencyKey: "pz-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Items: []order.OrderItem{{
			TitleSnapshot: "Custom figurine",
			SKUSnapshot:   "FIG-1",
			UnitPrice:     decimal.NewFromInt(80),
			Quantity:      1,
			LineTotal:     decimal.NewFromInt(80),
			CurrencyCode:  "EUR",
		}},
		Shipping:     order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Billing:      order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Subtotal:     decimal.NewFromInt(80),
		GrandTotal:   decimal.NewFromInt(80),
		CurrencyCode: "EUR",
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)

	orderID = res.Order.ID
	itemID := res.Items[0].ID
	imageID = uuid.New()
	textID = uuid.New()

	require.NoError(t, db.Exec(`
		INSERT INTO order_item_personalisations
			(id, order_item_id, field_key, field_label, kind, price_delta,
			 storage_key_original, storage_key, content_type, size_bytes,
			 original_filename, crop, position)
		VALUES (?, ?, 'photo', 'Your photo', 'image', 10.00,
			 'buyer-uploads/original.jpg', 'buyer-uploads/preview.jpg',
			 'image/jpeg', 204800, 'nana.jpg',
			 '{"x":10,"y":20,"width":400,"height":400}'::jsonb, 0)`,
		imageID, itemID).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO order_item_personalisations
			(id, order_item_id, field_key, field_label, kind, text_value,
			 price_delta, position)
		VALUES (?, ?, 'name', 'Name to engrave', 'text', 'Asha', 0.00, 1)`,
		textID, itemID).Error)

	return orderID, imageID, textID
}

func TestListPersonalisationsForOrder_ReturnsBothAnswersInOrder(t *testing.T) {
	ctx := context.Background()
	db := newPersonalisationDB(t)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	orderID, imageID, textID := seedOrderWithPersonalisation(t, db, storeID, tenantID, 1)

	rows, err := order.ListPersonalisationsForOrder(ctx, db, orderID)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Ordered by position, so the merchant reads the brief in the order
	// the form asked for it rather than in whatever order Postgres
	// happened to return.
	require.Equal(t, imageID, rows[0].ID)
	require.Equal(t, textID, rows[1].ID)

	img := rows[0]
	require.True(t, img.HasArtwork())
	// The two keys must not have collapsed into one another.
	require.NotNil(t, img.StorageKeyOriginal)
	require.NotNil(t, img.StorageKey)
	require.Equal(t, "buyer-uploads/original.jpg", *img.StorageKeyOriginal)
	require.Equal(t, "buyer-uploads/preview.jpg", *img.StorageKey)
	require.NotEqual(t, *img.StorageKeyOriginal, *img.StorageKey)
	require.NotNil(t, img.Crop)
	require.JSONEq(t, `{"x":10,"y":20,"width":400,"height":400}`, string(*img.Crop))
	require.Equal(t, "10", img.PriceDelta.StringFixed(0))

	txt := rows[1]
	require.False(t, txt.HasArtwork(), "a text answer must never look downloadable")
	require.NotNil(t, txt.TextValue)
	require.Equal(t, "Asha", *txt.TextValue)
}

func TestListPersonalisationsForOrder_IgnoresOtherOrders(t *testing.T) {
	ctx := context.Background()
	db := newPersonalisationDB(t)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	mine, _, _ := seedOrderWithPersonalisation(t, db, storeID, tenantID, 1)
	_, theirImage, _ := seedOrderWithPersonalisation(t, db, storeID, tenantID, 2)

	rows, err := order.ListPersonalisationsForOrder(ctx, db, mine)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.NotEqual(t, theirImage, r.ID,
			"the join leaked an answer belonging to a different order")
	}
}

func TestListPersonalisationsForOrder_EmptyForOrdinaryOrder(t *testing.T) {
	ctx := context.Background()
	db := newPersonalisationDB(t)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	res, err := svc.Create(ctx, order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "TST",
		OrderNumberSeq: 7,
		IdempotencyKey: "plain-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Items: []order.OrderItem{{
			TitleSnapshot: "Mug", SKUSnapshot: "MUG-1",
			UnitPrice: decimal.NewFromInt(10), Quantity: 1,
			LineTotal: decimal.NewFromInt(10), CurrencyCode: "EUR",
		}},
		Shipping:     order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Billing:      order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Subtotal:     decimal.NewFromInt(10),
		GrandTotal:   decimal.NewFromInt(10),
		CurrencyCode: "EUR",
	})
	require.NoError(t, err)

	rows, err := order.ListPersonalisationsForOrder(ctx, db, res.Order.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// The cross-tenant case. Not "a 404 is nicer than a 403" — the merchant
// who owns store B must not be able to turn an order id they learned from
// anywhere into a signed URL for store A's customer's photograph.
func TestGetPersonalisationForOrder_RefusesAnotherStoresOrder(t *testing.T) {
	ctx := context.Background()
	db := newPersonalisationDB(t)

	storeA, tenantA := uuid.New(), uuid.New()
	storeB, tenantB := uuid.New(), uuid.New()
	seedStoreWithTenant(t, db, storeA, tenantA)
	seedStoreWithTenant(t, db, storeB, tenantB)

	orderA, imageA, _ := seedOrderWithPersonalisation(t, db, storeA, tenantA, 1)

	// The owner can read it.
	got, err := order.GetPersonalisationForOrder(ctx, db, storeA, orderA, imageA)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, imageA, got.ID)

	// Store B, holding the real order id AND the real answer id, cannot.
	_, err = order.GetPersonalisationForOrder(ctx, db, storeB, orderA, imageA)
	require.Error(t, err, "store B read store A's buyer artwork")
}

func TestGetPersonalisationForOrder_RefusesMismatchedOrder(t *testing.T) {
	ctx := context.Background()
	db := newPersonalisationDB(t)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	orderOne, imageOne, _ := seedOrderWithPersonalisation(t, db, storeID, tenantID, 1)
	orderTwo, _, _ := seedOrderWithPersonalisation(t, db, storeID, tenantID, 2)

	// Same store, right answer id, wrong order id. Must still fail:
	// the order id is what the audit row records, so a mismatch that
	// succeeded would file the access under the wrong order.
	_, err := order.GetPersonalisationForOrder(ctx, db, storeID, orderTwo, imageOne)
	require.Error(t, err)

	_, err = order.GetPersonalisationForOrder(ctx, db, storeID, orderOne, imageOne)
	require.NoError(t, err)
}
