package platformadmin_test

import (
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
		Path:       "/api/v1/admin/audit-logs",
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
		"/api/v1/admin/audit-logs\n" +
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
		Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
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
		Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
		Body:      []byte(`{"reason_code":"fraud"}`),
		Timestamp: "1755859200", Nonce: "n1",
		Operator: "op_7f3a", Capability: "tenant.suspend",
	}
	sig, err := platformadmin.Sign("shhh", base)
	require.NoError(t, err)

	tampered := map[string]func(*platformadmin.SignatureInput){
		"method":     func(i *platformadmin.SignatureInput) { i.Method = "GET" },
		"path":       func(i *platformadmin.SignatureInput) { i.Path = "/api/v1/admin/tenants/t2/suspend" },
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
