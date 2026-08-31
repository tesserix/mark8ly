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

// newPartialFulfilmentOrder seeds a store + confirmed order the same way
// TestOrderLifecycle_FullJourney does, and returns its id. Cribbed from
// that test's fixture shape (see lifecycle_integration_test.go) rather
// than duplicated wholesale — it reuses seedStoreWithTenant, which is
// package-level in this _test package.
func newPartialFulfilmentOrder(t *testing.T, db *gorm.DB, svc *order.Service) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	storeID := uuid.New()
	tenantID := uuid.New()
	seedStoreWithTenant(t, db, storeID, tenantID)

	createInput := order.CreateInput{
		TenantID:       tenantID,
		StoreID:        storeID,
		StorePrefix:    "PFT",
		OrderNumberSeq: 1,
		IdempotencyKey: "partial-fulfilment-" + uuid.NewString(),
		CustomerEmail:  "partial@example.com",
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
	result, err := svc.Create(ctx, createInput)
	require.NoError(t, err)

	target := order.PaymentStatusPaid
	require.NoError(t, svc.Confirm(ctx, nil, result.Order.ID, &target, "stripe-auth"))

	return result.Order.ID
}

// TestMarkPartiallyFulfilled_SetsPartial: an unfulfilled order transitions
// to partial, and orders.status is left untouched — partial shipment does
// not fulfil the order.
func TestMarkPartiallyFulfilled_SetsPartial(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t,
		"order_events",
		"order_addresses",
		"order_items",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
	repo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	svc := order.NewService(db, repo, outboxRepo)

	orderID := newPartialFulfilmentOrder(t, db, svc)

	require.NoError(t, svc.MarkPartiallyFulfilled(ctx, nil, orderID))

	final, _, _, err := repo.GetByID(ctx, db, orderID)
	require.NoError(t, err)
	require.Equal(t, string(order.FulfillmentStatusPartial), final.FulfillmentStatus)
	require.Equal(t, string(order.OrderStatusConfirmed), final.Status,
		"MarkPartiallyFulfilled must not touch orders.status")

	var events []order.OrderEvent
	require.NoError(t, db.Where("order_id = ? AND kind = ?", orderID, string(order.EventKindPartiallyFulfilled)).
		Find(&events).Error)
	require.Len(t, events, 1, "exactly one partially_fulfilled event must be recorded")
}

// TestMarkPartiallyFulfilled_RefusesFromFulfilled: fulfilled -> partial is
// not a legal transition. A completed order must never be downgraded.
func TestMarkPartiallyFulfilled_RefusesFromFulfilled(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t,
		"order_events",
		"order_addresses",
		"order_items",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
	repo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	svc := order.NewService(db, repo, outboxRepo)

	orderID := newPartialFulfilmentOrder(t, db, svc)
	require.NoError(t, svc.MarkFulfilled(ctx, nil, orderID))

	err := svc.MarkPartiallyFulfilled(ctx, nil, orderID)
	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, apperrors.CodeInvalidTransition, ae.Code,
		"fulfilled -> partial must be refused, not silently applied")

	final, _, _, err := repo.GetByID(ctx, db, orderID)
	require.NoError(t, err)
	require.Equal(t, string(order.FulfillmentStatusFulfilled), final.FulfillmentStatus,
		"a completed order must not be downgraded to partial")
	require.Equal(t, string(order.OrderStatusFulfilled), final.Status)
}

// TestMarkPartiallyFulfilled_IsIdempotent: calling it twice on an
// already-partial order does not error — a retried shipment-creation call
// reporting "still owes a parcel" a second time is not an illegal
// transition.
func TestMarkPartiallyFulfilled_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t,
		"order_events",
		"order_addresses",
		"order_items",
		"orders",
		"outbox_events",
		"store_watermarks",
	)
	repo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	svc := order.NewService(db, repo, outboxRepo)

	orderID := newPartialFulfilmentOrder(t, db, svc)

	require.NoError(t, svc.MarkPartiallyFulfilled(ctx, nil, orderID))
	require.NoError(t, svc.MarkPartiallyFulfilled(ctx, nil, orderID))

	final, _, _, err := repo.GetByID(ctx, db, orderID)
	require.NoError(t, err)
	require.Equal(t, string(order.FulfillmentStatusPartial), final.FulfillmentStatus)
}
