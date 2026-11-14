package invitation

import "testing"

func TestIsAllowedTenantRole(t *testing.T) {
	for role, want := range map[string]bool{
		"admin": true, "staff": true, "viewer": true, "cashier": true,
		"owner": false, "": false, "manager": false,
	} {
		if got := isAllowedTenantRole(role); got != want {
			t.Errorf("isAllowedTenantRole(%q) = %v, want %v", role, got, want)
		}
	}
}
