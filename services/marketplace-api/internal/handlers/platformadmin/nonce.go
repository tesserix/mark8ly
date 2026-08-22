package platformadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Nonce is a single-use marker for a signed platform request.
type Nonce struct {
	Nonce     uuid.UUID `gorm:"column:nonce;type:uuid;primaryKey"`
	SeenAt    time.Time `gorm:"column:seen_at;not null;default:now()"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
}

// TableName pins the table so GORM's pluralizer can't drift.
func (Nonce) TableName() string { return "platform_request_nonces" }

// NonceStore records nonces so a captured request cannot be replayed inside
// its validity window.
type NonceStore interface {
	// Claim records the nonce and reports whether this was its first use.
	// False means replay. An error means the check could not be performed —
	// callers must treat that as a rejection, never as a pass.
	Claim(ctx context.Context, nonce string, expiresAt time.Time) (bool, error)
}

type gormNonceStore struct{ db *gorm.DB }

// NewNonceStore constructs a Postgres-backed NonceStore. The database is the
// only shared state on this path: mark8ly runs on Knative at 0-5 replicas, so
// an in-memory cache would let a replay routed to another pod through.
func NewNonceStore(db *gorm.DB) NonceStore { return &gormNonceStore{db: db} }

func (s *gormNonceStore) Claim(ctx context.Context, nonce string, expiresAt time.Time) (bool, error) {
	parsed, err := uuid.Parse(nonce)
	if err != nil {
		return false, fmt.Errorf("platformadmin: nonce must be a uuid: %w", err)
	}

	// ON CONFLICT DO NOTHING makes the unique constraint itself the replay
	// check — no read-then-write race to lose. The database decides who wins.
	res := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Nonce{Nonce: parsed, ExpiresAt: expiresAt})

	if res.Error != nil {
		return false, fmt.Errorf("platformadmin: claim nonce: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}
