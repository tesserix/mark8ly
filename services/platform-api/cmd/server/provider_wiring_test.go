package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	"github.com/mark8ly/platform-api/pkg/config"
)

// --- newTenantClaimSetter: EnsureTenantClaim always runs against GIP ----

// TestNewTenantClaimSetter_UsesGIPAdmin pins that a real gipAdmin client
// is returned as-is — this is the client invitation/service.go calls
// EnsureTenantClaim through on invite-accept, and it must be GIP
// regardless of ZITADEL_ENABLED (marketplace-api's flag-off path still
// reads the tenant_id claim written this way).
func TestNewTenantClaimSetter_UsesGIPAdmin(t *testing.T) {
	admin := &gipadmin.AdminClient{}

	claims := newTenantClaimSetter(admin)

	if claims != admin {
		t.Fatalf("claims = %v, want the gipAdmin client %v", claims, admin)
	}
}

// TestNewTenantClaimSetter_NilGIPStaysGenuinelyNil is the typed-nil
// regression test for this function, mirroring
// TestSelectAccountProviders_FlagUnset_NilGIPStaysGenuinelyNil.
func TestNewTenantClaimSetter_NilGIPStaysGenuinelyNil(t *testing.T) {
	var admin *gipadmin.AdminClient // nil concrete pointer

	claims := newTenantClaimSetter(admin)

	if claims != nil {
		t.Fatal("claims must be a genuinely nil interface when gipAdmin is nil, " +
			"not a non-nil interface wrapping a nil *gipadmin.AdminClient")
	}
}

// --- requireGIPForTenantClaim: the deploy-time half of the same guard ---

// TestRequireGIPForTenantClaim_FlagOff_NeverRequired pins flag-off
// byte-identical behaviour: today's production (ZITADEL_ENABLED unset)
// runs fine with GIP unconfigured (dev without real GIP credentials), and
// this new check must not turn that into a startup failure.
func TestRequireGIPForTenantClaim_FlagOff_NeverRequired(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false}
	if err := requireGIPForTenantClaim(cfg, nil); err != nil {
		t.Fatalf("requireGIPForTenantClaim() with the flag off = %v, want nil", err)
	}
}

// TestRequireGIPForTenantClaim_FlagOn_MissingGIP_FailsClearly is the
// regression test for the deploy-time gap: enabling Zitadel while also
// dropping GIP_PROJECT_ID/GIP_TENANT_ID/the GIP key (the "we've migrated"
// operator action) must fail startup loudly, not leave EnsureTenantClaim
// silently no-op-ing behind a log.Warn.
func TestRequireGIPForTenantClaim_FlagOn_MissingGIP_FailsClearly(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true}
	err := requireGIPForTenantClaim(cfg, nil)
	if err == nil {
		t.Fatal("requireGIPForTenantClaim() = nil, want an error when gipAdmin is nil and Zitadel is enabled")
	}
	for _, want := range []string{"EnsureTenantClaim", "marketplace-api"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q so an operator understands why", err.Error(), want)
		}
	}
}

// TestRequireGIPForTenantClaim_FlagOn_GIPPresent_Boots pins that a
// correctly configured Zitadel-enabled deployment (GIP client built
// successfully) boots without this new check getting in the way.
func TestRequireGIPForTenantClaim_FlagOn_GIPPresent_Boots(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true}
	admin := &gipadmin.AdminClient{}
	if err := requireGIPForTenantClaim(cfg, admin); err != nil {
		t.Fatalf("requireGIPForTenantClaim() with gipAdmin present = %v, want nil", err)
	}
}

// TestSelectAccountProviders_FlagUnset_SelectsGIP pins that with
// ZITADEL_ENABLED unset (production today), the reset-provider and
// deleter handed to auth.Service and the account teardown service are
// exactly the gipAdmin client already built for EnsureTenantClaim — no
// new behaviour on the GIP path.
func TestSelectAccountProviders_FlagUnset_SelectsGIP(t *testing.T) {
	admin := &gipadmin.AdminClient{}
	cfg := &config.Config{ZitadelEnabled: false}

	reset, del, err := selectAccountProviders(cfg, admin)
	if err != nil {
		t.Fatalf("selectAccountProviders() error = %v, want nil", err)
	}
	if reset != admin {
		t.Fatalf("reset provider = %v, want the gipAdmin client %v", reset, admin)
	}
	if del != admin {
		t.Fatalf("deleter = %v, want the gipAdmin client %v", del, admin)
	}
}

// TestSelectAccountProviders_FlagUnset_NilGIPStaysGenuinelyNil is the
// typed-nil regression test: this is what makes the test suite fail first
// against a naive implementation that assigns a possibly-nil
// *gipadmin.AdminClient straight into the interface return values.
//
// gipAdmin is declared as a nil *gipadmin.AdminClient (the "GIP
// unconfigured" case in cmd/server/main.go) with NO New() call — a typed
// nil pointer needs no working credentials to construct.
func TestSelectAccountProviders_FlagUnset_NilGIPStaysGenuinelyNil(t *testing.T) {
	var admin *gipadmin.AdminClient // nil concrete pointer
	cfg := &config.Config{ZitadelEnabled: false}

	reset, del, err := selectAccountProviders(cfg, admin)
	if err != nil {
		t.Fatalf("selectAccountProviders() error = %v, want nil", err)
	}
	if reset != nil {
		t.Fatal("reset provider must be a genuinely nil interface when gipAdmin is nil, " +
			"not a non-nil interface wrapping a nil *gipadmin.AdminClient")
	}
	if del != nil {
		t.Fatal("deleter must be a genuinely nil interface when gipAdmin is nil, " +
			"not a non-nil interface wrapping a nil *gipadmin.AdminClient")
	}
}

// TestSelectAccountProviders_FlagSet_SelectsZitadel pins that a fully
// configured, enabled Zitadel deployment gets a *zitadeladmin.Client for
// BOTH the reset provider and the deleter — and NOT the gipAdmin instance,
// even though gipAdmin is passed in (it must still be alive for
// EnsureTenantClaim elsewhere, but it must not be what this function
// returns once Zitadel is selected).
func TestSelectAccountProviders_FlagSet_SelectsZitadel(t *testing.T) {
	admin := &gipadmin.AdminClient{}
	cfg := &config.Config{
		ZitadelEnabled:          true,
		ZitadelIssuer:           "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken: "pat",
		ZitadelOrgID:            "339070697432875523",
		ZitadelAdminProjectID:   "389070376568619523",
		ZitadelStaffRoleKey:     "mark8ly.staff",
	}

	reset, del, err := selectAccountProviders(cfg, admin)
	if err != nil {
		t.Fatalf("selectAccountProviders() error = %v, want nil", err)
	}
	if _, ok := reset.(*zitadeladmin.Client); !ok {
		t.Fatalf("reset provider = %T, want *zitadeladmin.Client", reset)
	}
	if _, ok := del.(*zitadeladmin.Client); !ok {
		t.Fatalf("deleter = %T, want *zitadeladmin.Client", del)
	}
}

// TestSelectAccountProviders_FlagSet_Misconfigured_FailsClearly pins that a
// misconfigured Zitadel deployment refuses with an error wrapping
// config.ErrZitadelNotConfigured, rather than silently falling back to
// GIP — the deleter/reset provider must both come back nil so nothing
// downstream can mistake this for "GIP selected".
func TestSelectAccountProviders_FlagSet_Misconfigured_FailsClearly(t *testing.T) {
	admin := &gipadmin.AdminClient{}
	cfg := &config.Config{
		ZitadelEnabled: true,
		// ZitadelIssuer, ZitadelLoginClientToken, ZitadelOrgID all unset.
	}

	reset, del, err := selectAccountProviders(cfg, admin)
	if err == nil {
		t.Fatal("selectAccountProviders() error = nil, want a misconfiguration error")
	}
	if !errors.Is(err, config.ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap config.ErrZitadelNotConfigured", err)
	}
	if reset != nil {
		t.Fatal("reset provider must be nil on misconfiguration, never a silent GIP fallback")
	}
	if del != nil {
		t.Fatal("deleter must be nil on misconfiguration, never a silent GIP fallback")
	}
}

// --- newStaffProvisioner: invite-accept provisioning selection --------

// zitadelStaffConfig is a fully configured, Zitadel-enabled config.
func zitadelStaffConfig() *config.Config {
	return &config.Config{
		ZitadelEnabled:          true,
		ZitadelIssuer:           "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken: "pat",
		ZitadelOrgID:            "339070697432875523",
		ZitadelAdminProjectID:   "389070376568619523",
		ZitadelStaffRoleKey:     "mark8ly.staff",
	}
}

// TestNewStaffProvisioner_FlagUnset_StaysGenuinelyNil pins the GIP path:
// with ZITADEL_ENABLED unset, invitation.Service must receive a truly nil
// StaffProvisioner. A non-nil interface wrapping a nil pointer would flip
// invitation.Accept onto the Zitadel branch and panic on first accept.
func TestNewStaffProvisioner_FlagUnset_StaysGenuinelyNil(t *testing.T) {
	p, err := newStaffProvisioner(&config.Config{ZitadelEnabled: false})
	if err != nil {
		t.Fatalf("newStaffProvisioner() error = %v, want nil", err)
	}
	if p != nil {
		t.Fatal("provisioner must be a genuinely nil interface when Zitadel is disabled")
	}
}

// TestNewStaffProvisioner_FlagSet_BuildsZitadelProvisioner pins that an
// enabled, fully configured deployment gets the Zitadel provisioner.
func TestNewStaffProvisioner_FlagSet_BuildsZitadelProvisioner(t *testing.T) {
	p, err := newStaffProvisioner(zitadelStaffConfig())
	if err != nil {
		t.Fatalf("newStaffProvisioner() error = %v, want nil", err)
	}
	if _, ok := p.(*zitadeladmin.StaffProvisioner); !ok {
		t.Fatalf("provisioner = %T, want *zitadeladmin.StaffProvisioner", p)
	}
}

// TestNewStaffProvisioner_MissingProjectID_FailsClearly pins that the new
// REQUIRED variable fails startup rather than provisioning teammates who
// hold no grant on the admin project and therefore cannot sign in.
func TestNewStaffProvisioner_MissingProjectID_FailsClearly(t *testing.T) {
	cfg := zitadelStaffConfig()
	cfg.ZitadelAdminProjectID = ""

	p, err := newStaffProvisioner(cfg)
	if err == nil {
		t.Fatal("newStaffProvisioner() = nil error, want a misconfiguration error")
	}
	if !errors.Is(err, config.ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap config.ErrZitadelNotConfigured", err)
	}
	if !strings.Contains(err.Error(), "ZITADEL_ADMIN_PROJECT_ID") {
		t.Errorf("err = %q, want it to name ZITADEL_ADMIN_PROJECT_ID", err.Error())
	}
	if p != nil {
		t.Fatal("provisioner must be nil on misconfiguration")
	}
}
