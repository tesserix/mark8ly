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
			-- deleted_at IS NULL is load-bearing, not defensive: variants are
			-- SOFT-deleted (repository_variants.go), so a withdrawn variant's
			-- row survives with deleted_at set. Without the predicate a
			-- merchant who removes a $10 variant from a $10/$30 product keeps
			-- advertising "from $10" here for a product whose cheapest
			-- purchasable variant is now $30 — a price checkout will not
			-- honour. It errs low, which is the bad direction (#420).
			--
			-- #395 made GORM add this predicate implicitly by moving
			-- Variant.DeletedAt to gorm.DeletedAt, but this is RAW SQL against
			-- the table, so no implicit predicate reaches it.
			COALESCE((SELECT MIN(price)::text FROM product_variants
			           WHERE product_id = p.id AND deleted_at IS NULL), '0') AS product_min_price,
			COALESCE((SELECT MAX(price)::text FROM product_variants
			           WHERE product_id = p.id AND deleted_at IS NULL), '0') AS product_max_price,
			COALESCE(s.currency_code, 'AUD') AS currency_code,
			-- in_stock means what its name says: the product is purchasable.
			-- This was a bare p.status = 'active' test, so an active product whose
			-- every variant had zero inventory advertised itself as in stock —
			-- and disagreed with its OWN product page, which derives the flag
			-- per variant from InventoryQuantity > 0 (handlers/storefront/
			-- dto.go:139). Two surfaces, one word, opposite answers (#420).
			--
			-- Both halves are required: an inactive product is not purchasable
			-- however much stock it has, and an active one with none is not
			-- purchasable either. deleted_at IS NULL for the same reason as
			-- the price subqueries above — a withdrawn variant's stock must
			-- not keep a product looking available.
			(p.status = 'active' AND EXISTS (
				SELECT 1 FROM product_variants
				 WHERE product_id = p.id
				   AND deleted_at IS NULL
				   AND inventory_quantity > 0
			)) AS in_stock
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
