//go:build integration

// Spec criterion #40: 50% off ₹999 Starter → rejected with below_absolute_floor
// in the audit event, and uniform promo_invalid_or_expired in the HTTP response.
package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ptrOf is a local helper for the nullable promo_codes columns (#726).
func ptrOf[T any](v T) *T { return &v }

func promoApplyURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/subscription/apply-promo"
}

func seedPromoCode(t *testing.T, env *testEnv, code string, discountBps int) uuid.UUID {
	t.Helper()
	repo := promo.NewRepository()
	pc := &promo.PromoCode{
		Code:           code,
		StripeCouponID: ptrOf("co_test_" + code),
		DiscountType:   ptrOf(promo.DiscountTypePercentage),
		DiscountValue:  ptrOf(discountBps),
		MaxPerEmail:    3,
		ValidFrom:      time.Now().UTC().Add(-24 * time.Hour),
		CreatedBy:      "test",
	}
	if err := repo.Create(context.Background(), env.db, pc); err != nil {
		t.Fatalf("seed promo code: %v", err)
	}
	return pc.ID
}

// seedSubscriptionForPromoTest seeds a minimal store_subscriptions row for the
// given store with the specified plan and billing currency (INR for spec #40).
func seedSubscriptionForPromoTest(t *testing.T, db *gorm.DB, storeID, tenantID string, plan, currency string) {
	t.Helper()
	sid, _ := uuid.Parse(storeID)
	tid, _ := uuid.Parse(tenantID)
	inr := currency
	// The billing email is what the per-email redemption cap is counted
	// against. The handler reads it from this row, not from the request body
	// — see PromoHandler.merchantEmail.
	email := "merchant@example.com"
	sub := &subscription.StoreSubscription{
		TenantID:         tid,
		StoreID:          sid,
		StripeCustomerID: "cus_test_" + storeID[:8],
		Plan:             subscription.SubscriptionPlan(plan),
		Status:           subscription.StatusActive,
		BillingCurrency:  &inr,
		Email:            &email,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
}

// TestPromoApply_BelowAbsoluteFloor_INR is spec criterion #40.
// A 50% discount on a ₹999 Starter subscription falls below the ₹800 absolute
// floor (effective = ₹499.50 = 49950 paise < 80000 paise floor).
// The HTTP response must be 422 with error="promo_invalid_or_expired".
func TestPromoApply_BelowAbsoluteFloor_INR(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	// Seed a 50% off promo code.
	seedPromoCode(t, env, "HALFOFF123456", 5000)

	// Seed a subscription row for the store with INR currency and Starter plan.
	seedSubscriptionForPromoTest(t, env.db, storeID, tenantID, "starter", "inr")

	body := map[string]any{"code": "HALFOFF123456"}
	w := request(t, env.router, http.MethodPost, promoApplyURL(storeID), body, authHeaders(userID, tenantID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("spec criterion #40: expected 422, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["error"] != "promo_invalid_or_expired" {
		t.Errorf("spec criterion #40: expected error=promo_invalid_or_expired, got %v", resp["error"])
	}
	// #770: the reason must reach the merchant. "invalid or has expired" is
	// the wrong sentence here — the code is real and the merchant did nothing
	// wrong; the discount simply cannot go below the plan's floor.
	if resp["reason"] != "below_absolute_floor" {
		t.Errorf("reason = %v, want below_absolute_floor", resp["reason"])
	}
}

// TestPromoApply_NotFoundCollapsesToInvalidOrExpired is the enumeration-oracle
// guard at the HTTP layer. A code that does not exist must be answered
// identically to one that has expired.
func TestPromoApply_NotFoundCollapsesToInvalidOrExpired(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	seedSubscriptionForPromoTest(t, env.db, storeID, tenantID, "starter", "usd")

	body := map[string]any{"code": "NOSUCHCODEATALL"}
	w := request(t, env.router, http.MethodPost, promoApplyURL(storeID), body, authHeaders(userID, tenantID))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["reason"] != "invalid_or_expired" {
		t.Errorf("reason = %v, want invalid_or_expired — a distinct reason for a "+
			"non-existent code tells a caller which strings are real", resp["reason"])
	}
}

// TestPromoApply_IgnoresBodySuppliedEmail proves the per-email redemption cap
// cannot be aimed at another merchant.
//
// The body carries victim@example.com. If the handler still read it, the
// redemption row would be written under that address and the real owner's
// own attempt would later be refused max_per_email_reached. The row must be
// booked against the subscription's billing email instead.
func TestPromoApply_IgnoresBodySuppliedEmail(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	promoID := seedPromoCode(t, env, "WINBACK20OFF6MONTHS", 2000)
	seedSubscriptionForPromoTest(t, env.db, storeID, tenantID, "starter", "usd")

	body := map[string]any{"code": "WINBACK20OFF6MONTHS", "email": "victim@example.com"}
	w := request(t, env.router, http.MethodPost, promoApplyURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var email string
	if err := env.db.Raw(
		`SELECT email FROM promo_redemptions WHERE promo_code_id = ? AND store_id = ?`,
		promoID, storeID,
	).Scan(&email).Error; err != nil {
		t.Fatalf("read redemption: %v", err)
	}
	if email != "merchant@example.com" {
		t.Errorf("redemption booked against %q, want the subscription billing email "+
			"merchant@example.com — a client-chosen address spends someone else's cap", email)
	}
}

// TestPromoApply_AbovePriceFloor_USD is the negative control the floor test
// above never had: proof that a code CAN be applied at all.
//
// Before the BasePriceMinor fix, the handler passed a literal 0 and
// promo.Validate derived the floor comparison from it, so the effective price
// was always 0 and every priced plan was rejected below_absolute_floor. The
// INR test passed either way — it expects a rejection — which is precisely
// why it could not detect that no application ever succeeded.
//
// $19.00 Starter monthly less 20% is $15.20, above the $12.00 floor.
func TestPromoApply_AbovePriceFloor_USD(t *testing.T) {
	env := setupTestRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleOwner, tenantID)

	seedPromoCode(t, env, "WINBACK20OFF6MONTHS", 2000)
	seedSubscriptionForPromoTest(t, env.db, storeID, tenantID, "starter", "usd")

	body := map[string]any{"code": "WINBACK20OFF6MONTHS"}
	w := request(t, env.router, http.MethodPost, promoApplyURL(storeID), body, authHeaders(userID, tenantID))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := resp["effective_minor"]; got != float64(1520) {
		t.Errorf("effective_minor = %v, want 1520 ($19.00 less 20%%)", got)
	}
	// #770: the confirmation formats effective_minor, so the currency has to
	// come back with it — the GET subscription DTO does not carry one.
	if got := resp["currency"]; got != "usd" {
		t.Errorf("currency = %v, want usd", got)
	}
	if got := resp["percent_off_bps"]; got != float64(2000) {
		t.Errorf("percent_off_bps = %v, want 2000", got)
	}
}
