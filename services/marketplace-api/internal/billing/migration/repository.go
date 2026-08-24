// Package migration implements the fast-path review queue for merchants
// migrating from Shopify/WooCommerce/BigCommerce. A merchant submits evidence
// (WHOIS record or platform screenshot) which a CSM reviews; approval shortens
// the tax-ID validation window from 14d to 48h.
package migration

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrAlreadyPending is returned when a pending review already exists for
	// the given store. The unique EXCLUDE constraint surfaces this.
	ErrAlreadyPending = errors.New("migration: pending review already exists for this store")

	// ErrNotFound is returned when a review is not found or is no longer pending.
	ErrNotFound = errors.New("migration: review not found or not pending")
)

// Review mirrors the migration_fast_path_reviews table.
type Review struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID       uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	EvidenceType  string     `gorm:"column:evidence_type;not null"`
	EvidenceURL   string     `gorm:"column:evidence_url;not null"`
	PriorPlatform string     `gorm:"column:prior_platform"`
	WhoisDomain   string     `gorm:"column:whois_domain"`
	Status        string     `gorm:"column:status;not null;default:pending"`
	ReviewerID    *uuid.UUID `gorm:"column:reviewer_id;type:uuid"`
	ReviewerNotes string     `gorm:"column:reviewer_notes"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:now()"`
	ReviewedAt    *time.Time `gorm:"column:reviewed_at"`
}

// TableName pins the GORM table name so the struct can live in any package.
func (Review) TableName() string { return "migration_fast_path_reviews" }

// CreatePendingInput holds the caller-supplied fields for a new review row.
type CreatePendingInput struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	EvidenceType  string
	EvidenceURL   string
	PriorPlatform string
	WhoisDomain   string
}

// Repository holds the DB handle. Matches the pattern used by stores.Repository.
type Repository struct{ db *gorm.DB }

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// CreatePending writes a new row in 'pending' status. The unique-pending
// EXCLUDE constraint surfaces as ErrAlreadyPending.
func (r *Repository) CreatePending(ctx context.Context, in CreatePendingInput) (*Review, error) {
	row := Review{
		ID:            uuid.New(),
		TenantID:      in.TenantID,
		StoreID:       in.StoreID,
		EvidenceType:  in.EvidenceType,
		EvidenceURL:   in.EvidenceURL,
		PriorPlatform: in.PriorPlatform,
		WhoisDomain:   in.WhoisDomain,
		Status:        "pending",
		CreatedAt:     time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniquePendingViolation(err) {
			return nil, ErrAlreadyPending
		}
		return nil, err
	}
	return &row, nil
}

// Get fetches a review by id. Used by the CSM review handler to look up the
// store_id before sending an approval email (when the email package lands).
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Review, error) {
	var row Review
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// Approve marks the review approved and shortens the tax-ID window to 48h
// (from the 14d default) by stamping store_subscriptions.tax_id_window_shortened_at.
// Does NOT waive tax-ID validation — P7 still runs the registry lookup.
//
// Returns the post-update review row so callers (the Review handler) can
// attribute an audit event to the review's tenant/store without a second
// lookup — the row is already in hand from inside this transaction.
func (r *Repository) Approve(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*Review, error) {
	var review Review
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&review, "id = ? AND status = ?", id, "pending").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		now := time.Now().UTC()
		res := tx.Exec(`
			UPDATE migration_fast_path_reviews
			SET status = 'approved', reviewer_id = ?, reviewer_notes = ?, reviewed_at = ?
			WHERE id = ? AND status = 'pending'`,
			reviewerID, strings.TrimSpace(notes), now, id,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		review.Status = "approved"
		review.ReviewerID = &reviewerID
		trimmed := strings.TrimSpace(notes)
		review.ReviewerNotes = trimmed
		review.ReviewedAt = &now

		// Stamp tax_id_window_shortened_at — only if not already set (idempotent).
		return tx.Exec(`
			UPDATE store_subscriptions
			SET tax_id_window_shortened_at = ?, updated_at = now()
			WHERE store_id = ? AND tax_id_window_shortened_at IS NULL`,
			now, review.StoreID,
		).Error
	})
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// Reject marks the review rejected. No side effect on store_subscriptions.
//
// Returns the post-update review row for the same reason Approve does.
func (r *Repository) Reject(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*Review, error) {
	var review Review
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&review, "id = ? AND status = ?", id, "pending").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		now := time.Now().UTC()
		res := tx.Exec(`
			UPDATE migration_fast_path_reviews
			SET status = 'rejected', reviewer_id = ?, reviewer_notes = ?, reviewed_at = ?
			WHERE id = ? AND status = 'pending'`,
			reviewerID, strings.TrimSpace(notes), now, id,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		review.Status = "rejected"
		review.ReviewerID = &reviewerID
		review.ReviewerNotes = strings.TrimSpace(notes)
		review.ReviewedAt = &now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// isUniquePendingViolation detects the EXCLUDE-constraint violation produced by
// the only-one-open-per-store unique key on migration_fast_path_reviews.
// The postgres driver surfaces this as a pq error whose detail contains the
// constraint name "only_one_open_per_store" or the table+column fragment.
func isUniquePendingViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "only_one_open_per_store") ||
		strings.Contains(s, "migration_fast_path_reviews_store_id") ||
		(strings.Contains(s, "duplicate key") && strings.Contains(s, "pending"))
}
