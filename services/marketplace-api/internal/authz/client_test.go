package authz_test

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

func TestRole_Constants(t *testing.T) {
	want := []authz.Role{authz.RoleOwner, authz.RoleAdmin, authz.RoleStaff, authz.RoleViewer}
	for _, r := range want {
		if string(r) == "" {
			t.Errorf("role %q is empty", r)
		}
	}
}

func TestRole_Priority(t *testing.T) {
	if !authz.RoleOwner.HigherOrEqual(authz.RoleAdmin) {
		t.Error("owner should outrank admin")
	}
	if authz.RoleStaff.HigherOrEqual(authz.RoleAdmin) {
		t.Error("staff should not outrank admin")
	}
	if !authz.RoleAdmin.HigherOrEqual(authz.RoleAdmin) {
		t.Error("admin should be equal to itself")
	}
}
