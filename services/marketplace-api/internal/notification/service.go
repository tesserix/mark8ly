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

// Create inserts a new notification, bypassing preference checks. Reserved
// for non-toggleable types like system_alert. Feature code should prefer
// CreateIfEnabled so merchant preferences are honored.
func (s *Service) Create(ctx context.Context, n *Notification) error {
	return s.repo.Create(ctx, s.db, n)
}

// CreateIfEnabled inserts a notification only when the store's preferences
// have the matching type enabled. Missing preferences fall back to the
// defaults set by the UI (NotificationSettingsClient DEFAULT_PREFS):
// new_order=true, low_stock=true, return_requested=true,
// payment_received=false, review_submitted=false. Returns (created, error).
func (s *Service) CreateIfEnabled(ctx context.Context, n *Notification) (bool, error) {
	if enabled, err := s.isTypeEnabled(ctx, n.StoreID, n.Type); err != nil {
		return false, err
	} else if !enabled {
		return false, nil
	}
	if err := s.repo.Create(ctx, s.db, n); err != nil {
		return false, err
	}
	return true, nil
}

// uiPreferenceDefaults mirrors DEFAULT_PREFS in
// apps/admin/components/settings/NotificationSettingsClient.tsx — the
// fallback when a store has no preferences row yet.
var uiPreferenceDefaults = map[NotificationType]bool{
	TypeNewOrder:        true,
	TypeLowStock:        true,
	TypeReturnRequested: true,
	TypePaymentReceived: false,
	TypeReviewSubmitted: false,
}

// Emit is a fire-and-forget helper for feature code: checks preferences
// via CreateIfEnabled, logs on failure, never panics. Nil-safe on svc so
// handlers that boot without a notification service (tests, pure
// storefront mode) can unconditionally call Emit without guarding.
func Emit(ctx context.Context, svc *Service, logger *slog.Logger, n Notification) {
	if svc == nil {
		return
	}
	if _, err := svc.CreateIfEnabled(ctx, &n); err != nil && logger != nil {
		logger.Warn("notification emit failed",
			"type", string(n.Type),
			"store_id", n.StoreID.String(),
			"err", err.Error())
	}
}

// isTypeEnabled returns whether the given notification type should be
// delivered for a store. Non-toggleable types (anything not in
// uiPreferenceDefaults) are always enabled so critical messages still
// flow even when a merchant never touches preferences.
func (s *Service) isTypeEnabled(ctx context.Context, storeID uuid.UUID, t NotificationType) (bool, error) {
	fallback, toggleable := uiPreferenceDefaults[t]
	if !toggleable {
		return true, nil
	}

	prefs, err := s.repo.GetPreferences(ctx, s.db, storeID)
	if err != nil {
		// No row yet — use the UI fallback.
		return fallback, nil
	}

	var parsed map[string]bool
	if unmarshalErr := json.Unmarshal(prefs.Preferences, &parsed); unmarshalErr != nil {
		return fallback, nil
	}
	if val, ok := parsed[string(t)]; ok {
		return val, nil
	}
	return fallback, nil
}

// GetPreferences returns the notification preferences for a store.
func (s *Service) GetPreferences(ctx context.Context, storeID uuid.UUID) (*NotificationPreferences, error) {
	return s.repo.GetPreferences(ctx, s.db, storeID)
}

// UpsertPreferences creates or updates notification preferences for a store.
func (s *Service) UpsertPreferences(ctx context.Context, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error) {
	return s.repo.UpsertPreferences(ctx, s.db, storeID, tenantID, prefs)
}
