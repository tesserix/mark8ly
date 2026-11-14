//go:build integration

package instore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type noTax struct{}

func (noTax) QuoteCounterTax(context.Context, *stores.Store, *country.SupportedCountry, []tax.TaxableItem) (*tax.TaxBreakdown, error) {
	return &tax.TaxBreakdown{TaxTotal: decimal.Zero}, nil
}

type fixture struct {
	db    *gorm.DB
	svc   *instore.Service
	store *stores.Store
	tid   uuid.UUID
	sid   uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.NewTx(t)
	tid, sid := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tid, sid)
	require.NoError(t, db.Exec(`UPDATE stores SET country_code = 'AU', currency_code = 'AUD' WHERE id = ?`, sid).Error)
	var st stores.Store
	require.NoError(t, db.First(&st, "id = ?", sid).Error)
	orders := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))
	return &fixture{db: db, svc: instore.NewService(db, orders, stockhold.NewRepository(), noTax{}), store: &st, tid: tid, sid: sid}
}

// variant seeds an active, published product with one variant at price and
// stock qty at the store's warehouse. policy is "deny" or "continue".
func (f *fixture) variant(t *testing.T, price string, qty int, policy string) string {
	t.Helper()
	pid, vid := uuid.NewString(), uuid.NewString()
	require.NoError(t, f.db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, 'BMW M4 Laptop Skin', ?, 'active', ?, now())`,
		pid, f.tid, f.sid, "p-"+pid[:8], testdb.SeedVendor(t, f.db, f.tid)).Error)
	require.NoError(t, f.db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, currency_code, inventory_policy)
		 VALUES (?, ?, ?, ?, ?, 'AUD', ?)`,
		vid, pid, f.sid, "SKU-"+vid[:8], price, policy).Error)
	var wh string
	if err := f.db.Raw(`SELECT id::text FROM warehouses WHERE store_id = ? LIMIT 1`, f.sid).Row().Scan(&wh); err != nil || wh == "" {
		wh = uuid.NewString()
		require.NoError(t, f.db.Exec(
			`INSERT INTO warehouses (id, tenant_id, store_id, name, line1, city, region, postal_code, country_code, phone, is_default)
			 VALUES (?, ?, ?, 'Shop', '1 Main St', 'Sydney', 'NSW', '2000', 'AU', '+61200000000', true)`,
			wh, f.tid, f.sid).Error)
	}
	if qty > 0 {
		require.NoError(t, f.db.Exec(
			`INSERT INTO variant_stock (variant_id, location_id, quantity, updated_at) VALUES (?, ?, ?, now())`,
			vid, wh, qty).Error)
	}
	return vid
}

func (f *fixture) stock(t *testing.T, vid string) int {
	t.Helper()
	var q int
	require.NoError(t, f.db.Raw(`SELECT COALESCE(SUM(quantity), 0) FROM variant_stock WHERE variant_id = ?`, vid).Row().Scan(&q))
	return q
}

func (f *fixture) sale(lines ...instore.SaleLine) instore.SaleInput {
	return instore.SaleInput{
		TenantID: f.tid, StoreID: f.sid, Store: f.store,
		IdempotencyKey: uuid.NewString(), Lines: lines,
		PaymentMethod: instore.PaymentCard, SoldByUserID: "cashier-1", Fulfilled: true,
	}
}

func line(vid string, qty int) instore.SaleLine {
	return instore.SaleLine{VariantID: &vid, Quantity: qty}
}

func TestIntegration_InStore_PaidAndStockDecremented(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 5, "deny")

	res, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 2)))
	require.NoError(t, err)

	var o order.Order
	require.NoError(t, f.db.First(&o, "id = ?", res.Order.ID).Error)
	require.Equal(t, string(order.PaymentStatusPaid), o.PaymentStatus)
	require.Equal(t, string(order.FulfillmentStatusFulfilled), o.FulfillmentStatus)
	require.Equal(t, order.ChannelInStore, o.Channel)
	require.True(t, o.GrandTotal.Equal(decimal.RequireFromString("40.00")), "grand total %s", o.GrandTotal)
	require.Equal(t, 3, f.stock(t, vid))
}

func TestIntegration_InStore_SameVariantTwoLines(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 3, "deny")

	in := f.sale(line(vid, 2), line(vid, 2))
	_, err := f.svc.RecordSale(context.Background(), in)

	var oos checkoutcore.OutOfStockError
	require.True(t, errors.As(err, &oos), "err = %v, want OutOfStockError", err)
	require.Equal(t, vid, oos.VariantID)
	var n int64
	require.NoError(t, f.db.Table("orders").Where("idempotency_key = ?", in.IdempotencyKey).Count(&n).Error)
	require.Zero(t, n)
	require.Equal(t, 3, f.stock(t, vid))
}

func TestIntegration_InStore_RetryIsIdempotent(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 5, "deny")
	in := f.sale(line(vid, 1))

	first, err := f.svc.RecordSale(context.Background(), in)
	require.NoError(t, err)
	again, err := f.svc.RecordSale(context.Background(), in)
	require.NoError(t, err)

	require.True(t, again.Reused)
	require.Equal(t, first.Order.ID, again.Order.ID)
	require.Equal(t, 4, f.stock(t, vid))
}

func TestIntegration_InStore_NoEmail(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 5, "deny")

	res, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 1)))
	require.NoError(t, err)
	var email string
	require.NoError(t, f.db.Raw(`SELECT customer_email FROM orders WHERE id = ?`, res.Order.ID).Row().Scan(&email))
	require.Equal(t, "", email)
}

func TestIntegration_InStore_CatalogPriceUsed(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "599.00", 5, "deny")

	res, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 1)))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.True(t, res.Items[0].UnitPrice.Equal(decimal.RequireFromString("599.00")))
	require.Equal(t, "BMW M4 Laptop Skin", res.Items[0].TitleSnapshot)
}

func TestIntegration_InStore_CustomLineNoStock(t *testing.T) {
	f := newFixture(t)
	title, price := "Two for $30", decimal.RequireFromString("30.00")
	in := f.sale(instore.SaleLine{CustomTitle: &title, CustomPrice: &price, Quantity: 1})
	in.CanDiscount = true
	in.PaymentMethod = instore.PaymentCash
	tendered := decimal.RequireFromString("50.00")
	in.CashTendered = &tendered

	res, err := f.svc.RecordSale(context.Background(), in)
	require.NoError(t, err)
	require.Nil(t, res.Items[0].VariantID)
	require.Nil(t, res.Items[0].ProductID)
	require.Equal(t, "Two for $30", res.Items[0].TitleSnapshot)
}

func TestIntegration_InStore_ContinuePolicyAtZero(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 0, "continue")

	_, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 1)))
	require.NoError(t, err)
}

func TestIntegration_InStore_DraftProductRefused(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 5, "deny")
	require.NoError(t, f.db.Exec(
		`UPDATE products SET status = 'draft' WHERE id = (SELECT product_id FROM product_variants WHERE id = ?)`, vid).Error)

	_, err := f.svc.RecordSale(context.Background(), f.sale(line(vid, 1)))
	require.Error(t, err)
	require.Equal(t, 5, f.stock(t, vid))
}

func TestIntegration_InStore_MadeToOrderStaysUnfulfilled(t *testing.T) {
	f := newFixture(t)
	vid := f.variant(t, "20.00", 5, "deny")
	in := f.sale(line(vid, 1))
	in.Fulfilled = false

	res, err := f.svc.RecordSale(context.Background(), in)
	require.NoError(t, err)
	var o order.Order
	require.NoError(t, f.db.First(&o, "id = ?", res.Order.ID).Error)
	require.Equal(t, string(order.PaymentStatusPaid), o.PaymentStatus)
	require.Equal(t, string(order.FulfillmentStatusUnfulfilled), o.FulfillmentStatus)
}
