package auth

import "testing"

// tenant-service writes tenant_id as a multi-value array (its
// UpdateUserAttributes appends into []interface{}), so the array form is what
// production tokens actually carry. The scalar form is tolerated defensively.
func TestTenantIDFromClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]interface{}
		want   string
	}{
		{
			name:   "array form — what tenant-service actually writes",
			claims: map[string]interface{}{"tenant_id": []interface{}{"11111111-2222-3333-4444-555555555555"}},
			want:   "11111111-2222-3333-4444-555555555555",
		},
		{
			name:   "scalar form — tolerated",
			claims: map[string]interface{}{"tenant_id": "11111111-2222-3333-4444-555555555555"},
			want:   "11111111-2222-3333-4444-555555555555",
		},
		{
			name:   "multi-tenant user — first entry wins",
			claims: map[string]interface{}{"tenant_id": []interface{}{"tenant-a", "tenant-b"}},
			want:   "tenant-a",
		},
		{
			name:   "claim absent",
			claims: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "nil claims",
			claims: nil,
			want:   "",
		},
		{
			name:   "empty array",
			claims: map[string]interface{}{"tenant_id": []interface{}{}},
			want:   "",
		},
		{
			name:   "empty string in array is skipped",
			claims: map[string]interface{}{"tenant_id": []interface{}{"", "tenant-b"}},
			want:   "tenant-b",
		},
		{
			name:   "empty scalar",
			claims: map[string]interface{}{"tenant_id": ""},
			want:   "",
		},
		{
			name:   "wrong element type is ignored",
			claims: map[string]interface{}{"tenant_id": []interface{}{42, "tenant-b"}},
			want:   "tenant-b",
		},
		{
			name:   "wholly wrong type",
			claims: map[string]interface{}{"tenant_id": 42},
			want:   "",
		},
		{
			name:   "[]string form",
			claims: map[string]interface{}{"tenant_id": []string{"tenant-a"}},
			want:   "tenant-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantIDFromClaims(tt.claims); got != tt.want {
				t.Errorf("tenantIDFromClaims() = %q, want %q", got, tt.want)
			}
		})
	}
}
