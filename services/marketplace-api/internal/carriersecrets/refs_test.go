package carriersecrets

import (
	"strings"
	"testing"
)

func TestBaoPath_IsScopeInPathForm(t *testing.T) {
	s := Scope{TenantID: "11111111-2222-3333-4444-555555555555", Domain: "payment", Provider: "razorpay", Field: "secret_key"}
	want := "kv/mark8ly/marketplace-api/tenants/11111111-2222-3333-4444-555555555555/payment/razorpay/secret_key"
	if got := BaoPath(s); got != want {
		t.Fatalf("BaoPath = %q, want %q", got, want)
	}
}

func TestFormatBaoReference_RoundTrips(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "shipping", Provider: "delhivery", Field: "api_key"}
	ref := FormatBaoReference(s)
	if !IsBaoRef(ref) {
		t.Fatalf("IsBaoRef(%q) = false", ref)
	}
	path, ok := ParseBaoReference(ref)
	if !ok || path != BaoPath(s) {
		t.Fatalf("ParseBaoReference(%q) = (%q, %v), want (%q, true)", ref, path, ok, BaoPath(s))
	}
}

// A bao reference must carry NO version: rotation writes a new KV v2 version
// at the same path and must return the identical reference, or every rotation
// becomes a DB-wide reference rewrite.
func TestFormatBaoReference_HasNoVersion(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "payment", Provider: "stripe", Field: "secret_key"}
	ref := FormatBaoReference(s)
	if strings.Contains(ref, "versions") {
		t.Fatalf("reference must not encode a version: %q", ref)
	}
	if FormatBaoReference(s) != ref {
		t.Fatal("FormatBaoReference must be deterministic for the same scope")
	}
}

// The prefixes must be mutually exclusive — ChainStore routing depends on it.
func TestRefPrefixes_AreMutuallyExclusive(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "payment", Provider: "razorpay", Field: "api_key"}
	bao := FormatBaoReference(s)
	if IsGSMRef(bao) || IsInlineRef(bao) {
		t.Fatalf("bao reference %q also matched another prefix", bao)
	}
	gsm := GSMRefPrefix + "projects/p/secrets/x"
	if IsBaoRef(gsm) {
		t.Fatalf("gsm reference %q matched IsBaoRef", gsm)
	}
	for _, inline := range []string{NoopRefPrefix + "abc", AESRefPrefix + "abc"} {
		if IsBaoRef(inline) {
			t.Fatalf("inline reference %q matched IsBaoRef", inline)
		}
	}
}

// Scope segments must be sanitised the same way GCP's are, so a stray slash
// cannot escape the tenant's subtree and reach another tenant's secret.
func TestBaoPath_SanitisesSegments(t *testing.T) {
	s := Scope{TenantID: "../other-tenant", Domain: "payment", Provider: "raz/orpay", Field: "api_key"}
	got := BaoPath(s)
	rest := strings.TrimPrefix(got, "kv/mark8ly/marketplace-api/tenants/")
	if strings.Contains(rest, "..") {
		t.Fatalf("path traversal survived sanitisation: %q", got)
	}
	if strings.Count(got, "/") != 7 {
		t.Fatalf("unexpected segment count in %q — a segment contained an unsanitised separator", got)
	}
}
