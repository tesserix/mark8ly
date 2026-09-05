package subscription

// billing_email.go — the merchant's billing address for transactional mail.
//
// The symmetric partner to StoreNameFor: a caller holding only a store id and
// no StoreSubscription row (the migration fast-path handler, which addresses
// a review by id) needs both the address and the name to send.

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BillingEmailFor returns the store's billing email, or "" when there is no
// subscription row, no address on it, or the lookup fails.
//
// Never returns an error, and "" is deliberately not special-cased into a
// sentinel: every caller passes the result to email.ValidateRecipient, which
// classifies "" as ReasonNoAddress. One place decides what is undeliverable,
// and it is not this function.
func BillingEmailFor(ctx context.Context, db *gorm.DB, storeID uuid.UUID) string {
	var addr string
	err := db.WithContext(ctx).
		Raw(`SELECT email FROM store_subscriptions WHERE store_id = ?`, storeID).
		Scan(&addr).Error
	if err != nil {
		return ""
	}
	return addr
}
