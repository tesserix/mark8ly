package tenant

import (
	"context"
	"regexp"
	"strings"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for tenants.
//
// Most tenant operations originate from onboarding completion (which uses
// CreateInTx directly inside its own transaction). The Service exposes
// reads and validation helpers used by other domains and by handlers.
type Service struct {
	repo Repository
}

// NewService constructs a Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetByID returns a tenant by UUID.
func (s *Service) GetByID(ctx context.Context, id string) (*Tenant, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	return s.repo.GetByID(ctx, id)
}

// GetByOwnerUserID returns the tenant owned by the given GIP UID.
//
// Used by the returning-user sign-in flow: after Identity Toolkit hands
// us a fresh id_token, the server action looks up which Mark8ly tenant
// the UID owns so it can call auth-bff /auth/auto-login with a concrete
// workspace_tenant.
func (s *Service) GetByOwnerUserID(ctx context.Context, uid string) (*Tenant, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, apperrors.BadRequest("invalid_uid", "uid is required")
	}
	return s.repo.GetByOwnerUserID(ctx, uid)
}

// GetBySlug returns a tenant by its public slug.
//
// We TRIM whitespace but do NOT lowercase. The slug is the canonical
// public identifier and the user must commit to a single casing. Accepting
// "Acme" silently as "acme" would let two URLs (acme.mark8ly.com and
// Acme.mark8ly.com) point to the same tenant — confusing for SEO and
// analytics. Uppercase input is rejected, not normalized.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	slug = strings.TrimSpace(slug)
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	return s.repo.GetBySlug(ctx, slug)
}

// IsSlugAvailable returns true if the slug is well-formed and not yet taken.
// Used by the onboarding wizard's "check slug" step before final submission.
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

// validateSlug enforces the public slug rules:
//   - 3 to 63 characters
//   - lowercase alphanumeric, hyphens allowed
//   - cannot start or end with a hyphen
//
// Matches the CHECK constraint in migrations/0003_create_tenants.up.sql.
// Uppercase input is REJECTED, not normalized — see GetBySlug rationale.
func validateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return apperrors.BadRequest("invalid_slug",
			"slug must be 3-63 lowercase alphanumeric characters and hyphens, not starting or ending with a hyphen")
	}
	return nil
}

// slugPattern: 3 to 63 chars, lowercase alphanumeric + hyphens, no edge hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
