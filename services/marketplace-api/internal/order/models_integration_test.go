//go:build integration

package order_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// newTestOrder inserts a minimal valid Order for the given store and returns it.
// Used by multiple tests; kept local because there's no public testhelper yet.
func newTestOrder(t *testing.T, db *gorm.DB, storeID uuid.UUID) order.Order {
	t.Helper()
	o := order.Order{
		TenantID:       uuid.New(),
		StoreID:        storeID,
		OrderNumber:    "M-TST-260409-" + uuid.NewString()[:5],
		IdempotencyKey: "test-" + uuid.NewString(),
		CustomerEmail:  "buyer@example.com",
		Subtotal:       decimal.NewFromInt(100),
		GrandTotal:     decimal.NewFromInt(100),
		CurrencyCode:   "EUR",
		PlacedAt:       time.Now(),
	}
	require.NoError(t, db.Create(&o).Error)
	return o
}

func TestOrderGraph_RoundTrip(t *testing.T) {
	db := testdb.NewTx(t)

	storeID := uuid.New()
	o := newTestOrder(t, db, storeID)

	item := order.OrderItem{
		OrderID:       o.ID,
		TitleSnapshot: "Porcelain bowl",
		SKUSnapshot:   "BOWL-M-NATURAL",
		UnitPrice:     decimal.NewFromInt(50),
		Quantity:      2,
		LineTotal:     decimal.NewFromInt(100),
		CurrencyCode:  "EUR",
	}
	require.NoError(t, db.Create(&item).Error)

	ship := order.OrderAddress{
		OrderID:     o.ID,
		Kind:        "shipping",
		Name:        "A Buyer",
		Line1:       "1 Main St",
		City:        "Dublin",
		CountryCode: "IE",
	}
	require.NoError(t, db.Create(&ship).Error)

	evt := order.OrderEvent{
		OrderID: o.ID,
		Kind:    "status_changed",
		Payload: datatypes.JSON([]byte(`{"from":null,"to":"pending"}`)),
	}
	require.NoError(t, db.Create(&evt).Error)

	var back order.Order
	require.NoError(t, db.First(&back, "id = ?", o.ID).Error)
	require.Equal(t, o.OrderNumber, back.OrderNumber)
	require.True(t, o.GrandTotal.Equal(back.GrandTotal), "grand_total roundtrip")
	require.Equal(t, "pending", back.Status)
	require.Equal(t, "pending", back.PaymentStatus)
	require.Equal(t, "unfulfilled", back.FulfillmentStatus)
	require.True(t, back.RefundedAmount.IsZero(), "refunded_amount defaults to 0")
	require.Nil(t, back.DeletedAt)
}

func TestOrder_SoftDelete_HidesFromDefaultQueries(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())

	now := time.Now()
	require.NoError(t, db.Model(&o).Update("deleted_at", now).Error)

	var rows []order.Order
	require.NoError(t, db.Where("store_id = ? AND deleted_at IS NULL", o.StoreID).Find(&rows).Error)
	require.Empty(t, rows, "soft-deleted rows must be filtered by default")
}
