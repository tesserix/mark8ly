// Package storeidentity resolves the public identity of a store — the
// name, slug and contact address that customer-facing email puts in the
// From and Reply-To headers (#718).
//
// It exists because four mailers needed the same three columns and three
// of them were already issuing their own near-identical `stores` lookup
// while missing the fourth column. One joined query, one definition of
// "who this store is to a customer".
//
// The contact address lives in store_branding.support_email. That column
// is deliberately NOT mapped onto branding.StoreBranding: the branding
// Upsert writes with Select("*"), and no admin surface populates the
// column today, so a mapped field would be blanked on the merchant's
// next branding save. Reading it here keeps that hazard off the model.
package storeidentity

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Store is a store's public identity. Every string field is
// merchant-controlled and untrusted; email.StoreIdentity sanitises them
// before any of it reaches a header.
type Store struct {
	TenantID string
	Name     string
	Slug     string
	// ContactEmail is store_branding.support_email, empty when the
	// merchant has set none or has no branding row at all.
	ContactEmail string
}

// Loader resolves a store id to its public identity. An interface so
// mailers can be unit-tested without a database.
type Loader interface {
	Load(ctx context.Context, storeID uuid.UUID) (Store, error)
}

// DBLoader is the production Loader, backed by one LEFT JOIN.
type DBLoader struct{ db *gorm.DB }

// NewDBLoader constructs a Loader over the given connection.
func NewDBLoader(db *gorm.DB) *DBLoader { return &DBLoader{db: db} }

// Load returns the store's public identity.
//
// A store id with no row is NOT an error: it returns the zero Store, and
// email.StoreIdentity degrades that to the platform identity. Every
// caller is a best-effort mailer, and failing an order confirmation
// because a store row is missing would trade a cosmetic problem for a
// lost transactional email.
func (l *DBLoader) Load(ctx context.Context, storeID uuid.UUID) (Store, error) {
	var s Store
	err := l.db.WithContext(ctx).Raw(`
		SELECT s.tenant_id      AS tenant_id,
		       s.name           AS name,
		       s.slug           AS slug,
		       COALESCE(sb.support_email, '') AS contact_email
		FROM stores s
		LEFT JOIN store_branding sb ON sb.store_id = s.id
		WHERE s.id = ?
		LIMIT 1`, storeID).Scan(&s).Error
	if err != nil {
		return Store{}, fmt.Errorf("storeidentity: load store %s: %w", storeID, err)
	}
	return s, nil
}

// StaticLoader returns a fixed identity. For tests and for call sites
// that already hold the store row.
type StaticLoader struct{ Store Store }

// Load returns the configured Store.
func (l StaticLoader) Load(context.Context, uuid.UUID) (Store, error) { return l.Store, nil }
