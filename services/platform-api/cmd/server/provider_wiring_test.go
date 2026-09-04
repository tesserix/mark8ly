package main

import (
	"errors"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	"github.com/mark8ly/platform-api/pkg/config"
)

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
