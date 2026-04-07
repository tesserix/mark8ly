package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for verification tokens.
type Repository interface {
	// Create inserts a new pending token.
	Create(ctx context.Context, t *Token) error
	// LatestForSession returns the most recently created NON-CONSUMED token
	// for a session. Returns NotFound if there is none.
	LatestForSession(ctx context.Context, sessionID string) (*Token, error)
	// IncrementAttempts atomically bumps the attempts counter on a token.
	IncrementAttempts(ctx context.Context, id string) error
	// MarkConsumed atomically sets consumed_at on a token. Returns NotFound
	// if the row was not in a consumable state.
	MarkConsumed(ctx context.Context, id string) error
	// InvalidateForSession sets consumed_at on every outstanding token for
	// the session. Used before issuing a new one to prevent multiple active
	// codes per session (auth-bug #5 from Phase B writeup: prevents stale
	// tokens accumulating).
	InvalidateForSession(ctx context.Context, sessionID string) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, t *Token) error {
	if err := r.db.WithContext(ctx).Create(t).Error; err != nil {
		return fmt.Errorf("verification: create token: %w", err)
	}
	return nil
}

func (r *gormRepository) LatestForSession(ctx context.Context, sessionID string) (*Token, error) {
	var t Token
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND consumed_at IS NULL", sessionID).
		Order("created_at DESC").
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("token_not_found", "no active verification token for this session")
	}
	if err != nil {
		return nil, fmt.Errorf("verification: latest for session %q: %w", sessionID, err)
	}
	return &t, nil
}

func (r *gormRepository) IncrementAttempts(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).
		Model(&Token{}).
		Where("id = ?", id).
		UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if res.Error != nil {
		return fmt.Errorf("verification: increment attempts: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("token_not_found", "verification token does not exist")
	}
	return nil
}

func (r *gormRepository) MarkConsumed(ctx context.Context, id string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&Token{}).
		Where("id = ? AND consumed_at IS NULL", id).
		Update("consumed_at", now)
	if res.Error != nil {
		return fmt.Errorf("verification: mark consumed: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("token_not_found", "verification token is not in a consumable state")
	}
	return nil
}

func (r *gormRepository) InvalidateForSession(ctx context.Context, sessionID string) error {
	now := time.Now()
	if err := r.db.WithContext(ctx).
		Model(&Token{}).
		Where("session_id = ? AND consumed_at IS NULL", sessionID).
		Update("consumed_at", now).Error; err != nil {
		return fmt.Errorf("verification: invalidate for session %q: %w", sessionID, err)
	}
	return nil
}
