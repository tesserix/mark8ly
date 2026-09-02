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
//
// A fresh unsubscribe_token is generated for every call, but ON CONFLICT
// DO NOTHING means it is only ever *stored* on the first insert for a
// given email. Deliberately NOT re-issued on a repeat subscribe: an
// unsubscribe link already sitting in someone's inbox from an earlier
// email embeds the original token, and rotating it here would silently
// break that link the next time the same address re-subscribes. The
// generated-but-discarded token on a conflicting call is harmless — it
// is never persisted or returned to the caller.
func (r *Repository) Subscribe(email, source string) error {
	token, err := GenerateUnsubscribeToken()
	if err != nil {
		return err
	}

	sub := &Subscriber{
		Email:            NormalizeEmail(email),
		Source:           source,
		UnsubscribeToken: token,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoNothing: true,
	}).Create(sub).Error
}

// Unsubscribe deletes the subscriber row whose unsubscribe_token matches
// token, outright. This is an erasure, not a soft-delete: it is what
// makes good on the customererasure declaredExclusions promise that a
// Journal address "still carries an art.17 right, exercised against the
// platform" — the address must stop being held, not gain an
// unsubscribed_at flag while the row (and email) lives on.
//
// A token that matches no row deletes zero rows and returns nil, exactly
// like a token that matches one — the caller cannot distinguish "unknown
// token" from "already unsubscribed" from "just unsubscribed" by the
// return value alone, which is what makes the HTTP layer's uniform 200
// response non-enumerable. The length check below is a cheap short
// circuit for the common "empty or obviously-wrong-shaped token" case;
// it is not a correctness requirement, since a well-formed-but-unknown
// token also just deletes zero rows.
func (r *Repository) Unsubscribe(token string) error {
	if len(token) != UnsubscribeTokenLength {
		return nil
	}
	return r.db.Where("unsubscribe_token = ?", token).Delete(&Subscriber{}).Error
}
