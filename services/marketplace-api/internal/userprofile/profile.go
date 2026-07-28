// Package userprofile owns data access for the user_profiles table —
// one row per GIP user, keyed by the GIP `sub`. The row is not owned by
// a tenant: the same person may hold staff roles in several tenants and
// still has exactly one profile.
//
// The table is deliberately small and its access is deliberately behind
// an interface: the admin account handler seeds and mutates it, and the
// tickets/reviews handlers read the display name off it to label
// merchant-authored, customer-visible replies.
package userprofile

import (
	"context"
	"errors"
	"time"
)

// Profile is the GORM model for user_profiles. Keep in sync with
// migrations/000036_user_profiles.up.sql.
//
// Every column is NOT NULL DEFAULT '' in the schema, so the Go zero
// value round-trips cleanly and no field needs to be a pointer.
type Profile struct {
	UserID      string    `gorm:"column:user_id;primaryKey"`
	Email       string    `gorm:"column:email"`
	DisplayName string    `gorm:"column:display_name"`
	Phone       string    `gorm:"column:phone"`
	AvatarURL   string    `gorm:"column:avatar_url"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

// TableName pins the table so GORM's pluraliser can't drift.
func (Profile) TableName() string { return "user_profiles" }

// ErrNotFound is returned by Get and DisplayName when no row exists for
// the user. Callers distinguish "no profile yet" (seed one, or fall back
// to a generic label) from a real storage failure.
var ErrNotFound = errors.New("userprofile: not found")

// Store is the data-access contract the handlers depend on. Keeping the
// handlers on an interface rather than *gorm.DB is what lets the seed
// and author-name behaviour be tested without a live database.
type Store interface {
	// Get returns the profile for userID, or ErrNotFound.
	Get(ctx context.Context, userID string) (Profile, error)
	// Create inserts a new profile row.
	Create(ctx context.Context, p Profile) error
	// Update applies a column→value patch to an existing row. It does
	// not touch updated_at; callers include it in fields.
	Update(ctx context.Context, userID string, fields map[string]any) error
	// Delete removes the row. Deleting a non-existent row is not an error.
	Delete(ctx context.Context, userID string) error
	// DisplayName returns just the stored display name, or ErrNotFound.
	// Split out from Get so the hot label lookup on reply endpoints
	// selects one column instead of the whole row.
	DisplayName(ctx context.Context, userID string) (string, error)
}
