//go:build integration

package webhooks_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/handlers/webhooks"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

const testSecret = "whsec_test"

// fixedNow is the timestamp embedded in all test signatures. It must match
// the Now func passed to newHandler so VerifySignature accepts them.
var fixedNow = time.Unix(1_712_000_000, 0)

func newHandler(t *testing.T, db *gorm.DB, dispatch webhooks.DispatchFunc) *webhooks.StripeHandler {
	t.Helper()
	return webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
		DB:       db,
		Secret:   testSecret,
		Repo:     webhookevents.NewRepository(),
		Dispatch: dispatch,
		AllowedTypes: map[string]bool{
			"customer.subscription.updated": true,
			"checkout.session.completed":    true,
		},
		Now: func() time.Time { return fixedNow },
	})
}

func post(t *testing.T, h *webhooks.StripeHandler, body []byte, sig string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/stripe-billing", h.Handle)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe-billing", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestStripeWebhook_ValidSignature_InsertsAndDispatches verifies the happy path:
// a valid signature for a known event type resolves the store, dispatches inside
// the advisory lock, and stamps processed_at.
func TestStripeWebhook_ValidSignature_InsertsAndDispatches(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	var dispatched bool
	h := newHandler(t, db, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		dispatched = true
		return nil
	})

	payload := []byte(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"customer":"cus_x"}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	w := post(t, h, payload, sig)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "processed")
	require.True(t, dispatched, "dispatch must be called for allowed event type")

	// Event row must exist and be stamped processed.
	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id = ?", "evt_1").Error)
	require.NotNil(t, e.ProcessedAt, "processed_at must be set after successful dispatch")
}

// TestStripeWebhook_DuplicateEvent_ReturnsDuplicateWithoutDispatch verifies that
// posting the same event_id twice returns "duplicate" on the second call and
// does not invoke the dispatch function a second time.
func TestStripeWebhook_DuplicateEvent_ReturnsDuplicateWithoutDispatch(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_y",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	calls := 0
	h := newHandler(t, db, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		calls++
		return nil
	})

	payload := []byte(`{"id":"evt_dup","type":"customer.subscription.updated","data":{"object":{"customer":"cus_y"}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	w1 := post(t, h, payload, sig)
	require.Equal(t, 200, w1.Code)
	require.Contains(t, w1.Body.String(), "processed")

	w2 := post(t, h, payload, sig)
	require.Equal(t, 200, w2.Code)
	require.Contains(t, w2.Body.String(), "duplicate")

	require.Equal(t, 1, calls, "dispatch must be called exactly once even when event is POSTed twice")
}

// TestStripeWebhook_BadSignature_Returns401 verifies that a tampered or missing
// signature is rejected before any database write occurs.
func TestStripeWebhook_BadSignature_Returns401(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")

	h := newHandler(t, db, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		return nil
	})

	payload := []byte(`{"id":"evt_bad","type":"customer.subscription.updated","data":{"object":{}}}`)
	w := post(t, h, payload, "t=1712000000,v1=deadbeef")

	require.Equal(t, 401, w.Code)
	require.Contains(t, w.Body.String(), "invalid_signature")

	var count int64
	require.NoError(t, db.Table("stripe_webhook_events").Count(&count).Error)
	require.EqualValues(t, 0, count, "no row must be persisted when signature is invalid")
}

// TestStripeWebhook_BodyOver512K_Returns413 verifies that a request body
// exceeding MaxBodyBytes is rejected immediately with 413.
func TestStripeWebhook_BodyOver512K_Returns413(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")

	h := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
		DB:     db,
		Secret: testSecret,
		Repo:   webhookevents.NewRepository(),
		Dispatch: func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
			return nil
		},
		AllowedTypes: map[string]bool{},
		MaxBodyBytes: 100, // intentionally tiny to trigger the limit reliably
		Now:          func() time.Time { return fixedNow },
	})

	big := strings.Repeat("x", 500)
	w := post(t, h, []byte(big), "t=1712000000,v1=deadbeef")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

// TestStripeWebhook_UnknownType_PersistsButSkipsDispatch verifies that an event
// whose type is not in AllowedTypes is stored (for audit purposes) but the
// dispatch function is not called — returns 200 "persisted".
func TestStripeWebhook_UnknownType_PersistsButSkipsDispatch(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")

	called := false
	h := newHandler(t, db, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		called = true
		return nil
	})

	payload := []byte(`{"id":"evt_unknown","type":"unknown.event.type","data":{"object":{}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	w := post(t, h, payload, sig)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "persisted")
	require.False(t, called, "dispatch must NOT be called for event types not in AllowedTypes")

	var count int64
	require.NoError(t, db.Table("stripe_webhook_events").Where("event_id = ?", "evt_unknown").Count(&count).Error)
	require.EqualValues(t, 1, count, "event row must be persisted even when type is not in allowlist")
}

// TestStripeWebhook_OrphanCustomer_ReturnsRetryScheduled verifies that when the
// stripe_customer_id in the payload has no matching store_subscriptions row,
// the handler returns "retry_scheduled", bumps retry_count, and does NOT stamp
// processed_at — leaving the event for the cron retrier.
func TestStripeWebhook_OrphanCustomer_ReturnsRetryScheduled(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	called := false
	h := newHandler(t, db, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		called = true
		return nil
	})

	// cus_orphan has no row in store_subscriptions.
	payload := []byte(`{"id":"evt_orphan","type":"customer.subscription.updated","data":{"object":{"customer":"cus_orphan"}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	w := post(t, h, payload, sig)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "retry_scheduled")
	require.False(t, called, "dispatch must NOT be called for orphan events")

	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id = ?", "evt_orphan").Error)
	require.Nil(t, e.ProcessedAt, "processed_at must remain NULL for orphan events")
	require.Nil(t, e.StoreID, "store_id must remain NULL so cron can retry")
	require.Greater(t, e.RetryCount, 0, "retry_count must be bumped")
	require.NotNil(t, e.ProcessingError, "processing_error must record the orphan reason")
}
