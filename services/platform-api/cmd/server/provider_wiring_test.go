package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/zitadeladmin"
	"github.com/mark8ly/platform-api/pkg/config"
)

// --- selectAccountProviders: which client backs reset/delete ----------

// TestSelectAccountProviders_FlagUnset_NoProvider pins the flag-off
// shape after #791 deleted the GIP admin client: there is no second
// provider to fall back to, so both return values are a GENUINELY nil
// interface — not a non-nil interface wrapping a nil pointer.
//
// Callers branch on that nil (cmd/server/main.go skips auth.NewService
// and the merchant teardown route), and account.Service's
// `if s.gip != nil` guard would otherwise pass and panic on a nil
// receiver AFTER the teardown transaction committed. See
// account_wiring.go's newAccountService doc for that incident.
func TestSelectAccountProviders_FlagUnset_NoProvider(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false}

	reset, del, err := selectAccountProviders(cfg)
	if err != nil {
		t.Fatalf("selectAccountProviders() error = %v, want nil", err)
	}
	if reset != nil {
		t.Fatalf("reset provider = %v, want a genuinely nil interface with Zitadel disabled", reset)
	}
	if del != nil {
		t.Fatalf("deleter = %v, want a genuinely nil interface with Zitadel disabled", del)
	}
}

// TestSelectAccountProviders_FlagSet_SelectsZitadel pins that a fully
// configured, enabled Zitadel deployment gets a *zitadeladmin.Client for
// BOTH the reset provider and the deleter.
func TestSelectAccountProviders_FlagSet_SelectsZitadel(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled:          true,
		ZitadelIssuer:           "https://login.mark8ly.zitadel.cloud",
		ZitadelLoginClientToken: "pat",
		ZitadelOrgID:            "339070697432875523",
		ZitadelAdminProjectID:   "389070376568619523",
		ZitadelStaffRoleKey:     "mark8ly.staff",
	}

	reset, del, err := selectAccountProviders(cfg)
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
// config.ErrZitadelNotConfigured — the deleter/reset provider must both
// come back nil so nothing downstream can mistake a misconfigured
// deployment for a working one.
func TestSelectAccountProviders_FlagSet_Misconfigured_FailsClearly(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled: true,
		// ZitadelIssuer, ZitadelLoginClientToken, ZitadelOrgID all unset.
	}

	reset, del, err := selectAccountProviders(cfg)
	if err == nil {
		t.Fatal("selectAccountProviders() error = nil, want a misconfiguration error")
	}
	if !errors.Is(err, config.ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap config.ErrZitadelNotConfigured", err)
	}
	if reset != nil {
		t.Fatal("reset provider must be nil on misconfiguration, never a silent fallback")
	}
	if del != nil {
		t.Fatal("deleter must be nil on misconfiguration, never a silent fallback")
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

// --- newOwnerProvisioner: onboarding-completion provisioning (#685) ----

// TestNewOwnerProvisioner_FlagUnset_StaysGenuinelyNil pins the GIP path
// for onboarding, for the same reason its invite-accept twin above does:
// a non-nil interface wrapping a nil pointer would flip
// onboarding.Complete onto the Zitadel branch and panic on the first
// merchant to finish the wizard.
func TestNewOwnerProvisioner_FlagUnset_StaysGenuinelyNil(t *testing.T) {
	p, err := newOwnerProvisioner(&config.Config{ZitadelEnabled: false})
	if err != nil {
		t.Fatalf("newOwnerProvisioner() error = %v, want nil", err)
	}
	if p != nil {
		t.Fatal("provisioner must be a genuinely nil interface when Zitadel is disabled")
	}
}

// TestNewOwnerProvisioner_FlagSet_BuildsZitadelProvisioner pins that the
// merchant is provisioned by the SAME type an invited teammate is. The
// mark8ly-admin project has exactly one role and the grant only gates
// access to the project; the owner/staff distinction lives in FGA.
func TestNewOwnerProvisioner_FlagSet_BuildsZitadelProvisioner(t *testing.T) {
	p, err := newOwnerProvisioner(zitadelStaffConfig())
	if err != nil {
		t.Fatalf("newOwnerProvisioner() error = %v, want nil", err)
	}
	if _, ok := p.(*zitadeladmin.StaffProvisioner); !ok {
		t.Fatalf("provisioner = %T, want *zitadeladmin.StaffProvisioner", p)
	}
}

// TestNewOwnerProvisioner_MissingProjectID_FailsClearly pins that a
// misconfigured deployment crashloops rather than onboarding merchants
// who hold no grant on the admin project and therefore cannot sign in.
func TestNewOwnerProvisioner_MissingProjectID_FailsClearly(t *testing.T) {
	cfg := zitadelStaffConfig()
	cfg.ZitadelAdminProjectID = ""

	p, err := newOwnerProvisioner(cfg)
	if err == nil {
		t.Fatal("newOwnerProvisioner() = nil error, want a misconfiguration error")
	}
	if !errors.Is(err, config.ErrZitadelNotConfigured) {
		t.Fatalf("err = %v, want it to wrap config.ErrZitadelNotConfigured", err)
	}
	if p != nil {
		t.Fatal("provisioner must be nil on misconfiguration")
	}
}
