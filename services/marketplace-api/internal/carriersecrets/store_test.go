package carriersecrets

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// TestScope_Validate pins the "required field" contract. A handler
// that builds a Scope with an empty TenantID/domain/provider/field
// must be rejected before the DB write so we don't end up with
// malformed GCP SM secret IDs.
func TestScope_Validate(t *testing.T) {
	cases := []struct {
		name    string
		scope   Scope
		wantErr bool
	}{
		{"all-fields", Scope{TenantID: "t", Domain: "shipping", Provider: "delhivery", Field: "api_key"}, false},
		{"no-tenant", Scope{Domain: "shipping", Provider: "delhivery", Field: "api_key"}, true},
		{"no-domain", Scope{TenantID: "t", Provider: "delhivery", Field: "api_key"}, true},
		{"no-provider", Scope{TenantID: "t", Domain: "shipping", Field: "api_key"}, true},
		{"no-field", Scope{TenantID: "t", Domain: "shipping", Provider: "delhivery"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.scope.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSecretName_CanonicalLayout(t *testing.T) {
	s := Scope{
		TenantID: "4a47610c-3f0c-4ef7-a64c-892480c4635e",
		Domain:   "shipping",
		Provider: "delhivery",
		Field:    "api_key",
	}
	got := SecretName("mark8ly-prod", s)
	want := "mark8ly-prod-4a47610c-3f0c-4ef7-a64c-892480c4635e-shipping-delhivery-api_key"
	if got != want {
		t.Fatalf("SecretName = %q, want %q", got, want)
	}
}

func TestSecretName_SanitizesForbiddenCharacters(t *testing.T) {
	s := Scope{TenantID: "t/1", Domain: "ship.ping", Provider: "del!hivery", Field: "api key"}
	got := SecretName("p", s)
	// Every '/', '.', '!', ' ' must have been replaced with '_'.
	for _, ch := range []string{"/", ".", "!", " "} {
		if strings.Contains(got, ch) {
			t.Errorf("secret name leaked forbidden char %q: %s", ch, got)
		}
	}
}

func TestFormatReference_And_ParseReference(t *testing.T) {
	s := Scope{TenantID: "tenant", Domain: "payment", Provider: "razorpay", Field: "secret_key"}
	ref := FormatReference("proj", "mark8ly-dev", s)
	if !strings.HasPrefix(ref, GSMRefPrefix) {
		t.Fatalf("reference missing gsm:// prefix: %s", ref)
	}
	res, ok := ParseReference(ref)
	if !ok {
		t.Fatalf("ParseReference rejected a value we just formatted: %s", ref)
	}
	if res != "projects/proj/secrets/mark8ly-dev-tenant-payment-razorpay-secret_key" {
		t.Fatalf("parsed resource wrong: %s", res)
	}
}

func TestIsInlineRef(t *testing.T) {
	if !IsInlineRef("noop:abc") {
		t.Error("noop:abc should be an inline ref")
	}
	if !IsInlineRef("aes:abc") {
		t.Error("aes:abc should be an inline ref")
	}
	if IsInlineRef("gsm://projects/x/secrets/y") {
		t.Error("gsm:// is not inline")
	}
	if IsInlineRef("plain") {
		t.Error("unknown shapes are not inline")
	}
}

// ─────────────────────────────────────────────────────────────────────
// FakeClient sanity — covers the in-memory adapter on its own so
// hybrid_test can focus on Store-layer behaviour.
// ─────────────────────────────────────────────────────────────────────

func TestFakeClient_RoundTrip(t *testing.T) {
	fc := NewFakeClient()
	ctx := context.Background()
	if err := fc.CreateOrAddVersion(ctx, "projects/p/secrets/s", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	got, err := fc.AccessLatest(ctx, "projects/p/secrets/s")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v1" {
		t.Fatalf("got %q, want v1", got)
	}
	if !fc.Has("projects/p/secrets/s") {
		t.Fatal("Has returned false for a key we just wrote")
	}
	if err := fc.DeleteSecret(ctx, "projects/p/secrets/s"); err != nil {
		t.Fatal(err)
	}
	if fc.Has("projects/p/secrets/s") {
		t.Fatal("Has still true after DeleteSecret")
	}
	if _, err := fc.AccessLatest(ctx, "projects/p/secrets/s"); err != ErrSecretNotFound {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestFakeClient_DeleteMissingIsSuccess(t *testing.T) {
	fc := NewFakeClient()
	if err := fc.DeleteSecret(context.Background(), "projects/p/secrets/nope"); err != nil {
		t.Fatalf("DeleteSecret on missing key: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// InlineStore — fallback for dev/CI without GCP creds.
// ─────────────────────────────────────────────────────────────────────

func TestInlineStore_RoundTrip(t *testing.T) {
	s := NewInlineStore(crypto.NewNoopEncryptor())
	ctx := context.Background()
	ref, err := s.Put(ctx, Scope{TenantID: "t", Domain: "shipping", Provider: "delhivery", Field: "api_key"}, "my-token")
	if err != nil {
		t.Fatal(err)
	}
	if !IsInlineRef(ref) {
		t.Fatalf("InlineStore produced non-inline ref: %s", ref)
	}
	got, err := s.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-token" {
		t.Fatalf("got %q, want my-token", got)
	}
}

func TestInlineStore_GetEmptyRef(t *testing.T) {
	s := NewInlineStore(crypto.NewNoopEncryptor())
	got, err := s.Get(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestInlineStore_GetGSMFails(t *testing.T) {
	s := NewInlineStore(crypto.NewNoopEncryptor())
	_, err := s.Get(context.Background(), "gsm://projects/p/secrets/x")
	if err == nil {
		t.Fatal("InlineStore accepted a gsm:// ref; it shouldn't")
	}
}

func TestInlineStore_ValidatesScope(t *testing.T) {
	s := NewInlineStore(crypto.NewNoopEncryptor())
	if _, err := s.Put(context.Background(), Scope{}, "x"); err == nil {
		t.Fatal("Put accepted an empty scope")
	}
}
