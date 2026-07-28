//go:build integration

package order_test

// This file proves the mechanism behind the money-bug fix described in
// internal/handlers/storefront/checkout_ext.go: CreateInput.WithinTx runs
// INSIDE Service.Create's transaction, after the order aggregate and its
// outbox row are written, and an error from it rolls the WHOLE order back
// — no orders / order_items / order_addresses / outbox_events row survives.
//
// Before the fix, coupon-apply / gift-card-debit / loyalty-redeem ran in
// SEPARATE transactions after Create had already committed, so a failure
// there left a committed order discounted by value that was never charged.
// These tests exercise the mechanism directly;
// internal/handlers/storefront/checkout_ext_discount_consumption_integration_test.go
// proves the actual bug through the HTTP handler.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// withinTxTruncateTables is the dependency-ordered truncate list shared by
// every test in this file.
var withinTxTruncateTables = []string{
	"order_events",
	"order_addresses",
	"order_items",
	"orders",
	"outbox_events",
	"store_watermarks",
}

// baseCreateInput builds a minimal, valid CreateInput for storeID/tenantID
// with a unique idempotency key. Callers set WithinTx and any per-test
// overrides on the returned value.
func baseCreateInput(storeID, tenantID uuid.UUID, idempotencyKey string) order.CreateInput {
	return order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "TST",
		OrderNumberSeq: 1,
		IdempotencyKey: idempotencyKey,
		CustomerEmail:  "withintx@example.com",
		Items: []order.OrderItem{
			{
				TitleSnapshot: "Widget",
				SKUSnapshot:   "WID-1",
				UnitPrice:     decimal.NewFromInt(100),
				Quantity:      1,
				LineTotal:     decimal.NewFromInt(100),
				CurrencyCode:  "EUR",
			},
		},
		Shipping:     order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Billing:      order.OrderAddress{Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE"},
		Subtotal:     decimal.NewFromInt(100),
		GrandTotal:   decimal.NewFromInt(100),
		CurrencyCode: "EUR",
		PlacedAt:     time.Now().UTC(),
	}
}

// TestOrderCreate_WithinTxError_RollsBackOrder is THE mechanism test: a
// WithinTx hook that returns an error must cause Create to return that
// error AND leave no trace of the order anywhere — not in orders, not in
// its line items, not in its addresses, not in the outbox. This is what
// makes the checkout fix correct: an order that never fully priced itself
// (because the gift card / coupon / loyalty consumption failed) must not
// exist at all, rather than existing at a discounted total nobody paid.
func TestOrderCreate_WithinTxError_RollsBackOrder(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, withinTxTruncateTables...)
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, storeID)

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	sentinel := errors.New("withintx: simulated gift card debit failure")
	key := "withintx-err-" + uuid.NewString()

	var capturedOrderID uuid.UUID
	in := baseCreateInput(storeID, tenantID, key)
	in.WithinTx = func(tx *gorm.DB, o *order.Order) error {
		require.NotEqual(t, uuid.Nil, o.ID, "order should have an ID assigned by the INSERT even though the tx will roll back")
		capturedOrderID = o.ID
		return sentinel
	}

	result, err := svc.Create(ctx, in)
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel, "Create must surface the WithinTx error unwrapped")
	require.Nil(t, result)
	require.NotEqual(t, uuid.Nil, capturedOrderID, "WithinTx must have been invoked with a persisted (pre-rollback) order")

	// No orders row for this idempotency key.
	var orderCount int64
	require.NoError(t, db.Table("orders").
		Where("idempotency_key = ?", key).Count(&orderCount).Error)
	require.Zero(t, orderCount, "orders row must not survive a WithinTx failure")

	// Nothing under the captured order ID either — covers the case where
	// the caller might look it up by ID instead of idempotency key.
	require.NoError(t, db.Table("orders").
		Where("id = ?", capturedOrderID).Count(&orderCount).Error)
	require.Zero(t, orderCount, "orders row for the captured id must not exist")

	var itemCount int64
	require.NoError(t, db.Table("order_items").
		Where("order_id = ?", capturedOrderID).Count(&itemCount).Error)
	require.Zero(t, itemCount, "order_items must not leak past the rollback")

	var addrCount int64
	require.NoError(t, db.Table("order_addresses").
		Where("order_id = ?", capturedOrderID).Count(&addrCount).Error)
	require.Zero(t, addrCount, "order_addresses must not leak past the rollback")

	var outboxCount int64
	require.NoError(t, db.Table("outbox_events").
		Where("aggregate_id = ?", capturedOrderID.String()).Count(&outboxCount).Error)
	require.Zero(t, outboxCount, "outbox_events (order.placed) must not leak past the rollback")
}

// TestOrderCreate_WithinTxSuccess_Persists is the mirror-image happy path:
// a WithinTx hook that succeeds must see the persisted order (non-zero ID)
// and the order must actually commit.
func TestOrderCreate_WithinTxSuccess_Persists(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, withinTxTruncateTables...)
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, storeID)

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	key := "withintx-ok-" + uuid.NewString()
	var sawID uuid.UUID
	in := baseCreateInput(storeID, tenantID, key)
	in.WithinTx = func(tx *gorm.DB, o *order.Order) error {
		sawID = o.ID
		return nil
	}

	result, err := svc.Create(ctx, in)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Reused)
	require.NotEqual(t, uuid.Nil, sawID, "WithinTx must see a non-zero order ID")
	require.Equal(t, result.Order.ID, sawID)

	var orderCount int64
	require.NoError(t, db.Table("orders").
		Where("id = ? AND idempotency_key = ?", sawID, key).Count(&orderCount).Error)
	require.Equal(t, int64(1), orderCount, "order must be persisted when WithinTx succeeds")
}

// TestOrderCreate_WithinTx_NotCalledOnReplay verifies the documented
// contract on CreateInput.WithinTx: the idempotent-replay path returns the
// existing aggregate BEFORE opening a transaction, so WithinTx must not run
// a second time — the discount was already consumed by the original call.
func TestOrderCreate_WithinTx_NotCalledOnReplay(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, withinTxTruncateTables...)
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, storeID)

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	key := "withintx-replay-" + uuid.NewString()
	firstCallCount := 0
	in := baseCreateInput(storeID, tenantID, key)
	in.WithinTx = func(tx *gorm.DB, o *order.Order) error {
		firstCallCount++
		return nil
	}

	first, err := svc.Create(ctx, in)
	require.NoError(t, err)
	require.False(t, first.Reused)
	require.Equal(t, 1, firstCallCount, "WithinTx must run exactly once on the original create")

	replayCallCount := 0
	replayIn := baseCreateInput(storeID, tenantID, key) // same idempotency key
	replayIn.WithinTx = func(tx *gorm.DB, o *order.Order) error {
		replayCallCount++
		return nil
	}

	replay, err := svc.Create(ctx, replayIn)
	require.NoError(t, err)
	require.True(t, replay.Reused)
	require.Equal(t, first.Order.ID, replay.Order.ID)
	require.Equal(t, 0, replayCallCount, "WithinTx must NOT run on the idempotent-replay path")
}
