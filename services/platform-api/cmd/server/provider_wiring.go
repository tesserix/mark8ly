package main

import (
	"fmt"

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

// newStaffProvisioner builds the invitation.StaffProvisioner that
// invitation.Accept calls to create an invited teammate's Zitadel
// account and grant them the mark8ly-admin project role.
//
// Returns a true nil (never a typed nil — same trap selectAccountProviders
// documents) when cfg.ZitadelEnabled is false. That nil is what selects
// the GIP path inside invitation.Service: under GIP the accept form
// creates the account client-side before calling platform-api, so there
// is nothing for the server to provision and the behaviour must stay
// exactly as it was.
//
// When Zitadel IS enabled, a configuration problem is a startup failure,
// not a degraded mode: the caller panics on the returned error, matching
// selectAccountProviders. Silently wiring nil here would leave invite-
// accept writing a GIP-shaped tuple into a Zitadel world and produce the
// precise bug this function exists to fix — an invited teammate who is
// told "we couldn't find a store for this account" at every sign-in.
func newStaffProvisioner(cfg *config.Config) (invitation.StaffProvisioner, error) {
	if !cfg.ZitadelEnabled {
		return nil, nil
	}
	if err := cfg.ValidateZitadel(); err != nil {
		return nil, err
	}
	client, err := zitadeladmin.New(zitadeladmin.Config{
		BaseURL: cfg.ZitadelIssuer,
		Token:   cfg.ZitadelLoginClientToken,
		OrgID:   cfg.ZitadelOrgID,
	}, nil)
	if err != nil {
		return nil, err
	}
	return zitadeladmin.NewStaffProvisioner(client, cfg.ZitadelAdminProjectID, []string{cfg.ZitadelStaffRoleKey})
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

// requireGIPForTenantClaim is the DEPLOY-time half of the guard
// newTenantClaimSetter enforces in code. A code review (and
// TestMainCallsNewTenantClaimSetterUnconditionally) can prove main.go
// still WIRES gipAdmin into EnsureTenantClaim unconditionally, but neither
// can stop an operator's config change from pulling the rug out from
// under it: enabling Zitadel and, as the natural "we've migrated" action,
// also removing GIP_PROJECT_ID/GIP_TENANT_ID/the GIP key.
//
// Without this check, that combination leaves gipAdmin nil,
// newTenantClaimSetter correctly (per its own guard) returns a true nil
// invitation.TenantClaimSetter, and EnsureTenantClaim silently no-ops
// behind cmd/server/main.go's one log.Warn line — invite-accept
// (internal/invitation/service.go) stops writing the tenant_id custom
// claim, and every newly-invited merchant gets a permanent "No store yet"
// on mobile. Nothing fails at startup; it surfaces days later as a
// support ticket.
//
// So: when cfg.ZitadelEnabled is true, gipAdmin must be non-nil — for ANY
// reason it might not be (missing config, or gipadmin.New itself failing,
// e.g. ADC unavailable) — or this returns an error the caller is expected
// to panic on, exactly like config.ValidateZitadel's own missing-value
// errors and every other startup failure in cmd/server/main.go. A
// crashloop on a bad rollout is the correct outcome here; a quiet no-op
// that surfaces as "merchants can't see their store" is not.
//
// When cfg.ZitadelEnabled is false this is always nil — flag-off behaviour
// must stay byte-identical to before this check existed, including dev
// machines that run with no GIP credentials at all.
func requireGIPForTenantClaim(cfg *config.Config, gipAdmin *gipadmin.AdminClient) error {
	if !cfg.ZitadelEnabled {
		return nil
	}
	if gipAdmin != nil {
		return nil
	}
	return fmt.Errorf("gip: ZITADEL_ENABLED=true but the GIP client EnsureTenantClaim " +
		"depends on is not available (see the preceding log line for whether " +
		"GIP_PROJECT_ID/GIP_TENANT_ID/a GIP API key are missing, or gipadmin init " +
		"itself failed). This client is still REQUIRED after a Zitadel cutover here: " +
		"invite-accept (internal/invitation/service.go) writes the tenant_id custom " +
		"claim through it, and marketplace-api's flag-off path still reads that claim " +
		"— dropping GIP_PROJECT_ID/GIP_TENANT_ID/the GIP key is only safe once " +
		"ZITADEL_ENABLED is ALSO true on marketplace-api, a separate service and a " +
		"separate cutover. Restore the GIP_* variables, or do not enable Zitadel here yet")
}
