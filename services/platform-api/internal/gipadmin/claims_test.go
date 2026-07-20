package gipadmin

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExistingTenantIDs(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"array form (what we write)", []any{"t1", "t2"}, []string{"t1", "t2"}},
		{"scalar form (tolerated)", "t1", []string{"t1"}},
		{"empty scalar", "", nil},
		{"absent", nil, nil},
		{"empty array", []any{}, []string{}},
		{"non-string entries skipped", []any{"t1", 42, ""}, []string{"t1"}},
		{"unexpected type", map[string]any{"x": 1}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := existingTenantIDs(tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("existingTenantIDs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The merge must preserve unrelated claims. A real production account
// carries {"admin": true}; clobbering customAttributes would silently strip
// that and demote the user. GIP replaces the field wholesale, so this is a
// genuine hazard rather than a theoretical one.
func TestMergePreservesUnrelatedClaims(t *testing.T) {
	attrs := map[string]any{"admin": true, "region": "in"}

	tenants := existingTenantIDs(attrs[tenantClaimKey])
	attrs[tenantClaimKey] = append(tenants, "tenant-new")

	encoded, err := json.Marshal(attrs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(encoded, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if round["admin"] != true {
		t.Error("admin claim was lost — merge clobbered unrelated claims")
	}
	if round["region"] != "in" {
		t.Error("region claim was lost")
	}
	got := existingTenantIDs(round[tenantClaimKey])
	if !reflect.DeepEqual(got, []string{"tenant-new"}) {
		t.Errorf("tenant_id = %v, want [tenant-new]", got)
	}
}

// Appending a second tenant must keep the first — owners may hold more
// than one store.
func TestMergeAppendsAdditionalTenant(t *testing.T) {
	attrs := map[string]any{tenantClaimKey: []any{"tenant-a"}}

	tenants := existingTenantIDs(attrs[tenantClaimKey])
	attrs[tenantClaimKey] = append(tenants, "tenant-b")

	got := existingTenantIDs(attrs[tenantClaimKey])
	if !reflect.DeepEqual(got, []string{"tenant-a", "tenant-b"}) {
		t.Errorf("tenant_id = %v, want [tenant-a tenant-b]", got)
	}
}

func TestEnsureTenantClaimRejectsEmptyArgs(t *testing.T) {
	c := &AdminClient{}
	if err := c.EnsureTenantClaim(t.Context(), "", "tenant"); err == nil {
		t.Error("empty uid should error")
	}
	if err := c.EnsureTenantClaim(t.Context(), "uid", ""); err == nil {
		t.Error("empty tenantID should error")
	}
}
