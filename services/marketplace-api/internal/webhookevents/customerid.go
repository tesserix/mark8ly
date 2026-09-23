package webhookevents

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomerIDFromPayload extracts the Stripe customer id an event belongs to.
//
// Most events wrap an object that REFERENCES a customer, so the id is at
// data.object.customer. Customer events are the exception: the object IS the
// customer, so its id is data.object.id and there is no `customer` field at
// all.
//
// Reading only data.object.customer — which both call sites did, in
// separately maintained copies — meant every customer.updated resolved to
// nothing, was classified an orphan, and was answered 200 without ever
// reaching handleCustomerUpdated. That handler is what mirrors the merchant's
// billing email onto store_subscriptions and tracks whether they have a
// default payment method; via webhooks it had never once run. The events then
// retried to the cap in the resolver, which shared the defect, and were
// flagged for manual review.
//
// customer.subscription.* is NOT one of these: that object is a subscription,
// which does carry a `customer` field, so it takes the ordinary path.
func CustomerIDFromPayload(eventType string, payload []byte) string {
	var e struct {
		Data struct {
			Object struct {
				ID       string `json:"id"`
				Object   string `json:"object"`
				Customer string `json:"customer"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &e); err != nil {
		return ""
	}

	if id := strings.TrimSpace(e.Data.Object.Customer); id != "" {
		return id
	}
	if e.Data.Object.Object == "customer" || isCustomerEvent(eventType) {
		return strings.TrimSpace(e.Data.Object.ID)
	}
	return ""
}

// isCustomerEvent reports whether the event's object is the customer itself.
// Stripe stamps `"object":"customer"` on real payloads, so this is the
// belt to that braces — fixtures and replays do not always carry it.
func isCustomerEvent(eventType string) bool {
	return strings.HasPrefix(eventType, "customer.") &&
		!strings.HasPrefix(eventType, "customer.subscription.")
}

// LookupStoreByCustomer resolves a Stripe customer id to (store_id,
// tenant_id) via store_subscriptions. Returns ok=false when nothing matches,
// which is the orphan case: an event whose subscription row does not exist
// yet, or never will.
func LookupStoreByCustomer(ctx context.Context, db *gorm.DB, customerID string) (uuid.UUID, uuid.UUID, bool) {
	if customerID == "" {
		return uuid.Nil, uuid.Nil, false
	}
	var row struct {
		StoreID  string `gorm:"column:store_id"`
		TenantID string `gorm:"column:tenant_id"`
	}
	err := db.WithContext(ctx).Raw(
		`SELECT store_id::text AS store_id, tenant_id::text AS tenant_id
         FROM store_subscriptions
         WHERE stripe_customer_id = ?
         LIMIT 1`,
		customerID,
	).Scan(&row).Error
	if err != nil || row.StoreID == "" {
		return uuid.Nil, uuid.Nil, false
	}
	storeUUID, err := uuid.Parse(row.StoreID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	tenantUUID, err := uuid.Parse(row.TenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return storeUUID, tenantUUID, true
}
