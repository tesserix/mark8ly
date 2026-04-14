package store

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/authz"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for stores. Phase Q port of
// the tenant service bits that moved to store: slug lookup + slug
// availability + the editable update subset (name; more to come).
//
// Phase Q.2 added Create for the "add a second store" flow. The
// FGA client is used to write the `tenant:<tid> parent store:<sid>`
// tuple immediately after insert so the new store inherits all
// tenant-level permissions via the DSL's `from parent`. The write
// is best-effort — a failure leaves the store row in place without
// FGA inheritance, and the merchant can delete and retry.
type Service struct {
	repo Repository
	fga  authz.Client
}

func NewService(repo Repository, fga authz.Client) *Service {
	return &Service{repo: repo, fga: fga}
}

// CreateInput is the payload for creating a new store under an
// existing tenant. Used by the "Add store" flow in admin. Onboarding
// uses Repository.CreateInTx directly because it creates the tenant
// and the default store in the same transaction.
type CreateInput struct {
	TenantID     string
	Slug         string
	Name         string
	CountryCode  string
	CurrencyCode string
	Timezone     string
}

// Create validates the input and inserts a new store row. Runs
// outside a transaction because "add a second store" doesn't have
// the cross-row atomicity needs that onboarding completion does.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Store, error) {
	tenantID := strings.TrimSpace(in.TenantID)
	slug := strings.TrimSpace(in.Slug)
	name := strings.TrimSpace(in.Name)
	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, apperrors.BadRequest("invalid_name", "store name is required")
	}
	if len(name) > 200 {
		return nil, apperrors.BadRequest("invalid_name", "store name must be 200 characters or fewer")
	}
	if len(in.CountryCode) != 2 {
		return nil, apperrors.BadRequest("invalid_country_code", "country code must be a 2-letter ISO code")
	}
	if len(in.CurrencyCode) != 3 {
		return nil, apperrors.BadRequest("invalid_currency_code", "currency code must be a 3-letter ISO 4217 code")
	}
	if strings.TrimSpace(in.Timezone) == "" {
		return nil, apperrors.BadRequest("invalid_timezone", "timezone is required")
	}

	row := &Store{
		TenantID:        tenantID,
		Slug:            slug,
		Name:            name,
		CountryCode:     in.CountryCode,
		CurrencyCode:    in.CurrencyCode,
		Timezone:        in.Timezone,
		StorefrontTheme: json.RawMessage(`{}`),
		Status:          StatusActive,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	// Write the FGA parent tuple so tenant-level owners/admins
	// automatically inherit store-level permissions on the new
	// store. Best-effort: a failed write returns the store row
	// anyway so the admin UI can retry, and the tenant owner can
	// still access the store via tenant-level routes.
	if s.fga != nil {
		_ = s.fga.WriteStoreParent(ctx, row.ID, row.TenantID)
	}
	return row, nil
}

// GetByID returns a store by UUID.
func (s *Service) GetByID(ctx context.Context, id string) (*Store, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_store_id", "store id is required")
	}
	return s.repo.GetByID(ctx, id)
}

// GetBySlug returns a store by its public slug. Whitespace is
// trimmed but uppercase is NOT silently normalised — see the
// legacy tenant.GetBySlug rationale for why.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Store, error) {
	slug = strings.TrimSpace(slug)
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	return s.repo.GetBySlug(ctx, slug)
}

// IsSlugAvailable is called by the onboarding wizard's "check slug"
// step and by the future add-store flow. Returns true iff the slug
// is well-formed and not yet taken by any store across all tenants.
func (s *Service) IsSlugAvailable(ctx context.Context, slug string) (bool, error) {
	slug = strings.TrimSpace(slug)
	if err := validateSlug(slug); err != nil {
		return false, err
	}
	taken, err := s.repo.SlugExists(ctx, slug)
	if err != nil {
		return false, err
	}
	return !taken, nil
}

// ListByTenant returns every store under the given tenant.
func (s *Service) ListByTenant(ctx context.Context, tenantID string) ([]Store, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	return s.repo.ListByTenant(ctx, tenantID)
}

// UpdateInput is the editable subset of a store row.
//
// Phase Q ships with only Name editable — same as Phase N's tenant
// edit scope. Currency/timezone/country stay read-only until a
// dedicated slice lands because each has ripple effects (billing /
// picker UX / tax).
type UpdateInput struct {
	Name            *string
	Timezone        *string
	StorefrontTheme json.RawMessage
}

// Update applies an UpdateInput to the store row and returns the
// updated store. Validates each provided field; omitted fields are
// left untouched.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*Store, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_store_id", "store id is required")
	}

	patch := map[string]any{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperrors.BadRequest("invalid_name", "store name cannot be empty")
		}
		if len(name) > 200 {
			return nil, apperrors.BadRequest("invalid_name", "store name must be 200 characters or fewer")
		}
		patch["name"] = name
	}
	if in.Timezone != nil {
		tz := strings.TrimSpace(*in.Timezone)
		if tz == "" {
			return nil, apperrors.BadRequest("invalid_timezone", "timezone cannot be empty")
		}
		if len(tz) > 64 {
			return nil, apperrors.BadRequest("invalid_timezone", "timezone must be 64 characters or fewer")
		}
		// IANA source of truth: reject anything Go's tzdata can't load.
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, apperrors.BadRequest("invalid_timezone", "timezone is not a valid IANA identifier")
		}
		patch["timezone"] = tz
	}
	if len(in.StorefrontTheme) > 0 {
		if !json.Valid(in.StorefrontTheme) {
			return nil, apperrors.BadRequest("invalid_storefront_theme", "storefront theme must be valid JSON")
		}
		patch["storefront_theme"] = in.StorefrontTheme
	}

	if len(patch) == 0 {
		return nil, apperrors.BadRequest("empty_update", "no editable fields provided")
	}
	return s.repo.UpdateEditable(ctx, id, patch)
}

// validateSlug enforces the public slug rules:
//
//	3 to 63 characters, lowercase alphanumeric + hyphens, no edge
//	hyphens. Matches the CHECK constraint in migration 0008.
func validateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return apperrors.BadRequest("invalid_slug",
			"slug must be 3-63 lowercase alphanumeric characters and hyphens, not starting or ending with a hyphen")
	}
	return nil
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
