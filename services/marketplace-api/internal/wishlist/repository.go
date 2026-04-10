package wishlist

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository defines the data-access contract for wishlists.
type Repository interface {
	Add(ctx context.Context, item *Wishlist) error
	Remove(ctx context.Context, customerID, productID string) error
	List(ctx context.Context, customerID string, page, limit int) ([]WishlistItem, int64, error)
	Check(ctx context.Context, customerID, productID string) (bool, error)
	CountByCustomer(ctx context.Context, customerID string) (int64, error)
}

// gormRepo implements Repository backed by GORM.
type gormRepo struct {
	db *gorm.DB
}

// NewRepository constructs a GORM-backed wishlist repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepo{db: db}
}

// Add inserts a wishlist entry. ON CONFLICT DO NOTHING makes it idempotent.
func (r *gormRepo) Add(ctx context.Context, item *Wishlist) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(item).Error
}

// Remove deletes by (customer_id, product_id). Idempotent — no error if
// the row doesn't exist.
func (r *gormRepo) Remove(ctx context.Context, customerID, productID string) error {
	return r.db.WithContext(ctx).
		Where("customer_id = ? AND product_id = ?", customerID, productID).
		Delete(&Wishlist{}).Error
}

// List returns paginated wishlist items with joined product details.
func (r *gormRepo) List(ctx context.Context, customerID string, page, limit int) ([]WishlistItem, int64, error) {
	// Count total.
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&Wishlist{}).
		Where("customer_id = ?", customerID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []WishlistItem{}, 0, nil
	}

	offset := (page - 1) * limit

	var items []WishlistItem
	err := r.db.WithContext(ctx).Raw(`
		SELECT w.id, w.product_id, w.created_at,
			p.title AS product_title,
			p.handle AS product_handle,
			(SELECT url FROM product_media WHERE product_id = p.id ORDER BY position LIMIT 1) AS product_image_url,
			COALESCE((SELECT MIN(price)::text FROM product_variants WHERE product_id = p.id), '0') AS product_min_price,
			COALESCE((SELECT MAX(price)::text FROM product_variants WHERE product_id = p.id), '0') AS product_max_price,
			COALESCE(s.currency_code, 'AUD') AS currency_code,
			p.status = 'active' AS in_stock
		FROM wishlists w
		JOIN products p ON p.id = w.product_id
		LEFT JOIN stores s ON s.id = w.store_id
		WHERE w.customer_id = ?
		ORDER BY w.created_at DESC
		LIMIT ? OFFSET ?
	`, customerID, limit, offset).Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// Check returns true if the customer has wishlisted the given product.
func (r *gormRepo) Check(ctx context.Context, customerID, productID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM wishlists
			WHERE customer_id = ? AND product_id = ?
		)
	`, customerID, productID).Scan(&exists).Error
	return exists, err
}

// CountByCustomer returns the total wishlist count for a customer.
func (r *gormRepo) CountByCustomer(ctx context.Context, customerID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Wishlist{}).
		Where("customer_id = ?", customerID).
		Count(&count).Error
	return count, err
}
