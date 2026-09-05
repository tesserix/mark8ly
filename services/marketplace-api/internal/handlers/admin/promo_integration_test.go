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
	sub := &subscription.StoreSubscription{
		TenantID:         tid,
		StoreID:          sid,
		StripeCustomerID: "cus_test_" + storeID[:8],
		Plan:             subscription.SubscriptionPlan(plan),
		Status:           subscription.StatusActive,
		BillingCurrency:  &inr,
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

	body := map[string]any{
		"code":  "HALFOFF123456",
		"email": "merchant@example.com",
	}
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
}
