package pushtoken

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository provides data access for storefront push tokens.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Upsert inserts a push token or updates the token/platform/updated_at on
// conflict (customer_id, device_id).
func (r *Repository) Upsert(t *StorefrontPushToken) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"token", "platform", "store_slug", "updated_at"}),
	}).Create(t).Error
}

// DeleteByID deletes a push token by ID scoped to the customer (ownership check).
func (r *Repository) DeleteByID(customerID, tokenID uuid.UUID) error {
	return r.db.Where("id = ? AND customer_id = ?", tokenID, customerID).
		Delete(&StorefrontPushToken{}).Error
}

// FindByCustomer returns all push tokens for a customer.
func (r *Repository) FindByCustomer(customerID uuid.UUID) ([]StorefrontPushToken, error) {
	var tokens []StorefrontPushToken
	err := r.db.Where("customer_id = ?", customerID).Find(&tokens).Error
	return tokens, err
}

// FindByStore returns all push tokens for a store slug.
func (r *Repository) FindByStore(storeSlug string) ([]StorefrontPushToken, error) {
	var tokens []StorefrontPushToken
	err := r.db.Where("store_slug = ?", storeSlug).Find(&tokens).Error
	return tokens, err
}
