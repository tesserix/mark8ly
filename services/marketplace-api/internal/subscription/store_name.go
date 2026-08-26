package subscription

// store_name.go — the merchant-facing store name for email copy.
//
// The crons load StoreSubscription rows, which carry no name; the name lives
// in the local `stores` projection. A scalar lookup per row is acceptable on
// the reminder paths, which process tens of rows daily. The dunning ladder
// joins instead, being the higher-volume path.

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StoreNameFor returns the store's display name, or "your store" when the
// local projection has no row yet. Never returns an error: a cosmetic field
// must not be able to stop a billing email.
func StoreNameFor(ctx context.Context, db *gorm.DB, storeID uuid.UUID) string {
	var name string
	err := db.WithContext(ctx).
		Raw(`SELECT name FROM stores WHERE id = ?`, storeID).
		Scan(&name).Error
	if err != nil || name == "" {
		return "your store"
	}
	return name
}
