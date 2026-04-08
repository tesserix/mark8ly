package invitation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for invitations.
type Repository interface {
	Create(ctx context.Context, inv *Invitation) error
	GetByTokenHash(ctx context.Context, hash string) (*Invitation, error)
	GetByID(ctx context.Context, id string) (*Invitation, error)
	ListPendingByTenant(ctx context.Context, tenantID string) ([]Invitation, error)
	MarkAccepted(ctx context.Context, id string) error
	MarkRevoked(ctx context.Context, id string) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, inv *Invitation) error {
	if err := r.db.WithContext(ctx).Create(inv).Error; err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict(
				"invitation_exists",
				"a pending invitation already exists for this email",
			)
		}
		return fmt.Errorf("invitation: create: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByTokenHash(ctx context.Context, hash string) (*Invitation, error) {
	var inv Invitation
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("invitation_not_found", "no invitation matches this token")
	}
	if err != nil {
		return nil, fmt.Errorf("invitation: get by token hash: %w", err)
	}
	return &inv, nil
}

func (r *gormRepository) GetByID(ctx context.Context, id string) (*Invitation, error) {
	var inv Invitation
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("invitation_not_found", "no invitation with that id")
	}
	if err != nil {
		return nil, fmt.Errorf("invitation: get by id: %w", err)
	}
	return &inv, nil
}

func (r *gormRepository) ListPendingByTenant(ctx context.Context, tenantID string) ([]Invitation, error) {
	var rows []Invitation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, StatusPending).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("invitation: list pending: %w", err)
	}
	return rows, nil
}

func (r *gormRepository) MarkAccepted(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).
		Model(&Invitation{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":      StatusAccepted,
			"accepted_at": gorm.Expr("NOW()"),
		})
	if err := res.Error; err != nil {
		return fmt.Errorf("invitation: mark accepted: %w", err)
	}
	if res.RowsAffected == 0 {
		return apperrors.Conflict(
			"invitation_not_pending",
			"invitation is no longer pending",
		)
	}
	return nil
}

func (r *gormRepository) MarkRevoked(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).
		Model(&Invitation{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Update("status", StatusRevoked)
	if err := res.Error; err != nil {
		return fmt.Errorf("invitation: mark revoked: %w", err)
	}
	if res.RowsAffected == 0 {
		return apperrors.Conflict(
			"invitation_not_pending",
			"invitation is no longer pending",
		)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
