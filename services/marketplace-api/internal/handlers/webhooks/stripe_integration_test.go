//go:build integration

package webhooks_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/handlers/webhooks"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

const fixtureSecret = "whsec_fixture_test"

// TestFullWebhookFlow_AllAllowlistedEvents iterates every fixture JSON in
// scripts/webhook-fixtures/, signs it with fixtureSecret, POSTs to the
// webhook handler, and asserts the event is persisted in stripe_webhook_events.
//
// The test seeds a store_subscriptions row for cus_fixture so that customer
// resolution succeeds and the full dispatch path is exercised. The test is
// skipped when TEST_DATABASE_URL is not set.
func TestFullWebhookFlow_AllAllowlistedEvents(t *testing.T) {
	// Fixtures live at services/marketplace-api/scripts/webhook-fixtures/
	// relative to this file: ../../scripts/webhook-fixtures
	dir := filepath.Join("..", "..", "scripts", "webhook-fixtures")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "fixture directory must exist")
	require.NotEmpty(t, entries, "at least one fixture file required")

	db := testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")

	// Seed the fixture subscription so customer resolution succeeds.
	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_fixture",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusSignup,
	}).Error)

	dispatcher := dispatch.New(nil)
	allowed := map[string]bool{
		"checkout.session.completed":      true,
		"customer.subscription.updated":   true,
		"customer.subscription.deleted":   true,
		"invoice.paid":                    true,
		"invoice.payment_failed":          true,
		"invoice.payment_action_required": true,
		"customer.updated":                true,
		"charge.refunded":                 true,
		"payment_method.attached":         true,
		"payment_method.detached":         true,
		"radar.early_fraud_warning":       true,
	}
	now := time.Unix(1_712_000_000, 0)
	h := webhooks.NewStripeHandler(webhooks.StripeHandlerConfig{
		DB:     db,
		Secret: fixtureSecret,
		Repo:   webhookevents.NewRepository(),
		Dispatch: func(ctx context.Context, tx *gorm.DB, e webhookevents.StripeWebhookEvent) error {
			return dispatcher.Dispatch(ctx, tx, e)
		},
		AllowedTypes: allowed,
		Now:          func() time.Time { return now },
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/stripe-billing", h.Handle)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			require.NoError(t, err)

			sig := billingstripe.BuildSignatureForTesting(body, fixtureSecret, now)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe-billing", bytes.NewReader(body))
			req.Header.Set("Stripe-Signature", sig)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			_, _ = io.ReadAll(w.Body)
			require.Equal(t, 200, w.Code, "fixture %s must return 200", entry.Name())

			// Every fixture has a unique event_id — verify it persisted.
			eventID := extractEventID(t, body)
			var count int64
			require.NoError(t, db.Table("stripe_webhook_events").Where("event_id = ?", eventID).Count(&count).Error)
			require.EqualValues(t, 1, count, "event %s must be persisted exactly once", eventID)
		})
	}
}

// extractEventID pulls the top-level "id" field from a raw Stripe event JSON
// without importing encoding/json, using a byte-scan approach.
func extractEventID(t *testing.T, raw []byte) string {
	t.Helper()
	idx := bytes.Index(raw, []byte(`"id":"`))
	require.True(t, idx >= 0, "payload must contain id field")
	rest := raw[idx+6:]
	end := bytes.IndexByte(rest, '"')
	require.True(t, end > 0, "id field must be closed")
	return string(rest[:end])
}
