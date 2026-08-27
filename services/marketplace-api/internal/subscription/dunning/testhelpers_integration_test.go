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
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedStore delegates to the shared testdb.SeedStore helper, which was
// derived from this package's original implementation and is behaviourally
// identical: same signature, same column list, same slug derivation, same
// cleanup. Kept as a thin wrapper so callers in this package don't change.
func seedStore(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID) {
	t.Helper()
	testdb.SeedStore(t, db, tenantID, storeID)
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
