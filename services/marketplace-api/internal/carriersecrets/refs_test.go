package carriersecrets

import (
	"math/rand"
	"strings"
	"testing"
)

func TestBaoPath_IsScopeInPathForm(t *testing.T) {
	s := Scope{TenantID: "11111111-2222-3333-4444-555555555555", Domain: "payment", Provider: "razorpay", Field: "secret_key"}
	// "secret_key" contains a literal '_', which encodeSegment escapes to
	// "__" to stay injective (see #606).
	want := "kv/mark8ly/marketplace-api/tenants/11111111-2222-3333-4444-555555555555/payment/razorpay/secret__key"
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

// --- Issue #606: injective segment encoding ---

func TestEncodeSegment_RoundTrips(t *testing.T) {
	cases := []string{
		"",
		"api_key",
		"shop.example.com",
		"müller.com",
		"möller.com",
		"11111111-2222-3333-4444-555555555555",
		"a/b",
		"a..b",
		"has\x00nul",
	}
	for _, s := range cases {
		enc := encodeSegment(s)
		dec, err := decodeSegment(enc)
		if err != nil {
			t.Fatalf("decodeSegment(encodeSegment(%q)=%q) returned error: %v", s, enc, err)
		}
		if dec != s {
			t.Fatalf("decodeSegment(encodeSegment(%q)) = %q, want %q", s, dec, s)
		}
	}
}

// These are the specific pairs the old sanitizeSegment collided on. This
// test must FAIL against the old sanitizeSegment-based encoding — confirmed
// before encodeSegment was implemented (see #606 report).
func TestEncodeSegment_Injective(t *testing.T) {
	pairs := [][2]string{
		{"t_1", "t/1"},
		{"müller.com", "möller.com"},
	}
	for _, p := range pairs {
		a, b := encodeSegment(p[0]), encodeSegment(p[1])
		if a == b {
			t.Fatalf("encodeSegment(%q) == encodeSegment(%q) == %q, want distinct encodings", p[0], p[1], a)
		}
	}
}

func TestEncodeSegment_OutputCharset(t *testing.T) {
	rnd := rand.New(rand.NewSource(606))
	for i := 0; i < 300; i++ {
		n := rnd.Intn(24)
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = byte(rnd.Intn(256))
		}
		s := string(buf)
		enc := encodeSegment(s)
		for _, b := range []byte(enc) {
			ok := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
			if !ok {
				t.Fatalf("encodeSegment(%q) = %q contains disallowed byte %q", s, enc, b)
			}
		}
		if strings.Contains(enc, "/") {
			t.Fatalf("encodeSegment(%q) = %q contains '/'", s, enc)
		}
		if strings.Contains(enc, "..") {
			t.Fatalf("encodeSegment(%q) = %q contains '..'", s, enc)
		}
	}
}

func TestBaoPath_DistinctScopesDistinctPaths(t *testing.T) {
	a := Scope{TenantID: "11111111-2222-3333-4444-555555555555", Domain: "platform", Provider: "cloudflare", Field: "t_1"}
	b := Scope{TenantID: "11111111-2222-3333-4444-555555555555", Domain: "platform", Provider: "cloudflare", Field: "t/1"}
	if BaoPath(a) == BaoPath(b) {
		t.Fatalf("BaoPath collided for distinct Fields %q and %q: %q", a.Field, b.Field, BaoPath(a))
	}
}
