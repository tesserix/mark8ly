package vendor

import (
	"context"
	"strings"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service is the business-logic layer for vendors. Phase 1 only needs
// EnsureSelfVendor (idempotent create-or-return). Later phases add CRUD
// for marketplace vendor onboarding.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// EnsureSelfVendor is called once per tenant (from onboarding), and
// possibly again from the platform-api backfill CLI. If the tenant
// already has a self-vendor, returns that row unchanged — callers must
// use UpdateSelfVendor (Task 11) to overwrite placeholder names.
func (s *Service) EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	tenantID = strings.TrimSpace(tenantID)
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if tenantID == "" {
		return nil, apperrors.ValidationFailed("tenant_id", "tenant id is required")
	}
	if name == "" {
		return nil, apperrors.ValidationFailed("name", "vendor name is required")
	}
	if slug == "" {
		return nil, apperrors.ValidationFailed("slug", "vendor slug is required")
	}

	existing, err := s.repo.GetSelfByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	v := &Vendor{
		TenantID: tenantID,
		Name:     name,
		Slug:     slug,
		Status:   StatusActive,
		IsSelf:   true,
	}
	if err := s.repo.Create(ctx, v); err != nil {
		// Race: a concurrent caller may have inserted the self-vendor
		// between our Get and Create. Retry the Get — if that now finds
		// one, return it. Otherwise surface the original error.
		if again, err2 := s.repo.GetSelfByTenantID(ctx, tenantID); err2 == nil && again != nil {
			return again, nil
		}
		return nil, err
	}
	return v, nil
}

// UpdateSelfVendor overwrites the name and slug of the tenant's existing
// self-vendor. Used by the platform-api backfill CLI to replace the
// placeholder values written by migration 000027 with real tenant
// identity. Returns NotFound if the tenant has no self-vendor yet.
func (s *Service) UpdateSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	tenantID = strings.TrimSpace(tenantID)
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if tenantID == "" {
		return nil, apperrors.ValidationFailed("tenant_id", "tenant id is required")
	}
	if name == "" {
		return nil, apperrors.ValidationFailed("name", "vendor name is required")
	}
	if slug == "" {
		return nil, apperrors.ValidationFailed("slug", "vendor slug is required")
	}

	v, err := s.repo.GetSelfByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, apperrors.NotFound("self-vendor")
	}
	if err := s.repo.UpdateNameAndSlug(ctx, v.ID, name, slug); err != nil {
		return nil, err
	}
	v.Name = name
	v.Slug = slug
	return v, nil
}
