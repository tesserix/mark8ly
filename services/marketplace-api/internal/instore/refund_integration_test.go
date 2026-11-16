//go:build integration

package instore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/order"
)

func (f *fixture) refunds() *instore.RefundService { return instore.NewRefundService(f.db) }

// saleOf rings up qty of a fresh variant at price and returns the order and its line.
func (f *fixture) saleOf(t *testing.T, price string, stock, qty int, policy string) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	vid := f.variant(t, price, stock, policy)
	res, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, qty)))
	require.NoError(t, err)
	var itemID uuid.UUID
	require.NoError(t, f.db.Raw(`SELECT id FROM order_items WHERE order_id = ?`, res.Order.ID).Row().Scan(&itemID))
	return res.Order.ID, itemID, vid
}

func (f *fixture) refundIn(oid uuid.UUID, items ...instore.RefundItem) instore.RefundInput {
	return instore.RefundInput{StoreID: f.sid, OrderID: oid, RequestID: uuid.NewString(),
		Method: instore.PaymentCash, Reason: "changed mind", RefundedBy: "owner-1", Items: items}
}

func TestOutsideRefund_FullRestocksAndMarksRefunded(t *testing.T) {
	f := newFixture(t)
	oid, item, vid := f.saleOf(t, "20.00", 5, 2, "deny")
	require.Equal(t, 3, f.stock(t, vid))

	res, err := f.refunds().Record(context.Background(), f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 2}))
	require.NoError(t, err)
	require.Equal(t, string(order.PaymentStatusRefunded), res.PaymentStatus)
	require.True(t, res.RefundedAmount.Equal(decimal.RequireFromString("40")), "refunded %s", res.RefundedAmount)
	require.Equal(t, 5, f.stock(t, vid))
}

func TestOutsideRefund_PartialKeepsPartiallyRefunded(t *testing.T) {
	f := newFixture(t)
	oid, _, vid := f.saleOf(t, "20.00", 5, 2, "deny")
	in := f.refundIn(oid)
	amt := decimal.RequireFromString("15")
	in.Amount = &amt
	res, err := f.refunds().Record(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, string(order.PaymentStatusPartiallyRefunded), res.PaymentStatus)
	require.Equal(t, 3, f.stock(t, vid), "no items chosen, nothing restocked")
}

func TestOutsideRefund_RetryIsIdempotent(t *testing.T) {
	f := newFixture(t)
	oid, item, vid := f.saleOf(t, "20.00", 5, 2, "deny")
	in := f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 1})
	amt := decimal.RequireFromString("20")
	in.Amount = &amt
	_, err := f.refunds().Record(context.Background(), in)
	require.NoError(t, err)
	res, err := f.refunds().Record(context.Background(), in)
	require.NoError(t, err)
	require.True(t, res.Reused)
	require.True(t, res.RefundedAmount.Equal(decimal.RequireFromString("20")))
	require.Equal(t, 4, f.stock(t, vid))
}

func TestOutsideRefund_Bounds(t *testing.T) {
	f := newFixture(t)
	oid, item, vid := f.saleOf(t, "20.00", 5, 2, "deny")

	tooMuch := decimal.RequireFromString("40.01")
	in := f.refundIn(oid)
	in.Amount = &tooMuch
	_, err := f.refunds().Record(context.Background(), in)
	require.Error(t, err)

	_, err = f.refunds().Record(context.Background(), f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 3}))
	require.Error(t, err)

	half := decimal.RequireFromString("20")
	in = f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 2})
	in.Amount = &half
	_, err = f.refunds().Record(context.Background(), in)
	require.NoError(t, err)
	_, err = f.refunds().Record(context.Background(), f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 1}))
	require.Error(t, err, "both units already back in stock")
	require.Equal(t, 5, f.stock(t, vid))

	var o order.Order
	require.NoError(t, f.db.First(&o, "id = ?", oid).Error)
	require.True(t, o.RefundedAmount.Equal(half), "refused attempts wrote nothing: %s", o.RefundedAmount)
}

func TestOutsideRefund_OnlineRefused(t *testing.T) {
	f := newFixture(t)
	oid, _, _ := f.saleOf(t, "20.00", 5, 1, "deny")
	require.NoError(t, f.db.Exec(`UPDATE orders SET channel = 'online' WHERE id = ?`, oid).Error)
	_, err := f.refunds().Record(context.Background(), f.refundIn(oid))
	require.Error(t, err)
}

func TestOutsideRefund_RestockWithoutStockRow(t *testing.T) {
	f := newFixture(t)
	oid, item, vid := f.saleOf(t, "20.00", 0, 1, "continue")
	_, err := f.refunds().Record(context.Background(), f.refundIn(oid, instore.RefundItem{OrderItemID: item, Quantity: 1}))
	require.NoError(t, err)
	require.Equal(t, 1, f.stock(t, vid))
}

// A sale that took one variant from two locations puts each unit back
// where it came from, across more than one refund.
func TestOutsideRefund_RestocksEachLocationItCameFrom(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 2, "deny") // 2 at the store's first warehouse
	second := uuid.NewString()
	require.NoError(t, f.db.Exec(
		`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone, is_default)
		 VALUES (?, ?, ?, 'Stall', '2 Market St', 'Sydney', 'NSW', '2000', 'AU', '+61200000001', false)`,
		second, f.tid, f.sid).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES (?, ?, 3, now())`, vid, second).Error)
	perLocation := func() map[string]int {
		var rows []struct {
			LocationID string
			Quantity   int
		}
		require.NoError(t, f.db.Raw(`SELECT location_id::text, quantity FROM variant_stock WHERE variant_id = ?`, vid).Scan(&rows).Error)
		out := map[string]int{}
		for _, r := range rows {
			out[r.LocationID] = r.Quantity
		}
		return out
	}
	before := perLocation()

	res, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 5)))
	require.NoError(t, err)
	var item uuid.UUID
	require.NoError(t, f.db.Raw(`SELECT id FROM order_items WHERE order_id = ?`, res.Order.ID).Row().Scan(&item))
	for _, q := range perLocation() {
		require.Zero(t, q, "the sale drew from both locations")
	}

	in := f.refundIn(res.Order.ID, instore.RefundItem{OrderItemID: item, Quantity: 1})
	amt := decimal.RequireFromString("20")
	in.Amount = &amt
	_, err = f.refunds().Record(context.Background(), in)
	require.NoError(t, err)
	_, err = f.refunds().Record(context.Background(), f.refundIn(res.Order.ID, instore.RefundItem{OrderItemID: item, Quantity: 4}))
	require.NoError(t, err)

	require.Equal(t, before, perLocation())
}
