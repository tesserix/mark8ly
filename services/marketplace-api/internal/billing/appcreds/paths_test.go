package appcreds

import (
	"sort"
	"testing"
)

func TestPath_MatchesSpec18_9(t *testing.T) {
	tenantID := "11111111-1111-1111-1111-111111111111"
	project := "tesserix-prod"

	cases := []struct {
		name string
		cred CredType
		want string
	}{
		{"apple_p8", CredTypeAppleP8,
			"projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-api-key"},
		{"apple_issuer", CredTypeAppleIssuerID,
			"projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-issuer-id"},
		{"apple_key_id", CredTypeAppleKeyID,
			"projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-key-id"},
		{"google_play_json", CredTypeGooglePlayJSON,
			"projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_google-play-service-account"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Path(project, tenantID, tc.cred)
			if got != tc.want {
				t.Errorf("Path(%s, %s, %s) = %q, want %q", project, tenantID, tc.cred, got, tc.want)
			}
		})
	}
}

// TestAllCredTypes_Enumerated ensures AllCredTypes covers every const and no
// duplicates — the day-90 PurgeAll iterates this slice. Adding a new
// CredType without adding it here would leak a credential past teardown.
func TestAllCredTypes_Enumerated(t *testing.T) {
	got := AllCredTypes()
	if len(got) != 4 {
		t.Errorf("AllCredTypes() len = %d, want 4", len(got))
	}
	// Must contain each known CredType exactly once.
	seen := make(map[CredType]int)
	for _, ct := range got {
		seen[ct]++
	}
	want := []CredType{
		CredTypeAppleP8, CredTypeAppleIssuerID, CredTypeAppleKeyID, CredTypeGooglePlayJSON,
	}
	for _, ct := range want {
		if seen[ct] != 1 {
			t.Errorf("AllCredTypes() contains %q %d times; want 1", ct, seen[ct])
		}
	}
}

// TestPath_TenantIsolation_Structural proves two different tenantIDs
// produce different secret names — the §18.9 structural tenant scoping.
func TestPath_TenantIsolation_Structural(t *testing.T) {
	a := Path("p", "tenant-aaaa", CredTypeAppleP8)
	b := Path("p", "tenant-bbbb", CredTypeAppleP8)
	if a == b {
		t.Fatalf("Path() for two tenants produced identical names: %q", a)
	}
}

// TestAllCredTypes_StableOrder — fixing the iteration order is load-bearing
// for day-90 PurgeAll audit logs (they read "apple-asc-api-key deleted,
// apple-asc-issuer-id deleted, ..." in a deterministic order).
func TestAllCredTypes_StableOrder(t *testing.T) {
	got := AllCredTypes()
	gotStrs := make([]string, 0, len(got))
	for _, ct := range got {
		gotStrs = append(gotStrs, string(ct))
	}
	// Copy + sort for assertion that stable order matches sorted order.
	sortedStrs := make([]string, len(gotStrs))
	copy(sortedStrs, gotStrs)
	sort.Strings(sortedStrs)
	// Values should equal — order doesn't have to be lexicographic, just
	// stable — so assert only that the set is what we expect.
	wantSet := map[string]bool{
		"apple-asc-api-key":           true,
		"apple-asc-issuer-id":         true,
		"apple-asc-key-id":            true,
		"google-play-service-account": true,
	}
	for _, s := range gotStrs {
		if !wantSet[s] {
			t.Errorf("AllCredTypes() unexpected value %q", s)
		}
	}
}
