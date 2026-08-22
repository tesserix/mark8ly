package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// recoveryDedupWindow is the minimum gap between recovery emails for the same
// abandoned cart. Enforced in-service rather than at the DB layer so the
// reasoning is visible at the call site (and tests can override if needed in
// the future).
const recoveryDedupWindow = 24 * time.Hour

// AbandonedCartRepository is the data-access surface for abandoned_carts.
type AbandonedCartRepository interface {
	List(ctx context.Context, db *gorm.DB, storeID uuid.UUID, limit int) ([]AbandonedCart, error)
	GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*AbandonedCart, error)
	StampRecoverySent(tx *gorm.DB, id uuid.UUID, at time.Time) error
}

type gormAbandonedCartRepository struct{}

// NewAbandonedCartRepository constructs a stateless repository.
func NewAbandonedCartRepository() AbandonedCartRepository {
	return &gormAbandonedCartRepository{}
}

func (gormAbandonedCartRepository) List(ctx context.Context, db *gorm.DB, storeID uuid.UUID, limit int) ([]AbandonedCart, error) {
	var rows []AbandonedCart
	if limit <= 0 {
		limit = 50
	}
	err := db.WithContext(ctx).
		Where("store_id = ? AND converted_order_id IS NULL", storeID).
		Order("last_active_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (gormAbandonedCartRepository) GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*AbandonedCart, error) {
	var c AbandonedCart
	if err := db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("abandoned_cart")
		}
		return nil, err
	}
	return &c, nil
}

func (gormAbandonedCartRepository) StampRecoverySent(tx *gorm.DB, id uuid.UUID, at time.Time) error {
	res := tx.Model(&AbandonedCart{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"recovery_sent_at": at,
			"updated_at":       gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("abandoned_cart")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Service
// -----------------------------------------------------------------------------

// AbandonedCartService coordinates abandoned-cart reads and recovery email
// triggering. The trigger path enforces a 24h dedup window in-service so
// repeated calls (admin retries, scheduler races) cannot spam the customer.
type AbandonedCartService struct {
	db     *gorm.DB
	repo   AbandonedCartRepository
	outbox outbox.Repository
	now    func() time.Time
}

// NewAbandonedCartService constructs a service. now() is injected for tests.
func NewAbandonedCartService(db *gorm.DB, repo AbandonedCartRepository, outboxRepo outbox.Repository) *AbandonedCartService {
	return &AbandonedCartService{db: db, repo: repo, outbox: outboxRepo, now: func() time.Time { return time.Now().UTC() }}
}

// List returns the most recent abandoned (un-converted) carts for a store.
func (s *AbandonedCartService) List(ctx context.Context, storeID uuid.UUID, limit int) ([]AbandonedCart, error) {
	return s.repo.List(ctx, s.db, storeID, limit)
}

// Get returns a single abandoned cart by id.
func (s *AbandonedCartService) Get(ctx context.Context, id uuid.UUID) (*AbandonedCart, error) {
	return s.repo.GetByID(ctx, s.db, id)
}

// TriggerRecoveryEmail enqueues a recovery email outbox event for the cart,
// updating recovery_sent_at to the current time. If recovery_sent_at is
// within the 24h dedup window, returns apperrors.ErrRecoveryTooRecent
// without writing anything.
func (s *AbandonedCartService) TriggerRecoveryEmail(ctx context.Context, cartID uuid.UUID) error {
	c, err := s.repo.GetByID(ctx, s.db, cartID)
	if err != nil {
		return err
	}
	if c.ConvertedOrderID != nil {
		return apperrors.ValidationFailed("converted_order_id", "cart has already converted")
	}
	now := s.now()
	if c.RecoverySentAt != nil && now.Sub(*c.RecoverySentAt) < recoveryDedupWindow {
		return apperrors.RecoveryTooRecent(c.RecoverySentAt.Format(time.RFC3339))
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.StampRecoverySent(tx, cartID, now); err != nil {
			return err
		}
		payload := map[string]any{
			"store_id":          c.StoreID.String(),
			"abandoned_cart_id": cartID.String(),
			"customer_email":    derefString(c.CustomerEmail),
			"recovery_url":      derefString(c.RecoveryURL),
			"item_count":        c.ItemCount,
			"subtotal":          c.Subtotal.String(),
			"currency":          c.CurrencyCode,
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("abandoned_cart: marshal outbox payload: %w", err)
		}
		return s.outbox.EnqueueInTx(ctx, tx, &outbox.OutboxEvent{
			TenantID:    c.TenantID.String(),
			Aggregate:   outbox.AggregateAbandonedCart,
			AggregateID: cartID.String(),
			EventType:   outbox.EventAbandonedCartRecoveryEmail,
			Payload:     datatypes.JSON(b),
		})
	})
}
