package notification

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// PushPublisher publishes a device-push event for a merchant notification, so
// the store's admin devices get a banner in addition to the in-app entry.
// Optional — a nil publisher leaves notifications in-app only. Implemented by
// pushevents.Publisher.
type PushPublisher interface {
	PublishPush(ctx context.Context, storeID uuid.UUID, notifType, title, body, deepLink string)
}

// ServiceConfig groups dependencies for the notification service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
	// Pusher, when set, receives every successfully-created merchant
	// notification and fans it out to the store's admin devices. Nil-safe.
	Pusher PushPublisher
}

// Service implements notification CRUD and preference management.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
	pusher PushPublisher
}

// NewService constructs a notification Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
		pusher: cfg.Pusher,
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

// --- Customer-scoped (storefront notification bell) ---

// ListForCustomer returns a customer's notifications in a store.
func (s *Service) ListForCustomer(ctx context.Context, storeID uuid.UUID, recipientUserID string, page, perPage int) (ListResult, error) {
	return s.repo.ListByCustomer(ctx, s.db, storeID, recipientUserID, page, perPage)
}

// GetUnreadCountForCustomer returns a customer's unread count in a store.
func (s *Service) GetUnreadCountForCustomer(ctx context.Context, storeID uuid.UUID, recipientUserID string) (int64, error) {
	return s.repo.GetUnreadCountByCustomer(ctx, s.db, storeID, recipientUserID)
}

// MarkReadForCustomer marks one of a customer's notifications as read.
func (s *Service) MarkReadForCustomer(ctx context.Context, storeID uuid.UUID, recipientUserID string, id uuid.UUID) error {
	return s.repo.MarkReadByCustomer(ctx, s.db, storeID, recipientUserID, id)
}

// MarkAllReadForCustomer marks all of a customer's notifications as read.
func (s *Service) MarkAllReadForCustomer(ctx context.Context, storeID uuid.UUID, recipientUserID string) error {
	return s.repo.MarkAllReadByCustomer(ctx, s.db, storeID, recipientUserID)
}

// Create inserts a new notification, bypassing preference checks. Reserved
// for non-toggleable types like system_alert. Feature code should prefer
// CreateIfEnabled so merchant preferences are honored.
func (s *Service) Create(ctx context.Context, n *Notification) error {
	if err := s.repo.Create(ctx, s.db, n); err != nil {
		return err
	}
	s.emitPush(ctx, n)
	return nil
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
	s.emitPush(ctx, n)
	return true, nil
}

// emitPush fans a freshly-created MERCHANT notification out to the store's
// admin devices. Customer (storefront) notifications carry a recipient and
// are never pushed to admins. Nil-safe on the publisher.
func (s *Service) emitPush(ctx context.Context, n *Notification) {
	if s.pusher == nil || n.RecipientUserID != nil {
		return
	}
	body := ""
	if n.Message != nil {
		body = *n.Message
	}
	s.pusher.PublishPush(ctx, n.StoreID, string(n.Type), n.Title, body, pushDeepLink(n))
}

// pushDeepLink maps a notification's resource to a mobile-admin route the push
// tap can open. Unknown/absent resources yield "" (tap just opens the app).
func pushDeepLink(n *Notification) string {
	if n.ResourceType == nil || n.ResourceID == nil {
		return ""
	}
	switch *n.ResourceType {
	case "order", "return":
		return "/orders/" + n.ResourceID.String()
	case "product":
		return "/products/" + n.ResourceID.String()
	case "review":
		return "/reviews/" + n.ResourceID.String()
	default:
		return ""
	}
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

// defaultPreferences returns the UI fallback prefs for a store with no explicit
// row — matching isTypeEnabled's per-type fallback and the web DEFAULT_PREFS — so
// GetPreferences returns 200 defaults instead of a 404 the mobile client misreads
// as an invalid store (which bounces the user to the dashboard).
func defaultPreferences(storeID uuid.UUID) *NotificationPreferences {
	m := make(map[string]bool, len(uiPreferenceDefaults))
	for t, v := range uiPreferenceDefaults {
		m[string(t)] = v
	}
	raw, _ := json.Marshal(m)
	return &NotificationPreferences{StoreID: storeID, Preferences: raw}
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

// EmitToCustomer is a fire-and-forget helper for customer-facing notifications
// (recipient_user_id set). Customer notifications are not gated by merchant
// preferences, so it inserts directly. Nil-safe on svc + no-op without a
// recipient.
func EmitToCustomer(ctx context.Context, svc *Service, logger *slog.Logger, n Notification) {
	if svc == nil || n.RecipientUserID == nil || *n.RecipientUserID == "" {
		return
	}
	if err := svc.Create(ctx, &n); err != nil && logger != nil {
		logger.Warn("customer notification emit failed",
			"type", string(n.Type),
			"recipient", *n.RecipientUserID,
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

// GetPreferences returns the notification preferences for a store. A store
// with no explicit preferences row yields the UI defaults (200) rather than
// a 404 — see defaultPreferences.
func (s *Service) GetPreferences(ctx context.Context, storeID uuid.UUID) (*NotificationPreferences, error) {
	prefs, err := s.repo.GetPreferences(ctx, s.db, storeID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return defaultPreferences(storeID), nil
		}
		return nil, err
	}
	return prefs, nil
}

// UpsertPreferences creates or updates notification preferences for a store.
func (s *Service) UpsertPreferences(ctx context.Context, storeID, tenantID uuid.UUID, prefs json.RawMessage) (*NotificationPreferences, error) {
	return s.repo.UpsertPreferences(ctx, s.db, storeID, tenantID, prefs)
}
