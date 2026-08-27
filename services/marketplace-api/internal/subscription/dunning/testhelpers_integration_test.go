//go:build integration

package dunning_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// seedStore inserts a minimal stores row for (tenantID, storeID) so that a
// store_subscriptions row referencing storeID satisfies
// store_subscriptions_store_id_fkey.
//
// Unlike internal/order's seedStore helper, tenantID is supplied by the
// caller rather than generated here: dunning tests already mint their own
// tenantID for the subscription/audit rows they seed, and the store must
// carry that same tenant — otherwise the subscription and its store
// disagree about tenancy and any tenant-scoped assertion becomes
// meaningless.
//
// Registers a t.Cleanup that deletes the store row so consecutive test runs
// start from a clean state. No per-store sequences are dropped here (unlike
// internal/order's helper) — dunning doesn't touch order/return numbering.
func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()

	// Slug is derived from the store id to dodge the stores_slug_unique
	// constraint when multiple tests (or multiple stores within one test)
	// run in the same package.
	slug := "tst-" + strings.ReplaceAll(storeID.String(), "-", "")[:20]

	err := db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status, storefront_customer_portal_secret)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, encode(gen_random_bytes(32), 'hex'))`,
		storeID, tenantID, slug, "Test Store", "IE", "EUR", "Europe/Dublin", "active",
	).Error
	if err != nil {
		t.Fatalf("seedStore: insert stores row: %v", err)
	}

	t.Cleanup(func() {
		db.Exec("DELETE FROM stores WHERE id = ?", storeID)
	})
}

// seedPastDueSubscription inserts a stores row, a past_due
// store_subscriptions row with the given email, and the audit_logs entry
// recording the transition INTO past_due at transitionedAt. It mirrors the
// exact seeding shape used by TestSendDunningEmails_SendsOnDay5AndDay7 so
// dunning cron tests share one source of truth for what "eligible" means.
func seedPastDueSubscription(t *testing.T, db *gorm.DB, transitionedAt time.Time, emailAddr *string) subscription.StoreSubscription {
	t.Helper()

	storeID := uuid.New()
	tenantID := uuid.New()
	seedStore(t, db, tenantID, storeID)

	subscriptionID := uuid.New()
	sub := subscription.StoreSubscription{
		ID:               subscriptionID,
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_" + strings.ReplaceAll(subscriptionID.String(), "-", "")[:14],
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusPastDue,
		Email:            emailAddr,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seedPastDueSubscription: seed subscription: %v", err)
	}

	auditEntry := audit.Entry{
		ID:           uuid.New(),
		TenantID:     tenantID,
		StoreID:      &storeID,
		ActorType:    audit.ActorSystem,
		Action:       "subscription.state_transition",
		ResourceType: "subscription",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityInfo,
		Metadata:     audit.Metadata{"to_status": "past_due", "from_status": "active"},
		CreatedAt:    transitionedAt,
	}
	if err := db.Create(&auditEntry).Error; err != nil {
		t.Fatalf("seedPastDueSubscription: seed audit: %v", err)
	}

	return sub
}
