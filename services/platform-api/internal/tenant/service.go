package tenant

import (
	"context"
	"strings"

	"github.com/mark8ly/platform-api/internal/authz"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for tenants.
//
// Most tenant operations originate from onboarding completion (which uses
// CreateInTx directly inside its own transaction). The Service exposes
// reads and validation helpers used by other domains and by handlers.
//
// Phase P: fga is optional. When set, the service can answer
// "which tenants does this user belong to" by combining FGA
// ListObjects with a repo lookup. When nil (dev without OpenFGA),
// ListMemberTenants degrades to returning the single owner tenant
// via GetByOwnerUserID so local iteration still works.
type Service struct {
	repo Repository
	fga  authz.Client
}

// NewService constructs a Service. fga may be nil — see the
// Service doc comment for the degraded-mode semantics.
func NewService(repo Repository, fga authz.Client) *Service {
	return &Service{repo: repo, fga: fga}
}

// Membership is the admin-facing view of "a tenant I belong to".
// Returned by ListMemberTenants and serialised directly to JSON.
// Phase Q: slug moved to store-level, so a tenant no longer has a
// single URL identity — the tenant switcher renders the tenant
// name, and a separate store switcher handles store selection.
type Membership struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// ListMemberTenants returns every tenant the user has a role on,
// enriched with the tenant's name/slug from Postgres and the role
// from FGA.
//
// Implementation: one FGA ListObjects + one batched SQL lookup + N
// FGA GetRole calls (one per returned tenant). For the common solo
// founder case that's 2 FGA calls and 1 SQL query. The per-tenant
// GetRole calls are acceptable for Phase P (a typical user is in
// 1–5 tenants); if multi-tenant users become common, add an
// FGA batched-read or cache the role in the tenant list.
//
// If fga is nil, falls back to GetByOwnerUserID so dev without
// OpenFGA still sees their owner tenant.
func (s *Service) ListMemberTenants(ctx context.Context, uid string) ([]Membership, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, apperrors.BadRequest("invalid_uid", "uid is required")
	}

	if s.fga == nil {
		t, err := s.repo.GetByOwnerUserID(ctx, uid)
		if err != nil {
			if ae, ok := apperrors.As(err); ok && ae.Code == "tenant_not_found" {
				return []Membership{}, nil
			}
			return nil, err
		}
		return []Membership{{
			TenantID: t.ID,
			Name:     t.Name,
			Role:     string(authz.RoleOwner),
		}}, nil
	}

	ids, err := s.fga.ListMemberTenants(ctx, uid)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Membership{}, nil
	}
	rows, err := s.repo.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Membership, 0, len(rows))
	for _, t := range rows {
		role, err := s.fga.GetRole(ctx, uid, t.ID)
		if err != nil {
			return nil, err
		}
		if role == "" {
			// FGA reported membership but role lookup came back
			// empty — skip rather than surface a broken row.
			continue
		}
		out = append(out, Membership{
			TenantID: t.ID,
			Name:     t.Name,
			Role:     string(role),
		})
	}
	return out, nil
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

// UpdateInput is the editable subset of a Tenant row.
//
// Phase N ships with only Name editable. Additional fields (currency,
// timezone) are deliberately omitted: currency change has invoicing
// implications and timezone affects daily rollups — both are admin-
// op requests, not self-service toggles. Slug and country_code are
// permanently immutable.
//
// Pointer fields allow the caller to distinguish "unset" (leave as-is)
// from "set to empty string" (validated and rejected).
type UpdateInput struct {
	Name *string
}

// Update applies an UpdateInput to the tenant row and returns the
// updated tenant. Validates each provided field; fields omitted from
// the input are left untouched.
//
// Authorization is the caller's responsibility. In Phase N the admin
// BFF enforces that the session tenant matches the path param before
// calling this method. Phase O will retrofit an OpenFGA Check() onto
// the handler directly.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*Tenant, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
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

// ListDirectory returns a page of the platform-wide tenant directory (#277).
// Deliberately not caller-scoped — see DirectoryFilter doc comment.
func (s *Service) ListDirectory(ctx context.Context, f DirectoryFilter) (DirectoryResult, error) {
	return s.repo.ListDirectory(ctx, f)
}

// GetWithStores returns a tenant plus its store rollup (#277).
func (s *Service) GetWithStores(ctx context.Context, id string) (*TenantWithStores, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	return s.repo.GetWithStores(ctx, id)
}

// Phase Q: GetBySlug, IsSlugAvailable, validateSlug and slugPattern
// moved to the store package — slug is a store-level identifier now.
