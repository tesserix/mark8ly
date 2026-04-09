//go:build integration

package order_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// newTestOrderItem inserts a minimal valid OrderItem for the given order.
func newTestOrderItem(t *testing.T, db *gorm.DB, orderID uuid.UUID) order.OrderItem {
	t.Helper()
	item := order.OrderItem{
		OrderID:       orderID,
		TitleSnapshot: "Test item",
		SKUSnapshot:   "TEST-1",
		UnitPrice:     decimal.NewFromInt(50),
		Quantity:      1,
		LineTotal:     decimal.NewFromInt(50),
		CurrencyCode:  "EUR",
	}
	require.NoError(t, db.Create(&item).Error)
	return item
}

// TestOrder_Status_RefundedIsRejected asserts that 'refunded' is deliberately
// absent from the orders.status CHECK constraint. If this test ever fails, the
// spec's state machine orthogonality has been silently broken — see
// docs/superpowers/specs/2026-04-09-orders-feature-slice-1-design.md §2 #4.
func TestOrder_Status_RefundedIsRejected(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())

	err := db.Model(&o).Update("status", "refunded").Error
	require.Error(t, err, "orders.status must not accept 'refunded'")
}

func TestOrder_RefundedAmount_CannotExceedGrandTotal(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New()) // GrandTotal = 100

	// 101 > 100 must be rejected by the CHECK constraint.
	err := db.Model(&o).Update("refunded_amount", decimal.NewFromInt(101)).Error
	require.Error(t, err, "refunded_amount must not exceed grand_total")
}

func TestOrder_IdempotencyKey_UniquePerStore(t *testing.T) {
	db := testdb.NewTx(t)
	o1 := newTestOrder(t, db, uuid.New())

	// Fresh row, same store, same idempotency_key -> unique violation.
	dup := order.Order{
		TenantID:       o1.TenantID,
		StoreID:        o1.StoreID, // same store
		OrderNumber:    "M-TST-260409-dup00",
		IdempotencyKey: o1.IdempotencyKey, // same key
		CustomerEmail:  "other@example.com",
		Subtotal:       decimal.NewFromInt(10),
		GrandTotal:     decimal.NewFromInt(10),
		CurrencyCode:   "EUR",
		PlacedAt:       o1.PlacedAt,
	}
	err := db.Create(&dup).Error
	require.Error(t, err, "(store_id, idempotency_key) must be unique")
}

// TestOrder_HardDelete_BlockedByReturnItems proves the CASCADE->RESTRICT chain
// that makes hard-deleting an order with a linked return impossible. This is
// a foundational invariant — soft delete via orders.deleted_at is the only
// delete path in slice 1.
func TestOrder_HardDelete_BlockedByReturnItems(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())
	item := newTestOrderItem(t, db, o.ID)

	r := order.Return{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		OrderID:      o.ID,
		ReturnNumber: "R-TST-260409-" + uuid.NewString()[:5],
		CurrencyCode: "EUR",
	}
	require.NoError(t, db.Create(&r).Error)

	ri := order.ReturnItem{ReturnID: r.ID, OrderItemID: item.ID, Quantity: 1}
	require.NoError(t, db.Create(&ri).Error)

	// Hard-delete via Unscoped() must fail because return_items RESTRICTs
	// order_items which would cascade from orders.
	err := db.Unscoped().Delete(&o).Error
	require.Error(t, err, "expected RESTRICT from return_items chain")
}

func TestOrder_Status_InvalidValueRejected(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())

	err := db.Model(&o).Update("status", "wat").Error
	require.Error(t, err, "orders.status must reject unknown values")
}

func TestOrderAddress_InvalidKindRejected(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())

	addr := order.OrderAddress{
		OrderID:     o.ID,
		Kind:        "pickup", // not in ('shipping','billing')
		Name:        "x",
		Line1:       "1",
		City:        "c",
		CountryCode: "IE",
	}
	err := db.Create(&addr).Error
	require.Error(t, err, "order_addresses.kind must reject unknown values")
}

func TestReturn_InvalidStatusRejected(t *testing.T) {
	db := testdb.NewTx(t)
	o := newTestOrder(t, db, uuid.New())

	r := order.Return{
		TenantID:     o.TenantID,
		StoreID:      o.StoreID,
		OrderID:      o.ID,
		ReturnNumber: "R-TST-260409-badst",
		Status:       "pending", // not in ('requested','approved','received','refunded','rejected')
		CurrencyCode: "EUR",
	}
	err := db.Create(&r).Error
	require.Error(t, err, "returns.status must reject unknown values")
}
