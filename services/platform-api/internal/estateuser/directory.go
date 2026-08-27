// Package estateuser serves the platform console's estate-wide staff
// directory (#278).
//
// # There is no users table
//
// Identity in this estate is DERIVED, not stored: a person is staff because
// they own a tenant (tenants.owner_email) or because they accepted an
// invitation to one (invitations). Nothing records a person independently of
// a tenant, which is why this package unions two sources rather than reading
// one.
//
// # Scope: staff and operators, NOT merchants' end customers
//
// Deliberate, per #278. The console's end-user lookup is a per-product opt-in
// recorded in its own EstateProduct.endUserLookup field, and mark8ly has never
// declared itself in. Customer rows (customer_profiles and friends, which live
// in marketplace-api's database, not this one) must never appear here. The
// separation is structural rather than a filter: this package cannot reach
// that database at all.
package estateuser

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// DefaultPageSize applies when the caller sends no limit.
const DefaultPageSize = 50

// MaxPageSize caps a page, mirroring the tenant directory's ceiling.
const MaxPageSize = 200

// User is one person in the estate, aggregated across every tenant they
// belong to.
//
// One row per PERSON, not per membership: the console's global search looks
// for someone, and returning the same email three times because they belong
// to three tenants is a worse answer than one row saying so.
type User struct {
	Email string `json:"email"`
	// UserID is the GIP uid where one is known. Owners always have one;
	// an invitee has one only once they have signed in, so this is
	// legitimately empty for a recently-accepted invitation.
	UserID string `json:"user_id,omitempty"`
	// Roles is the distinct set this person holds across the estate,
	// comma-separated and sorted. "owner" is a role here even though it is
	// not one of the invitations_role_valid values, because from the
	// console's point of view owning a tenant is how most people are staff.
	Roles string `json:"roles"`
	// TenantName is one tenant this person belongs to. With TenantCount > 1
	// it is a sample, chosen deterministically (lowest name) so repeated
	// requests agree.
	TenantName  string `json:"tenant_name"`
	TenantCount int64  `json:"tenant_count"`
}

// Filter narrows the directory. Q matches email and tenant name.
type Filter struct {
	Q     string
	Page  int
	Limit int
}

// Result is a page of users plus the unpaginated total.
type Result struct {
	Users []User
	Total int64
}

// Repository reads the estate staff directory.
type Repository interface {
	List(ctx context.Context, f Filter) (Result, error)
}

type gormRepository struct{ db *gorm.DB }

// NewRepository constructs a Postgres-backed repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// peopleCTE is the union of the two identity sources.
//
// Emails are lower-cased on both sides so the GROUP BY treats
// Owner@example.com and owner@example.com as one person — invitations are
// typed by humans and tenants.owner_email arrives from a signup form.
//
// The owner-self-invite exclusion mirrors invitation.ListMembers: an owner who
// somehow accepted an invitation to their own tenant is one person on one
// tenant, not two memberships.
const peopleCTE = `
WITH people AS (
    SELECT lower(t.owner_email) AS email,
           t.owner_user_id      AS user_id,
           'owner'              AS role,
           t.id                 AS tenant_id,
           t.name               AS tenant_name
    FROM tenants t
    WHERE coalesce(t.owner_email, '') <> ''
    UNION ALL
    SELECT lower(i.email),
           i.accepted_by_user_id,
           i.role,
           i.tenant_id,
           t.name
    FROM invitations i
    JOIN tenants t ON t.id = i.tenant_id
    WHERE i.status = 'accepted'
      AND lower(i.email) <> lower(coalesce(t.owner_email, ''))
),
matched AS (
    SELECT * FROM people
    WHERE (? = '' OR email ILIKE ? OR tenant_name ILIKE ?)
),
grouped AS (
    SELECT email,
           min(user_id)                                   AS user_id,
           string_agg(DISTINCT role, ',' ORDER BY role)   AS roles,
           min(tenant_name)                               AS tenant_name,
           count(DISTINCT tenant_id)                      AS tenant_count
    FROM matched
    GROUP BY email
)`

func (r *gormRepository) List(ctx context.Context, f Filter) (Result, error) {
	// Allocate before scanning: a nil slice marshals to null downstream,
	// which defeats a caller's `?? []`.
	result := Result{Users: make([]User, 0)}

	q := strings.TrimSpace(f.Q)
	like := "%" + q + "%"

	if err := r.db.WithContext(ctx).
		Raw(peopleCTE+` SELECT count(*) FROM grouped`, q, like, like).
		Scan(&result.Total).Error; err != nil {
		return result, fmt.Errorf("estateuser: count: %w", err)
	}

	page := max(f.Page, 1)
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}

	// ORDER BY email, not by anything derived: it is the group key, so it is
	// unique here and paging is stable without a tiebreaker.
	if err := r.db.WithContext(ctx).
		Raw(peopleCTE+`
			SELECT email, coalesce(user_id, '') AS user_id, roles,
			       coalesce(tenant_name, '') AS tenant_name, tenant_count
			FROM grouped ORDER BY email LIMIT ? OFFSET ?`,
			q, like, like, limit, (page-1)*limit).
		Scan(&result.Users).Error; err != nil {
		return result, fmt.Errorf("estateuser: list: %w", err)
	}
	return result, nil
}
