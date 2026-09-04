package zitadeladmin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// passwordResetProviderShape mirrors internal/auth.PasswordResetProvider's
// method set. Redeclared locally, following gipadmin's own
// password_reset_provider_test.go convention, rather than importing
// internal/auth directly: this proves *Client's methods are reachable
// through dynamic dispatch, which is the property that actually matters,
// without taking on a dependency from the provider package back up to the
// consumer package.
type passwordResetProviderShape interface {
	SendPasswordResetOobCode(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, oobCode, newPassword string) error
}

// gipDeleterShape mirrors internal/account's unexported gipDeleter
// interface (DeleteAccount(ctx, uid) error).
type gipDeleterShape interface {
	DeleteAccount(ctx context.Context, uid string) error
}

func TestClient_SatisfiesPasswordResetProviderShape(t *testing.T) {
	// sendHit and resetHit are each set from their OWN request path, not
	// from a shared branch: a shared "any POST" branch would leave both
	// true even if only one of the two calls under test actually ran,
	// silently weakening the two separate assertions below to one.
	var sendHit, resetHit bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse(humanEntry("user-1", "merchant@example.com", true)))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/password_reset"):
			sendHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"verificationCode":"code-1"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/password"):
			resetHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	var provider passwordResetProviderShape = c
	oobCode, err := provider.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
	if err != nil {
		t.Fatalf("SendPasswordResetOobCode via interface: %v", err)
	}
	if !sendHit {
		t.Error("expected the password_reset endpoint to be hit")
	}

	// ResetPassword against the same fake server, which answers every POST
	// with 200, proving dispatch reaches the real method.
	if err := provider.ResetPassword(context.Background(), oobCode, "correct horse battery staple"); err != nil {
		t.Fatalf("ResetPassword via interface: %v", err)
	}
	if !resetHit {
		t.Error("expected dispatch to reach ResetPassword's HTTP call")
	}
}

func TestClient_SatisfiesGipDeleterShape(t *testing.T) {
	var hit bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	var deleter gipDeleterShape = c
	if err := deleter.DeleteAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("DeleteAccount via interface: %v", err)
	}
	if !hit {
		t.Error("expected dispatch to reach DeleteAccount's HTTP call")
	}
}
