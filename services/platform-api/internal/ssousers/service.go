// Package ssousers answers, for marketplace-api's SSO just-in-time
// provisioning, the two questions it cannot answer for itself
// (mark8ly#820):
//
//	"is there already a Mark8ly user with this email in this tenant?"
//	"there is not — make one, with a role."
//
// marketplace-api has no user identity store. `user_profiles` is keyed on a
// provider subject, is not tenant-scoped, and holds display preferences;
// membership is FGA tuples and identity is Zitadel. FGA can check a relation
// for a KNOWN id but cannot search by email, so the lookup has to happen
// where Zitadel is — here.
//
// # Why this matters more than it looks
//
// Getting the answer wrong is silently wrong. A merchant admin who already
// signs in with a password would, on their first SSO login, be minted a
// SECOND identity: their existing role would not follow them, their audit
// trail would split in two, and they would most likely land with no access at
// all. Resolving through Zitadel is what keeps one human as one user.
package ssousers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/idperr"
)

// EmailResolver maps an email to a Zitadel user id. *zitadeladmin.Client
// satisfies it.
type EmailResolver interface {
	ResolveUserIDByEmail(ctx context.Context, email string) (string, error)
}

// StaffProvisioner makes an email a sign-in-capable identity on the admin
// project WITHOUT giving it a password. *zitadeladmin.StaffProvisioner
// satisfies it.
type StaffProvisioner interface {
	ProvisionSSOStaff(ctx context.Context, email, firstName, lastName string) (string, error)
}

// Directory is both halves. *zitadeladmin.StaffProvisioner satisfies it, so
// the wiring passes one dependency rather than the client and the provisioner
// separately — which also means the lookup and the create can never be
// pointed at different Zitadel orgs.
type Directory interface {
	EmailResolver
	StaffProvisioner
}

// Service resolves and provisions SSO identities.
type Service struct {
	dir Directory
	fga authz.Client
}

// NewService constructs a Service.
func NewService(dir Directory, fga authz.Client) *Service {
	return &Service{dir: dir, fga: fga}
}

// LookupResult is the answer to "who is this email in this tenant".
type LookupResult struct {
	UserID string
	// Found reports whether this email already belongs to a MEMBER of the
	// tenant — someone with a role, whose identity should simply be reused.
	//
	// It is deliberately NOT "a Zitadel account with this email exists".
	// An account that exists but holds no role in this tenant still needs a
	// grant, and reporting it as found would bind the SSO login to an
	// identity that cannot do anything. Answering false sends the caller to
	// Provision, which resolves that same account rather than duplicating it
	// and then grants the role. Both paths therefore end on ONE identity,
	// with no change to marketplace-api's JIT algorithm.
	Found bool
}

// Lookup resolves email within tenantID.
//
// A resolution failure is not "not found": the two are different answers and
// only one of them means "go and create this person". An unreachable Zitadel
// reported as not-found would provision duplicates for every login during an
// outage.
func (s *Service) Lookup(ctx context.Context, tenantID, email string) (LookupResult, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return LookupResult{}, fmt.Errorf("ssousers: email is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return LookupResult{}, fmt.Errorf("ssousers: tenant id is required")
	}

	userID, err := s.dir.ResolveUserIDByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, idperr.ErrUserNotFound) {
			return LookupResult{}, nil
		}
		return LookupResult{}, fmt.Errorf("ssousers: resolve %s: %w", email, err)
	}

	member, err := s.fga.CheckMembership(ctx, userID, tenantID)
	if err != nil {
		return LookupResult{}, fmt.Errorf("ssousers: membership check: %w", err)
	}
	if !member {
		// The account exists but holds nothing here. Report the id anyway —
		// the caller may want it for logging — but not as Found, so the
		// grant still happens.
		return LookupResult{UserID: userID, Found: false}, nil
	}
	return LookupResult{UserID: userID, Found: true}, nil
}

// ProvisionInput describes the account to ensure.
type ProvisionInput struct {
	TenantID  string
	Email     string
	FirstName string
	LastName  string
	// Role is the tenant role to grant. Restricted to the non-owner roles:
	// an IdP assertion must never be able to mint an owner, which is the one
	// role that can remove the actual owner.
	Role string
}

// Provision ensures a Zitadel account for the email and grants it Role on the
// tenant. Idempotent: an existing account is resolved rather than duplicated,
// and an existing role write is a no-op.
//
// The account is created WITHOUT a password (see zitadeladmin.HumanUser's
// WithoutPassword). An SSO-provisioned account with a password would be
// signable-into through Zitadel directly, and a password could be set later
// through password reset — either one bypassing the tenant's IdP and
// everything it enforces. A tenant buys SSO to centralise that; the second
// credential path is never created.
func (s *Service) Provision(ctx context.Context, in ProvisionInput) (string, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return "", fmt.Errorf("ssousers: email is required")
	}
	if strings.TrimSpace(in.TenantID) == "" {
		return "", fmt.Errorf("ssousers: tenant id is required")
	}
	role, err := tenantRole(in.Role)
	if err != nil {
		return "", err
	}

	first, last := names(in.FirstName, in.LastName, email)

	// Resolve-or-create plus the admin-project grant, in one call that
	// marketplace-api's invitation path already uses. The grant is what makes
	// the admin app reachable at all (mark8ly-admin sets projectRoleCheck),
	// and it happens before the tenant role so a partial failure leaves a
	// user who cannot reach anything rather than one holding a role they
	// cannot exercise.
	userID, err := s.dir.ProvisionSSOStaff(ctx, email, first, last)
	if err != nil {
		return "", fmt.Errorf("ssousers: ensure user %s: %w", email, err)
	}

	if err := s.fga.WriteRole(ctx, userID, role, in.TenantID); err != nil {
		return "", fmt.Errorf("ssousers: grant %s on %s: %w", role, in.TenantID, err)
	}
	return userID, nil
}

// tenantRole validates the requested role.
//
// Owner is refused outright. Every other role is additive, but owner is the
// one that can remove the real owner — and the assertion driving this call
// comes from a customer-controlled IdP, so it must not be able to reach it.
func tenantRole(raw string) (authz.Role, error) {
	switch authz.Role(strings.ToLower(strings.TrimSpace(raw))) {
	case authz.RoleAdmin:
		return authz.RoleAdmin, nil
	case authz.RoleStaff:
		return authz.RoleStaff, nil
	case authz.RoleViewer:
		return authz.RoleViewer, nil
	case authz.RoleOwner:
		return "", fmt.Errorf("ssousers: owner cannot be granted by SSO provisioning")
	default:
		return "", fmt.Errorf("ssousers: unknown role %q", raw)
	}
}

// names fills the givenName/familyName Zitadel requires, deriving from the
// email local part when the IdP sent nothing. Empty strings are rejected by
// Zitadel outright, so a nameless assertion would otherwise fail the login.
func names(first, last, email string) (string, string) {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	if first != "" && last != "" {
		return first, last
	}
	local := email
	if at := strings.Index(email, "@"); at > 0 {
		local = email[:at]
	}
	if first == "" {
		first = local
	}
	if last == "" {
		last = local
	}
	return first, last
}
