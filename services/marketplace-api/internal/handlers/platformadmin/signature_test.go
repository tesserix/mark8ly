package platformadmin_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// The canonical string is where every design decision lives — field order,
// separator, how an absent body is hashed. Assert it byte-exactly. If the
// console disagrees with mark8ly, this is the artifact both sides compare.
func TestCanonicalStringIsExact(t *testing.T) {
	in := platformadmin.SignatureInput{
		Method:     "get",
		Path:       "/api/v1/platform/admin/audit-logs",
		RawQuery:   "since_hours=720&limit=200",
		Body:       nil,
		Timestamp:  "1755859200",
		Nonce:      "018f3c2a-0000-7000-8000-000000000001",
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}

	got, err := platformadmin.CanonicalString(in)
	require.NoError(t, err)

	// sha256 of the empty string, for the absent body.
	const emptyBodyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	want := "GET\n" +
		"/api/v1/platform/admin/audit-logs\n" +
		"limit=200&since_hours=720\n" +
		emptyBodyHash + "\n" +
		"1755859200\n" +
		"018f3c2a-0000-7000-8000-000000000001\n" +
		"op_7f3a\n" +
		"audit.read"

	require.Equal(t, want, got)
}

func TestCanonicalQuerySortsKeysAndValues(t *testing.T) {
	got, err := platformadmin.CanonicalQuery("b=2&a=z&a=a")
	require.NoError(t, err)
	require.Equal(t, "a=a&a=z&b=2", got)
}

func TestCanonicalQueryEmpty(t *testing.T) {
	got, err := platformadmin.CanonicalQuery("")
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestVerifyAcceptsOwnSignature(t *testing.T) {
	in := platformadmin.SignatureInput{
		Method: "POST", Path: "/api/v1/platform/admin/tenants/t1/suspend",
		Body:      []byte(`{"reason_code":"fraud"}`),
		Timestamp: "1755859200", Nonce: "n1",
		Operator: "op_7f3a", Capability: "tenant.suspend",
	}

	sig, err := platformadmin.Sign("shhh", in)
	require.NoError(t, err)

	ok, err := platformadmin.Verify("shhh", sig, in)
	require.NoError(t, err)
	require.True(t, ok)
}

// Each signed component must actually change the signature. A component that
// does not is a component an attacker can swap after signing.
func TestVerifyRejectsTampering(t *testing.T) {
	base := platformadmin.SignatureInput{
		Method: "POST", Path: "/api/v1/platform/admin/tenants/t1/suspend",
		Body:      []byte(`{"reason_code":"fraud"}`),
		Timestamp: "1755859200", Nonce: "n1",
		Operator: "op_7f3a", Capability: "tenant.suspend",
	}
	sig, err := platformadmin.Sign("shhh", base)
	require.NoError(t, err)

	tampered := map[string]func(*platformadmin.SignatureInput){
		"method":     func(i *platformadmin.SignatureInput) { i.Method = "GET" },
		"path":       func(i *platformadmin.SignatureInput) { i.Path = "/api/v1/platform/admin/tenants/t2/suspend" },
		"query":      func(i *platformadmin.SignatureInput) { i.RawQuery = "force=true" },
		"body":       func(i *platformadmin.SignatureInput) { i.Body = []byte(`{"reason_code":"other"}`) },
		"timestamp":  func(i *platformadmin.SignatureInput) { i.Timestamp = "1755859999" },
		"nonce":      func(i *platformadmin.SignatureInput) { i.Nonce = "n2" },
		"operator":   func(i *platformadmin.SignatureInput) { i.Operator = "op_evil" },
		"capability": func(i *platformadmin.SignatureInput) { i.Capability = "tenant.purge" },
	}

	for name, mutate := range tampered {
		t.Run(name, func(t *testing.T) {
			in := base
			in.Body = append([]byte(nil), base.Body...)
			mutate(&in)

			ok, err := platformadmin.Verify("shhh", sig, in)
			require.NoError(t, err)
			require.False(t, ok, "%s must be covered by the signature", name)
		})
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	in := platformadmin.SignatureInput{Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n"}
	sig, err := platformadmin.Sign("right", in)
	require.NoError(t, err)

	ok, err := platformadmin.Verify("wrong", sig, in)
	require.NoError(t, err)
	require.False(t, ok)
}

// Verify must accept an uppercase-hex signature: .NET's BitConverter.ToString
// and several Java HMAC helpers emit uppercase by default. A naive
// string-equality check against our (always-lowercase) output would fail
// this with an unexplained 401.
func TestVerifyAcceptsUppercaseHexSignature(t *testing.T) {
	in := platformadmin.SignatureInput{
		Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n",
		Operator: "op_7f3a", Capability: "audit.read",
	}
	sig, err := platformadmin.Sign("shhh", in)
	require.NoError(t, err)

	ok, err := platformadmin.Verify("shhh", strings.ToUpper(sig), in)
	require.NoError(t, err)
	require.True(t, ok)
}

// A presented signature that isn't valid hex at all is a failed
// verification, not a caller error — indistinguishable in effect from a
// client that simply got the signature wrong.
func TestVerifyRejectsMalformedHexSignature(t *testing.T) {
	in := platformadmin.SignatureInput{Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n"}

	ok, err := platformadmin.Verify("shhh", "not-hex-at-all!!", in)
	require.NoError(t, err)
	require.False(t, ok)
}

// An empty secret reaching this layer is a misconfiguration, not something
// that should silently produce a valid-looking HMAC.
func TestSignRejectsEmptySecret(t *testing.T) {
	in := platformadmin.SignatureInput{Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n"}

	_, err := platformadmin.Sign("", in)
	require.Error(t, err)
}

func TestVerifyRejectsEmptySecret(t *testing.T) {
	in := platformadmin.SignatureInput{Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n"}

	_, err := platformadmin.Verify("", "deadbeef", in)
	require.Error(t, err)
}

// CanonicalString joins fields with "\n" and has no length prefixes, so
// without this guard ("a", "b\nc") and ("a\nb", "c") would produce
// byte-identical canonical strings. Not exploitable today (RawQuery is
// percent-escaped, Body is a fixed-width hash, net/http rejects '\n' in
// header values) but the invariant must be explicit, not accidental —
// especially for Path, which is populated from a decoded URL where a
// literal '%0A' becomes a real newline.
func TestCanonicalStringRejectsLineBreaksInSignedFields(t *testing.T) {
	base := platformadmin.SignatureInput{
		Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n",
		Operator: "op_7f3a", Capability: "audit.read",
	}

	cases := map[string]func(*platformadmin.SignatureInput){
		"method":     func(i *platformadmin.SignatureInput) { i.Method = "GET\n" },
		"path":       func(i *platformadmin.SignatureInput) { i.Path = "/x\ny" },
		"timestamp":  func(i *platformadmin.SignatureInput) { i.Timestamp = "1\r" },
		"nonce":      func(i *platformadmin.SignatureInput) { i.Nonce = "n\n" },
		"operator":   func(i *platformadmin.SignatureInput) { i.Operator = "op\n7f3a" },
		"capability": func(i *platformadmin.SignatureInput) { i.Capability = "audit\r\n.read" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := base
			mutate(&in)

			_, err := platformadmin.CanonicalString(in)
			require.Error(t, err, "%s must reject embedded newlines/carriage returns", name)
		})
	}
}

// vectorFile mirrors the JSON shape written by cmd/genvectors. It is a
// committed contract document produced by a `main` that nothing re-runs, so
// this test is the guard against silent drift between signature.go and the
// published testdata/vectors.json.
type vectorFile struct {
	Name          string `json:"name"`
	Secret        string `json:"secret"`
	Method        string `json:"method"`
	RequestTarget string `json:"request_target"`
	Path          string `json:"path"`
	RawQuery      string `json:"raw_query"`
	Body          string `json:"body"`
	Timestamp     string `json:"timestamp"`
	Nonce         string `json:"nonce"`
	Operator      string `json:"operator"`
	Capability    string `json:"capability"`
	Canonical     string `json:"canonical"`
	Signature     string `json:"signature"`
}

func TestTestdataVectorsMatchImplementation(t *testing.T) {
	raw, err := os.ReadFile("testdata/vectors.json")
	require.NoError(t, err)

	var vectors []vectorFile
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors, "testdata/vectors.json must contain at least one vector")

	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			in := platformadmin.SignatureInput{
				Method:     v.Method,
				Path:       v.Path,
				RawQuery:   v.RawQuery,
				Body:       []byte(v.Body),
				Timestamp:  v.Timestamp,
				Nonce:      v.Nonce,
				Operator:   v.Operator,
				Capability: v.Capability,
			}

			canonical, err := platformadmin.CanonicalString(in)
			require.NoError(t, err)
			require.Equal(t, v.Canonical, canonical, "canonical string drifted from testdata/vectors.json")

			sig, err := platformadmin.Sign(v.Secret, in)
			require.NoError(t, err)
			require.Equal(t, v.Signature, sig, "signature drifted from testdata/vectors.json")
		})
	}
}
