package journal

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository provides data access for journal subscribers.
type Repository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Subscribe inserts a subscriber, normalizing the email first. Re-
// subscribing an address that already exists is a no-op — ON CONFLICT DO
// NOTHING against the unique index on email — so callers (and, in turn,
// the HTTP handler) can treat every syntactically valid submission as
// success without ever learning whether the address was already present.
func (r *Repository) Subscribe(email, source string) error {
	sub := &Subscriber{
		Email:  NormalizeEmail(email),
		Source: source,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoNothing: true,
	}).Create(sub).Error
}
