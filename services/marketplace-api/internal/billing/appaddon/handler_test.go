package appaddon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
	"github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

type fakeStripe struct {
	lastInput stripe.OneOffInvoiceInput
	resp      *stripe.Invoice
	err       error
}

func (f *fakeStripe) CreateOneOffInvoice(_ context.Context, in stripe.OneOffInvoiceInput) (*stripe.Invoice, error) {
	f.lastInput = in
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type fakeSubRepo struct {
	sub *subscription.StoreSubscription
	err error
}

func (f *fakeSubRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return f.sub, f.err
}

func router(h *appaddon.Handler, tenantID, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID.String())
		c.Set("user_id", userID.String())
		c.Next()
	})
	r.POST("/admin/stores/:storeId/subscription/add-on/white-label-app", h.Purchase)
	return r
}

func subPro(storeID uuid.UUID) *subscription.StoreSubscription {
	period := time.Now().Add(183 * 24 * time.Hour)
	return &subscription.StoreSubscription{
		ID:                    uuid.New(),
		TenantID:              uuid.New(),
		StoreID:               storeID,
		StripeCustomerID:      "cus_abc",
		Plan:                  subscription.PlanPro,
		Status:                subscription.StatusActive,
		CurrentPeriodEnd:      &period,
		HasWhiteLabelAppAddOn: false,
	}
}

// Note: handler.go calls attestations.Record(...) which writes to the
// database. These unit tests cover the pre-DB paths (validation, gate
// logic, Stripe call shape) and mock the repo lookup. A full
// integration test that hits testdb exists alongside the advancer.

func TestPurchase_MissingAck_400(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  &fakeStripe{},
		SubRepo: &fakeSubRepo{sub: subPro(storeID)},
	})

	body, _ := json.Marshal(map[string]any{"apple_4_2_6_acknowledged": false})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("apple_4_2_6_ack_required")) {
		t.Errorf("body = %s, want apple_4_2_6_ack_required", w.Body.String())
	}
}

func TestPurchase_MissingAttestationText_400(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  &fakeStripe{},
		SubRepo: &fakeSubRepo{sub: subPro(storeID)},
	})

	body, _ := json.Marshal(map[string]any{"apple_4_2_6_acknowledged": true, "attestation_text": ""})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestPurchase_NonProStore_403(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	sub := subPro(storeID)
	sub.Plan = subscription.PlanStudio
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  &fakeStripe{},
		SubRepo: &fakeSubRepo{sub: sub},
	})

	body, _ := json.Marshal(map[string]any{
		"apple_4_2_6_acknowledged": true,
		"attestation_text":         "I ack",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("pro_plan_required")) {
		t.Errorf("body = %s, want pro_plan_required", w.Body.String())
	}
}

func TestPurchase_AlreadyHasAddOn_409(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	sub := subPro(storeID)
	sub.HasWhiteLabelAppAddOn = true
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  &fakeStripe{},
		SubRepo: &fakeSubRepo{sub: sub},
	})

	body, _ := json.Marshal(map[string]any{
		"apple_4_2_6_acknowledged": true,
		"attestation_text":         "x",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("already_active")) {
		t.Errorf("body = %s, want already_active", w.Body.String())
	}
}

func TestPurchase_SubscriptionLookupFailure_500(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  &fakeStripe{},
		SubRepo: &fakeSubRepo{err: errors.New("db down")},
	})

	body, _ := json.Marshal(map[string]any{"apple_4_2_6_acknowledged": true, "attestation_text": "x"})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestPurchase_StripeShapeAndMetadata(t *testing.T) {
	tenantID, userID, storeID := uuid.New(), uuid.New(), uuid.New()
	sub := subPro(storeID)
	fs := &fakeStripe{err: errors.New("stop after stripe call")} // short-circuit before DB write
	h := appaddon.NewHandler(appaddon.Config{
		Stripe:  fs,
		SubRepo: &fakeSubRepo{sub: sub},
		Clock: func() time.Time {
			return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		},
	})

	body, _ := json.Marshal(map[string]any{
		"apple_4_2_6_acknowledged": true,
		"attestation_text":         "I acknowledge Apple 4.2.6",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router(h, tenantID, userID).ServeHTTP(w, req)

	// Stripe errored → handler returns 500, but we care about what was
	// sent TO Stripe.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if fs.lastInput.CustomerID != "cus_abc" {
		t.Errorf("CustomerID = %q, want cus_abc", fs.lastInput.CustomerID)
	}
	if fs.lastInput.Currency != "usd" {
		t.Errorf("Currency = %q, want usd", fs.lastInput.Currency)
	}
	if fs.lastInput.Metadata["kind"] != "white_label_app_add_on" {
		t.Errorf("Metadata[kind] = %q, want white_label_app_add_on",
			fs.lastInput.Metadata["kind"])
	}
	if fs.lastInput.Metadata["store_id"] != storeID.String() {
		t.Errorf("Metadata[store_id] = %q, want %s",
			fs.lastInput.Metadata["store_id"], storeID)
	}
	if fs.lastInput.AmountCents < 200_000 {
		t.Errorf("AmountCents = %d, want >= 200_000 (setup fee minimum)",
			fs.lastInput.AmountCents)
	}
}
