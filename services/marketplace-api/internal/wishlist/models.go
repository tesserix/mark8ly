// Package wishlist provides the domain model, repository, and handler
// wiring for customer wishlists (C4). A wishlist entry is a simple
// (customer, product) pair — one row per wishlisted product.
package wishlist

import (
	"time"
)

// Wishlist maps to the wishlists table.
type Wishlist struct {
	ID         string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID   string    `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID    string    `gorm:"column:store_id;type:uuid;not null"`
	CustomerID string    `gorm:"column:customer_id;type:uuid;not null"`
	ProductID  string    `gorm:"column:product_id;type:uuid;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()"`
}

// TableName returns the SQL table name.
func (Wishlist) TableName() string { return "wishlists" }

// WishlistItem is the read projection returned by List, joining product
// details so the storefront can render the wishlist without extra calls.
type WishlistItem struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
	// Joined from products.
	ProductTitle    string  `json:"product_title"`
	ProductHandle   string  `json:"product_handle"`
	ProductImageURL *string `json:"product_image_url"`
	ProductMinPrice string  `json:"product_min_price"`
	ProductMaxPrice string  `json:"product_max_price"`
	CurrencyCode    string  `json:"currency_code"`
	InStock         bool    `json:"in_stock"`
}
