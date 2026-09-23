//go:build integration

package webhooks_test

import (
	"bytes"
	"context"
	"errors"
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
	return newHandlerWithMaxRetries(t, db, 0, dispatch)
}

// newHandlerWithMaxRetries builds a handler with an explicit retry cap.
// maxRetries 0 means "use the default", as the config does.
func newHandlerWithMaxRetries(t *testing.T, db *gorm.DB, maxRetries int, dispatch webhooks.DispatchFunc) *webhooks.StripeHandler {
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
		MaxRetries: maxRetries,
		Now:        func() time.Time { return fixedNow },
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
	testdb.SeedStore(t, db, tenantID, storeID)
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
	testdb.SeedStore(t, db, tenantID, storeID)
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

	// Kept for audit, and finished. Leaving it unprocessed enrolled every
	// ignored event in the recovery loop, where it burned the retry budget
	// and the alert channel on an event no handler will ever want.
	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id = ?", "evt_unknown").Error)
	require.NotNil(t, e.ProcessedAt, "an event nothing will ever handle must not sit in the retry queue")
}

// TestStripeWebhook_OrphanCustomer_AsksStripeToRedeliver verifies that when
// the stripe_customer_id in the payload has no matching store_subscriptions
// row, the handler bumps retry_count, leaves processed_at NULL, and answers
// 503 so Stripe redelivers.
//
// It used to answer 200. The orphan case is usually a race — the event
// arrives before the subscription row commits — and Stripe's own redelivery
// resolves it sooner than a five-minute cron, for free, if we simply ask.
func TestStripeWebhook_OrphanCustomer_AsksStripeToRedeliver(t *testing.T) {
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
	require.Equal(t, 503, w.Code, "a non-2xx is what makes Stripe send it again")
	require.Contains(t, w.Body.String(), "dispatch_failed")
	require.False(t, called, "dispatch must NOT be called for orphan events")

	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id = ?", "evt_orphan").Error)
	require.Nil(t, e.ProcessedAt, "processed_at must remain NULL for orphan events")
	require.Nil(t, e.StoreID, "store_id must remain NULL so the resolver can attribute it")
	require.Greater(t, e.RetryCount, 0, "retry_count must be bumped")
	require.NotNil(t, e.ProcessingError, "processing_error must record the orphan reason")
}

// TestStripeWebhook_RedeliveryOfUnprocessedEvent_DispatchesAgain is the
// defect that made every other retry mechanism moot.
//
// Stripe resends a failed webhook for three days. The handler answered every
// resend "duplicate" on the strength of the row existing, without asking
// whether it had ever processed — so the first failure was final no matter
// how many times Stripe tried. A redelivery is a duplicate of DELIVERY, not
// of work.
func TestStripeWebhook_RedeliveryOfUnprocessedEvent_DispatchesAgain(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_redeliver",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	calls := 0
	failFirst := func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		calls++
		if calls == 1 {
			return errors.New("handler blew up")
		}
		return nil
	}
	h := newHandler(t, db, failFirst)

	payload := []byte(`{"id":"evt_redeliver","type":"customer.subscription.updated","data":{"object":{"customer":"cus_redeliver"}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	// First delivery fails inside the handler.
	w1 := post(t, h, payload, sig)
	require.Equal(t, 503, w1.Code)

	var afterFirst webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&afterFirst, "event_id = ?", "evt_redeliver").Error)
	require.Nil(t, afterFirst.ProcessedAt)
	require.NotNil(t, afterFirst.StoreID,
		"the store was resolved before dispatch — which is exactly why recovery must not filter on store_id IS NULL")

	// Stripe sends it again; this time the handler succeeds.
	w2 := post(t, h, payload, sig)
	require.Equal(t, 200, w2.Code)
	require.Contains(t, w2.Body.String(), "processed")
	require.Equal(t, 2, calls, "the redelivery must reach the dispatcher, not be dismissed as a duplicate")

	var afterSecond webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&afterSecond, "event_id = ?", "evt_redeliver").Error)
	require.NotNil(t, afterSecond.ProcessedAt, "the retry is what finally processes it")

	// A third delivery is now a genuine duplicate and must not re-apply.
	w3 := post(t, h, payload, sig)
	require.Equal(t, 200, w3.Code)
	require.Contains(t, w3.Body.String(), "duplicate")
	require.Equal(t, 2, calls, "an already-processed event must never dispatch again")
}

// TestStripeWebhook_PastTheRetryCap_StopsAskingStripe — an event that fails
// the same way every time must stop churning: flagged for a human, 200 to
// Stripe so it gives up, and visible to cmd/webhook-replay.
func TestStripeWebhook_PastTheRetryCap_StopsAskingStripe(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events", "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_capped",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	h := newHandlerWithMaxRetries(t, db, 2, func(_ context.Context, _ *gorm.DB, _ webhookevents.StripeWebhookEvent) error {
		return errors.New("always fails")
	})

	payload := []byte(`{"id":"evt_capped","type":"customer.subscription.updated","data":{"object":{"customer":"cus_capped"}}}`)
	sig := billingstripe.BuildSignatureForTesting(payload, testSecret, fixedNow)

	require.Equal(t, 503, post(t, h, payload, sig).Code, "first failure: ask again")

	last := post(t, h, payload, sig)
	require.Equal(t, 200, last.Code, "at the cap, stop asking Stripe to resend")
	require.Contains(t, last.Body.String(), "manual_review_required")

	var e webhookevents.StripeWebhookEvent
	require.NoError(t, db.First(&e, "event_id = ?", "evt_capped").Error)
	require.True(t, e.ManualReviewRequired)
	require.Nil(t, e.ProcessedAt)

	// And a further redelivery is answered without another attempt.
	again := post(t, h, payload, sig)
	require.Equal(t, 200, again.Code)
	require.Contains(t, again.Body.String(), "manual_review_required")
}
