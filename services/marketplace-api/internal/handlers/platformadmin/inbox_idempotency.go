package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InboxActionIdempotencyTTL bounds how long a key is remembered.
//
// Long enough to cover any realistic client retry (including an operator
// reloading and resubmitting), short enough that the table does not grow
// without bound. A key presented after this window is treated as new, which
// is the correct trade: the alternative is refusing a legitimate action
// forever because an identical key was used once, days ago.
const InboxActionIdempotencyTTL = 24 * time.Hour

// InboxActionRecord is one remembered action execution.
type InboxActionRecord struct {
	Key        string          `gorm:"column:idempotency_key;primaryKey"`
	Kind       string          `gorm:"column:kind;not null"`
	ItemID     string          `gorm:"column:item_id;not null"`
	ActionID   string          `gorm:"column:action_id;not null"`
	OperatorID string          `gorm:"column:operator_id;not null"`
	Outcome    json.RawMessage `gorm:"column:outcome;type:jsonb;not null"`
	CreatedAt  time.Time       `gorm:"column:created_at;not null;default:now()"`
	ExpiresAt  time.Time       `gorm:"column:expires_at;not null"`
}

// TableName pins the table so GORM's pluralizer can't drift.
func (InboxActionRecord) TableName() string { return "inbox_action_idempotency" }

// InboxActionIdempotency remembers destructive action executions so a retry
// after a client timeout answers identically instead of firing twice.
//
// This cannot be delegated to the domain writes. migration.Repository.Approve
// matches on `status = 'pending'` and returns ErrNotFound on a second call —
// a failure response for an operation that in fact succeeded, which would show
// an operator an error for work that was done.
type InboxActionIdempotency interface {
	// Claim reserves the key. first=true means this caller owns the
	// execution. first=false returns the record the original attempt stored.
	Claim(ctx context.Context, rec InboxActionRecord) (first bool, existing *InboxActionRecord, err error)
	// Complete stores the outcome of an execution this caller owns.
	Complete(ctx context.Context, key string, outcome json.RawMessage) error
}

type gormInboxActionIdempotency struct{ db *gorm.DB }

// NewInboxActionIdempotency constructs a Postgres-backed store. The database
// is the only shared state on this path: mark8ly runs multiple replicas, so an
// in-memory map would let a retry routed to another pod through.
func NewInboxActionIdempotency(db *gorm.DB) InboxActionIdempotency {
	return &gormInboxActionIdempotency{db: db}
}

func (s *gormInboxActionIdempotency) Claim(ctx context.Context, rec InboxActionRecord) (bool, *InboxActionRecord, error) {
	if rec.Outcome == nil {
		rec.Outcome = json.RawMessage(`{}`)
	}
	if rec.ExpiresAt.IsZero() {
		rec.ExpiresAt = time.Now().UTC().Add(InboxActionIdempotencyTTL)
	}

	// ON CONFLICT DO NOTHING makes the primary key the check — no
	// read-then-write race to lose. The database decides who wins.
	res := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&rec)
	if res.Error != nil {
		return false, nil, fmt.Errorf("platformadmin: claim inbox action key: %w", res.Error)
	}
	if res.RowsAffected == 1 {
		return true, nil, nil
	}

	var existing InboxActionRecord
	if err := s.db.WithContext(ctx).
		Where("idempotency_key = ?", rec.Key).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The row expired and was swept between the failed insert and
			// this read. Refusing is the safe answer: this caller cannot know
			// whether the original execution happened.
			return false, nil, fmt.Errorf("platformadmin: inbox action key vanished during claim")
		}
		return false, nil, fmt.Errorf("platformadmin: load inbox action key: %w", err)
	}
	return false, &existing, nil
}

func (s *gormInboxActionIdempotency) Complete(ctx context.Context, key string, outcome json.RawMessage) error {
	if outcome == nil {
		outcome = json.RawMessage(`{}`)
	}
	err := s.db.WithContext(ctx).
		Model(&InboxActionRecord{}).
		Where("idempotency_key = ?", key).
		Update("outcome", outcome).Error
	if err != nil {
		return fmt.Errorf("platformadmin: complete inbox action key: %w", err)
	}
	return nil
}
