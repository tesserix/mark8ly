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
	// GetByHash looks up a token by its SHA-256 hash. Returns the row even
	// if consumed/expired so the caller can produce a precise error.
	GetByHash(ctx context.Context, hash string) (*Token, error)
	// MarkConsumed atomically sets consumed_at on a token. Returns NotFound
	// if the row was not in a consumable state.
	MarkConsumed(ctx context.Context, id string) error
	// InvalidateForSession sets consumed_at on every outstanding token for
	// the session. Used before issuing a new one so only the latest magic
	// link is valid (e.g. after the user clicks "resend").
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

func (r *gormRepository) GetByHash(ctx context.Context, hash string) (*Token, error) {
	var t Token
	// We look up by code_hash directly. Because the hash is SHA-256 of a
	// high-entropy random token, equality lookup is safe and fast (and we
	// have an index from the migration). bcrypt would not work here because
	// its salt makes equality lookups impossible.
	err := r.db.WithContext(ctx).Where("code_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("token_not_found", "verification link is invalid or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("verification: get by hash: %w", err)
	}
	return &t, nil
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
