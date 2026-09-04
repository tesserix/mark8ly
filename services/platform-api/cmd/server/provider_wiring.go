package main

import (
	"github.com/mark8ly/platform-api/internal/auth"
	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/invitation"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	"github.com/mark8ly/platform-api/pkg/config"
)

// selectAccountProviders picks which identity provider backs the three
// DESTRUCTIVE, user-visible account operations platform-api performs
// against an IdP: send a password-reset code, redeem one, and delete an
// account. It exists as a function purely so this selection is testable
// against production wiring — see provider_wiring_test.go — mirroring
// newAccountService's reason for existing in account_wiring.go.
//
// # Two concerns, two lifetimes — do not conflate them
//
// gipAdmin, the parameter, is the SAME *gipadmin.AdminClient main.go
// already builds (or leaves nil) for EnsureTenantClaim: the tenant_id
// custom claim written on invite-accept (internal/invitation/service.go)
// and consumed via marketplace-api's flag-off GIP path. That client's
// lifetime is governed ENTIRELY by GIP_PROJECT_ID/GIP_TENANT_ID/a GIP API
// key being present — never by cfg.ZitadelEnabled. A Zitadel deployment
// still needs it alive; do not remove or gate its construction here or in
// main.go on the Zitadel flag, or invite-accept breaks (see
// cmd/server/main.go's gipAdmin construction comment and the phase plan's
// "What is deliberately NOT in scope" section).
//
// This function's job is a SEPARATE, narrower concern: which client
// backs password-reset and account-delete. When cfg.ZitadelEnabled is
// false (default), that is the same gipAdmin instance, guarded against
// the typed-nil trap below. When true, it is a freshly constructed
// *zitadeladmin.Client instead — gipAdmin is simply not used for these
// three operations, but the caller must keep it alive regardless for
// EnsureTenantClaim.
//
// # The typed-nil trap
//
// gipAdmin is a CONCRETE *gipadmin.AdminClient. Assigning a nil one
// straight into an interface-typed return value produces a NON-NIL
// interface holding a nil pointer: a caller's `if reset != nil` guard
// would pass, and the eventual call would panic on a nil receiver. See
// cmd/server/account_wiring.go's newAccountService doc for the full
// incident this mirrors. The interface-typed locals below are assigned
// ONLY inside the `gipAdmin != nil` guard, exactly like that function and
// cmd/server/main.go's existing Admin/inviteClaims wiring.
//
// # Fail clearly, never fall back silently
//
// When cfg.ZitadelEnabled is true but misconfigured, this returns the
// error from cfg.ValidateZitadel() (or from constructing the Zitadel
// client) and BOTH return values nil — never the GIP client. A
// misconfigured Zitadel deployment must fail startup loudly (the caller
// is expected to panic on a non-nil error, exactly like every other
// startup failure in cmd/server/main.go), not silently keep serving
// merchants against GIP while believing it migrated.
func selectAccountProviders(cfg *config.Config, gipAdmin *gipadmin.AdminClient) (auth.PasswordResetProvider, gipAccountDeleter, error) {
	if cfg.ZitadelEnabled {
		if err := cfg.ValidateZitadel(); err != nil {
			return nil, nil, err
		}
		client, err := zitadeladmin.New(zitadeladmin.Config{
			BaseURL: cfg.ZitadelIssuer,
			Token:   cfg.ZitadelLoginClientToken,
			OrgID:   cfg.ZitadelOrgID,
		}, nil)
		if err != nil {
			return nil, nil, err
		}
		return client, client, nil
	}

	var reset auth.PasswordResetProvider
	var del gipAccountDeleter
	if gipAdmin != nil {
		reset = gipAdmin
		del = gipAdmin
	}
	return reset, del, nil
}

// newTenantClaimSetter builds the invitation.TenantClaimSetter that
// invitation/service.go calls on invite-accept to stamp the tenant_id
// custom claim onto the invited user.
//
// Deliberately takes ONLY gipAdmin — no *config.Config, no
// cfg.ZitadelEnabled anywhere in reach — so this is structurally, not
// just conventionally, independent of the Zitadel flag: there is no
// config value in scope for a future edit to branch on. EnsureTenantClaim
// must keep running against GIP regardless of which provider
// selectAccountProviders chose for password-reset/delete, because
// marketplace-api's flag-off path still reads the tenant_id claim and
// invitation/service.go writes it on accept. See selectAccountProviders'
// doc above for the full two-concerns-two-lifetimes reasoning this
// mirrors — this function IS the "EnsureTenantClaim" lifetime; that
// function's return values are the other one.
//
// cmd/server/main_test.go's TestMainCallsNewTenantClaimSetterUnconditionally
// pins the ONE thing this function's own signature cannot prove by
// itself: that main.go actually calls it as a top-level, unconditional
// statement rather than nesting it inside an `if` that could be
// flag-gated later.
//
// Guards the typed-nil trap the same way selectAccountProviders does:
// gipAdmin is a CONCRETE *gipadmin.AdminClient, so the interface-typed
// local is assigned only inside the `gipAdmin != nil` guard — never a
// possibly-nil pointer straight into the interface return.
func newTenantClaimSetter(gipAdmin *gipadmin.AdminClient) invitation.TenantClaimSetter {
	var claims invitation.TenantClaimSetter
	if gipAdmin != nil {
		claims = gipAdmin
	}
	return claims
}
