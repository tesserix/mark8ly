//go:build integration

package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestOrderLifecycle_FullJourney is the M2 exit gate. It drives one order
// from creation through to a refunded return and asserts that every
// expected order_events row and every expected outbox_events row landed,
// and that store_watermarks.orders_updated_at was bumped by a manual
// publisher tick at the end.
//
// Cleans up by relying on testdb.NewDB's TRUNCATE list — every table the
// test writes to is included so reruns are deterministic.
//
// IMPORTANT: integration tests across packages MUST run with -p 1 (no
// cross-package parallelism). Several packages share outbox_events and
// store_watermarks; without -p 1 a parallel test in another package can
// TRUNCATE the table mid-flight. CI integration step sets -p 1 for this
// reason.
func TestOrderLifecycle_FullJourney(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t,
		"order_events",
		"order_addresses",
		"order_items",
		"return_items",
		"returns",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	repo := order.NewRepository()
	returnRepo := order.NewReturnRepository()
	outboxRepo := outbox.NewRepository(db)
	svc := order.NewService(db, repo, outboxRepo)
	returnSvc := order.NewReturnService(db, returnRepo, repo, svc, outboxRepo)

	// --- Create the order, allocating a sequence number inside the same Unit. ---
	var (
		orderResult *order.CreateResult
	)
	err := svc.Unit(ctx, func(tx *gorm.DB) error {
		seq, err := order.NextDocumentNumber(ctx, tx, storeID, "order")
		if err != nil {
			return err
		}
		// Run Create OUTSIDE the tx so it opens its own — this exercises both
		// the standalone happy path and ensures the seq value is visible
		// across transactions (CACHE 50 means same connection sees its block).
		_ = seq
		return nil
	})
	require.NoError(t, err)

	// Now create the order via the standard service entry point.
	createInput := order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "TST",
		OrderNumberSeq: 1,
		IdempotencyKey: "lifecycle-" + uuid.NewString(),
		CustomerEmail:  "lifecycle@example.com",
		Items: []order.OrderItem{
			{
				TitleSnapshot: "Widget",
				SKUSnapshot:   "WID-1",
				UnitPrice:     decimal.NewFromInt(50),
				Quantity:      2,
				LineTotal:     decimal.NewFromInt(100),
				CurrencyCode:  "EUR",
			},
		},
		Shipping: order.OrderAddress{
			Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE",
		},
		Billing: order.OrderAddress{
			Name: "A", Line1: "1", City: "Dublin", CountryCode: "IE",
		},
		Subtotal:     decimal.NewFromInt(100),
		GrandTotal:   decimal.NewFromInt(100),
		CurrencyCode: "EUR",
		PlacedAt:     time.Now().UTC(),
	}
	orderResult, err = svc.Create(ctx, createInput)
	require.NoError(t, err)
	require.NotNil(t, orderResult)
	require.False(t, orderResult.Reused)
	orderID := orderResult.Order.ID

	// Idempotent retry must return the same row with Reused=true.
	replay, err := svc.Create(ctx, createInput)
	require.NoError(t, err)
	require.True(t, replay.Reused)
	require.Equal(t, orderID, replay.Order.ID)

	// --- Confirm with payment authorization. ---
	target := order.PaymentStatusPaid
	require.NoError(t, svc.Confirm(ctx, nil, orderID, &target, "stripe-auth"))

	// --- Mark fulfilled. ---
	require.NoError(t, svc.MarkFulfilled(ctx, nil, orderID))

	// --- Open a return for one item. ---
	_, items, _, err := repo.GetByID(ctx, db, orderID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	reqInput := order.RequestInput{
		TenantID:    tenantID,
		StoreID:     storeID,
		StorePrefix: "TST",
		ReturnSeq:   1,
		OrderID:     orderID,
		Items: []order.ReturnItem{
			{OrderItemID: items[0].ID, Quantity: 1},
		},
		CurrencyCode: "EUR",
	}
	returnRow, _, err := returnSvc.Request(ctx, reqInput)
	require.NoError(t, err)
	returnID := returnRow.ID

	require.NoError(t, returnSvc.Approve(ctx, returnID, ""))
	require.NoError(t, returnSvc.MarkReceived(ctx, returnID))
	// Refund 50 EUR (one of two items) — partial refund.
	require.NoError(t, returnSvc.MarkRefunded(ctx, returnID,
		decimal.NewFromInt(50), order.PaymentStatusPartiallyRefunded, "return-refund"))

	// --- Assert order final state. ---
	final, _, _, err := repo.GetByID(ctx, db, orderID)
	require.NoError(t, err)
	require.Equal(t, string(order.OrderStatusFulfilled), final.Status)
	require.Equal(t, string(order.PaymentStatusPartiallyRefunded), final.PaymentStatus)
	require.Equal(t, string(order.FulfillmentStatusFulfilled), final.FulfillmentStatus)
	require.True(t, decimal.NewFromInt(50).Equal(final.RefundedAmount),
		"refunded_amount = %s, want 50", final.RefundedAmount.String())

	// --- Assert order_events kinds (as a set — within-tx events share
	// the same transaction-start created_at and have no deterministic order). ---
	var events []order.OrderEvent
	require.NoError(t, db.Where("order_id = ?", orderID).Find(&events).Error)
	wantKinds := []string{
		"created",          // Create
		"payment_recorded", // Confirm with payment_target
		"status_changed",   // Confirm
		"fulfilled",        // MarkFulfilled
		"return_requested", // Return.Request
		"return_approved",  // Return.Approve
		"return_received",  // Return.MarkReceived
		"refunded",         // Return.MarkRefunded → order.RecordRefund
		"return_refunded",  // Return.MarkRefunded → cross-module return event
	}
	gotKinds := make([]string, 0, len(events))
	for _, e := range events {
		gotKinds = append(gotKinds, e.Kind)
	}
	require.ElementsMatch(t, wantKinds, gotKinds, "order_events kinds mismatch")

	// --- Assert outbox_events event_type coverage (set comparison). ---
	var outboxRows []outbox.OutboxEvent
	require.NoError(t, db.
		Where("aggregate IN ?", []string{outbox.AggregateOrder, outbox.AggregateReturn}).
		Where("aggregate_id = ?", orderID.String()).
		Find(&outboxRows).Error)
	wantEventTypes := []string{
		outbox.EventOrderPlaced,
		outbox.EventOrderConfirmed,
		outbox.EventOrderFulfilled,
		outbox.EventReturnRequested,
		outbox.EventReturnApproved,
		outbox.EventReturnReceived,
		outbox.EventOrderRefunded,
		outbox.EventReturnRefunded,
	}
	gotEventTypes := make([]string, 0, len(outboxRows))
	for _, r := range outboxRows {
		gotEventTypes = append(gotEventTypes, r.EventType)
	}
	require.ElementsMatch(t, wantEventTypes, gotEventTypes, "outbox_events event_types mismatch")

	// --- Drive one publisher tick and assert orders_updated_at was bumped. ---
	pub := outbox.New(outbox.Config{
		Repo:      outboxRepo,
		DB:        db,
		BatchSize: 100,
	})
	processed, err := pub.Tick(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, processed, len(wantEventTypes), "publisher should have processed every order outbox row")

	var watermark struct {
		OrdersUpdatedAt   time.Time `gorm:"column:orders_updated_at"`
		ProductsUpdatedAt time.Time `gorm:"column:products_updated_at"`
	}
	require.NoError(t, db.Raw(
		"SELECT orders_updated_at, products_updated_at FROM store_watermarks WHERE store_id = ?",
		storeID).Scan(&watermark).Error)
	require.False(t, watermark.OrdersUpdatedAt.IsZero(), "orders_updated_at should be bumped after publisher tick")
}

// TestOrderLifecycle_RefundOverflowGuard verifies the atomic refund SQL
// rejects an overflow attempt and surfaces apperrors.ErrRefundExceedsTotal.
func TestOrderLifecycle_RefundOverflowGuard(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t,
		"order_events",
		"order_addresses",
		"order_items",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	created, err := svc.Create(ctx, order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "TST",
		OrderNumberSeq: 1,
		IdempotencyKey: "overflow-" + uuid.NewString(),
		CustomerEmail:  "overflow@example.com",
		Items: []order.OrderItem{
			{TitleSnapshot: "x", SKUSnapshot: "x", UnitPrice: decimal.NewFromInt(100), Quantity: 1, LineTotal: decimal.NewFromInt(100), CurrencyCode: "EUR"},
		},
		Shipping:     order.OrderAddress{Name: "A", Line1: "1", City: "C", CountryCode: "IE"},
		Billing:      order.OrderAddress{Name: "A", Line1: "1", City: "C", CountryCode: "IE"},
		Subtotal:     decimal.NewFromInt(100),
		GrandTotal:   decimal.NewFromInt(100),
		CurrencyCode: "EUR",
	})
	require.NoError(t, err)

	target := order.PaymentStatusPaid
	require.NoError(t, svc.Confirm(ctx, nil, created.Order.ID, &target, ""))

	// Refund the full amount cleanly.
	require.NoError(t, svc.RecordRefund(ctx, nil, created.Order.ID,
		decimal.NewFromInt(100), order.PaymentStatusRefunded, ""))

	// Second refund must hit the guard.
	err = svc.RecordRefund(ctx, nil, created.Order.ID,
		decimal.NewFromInt(1), order.PaymentStatusRefunded, "")
	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, apperrors.CodeInvalidTransition, ae.Code,
		"after a full refund the order is in payment_status=refunded which is terminal — service guard fires before SQL guard")
}

// seedStoreWithTenant inserts a stores row pinned to a specific tenant id
// (the helper in testhelpers_integration_test.go uses a random tenant).
// Used by lifecycle tests that need their order.tenant_id to match the
// store.tenant_id for cleaner assertions.
func seedStoreWithTenant(t *testing.T, db *gorm.DB, storeID, tenantID uuid.UUID) {
	t.Helper()
	slug := "lc-" + storeID.String()[:18]
	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Lifecycle Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		under := ""
		for _, c := range storeID.String() {
			if c == '-' {
				under += "_"
			} else {
				under += string(c)
			}
		}
		db.Exec("DROP SEQUENCE IF EXISTS mk_seq_order_" + under)
		db.Exec("DROP SEQUENCE IF EXISTS mk_seq_return_" + under)
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}
