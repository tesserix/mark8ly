package notifyme

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository provides data access for product notification subscriptions.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Upsert inserts a subscription or does nothing on conflict
// (customer_id, product_id, notify_type).
func (r *Repository) Upsert(sub *ProductNotifySubscription) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "product_id"}, {Name: "notify_type"}},
		DoNothing: true,
	}).Create(sub).Error
}

// Delete removes a subscription by customer, product, and notify_type.
func (r *Repository) Delete(customerID, productID uuid.UUID, notifyType string) error {
	return r.db.
		Where("customer_id = ? AND product_id = ? AND notify_type = ?",
			customerID, productID, notifyType).
		Delete(&ProductNotifySubscription{}).Error
}

// FindByCustomer returns all subscriptions for a customer in a store.
func (r *Repository) FindByCustomer(customerID uuid.UUID, storeSlug string) ([]ProductNotifySubscription, error) {
	var subs []ProductNotifySubscription
	err := r.db.
		Where("customer_id = ? AND store_slug = ?", customerID, storeSlug).
		Order("created_at DESC").
		Find(&subs).Error
	return subs, err
}

// FindByProduct returns all subscriptions for a product (for sending notifications).
func (r *Repository) FindByProduct(productID uuid.UUID, notifyType string) ([]ProductNotifySubscription, error) {
	var subs []ProductNotifySubscription
	err := r.db.
		Where("product_id = ? AND notify_type = ?", productID, notifyType).
		Find(&subs).Error
	return subs, err
}
