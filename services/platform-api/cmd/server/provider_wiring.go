package main

import (
	"github.com/mark8ly/platform-api/internal/auth"
	"github.com/mark8ly/platform-api/internal/invitation"
	"github.com/mark8ly/platform-api/internal/onboarding"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	"github.com/mark8ly/platform-api/pkg/config"
)

// selectAccountProviders picks the client backing the three DESTRUCTIVE,
// user-visible account operations platform-api performs against an
// identity provider: send a password-reset code, redeem one, and delete
// an account. It exists as a function purely so this selection is
// testable against production wiring — see provider_wiring_test.go —
// mirroring newAccountService's reason for existing in account_wiring.go.
//
// # There is one provider now
//
// Until #791 this function also had to keep a GIP client alive for a
// second, unrelated concern: the tenant_id custom claim invite-accept
// stamped for marketplace-api's GIP bearer path. That claim has no
// readers left (#786/#800 removed the last one), the outbox is drained
// of gip.set_tenant_claim, and the GIP admin client is deleted. What
// remains is this single concern.
//
// When cfg.ZitadelEnabled is false — dev machines, and nothing else,
// since prod has run with it true since the #524 cutover — BOTH return
// values are a true nil and the caller disables the password-reset
// endpoints and the merchant teardown route. That is the same
// "unconfigured" shape the GIP-less path always produced; it is not a
// new failure mode.
//
// # The typed-nil trap
//
// The returns are INTERFACES. Assigning a nil concrete pointer straight
// into one produces a NON-NIL interface holding a nil pointer: a caller's
// `if reset != nil` guard would pass, and the eventual call would panic
// on a nil receiver. See cmd/server/account_wiring.go's newAccountService
// doc for the full incident. Every return below is either a genuinely
// non-nil client or an untouched nil interface variable — never a
// possibly-nil pointer.
//
// # Fail clearly, never fall back silently
//
// When cfg.ZitadelEnabled is true but misconfigured, this returns the
// error from cfg.ValidateZitadel() (or from constructing the Zitadel
// client) and BOTH return values nil. The caller panics, exactly like
// every other startup failure in cmd/server/main.go.
func selectAccountProviders(cfg *config.Config) (auth.PasswordResetProvider, gipAccountDeleter, error) {
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

	// Flag off: no provider at all. Returned as untouched nil interface
	// variables, never a nil concrete pointer — see the typed-nil section.
	var reset auth.PasswordResetProvider
	var del gipAccountDeleter
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
	p, err := buildZitadelProvisioner(cfg)
	if err != nil || p == nil {
		// Never `return p, err` — p is a CONCRETE pointer, and a nil one
		// assigned into the interface return would produce a NON-NIL
		// interface holding nil, which is exactly the GIP-path selector
		// invitation.Service branches on. See selectAccountProviders'
		// typed-nil section.
		return nil, err
	}
	return p, nil
}

// newOwnerProvisioner builds the onboarding.OwnerProvisioner that
// onboarding.Complete calls to create the MERCHANT's Zitadel account and
// grant them the mark8ly-admin project role (#685).
//
// Same construction, same true-nil discipline, and deliberately the same
// underlying *zitadeladmin.StaffProvisioner as newStaffProvisioner —
// including the same role key. The mark8ly-admin project defines exactly
// one role, mark8ly.staff, and a Zitadel project grant only decides
// whether the OIDC flow may complete at all; it carries no authority.
// Authority is the FGA `owner` tuple onboarding writes. Minting a second
// Zitadel role to express "owner" would duplicate that decision in a
// place nothing reads.
//
// Returns a true nil when cfg.ZitadelEnabled is false, which selects the
// GIP path inside onboarding.Service: under GIP the set-password form
// creates the account client-side before calling platform-api, so the
// server has nothing to provision and the behaviour must stay exactly as
// it was. A configuration problem when Zitadel IS enabled is a startup
// failure, not a degraded mode — wiring nil here would leave onboarding
// writing a GIP-shaped tuple into a Zitadel world and reproduce the
// precise bug this exists to fix: a merchant told "We couldn't find a
// store for this account. Did you finish onboarding?" seconds after
// finishing onboarding.
func newOwnerProvisioner(cfg *config.Config) (onboarding.OwnerProvisioner, error) {
	p, err := buildZitadelProvisioner(cfg)
	if err != nil || p == nil {
		return nil, err
	}
	return p, nil
}

// buildZitadelProvisioner returns the shared Zitadel provisioner, or a
// nil pointer (and nil error) when Zitadel is not enabled. Both callers
// above convert that nil pointer into a true nil interface themselves —
// this function must NOT return an interface, or the typed-nil trap
// moves in here where it is harder to see.
func buildZitadelProvisioner(cfg *config.Config) (*zitadeladmin.StaffProvisioner, error) {
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
