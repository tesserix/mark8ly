package store

import (
	"context"
	"regexp"
	"strings"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for stores. Phase Q port of
// the tenant service bits that moved to store: slug lookup + slug
// availability + the editable update subset (name; more to come).
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
	Name *string
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
