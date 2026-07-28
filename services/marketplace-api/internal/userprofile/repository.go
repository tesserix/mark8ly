package userprofile

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Repository is the GORM-backed Store.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository. db must not be nil.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// compile-time proof the concrete type satisfies the contract.
var _ Store = (*Repository)(nil)

// Get returns the profile for userID, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("userprofile: get: %w", err)
	}
	return p, nil
}

// Create inserts a new profile row.
func (r *Repository) Create(ctx context.Context, p Profile) error {
	if err := r.db.WithContext(ctx).Create(&p).Error; err != nil {
		return fmt.Errorf("userprofile: create: %w", err)
	}
	return nil
}

// Update applies a column→value patch to an existing row.
func (r *Repository) Update(ctx context.Context, userID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).
		Model(&Profile{}).
		Where("user_id = ?", userID).
		Updates(fields).Error
	if err != nil {
		return fmt.Errorf("userprofile: update: %w", err)
	}
	return nil
}

// Delete removes the row for userID.
func (r *Repository) Delete(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&Profile{}).Error
	if err != nil {
		return fmt.Errorf("userprofile: delete: %w", err)
	}
	return nil
}

// DisplayName returns the stored display name for userID, or ErrNotFound
// when the user has no profile row yet. An existing row with a blank
// name returns ("", nil) — that is a real answer, not a miss.
func (r *Repository) DisplayName(ctx context.Context, userID string) (string, error) {
	var p Profile
	err := r.db.WithContext(ctx).
		Select("display_name").
		Where("user_id = ?", userID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("userprofile: display name: %w", err)
	}
	return p.DisplayName, nil
}
