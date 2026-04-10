package notification

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ServiceConfig groups dependencies for the notification service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// Service implements notification CRUD and preference management.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a notification Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}

// List returns a paginated list of notifications for a store.
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	return s.repo.ListByStore(ctx, s.db, f)
}

// GetUnreadCount returns the number of unread notifications for a store.
func (s *Service) GetUnreadCount(ctx context.Context, storeID uuid.UUID) (int64, error) {
	return s.repo.GetUnreadCount(ctx, s.db, storeID)
}

// MarkRead marks a single notification as read.
func (s *Service) MarkRead(ctx context.Context, storeID, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, s.db, storeID, id)
}

// MarkAllRead marks all unread notifications as read for a store.
func (s *Service) MarkAllRead(ctx context.Context, storeID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, s.db, storeID)
}

// Create inserts a new notification.
func (s *Service) Create(ctx context.Context, n *Notification) error {
	return s.repo.Create(ctx, s.db, n)
}

// GetPreferences returns the notification preferences for a store.
func (s *Service) GetPreferences(ctx context.Context, storeID uuid.UUID) (*NotificationPreferences, error) {
	return s.repo.GetPreferences(ctx, s.db, storeID)
}

// UpsertPreferences creates or updates notification preferences for a store.
func (s *Service) UpsertPreferences(ctx context.Context, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error) {
	return s.repo.UpsertPreferences(ctx, s.db, storeID, tenantID, prefs)
}
