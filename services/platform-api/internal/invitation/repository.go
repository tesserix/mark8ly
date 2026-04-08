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
	ListAcceptedByTenant(ctx context.Context, tenantID string) ([]Invitation, error)
	FindAcceptedByEmail(ctx context.Context, tenantID, email string) (*Invitation, error)
	MarkAccepted(ctx context.Context, id, acceptedByUserID string) error
	UpdateRoleByEmail(ctx context.Context, tenantID, email, newRole string) error
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

func (r *gormRepository) ListAcceptedByTenant(ctx context.Context, tenantID string) ([]Invitation, error) {
	var rows []Invitation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, StatusAccepted).
		Order("accepted_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("invitation: list accepted: %w", err)
	}
	return rows, nil
}

func (r *gormRepository) MarkAccepted(ctx context.Context, id, acceptedByUserID string) error {
	res := r.db.WithContext(ctx).
		Model(&Invitation{}).
		Where("id = ? AND status = ?", id, StatusPending).
		Updates(map[string]any{
			"status":              StatusAccepted,
			"accepted_at":         gorm.Expr("NOW()"),
			"accepted_by_user_id": acceptedByUserID,
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

// FindAcceptedByEmail finds the most recent accepted invitation for
// the given email on the tenant. Used by the role-change path to
// resolve the target user's UID.
func (r *gormRepository) FindAcceptedByEmail(ctx context.Context, tenantID, email string) (*Invitation, error) {
	var inv Invitation
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND status = ?", tenantID, email, StatusAccepted).
		Order("accepted_at DESC").
		First(&inv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("member_not_found", "no accepted invitation matches this email")
	}
	if err != nil {
		return nil, fmt.Errorf("invitation: find accepted by email: %w", err)
	}
	return &inv, nil
}

// UpdateRoleByEmail changes the role on the most recent accepted
// invitation for the given email on the tenant. Used by the role-
// change path so the team list + audit trail reflect the new role.
func (r *gormRepository) UpdateRoleByEmail(ctx context.Context, tenantID, email, newRole string) error {
	res := r.db.WithContext(ctx).
		Model(&Invitation{}).
		Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND status = ?", tenantID, email, StatusAccepted).
		Update("role", newRole)
	if err := res.Error; err != nil {
		return fmt.Errorf("invitation: update role by email: %w", err)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("member_not_found", "no accepted invitation matches this email")
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
