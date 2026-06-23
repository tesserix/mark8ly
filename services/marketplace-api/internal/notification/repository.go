package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ListFilter holds the query parameters for listing notifications.
type ListFilter struct {
	StoreID uuid.UUID
	Page    int
	PerPage int
}

// ListResult holds a page of notifications and the total count.
type ListResult struct {
	Notifications []Notification
	Total         int64
}

// Repository is the data-access surface for notifications.
type Repository interface {
	// ListByStore returns a paginated list of notifications for a store.
	ListByStore(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// GetUnreadCount returns the number of unread notifications for a store.
	GetUnreadCount(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (int64, error)

	// MarkRead marks a single notification as read.
	MarkRead(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error

	// MarkAllRead marks all unread notifications as read for a store.
	MarkAllRead(ctx context.Context, db *gorm.DB, storeID uuid.UUID) error

	// --- Customer-scoped (recipient_user_id) — storefront notification bell ---

	// ListByCustomer returns a paginated list of a customer's notifications in a store.
	ListByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, page, perPage int) (ListResult, error)

	// GetUnreadCountByCustomer returns a customer's unread count in a store.
	GetUnreadCountByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) (int64, error)

	// MarkReadByCustomer marks one of a customer's notifications as read.
	MarkReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, id uuid.UUID) error

	// MarkAllReadByCustomer marks all of a customer's unread notifications as read.
	MarkAllReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) error

	// Create inserts a new notification row.
	Create(ctx context.Context, db *gorm.DB, n *Notification) error

	// GetPreferences returns the notification preferences for a store.
	GetPreferences(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*NotificationPreferences, error)

	// UpsertPreferences creates or updates notification preferences for a store.
	UpsertPreferences(ctx context.Context, db *gorm.DB, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error)
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) ListByStore(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Notification{}).Where("store_id = ?", f.StoreID)

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("notification list count: %w", err)
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	perPage := f.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&result.Notifications).Error; err != nil {
		return result, fmt.Errorf("notification list: %w", err)
	}
	return result, nil
}

func (gormRepository) GetUnreadCount(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (int64, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND is_read = false", storeID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("notification unread count: %w", err)
	}
	return count, nil
}

func (gormRepository) MarkRead(ctx context.Context, db *gorm.DB, storeID, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND id = ?", storeID, id).
		Update("is_read", true)
	if res.Error != nil {
		return fmt.Errorf("notification mark read: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("notification")
	}
	return nil
}

func (gormRepository) MarkAllRead(ctx context.Context, db *gorm.DB, storeID uuid.UUID) error {
	if err := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND is_read = false", storeID).
		Update("is_read", true).Error; err != nil {
		return fmt.Errorf("notification mark all read: %w", err)
	}
	return nil
}

func (gormRepository) ListByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, page, perPage int) (ListResult, error) {
	var result ListResult
	q := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND recipient_user_id = ?", storeID, recipientUserID)
	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("notification customer list count: %w", err)
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	if err := q.Order("created_at DESC").Offset(offset).Limit(perPage).Find(&result.Notifications).Error; err != nil {
		return result, fmt.Errorf("notification customer list: %w", err)
	}
	return result, nil
}

func (gormRepository) GetUnreadCountByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) (int64, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND recipient_user_id = ? AND is_read = false", storeID, recipientUserID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("notification customer unread count: %w", err)
	}
	return count, nil
}

func (gormRepository) MarkReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND recipient_user_id = ? AND id = ?", storeID, recipientUserID, id).
		Update("is_read", true)
	if res.Error != nil {
		return fmt.Errorf("notification customer mark read: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("notification")
	}
	return nil
}

func (gormRepository) MarkAllReadByCustomer(ctx context.Context, db *gorm.DB, storeID uuid.UUID, recipientUserID string) error {
	if err := db.WithContext(ctx).Model(&Notification{}).
		Where("store_id = ? AND recipient_user_id = ? AND is_read = false", storeID, recipientUserID).
		Update("is_read", true).Error; err != nil {
		return fmt.Errorf("notification customer mark all read: %w", err)
	}
	return nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, n *Notification) error {
	if err := db.WithContext(ctx).Create(n).Error; err != nil {
		return fmt.Errorf("notification create: %w", err)
	}
	return nil
}

func (gormRepository) GetPreferences(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*NotificationPreferences, error) {
	var p NotificationPreferences
	if err := db.WithContext(ctx).Where("store_id = ?", storeID).First(&p).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFound("notification preferences")
		}
		return nil, fmt.Errorf("notification preferences get: %w", err)
	}
	return &p, nil
}

func (gormRepository) UpsertPreferences(ctx context.Context, db *gorm.DB, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error) {
	p := NotificationPreferences{
		TenantID:    tenantID,
		StoreID:     storeID,
		Preferences: prefs,
		UpdatedAt:   time.Now(),
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "store_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"preferences", "updated_at"}),
		}).
		Create(&p).Error; err != nil {
		return nil, fmt.Errorf("notification preferences upsert: %w", err)
	}
	return &p, nil
}
