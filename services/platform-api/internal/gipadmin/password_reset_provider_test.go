package gipadmin

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// passwordResetProviderShape mirrors internal/auth.PasswordResetProvider's
// method set. It is redeclared locally, rather than importing internal/auth,
// because internal/auth imports gipadmin — importing auth back from a
// gipadmin_test file (compiled as part of package gipadmin) would be a
// import cycle. auth/service.go itself carries the compile-time assertion
// (`var _ PasswordResetProvider = (*gipadmin.AdminClient)(nil)`) proving
// *AdminClient satisfies the real interface; this test additionally proves
// the methods are reachable through dispatch, not just structurally
// matching, by driving both calls through a variable of this shape against
// a real HTTP round trip.
type passwordResetProviderShape interface {
	SendPasswordResetOobCode(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, oobCode, newPassword string) error
}

// TestAdminClient_SatisfiesPasswordResetProviderShape proves
// *AdminClient's SendPasswordResetOobCode and ResetPassword are reachable
// through an interface value — i.e. dynamic dispatch actually reaches the
// real HTTP-calling methods — not merely that the method set matches.
func TestAdminClient_SatisfiesPasswordResetProviderShape(t *testing.T) {
	var sendHit, resetHit bool

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "accounts:sendOobCode"):
			sendHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"oobCode":"oob-reachable"}`))
		case strings.HasSuffix(r.URL.Path, "accounts:resetPassword"):
			resetHit = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	var provider passwordResetProviderShape = c

	code, err := provider.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
	if err != nil {
		t.Fatalf("SendPasswordResetOobCode through interface: %v", err)
	}
	if code != "oob-reachable" {
		t.Fatalf("code = %q, want %q", code, "oob-reachable")
	}
	if !sendHit {
		t.Fatal("SendPasswordResetOobCode dispatched through the interface never reached the HTTP layer")
	}

	if err := provider.ResetPassword(context.Background(), "oob-reachable", "correct horse battery staple"); err != nil {
		t.Fatalf("ResetPassword through interface: %v", err)
	}
	if !resetHit {
		t.Fatal("ResetPassword dispatched through the interface never reached the HTTP layer")
	}
}
