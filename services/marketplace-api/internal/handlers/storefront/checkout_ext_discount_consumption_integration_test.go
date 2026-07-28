//go:build integration

package storefront_test

// This file proves the actual money bug at the HTTP layer: the storefront
// checkout used to commit the order FIRST (already discounted by a gift
// card / coupon) and consume the discount in a SEPARATE transaction
// afterwards. A failure in that second transaction left a real, committed
// order that nobody paid the discounted amount for.
//
// The fix moved coupon-apply / gift-card-debit / loyalty-redeem inside
// Service.Create's transaction via CreateInput.WithinTx (see
// internal/order/withintx_integration_test.go for the mechanism proof).
// These tests drive CheckoutExtHandler against a real Postgres and force
// the gift-card debit (and, separately, the coupon apply) to fail
// deterministically, then assert NO order row exists — against the
// pre-fix code this count was 1.

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/giftcard"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// discountConsumptionTruncateTables is the dependency-ordered truncate list
// for this file's tests. Covers everything checkoutTruncateTables does
// (checkout_integration_test.go) plus coupon and gift-card tables.
var discountConsumptionTruncateTables = []string{
	"coupon_usage",
	"coupons",
	"gift_card_transactions",
	"gift_cards",
	"order_events",
	"order_addresses",
	"order_items",
	"orders",
	"abandoned_carts",
	"outbox_events",
	"store_watermarks",
	"stores",
}

// insertNoCarrierCountry inserts a supported_countries row with EMPTY
// shipping_carriers so CheckoutExtHandler.calculateShipping short-circuits
// to a zero rate (see checkout_ext.go: "if len(sc.ShippingCarriers) == 0
// { return decimal.Zero, "", nil }") without needing a real carrier config.
// tax_strategy=flat with rate=0 similarly keeps calculateTax deterministic
// with zero setup. Registers cleanup to delete the row (this table is
// shared seed data — never truncated by tests).
func insertNoCarrierCountry(t *testing.T, db *gorm.DB, code string) {
	t.Helper()
	rate := 0.0
	err := db.Exec(`
		INSERT INTO supported_countries
			(country_code, name, currency_code, region, payment_providers, shipping_carriers, tax_strategy, tax_rate, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (country_code) DO UPDATE SET is_active = EXCLUDED.is_active`,
		code, "Test No-Carrier Country", "EUR", "test",
		pq.StringArray{}, pq.StringArray{}, "flat", rate, true,
	).Error
	require.NoError(t, err, "seed supported_countries row for %s", code)

	t.Cleanup(func() {
		db.Exec("DELETE FROM supported_countries WHERE country_code = ?", code)
	})
}

// seedDiscountConsumptionStore inserts a stores row pinned to countryCode
// (a code seeded by insertNoCarrierCountry) and EUR currency.
func seedDiscountConsumptionStore(t *testing.T, db *gorm.DB, countryCode string) *stores.Store {
	t.Helper()
	storeID := uuid.NewString()
	tenantID := uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "dc-" + storeID[:8],
		Name:         "Discount Consumption Test Store",
		CountryCode:  countryCode,
		CurrencyCode: "EUR",
		Timezone:     "Europe/Dublin",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	require.NoError(t, db.Create(s).Error, "seed store")
	return s
}

// debitFailGiftCardRepo delegates every giftcard.Repository method to the
// real gormRepository EXCEPT DebitInTx, which always fails. This is the
// cleanest deterministic trigger for a debit failure: the real DebitInTx
// cannot be made to fail while CheckBalance passes, since both read the
// same row (see task brief — "the real DebitInTx cannot be made to fail
// while CheckBalance passes").
type debitFailGiftCardRepo struct {
	giftcard.Repository
}

func (debitFailGiftCardRepo) DebitInTx(tx *gorm.DB, cardID uuid.UUID, amount decimal.Decimal, orderID uuid.UUID, tenantID uuid.UUID) (decimal.Decimal, error) {
	return decimal.Zero, errors.New("simulated gift card debit failure")
}

// incrementUsageFailCouponRepo mirrors debitFailGiftCardRepo for the
// coupon-apply path (Test 3): every method delegates to the real
// gormRepository except IncrementUsageInTx, which always fails. This is
// the atomic step CouponApplier.Apply calls right before recording usage,
// so failing it exercises the same "consumption failed mid-flight" shape
// as the gift card debit.
type incrementUsageFailCouponRepo struct {
	coupon.Repository
}

func (incrementUsageFailCouponRepo) IncrementUsageInTx(tx *gorm.DB, couponID uuid.UUID) error {
	return errors.New("simulated coupon usage increment failure")
}

// checkoutExtBody builds a minimal valid CheckoutExtRequest body for the
// no-carrier test store. giftCardCode / couponCode are optional overrides.
func checkoutExtBody(idempotencyKey string, giftCardCode, couponCode *string) map[string]any {
	body := map[string]any{
		"idempotency_key":  idempotencyKey,
		"customer_email":   "discount-consumption@example.com",
		"shipping_service": "standard",
		"payment_provider": "stripe", // no gateway config seeded — createPaymentIntent fails non-fatally, order still returns 201 on the happy path
		"items": []map[string]any{
			{
				"title_snapshot": "Widget",
				"sku_snapshot":   "WID-1",
				"unit_price":     "100.00",
				"quantity":       1,
				"line_total":     "100.00",
				"currency_code":  "EUR",
			},
		},
		"shipping_address": map[string]any{
			"name": "Buyer", "line1": "1 High St", "city": "Dublin", "country_code": "IE",
		},
		"subtotal": "100.00",
	}
	if giftCardCode != nil {
		body["gift_card_code"] = *giftCardCode
	}
	if couponCode != nil {
		body["coupon_code"] = *couponCode
	}
	return body
}

// TestStorefrontCheckout_GiftCardDebitFailure_RollsBackOrder is THE bug
// regression test. Against the pre-fix code (Step 4.6 debiting the gift
// card in a SEPARATE transaction after Service.Create had already
// committed) this test FAILS: the debit failure is reported to the buyer,
// but `orders` already has a row. Against the fix, the debit runs inside
// Service.Create's transaction via WithinTx, so a debit failure rolls the
// whole order back — orders count stays 0.
func TestStorefrontCheckout_GiftCardDebitFailure_RollsBackOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db := testdb.NewDB(t, discountConsumptionTruncateTables...)
	countryCode := "Z1"
	insertNoCarrierCountry(t, db, countryCode)
	store := seedDiscountConsumptionStore(t, db, countryCode)

	// Gift card service wired with a repo whose DebitInTx always fails.
	// Every other call (CheckBalance -> GetByCode, GetByCode) hits the
	// REAL repository, so the pre-tx lookups the handler does before
	// reaching WithinTx succeed exactly as they would in production.
	giftCardRepo := debitFailGiftCardRepo{giftcard.NewRepository()}
	giftCardSvc := giftcard.NewService(db, giftCardRepo, logger)

	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)

	checkoutExtHandler := storefront.NewCheckoutExtHandler(db, orderSvc, nil, giftCardSvc, crypto.NoopEncryptor{}, logger)

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		CheckoutExtHandler: checkoutExtHandler,
		SlugCache: stores.NewSlugCache(stores.NewRepository(db), fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute),
	})

	// Seed an active, funded gift card covering the whole order.
	storeUUID := uuid.MustParse(store.ID)
	tenantUUID := uuid.MustParse(store.TenantID)
	code, err := giftcard.GenerateCode()
	require.NoError(t, err)
	card := giftcard.GiftCard{
		TenantID:       tenantUUID,
		StoreID:        storeUUID,
		Code:           code,
		InitialBalance: decimal.NewFromInt(500),
		CurrentBalance: decimal.NewFromInt(500),
		CurrencyCode:   "EUR",
		Status:         giftcard.StatusActive,
	}
	require.NoError(t, db.Create(&card).Error)

	url := "/api/v1/storefront/stores/" + store.Slug + "/checkout"
	key := "gc-debit-fail-" + uuid.NewString()
	body := checkoutExtBody(key, &code, nil)

	w := doRequest(t, r, http.MethodPost, url, body)

	// The point of the fix: this must NOT be a 2xx. Report the exact body
	// so a red run shows what the pre-fix code actually returned (201 +
	// an order id) versus what the fix returns (an error status).
	if w.Code >= 200 && w.Code < 300 {
		t.Fatalf("checkout: got 2xx (%d) body=%s — a gift card debit failure must not succeed", w.Code, w.Body.String())
	}

	// THE assertion: no order exists for this store. Against the old
	// code this count is 1 (the order committed before the debit ran).
	var orderCount int64
	require.NoError(t, db.Table("orders").
		Where("store_id = ?", store.ID).Count(&orderCount).Error)
	require.Zero(t, orderCount,
		"orders count for store must be 0 after a gift-card-debit failure; got %d (status=%d body=%s)",
		orderCount, w.Code, w.Body.String())

	// No gift_card_transactions row leaked either — the stubbed DebitInTx
	// never touches the DB, but this also guards against a future repo
	// change reintroducing a partial write outside the failure path.
	var txnCount int64
	require.NoError(t, db.Table("gift_card_transactions").
		Where("gift_card_id = ?", card.ID).Count(&txnCount).Error)
	require.Zero(t, txnCount, "gift_card_transactions must not leak past the rollback")

	// The card balance itself must be untouched.
	var reloaded giftcard.GiftCard
	require.NoError(t, db.First(&reloaded, "id = ?", card.ID).Error)
	require.True(t, decimal.NewFromInt(500).Equal(reloaded.CurrentBalance),
		"gift card balance must be unchanged; got %s", reloaded.CurrentBalance.String())
}

// TestStorefrontCheckout_CouponApplyFailure_RollsBackOrder is Test 3 from
// the task brief: same shape as the gift-card test, but forcing the
// coupon-apply step (the former "Step 4.5") to fail instead. Proves that
// step got the identical treatment as the gift card debit.
func TestStorefrontCheckout_CouponApplyFailure_RollsBackOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db := testdb.NewDB(t, discountConsumptionTruncateTables...)
	countryCode := "Z2"
	insertNoCarrierCountry(t, db, countryCode)
	store := seedDiscountConsumptionStore(t, db, countryCode)

	// Coupon service wired with a repo whose IncrementUsageInTx always
	// fails. GetByCode / CountCustomerUsage hit the real repository, so
	// Validate() (the pre-tx preview) and the GetByCode inside Apply()
	// both succeed normally — only the atomic usage increment fails.
	couponRepo := incrementUsageFailCouponRepo{coupon.NewRepository()}
	couponSvc := coupon.NewService(coupon.ServiceConfig{DB: db, Repo: couponRepo, Logger: logger})

	orderRepo := order.NewRepository()
	outboxRepo := outbox.NewRepository(db)
	orderSvc := order.NewService(db, orderRepo, outboxRepo)

	checkoutExtHandler := storefront.NewCheckoutExtHandler(db, orderSvc, couponSvc, nil, crypto.NoopEncryptor{}, logger)

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		CheckoutExtHandler: checkoutExtHandler,
		SlugCache: stores.NewSlugCache(stores.NewRepository(db), fakeStoresClient{}, &singleflight.Group{}, 5*time.Minute),
	})

	storeUUID := uuid.MustParse(store.ID)
	tenantUUID := uuid.MustParse(store.TenantID)
	couponCode := "SAVE10-" + uuid.NewString()[:8]
	c := coupon.Coupon{
		TenantID:    tenantUUID,
		StoreID:     storeUUID,
		Code:        couponCode,
		Title:       "Test coupon",
		Type:        coupon.CouponTypeFixedAmount,
		Value:       decimal.NewFromInt(10),
		PerCustomer: 10,
		TargetType:  coupon.CouponTargetAll,
		Stackable:   true,
		StartsAt:    time.Now().Add(-1 * time.Hour),
		Status:      coupon.CouponStatusActive,
	}
	require.NoError(t, db.Create(&c).Error)

	url := "/api/v1/storefront/stores/" + store.Slug + "/checkout"
	key := "coupon-apply-fail-" + uuid.NewString()
	body := checkoutExtBody(key, nil, &couponCode)

	w := doRequest(t, r, http.MethodPost, url, body)

	if w.Code >= 200 && w.Code < 300 {
		t.Fatalf("checkout: got 2xx (%d) body=%s — a coupon apply failure must not succeed", w.Code, w.Body.String())
	}

	var orderCount int64
	require.NoError(t, db.Table("orders").
		Where("store_id = ?", store.ID).Count(&orderCount).Error)
	require.Zero(t, orderCount,
		"orders count for store must be 0 after a coupon-apply failure; got %d (status=%d body=%s)",
		orderCount, w.Code, w.Body.String())

	var usageCount int64
	require.NoError(t, db.Table("coupon_usage").
		Where("coupon_id = ?", c.ID).Count(&usageCount).Error)
	require.Zero(t, usageCount, "coupon_usage must not leak past the rollback")

	var reloaded coupon.Coupon
	require.NoError(t, db.First(&reloaded, "id = ?", c.ID).Error)
	require.Equal(t, 0, reloaded.UsageCount, "coupon usage_count must be unchanged")
}
