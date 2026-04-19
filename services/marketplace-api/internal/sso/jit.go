// Package sso — jit.go implements Just-In-Time user provisioning.
//
// On the first successful SSO callback for a user, JITProvisioner binds
// their external IdP identity (SAML NameID or OIDC sub) to an internal
// Mark8ly user account.  The lookup order is:
//
//  1. Existing mapping by (tenant_id, external_user_id) — repeat logins, fast path.
//  2. Existing mapping by (tenant_id, email) — the same user logged in via a
//     different IdP credential; bind the new external_user_id to that user.
//  3. Existing Mark8ly user by email — account pre-existed before SSO; bind.
//  4. No match anywhere — create a new user + mapping.
package sso

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// UsersRepo is the narrow data-access surface JITProvisioner needs. The
// production implementation wraps whatever users/staff table marketplace-api
// owns. Tests supply a fake.
//
// TODO(wiring): wire a concrete implementation against the staff/customer
// table once the users service interface is settled.
type UsersRepo interface {
	// FindByTenantAndEmail returns the user's UUID for a (tenant, email)
	// pair. found=false (no error) means the email is unknown.
	FindByTenantAndEmail(ctx context.Context, tenantID uuid.UUID, email string) (userID uuid.UUID, found bool, err error)

	// CreateForTenant inserts a new user record scoped to tenant with the
	// given email, attributes (displayName, firstName, etc.) and default
	// role. Returns the newly assigned UUID.
	CreateForTenant(ctx context.Context, tenantID uuid.UUID, email string, attrs map[string]any, defaultRole string) (userID uuid.UUID, err error)
}

// JITInput carries every field that Provision needs to determine the
// right action. All string-typed IDs are exact values from the IdP
// assertion — never rewritten by this layer.
type JITInput struct {
	TenantID       uuid.UUID
	Provider       Provider
	ExternalUserID string         // SAML NameID or OIDC sub claim
	Email          string         // normalised to lower-case before use
	Attributes     map[string]any // resolved via AttrMapping.Resolve()
	DefaultRole    string         // applied only when creating a new user
}

// JITOutput is returned by Provision. InternalUserID is always non-zero
// on success; Created distinguishes a brand-new account from a bind.
type JITOutput struct {
	InternalUserID uuid.UUID
	Created        bool
}

// JITProvisioner implements the four-step binding algorithm. It is safe
// for concurrent use — all mutable state lives in the underlying repos.
type JITProvisioner struct {
	users    UsersRepo
	mappings *Repository
}

// NewJITProvisioner constructs a JITProvisioner.
func NewJITProvisioner(users UsersRepo, mappings *Repository) *JITProvisioner {
	return &JITProvisioner{users: users, mappings: mappings}
}

// Provision runs the four-step lookup and returns the resolved internal
// user identity. It always refreshes last_login_at on the mapping row.
func (p *JITProvisioner) Provision(ctx context.Context, in JITInput) (JITOutput, error) {
	if err := validateInput(in); err != nil {
		return JITOutput{}, err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))

	// Step 1 — existing mapping for this external identity (hot path).
	if m, err := p.mappings.GetMapping(ctx, in.TenantID, in.ExternalUserID); err == nil {
		// Refresh last_login_at.
		_ = p.mappings.UpsertMapping(ctx, m)
		return JITOutput{InternalUserID: m.InternalUserID, Created: false}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return JITOutput{}, fmt.Errorf("jit: mapping lookup: %w", err)
	}

	// Step 2 — existing mapping by email (same user, different credential).
	if m, err := p.mappings.FindMappingByEmail(ctx, in.TenantID, email); err == nil {
		// Bind the new external_user_id to the same internal user.
		newMapping := &UserMapping{
			TenantID:       in.TenantID,
			ExternalUserID: in.ExternalUserID,
			InternalUserID: m.InternalUserID,
			Email:          email,
		}
		if err := p.mappings.UpsertMapping(ctx, newMapping); err != nil {
			return JITOutput{}, fmt.Errorf("jit: bind mapping by email: %w", err)
		}
		return JITOutput{InternalUserID: m.InternalUserID, Created: false}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return JITOutput{}, fmt.Errorf("jit: email mapping lookup: %w", err)
	}

	// Step 3 — pre-existing Mark8ly user account with the same email.
	if userID, found, err := p.users.FindByTenantAndEmail(ctx, in.TenantID, email); err != nil {
		return JITOutput{}, fmt.Errorf("jit: user lookup: %w", err)
	} else if found {
		m := &UserMapping{
			TenantID:       in.TenantID,
			ExternalUserID: in.ExternalUserID,
			InternalUserID: userID,
			Email:          email,
		}
		if err := p.mappings.UpsertMapping(ctx, m); err != nil {
			return JITOutput{}, fmt.Errorf("jit: bind mapping to user: %w", err)
		}
		return JITOutput{InternalUserID: userID, Created: false}, nil
	}

	// Step 4 — brand new user; create + bind.
	userID, err := p.users.CreateForTenant(ctx, in.TenantID, email, in.Attributes, in.DefaultRole)
	if err != nil {
		return JITOutput{}, fmt.Errorf("jit: create user: %w", err)
	}
	m := &UserMapping{
		TenantID:       in.TenantID,
		ExternalUserID: in.ExternalUserID,
		InternalUserID: userID,
		Email:          email,
	}
	if err := p.mappings.UpsertMapping(ctx, m); err != nil {
		return JITOutput{}, fmt.Errorf("jit: upsert mapping after create: %w", err)
	}
	return JITOutput{InternalUserID: userID, Created: true}, nil
}

// validateInput checks that the required JITInput fields are present.
func validateInput(in JITInput) error {
	if in.TenantID == uuid.Nil {
		return fmt.Errorf("jit: tenant_id is required")
	}
	if strings.TrimSpace(in.ExternalUserID) == "" {
		return fmt.Errorf("jit: external_user_id is required")
	}
	if strings.TrimSpace(in.Email) == "" {
		return fmt.Errorf("jit: email is required")
	}
	return nil
}
