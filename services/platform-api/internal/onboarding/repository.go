package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for onboarding sessions.
type Repository interface {
	Create(ctx context.Context, s *Session) error
	GetByID(ctx context.Context, id string) (*Session, error)
	UpdateDraft(ctx context.Context, id string, draft json.RawMessage) error
	MarkEmailVerified(ctx context.Context, id string) error
	// CompleteInTx marks the session completed and links it to a tenant.
	// Called inside the onboarding completion transaction.
	CompleteInTx(ctx context.Context, tx *gorm.DB, id, tenantID string) error
	// GetFunnel returns the onboarding funnel counters for the given
	// window (see funnel.go).
	GetFunnel(ctx context.Context, f FunnelFilter) (*FunnelStats, error)
	// ListSessions returns a page of onboarding sessions for the given
	// window/filter, plus the unpaginated total (see funnel.go).
	ListSessions(ctx context.Context, f FunnelFilter) ([]SessionRow, int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, s *Session) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("onboarding: create session: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByID(ctx context.Context, id string) (*Session, error) {
	var s Session
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("session_not_found", fmt.Sprintf("onboarding session %q does not exist", id))
	}
	if err != nil {
		return nil, fmt.Errorf("onboarding: get session %q: %w", id, err)
	}
	return &s, nil
}

func (r *gormRepository) UpdateDraft(ctx context.Context, id string, draft json.RawMessage) error {
	res := r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ? AND status IN ?", id, []string{StatusInProgress, StatusVerifying}).
		Updates(map[string]any{
			"draft":            draft,
			"last_activity_at": time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("onboarding: update draft: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("session_not_found", "session does not exist or is not editable")
	}
	return nil
}

func (r *gormRepository) MarkEmailVerified(ctx context.Context, id string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&Session{}).
		Where("id = ? AND email_verified_at IS NULL", id).
		Updates(map[string]any{
			"email_verified_at": now,
			"last_activity_at":  now,
		})
	if res.Error != nil {
		return fmt.Errorf("onboarding: mark verified: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Either the session doesn't exist, or email_verified_at is already
		// set. Both are non-fatal; treat as success (idempotent).
		return nil
	}
	return nil
}

func (r *gormRepository) CompleteInTx(ctx context.Context, tx *gorm.DB, id, tenantID string) error {
	now := time.Now()
	res := tx.WithContext(ctx).
		Model(&Session{}).
		Where("id = ? AND status <> ?", id, StatusCompleted).
		Updates(map[string]any{
			"status":           StatusCompleted,
			"tenant_id":        tenantID,
			"completed_at":     now,
			"last_activity_at": now,
		})
	if res.Error != nil {
		return fmt.Errorf("onboarding: complete in tx: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.Conflict("already_completed", "onboarding session is already completed")
	}
	return nil
}
