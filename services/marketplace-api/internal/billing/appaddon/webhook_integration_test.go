//go:build integration

package appaddon_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/appaddon"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedProSub(t *testing.T, tenantID, storeID uuid.UUID, plan subscription.SubscriptionPlan) *subscription.StoreSubscription {
	db := testdb.NewDB(t, "store_subscriptions")
	sub := &subscription.StoreSubscription{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		StripeCustomerID: "cus_" + storeID.String()[:8],
		Plan: plan, Status: subscription.StatusActive,
	}
	require.NoError(t, db.Create(sub).Error)
	return sub
}

func TestWebhook_InvoicePaid_AddOn_FlipsFlag(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	tenantID, storeID := uuid.New(), uuid.New()
	_ = seedProSub(t, tenantID, storeID, subscription.PlanPro)

	raw := []byte(fmt.Sprintf(`{
		"id":"evt_1","type":"invoice.paid",
		"data":{"object":{
			"id":"in_addon",
			"metadata":{
				"kind":"white_label_app_add_on",
				"tenant_id":"%s",
				"store_id":"%s"
			}
		}}
	}`, tenantID, storeID))

	require.NoError(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.True(t, sub.HasWhiteLabelAppAddOn,
		"handler must flip has_white_label_app_add_on to true")
}

func TestWebhook_InvoicePaid_NotAddOnKind_NoOp(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	tenantID, storeID := uuid.New(), uuid.New()
	_ = seedProSub(t, tenantID, storeID, subscription.PlanPro)

	raw := []byte(`{
		"id":"evt_1","type":"invoice.paid",
		"data":{"object":{
			"id":"in_normal",
			"metadata":{"some":"other"}
		}}
	}`)

	require.NoError(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.False(t, sub.HasWhiteLabelAppAddOn,
		"non-add-on invoice.paid must leave the flag alone")
}

func TestWebhook_InvoicePaid_Replay_Idempotent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	tenantID, storeID := uuid.New(), uuid.New()
	_ = seedProSub(t, tenantID, storeID, subscription.PlanPro)

	raw := []byte(fmt.Sprintf(`{
		"id":"evt_1","type":"invoice.paid",
		"data":{"object":{
			"id":"in_addon",
			"metadata":{"kind":"white_label_app_add_on","store_id":"%s"}
		}}
	}`, storeID))

	// First fire flips.
	require.NoError(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw))
	// Second fire must be a no-op and not error.
	require.NoError(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.True(t, sub.HasWhiteLabelAppAddOn)
}

func TestWebhook_InvoicePaid_NonProStore_NoFlip(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	tenantID, storeID := uuid.New(), uuid.New()
	_ = seedProSub(t, tenantID, storeID, subscription.PlanStarter)

	raw := []byte(fmt.Sprintf(`{
		"id":"evt_1","type":"invoice.paid",
		"data":{"object":{
			"id":"in_addon",
			"metadata":{"kind":"white_label_app_add_on","store_id":"%s"}
		}}
	}`, storeID))

	require.NoError(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw))

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
	require.False(t, sub.HasWhiteLabelAppAddOn,
		"non-Pro store must not have the flag flipped (plan check in CAS)")
}

func TestWebhook_InvoicePaid_MalformedMetadata_ReturnsError(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	// Missing store_id.
	raw := []byte(`{
		"data":{"object":{"id":"in_x","metadata":{"kind":"white_label_app_add_on"}}}
	}`)
	require.Error(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw),
		"missing store_id must error")

	// Invalid store_id.
	raw = []byte(`{
		"data":{"object":{"id":"in_x","metadata":{
			"kind":"white_label_app_add_on","store_id":"not-a-uuid"
		}}}
	}`)
	require.Error(t, appaddon.HandleInvoicePaidForAppAddOn(context.Background(), db, raw),
		"invalid store_id must error")
}
