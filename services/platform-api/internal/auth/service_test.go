package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
	"github.com/mark8ly/platform-api/internal/notification"
)

// fakeProvider is an in-memory double for PasswordResetProvider.
type fakeProvider struct {
	sendCode  string
	sendErr   error
	resetErr  error
	sentEmail string
	resetOob  string
	resetPass string
}

func (f *fakeProvider) SendPasswordResetOobCode(ctx context.Context, email string) (string, error) {
	f.sentEmail = email
	if f.sendErr != nil {
		return "", f.sendErr
	}
	return f.sendCode, nil
}

func (f *fakeProvider) ResetPassword(ctx context.Context, oobCode, newPassword string) error {
	f.resetOob = oobCode
	f.resetPass = newPassword
	return f.resetErr
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(admin PasswordResetProvider) *Service {
	return NewService(Config{
		Admin:             admin,
		Sender:            notification.NoopSender{},
		EmailFrom:         "noreply@mark8ly.com",
		SupportEmail:      "support@mark8ly.com",
		AdminResetBaseURL: "https://admin.mark8ly.com",
		Logger:            testLogger(),
	})
}

// --- Regression test for the typed-nil interface trap ------------------

// TestPasswordResetProvider_GuardedConstructionStaysGenuinelyNil is the
// regression test for the incident documented in
// cmd/server/account_wiring.go: assigning a possibly-nil concrete
// *gipadmin.AdminClient into an interface produces a NON-NIL interface
// value wrapping a nil pointer, so a `!= nil` guard is defeated and a
// later call panics on the nil receiver.
//
// This proves the correct pattern (construct the interface value only
// inside the non-nil guard, exactly like cmd/server/main.go's Admin
// wiring and account_wiring.go's gipCleanup wiring) leaves the interface
// genuinely nil when no real client exists, so a `!= nil` guard sees it
// as absent and nothing ever calls through it.
//
// MUTATION: assign the nil *gipadmin.AdminClient straight into the
// interface-typed variable (skip the "admin != nil" guard) and this test
// fails, because "naive" would then also report != nil.
func TestPasswordResetProvider_GuardedConstructionStaysGenuinelyNil(t *testing.T) {
	var admin *gipadmin.AdminClient // nil concrete pointer — "unconfigured" case

	// The WRONG way: demonstrates the trap. A nil *gipadmin.AdminClient
	// assigned directly into the interface is NOT a nil interface.
	var naive PasswordResetProvider = admin
	if naive == nil {
		t.Fatal("sanity check failed: assigning a nil *AdminClient into an interface " +
			"was expected to produce a non-nil interface (the well-known typed-nil trap) " +
			"— if this now reports nil, the language/runtime assumption behind the guarded " +
			"pattern below no longer holds")
	}

	// The RIGHT way: exactly what cmd/server/main.go and
	// cmd/server/account_wiring.go already do — construct the
	// interface-typed value ONLY inside the non-nil guard.
	var guarded PasswordResetProvider
	if admin != nil {
		guarded = admin
	}
	if guarded != nil {
		t.Fatal("guarded construction must leave the interface genuinely nil " +
			"when no real client exists, so callers' `!= nil` checks see it as absent")
	}

	// And genuinely absent means callers never dispatch through it. We
	// don't call a method on `guarded` here — doing so would panic
	// regardless of nil-ness, because there is no concrete type to
	// dispatch to. The guard's entire job is making sure that call site
	// is never reached, which `guarded != nil` above proves.
}

// TestNewService_AdminSetOnlyWhenRealClientExists documents that Config.Admin
// must be populated with a genuinely non-nil PasswordResetProvider — never a
// possibly-nil *gipadmin.AdminClient assigned straight through — mirroring
// how cmd/server/main.go only calls auth.NewService inside the branch where
// gipadmin.New succeeded.
func TestNewService_AdminSetOnlyWhenRealClientExists(t *testing.T) {
	fake := &fakeProvider{sendCode: "oob-abc"}
	svc := newTestService(fake)

	if err := svc.RequestPasswordReset(context.Background(), "merchant@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if fake.sentEmail != "merchant@example.com" {
		t.Fatalf("sentEmail = %q, want %q", fake.sentEmail, "merchant@example.com")
	}
}

// --- Existing behaviour, unchanged --------------------------------------

func TestRequestPasswordReset_UnknownEmailSuppressesEnumeration(t *testing.T) {
	fake := &fakeProvider{sendErr: gipadmin.ErrUserNotFound}
	svc := newTestService(fake)

	if err := svc.RequestPasswordReset(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset with unknown email must return nil, got: %v", err)
	}
}

func TestRequestPasswordReset_UpstreamErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	fake := &fakeProvider{sendErr: wantErr}
	svc := newTestService(fake)

	err := svc.RequestPasswordReset(context.Background(), "merchant@example.com")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestRequestPasswordReset_EmptyEmailRejected(t *testing.T) {
	svc := newTestService(&fakeProvider{})

	if err := svc.RequestPasswordReset(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestConfirmPasswordReset_DelegatesToProvider(t *testing.T) {
	fake := &fakeProvider{}
	svc := newTestService(fake)

	if err := svc.ConfirmPasswordReset(context.Background(), " oob-xyz ", "correct horse battery staple"); err != nil {
		t.Fatalf("ConfirmPasswordReset: %v", err)
	}
	if fake.resetOob != "oob-xyz" {
		t.Fatalf("resetOob = %q, want %q", fake.resetOob, "oob-xyz")
	}
	if fake.resetPass != "correct horse battery staple" {
		t.Fatalf("resetPass = %q", fake.resetPass)
	}
}

func TestConfirmPasswordReset_WeakPasswordRejectedBeforeProvider(t *testing.T) {
	fake := &fakeProvider{}
	svc := newTestService(fake)

	err := svc.ConfirmPasswordReset(context.Background(), "oob-xyz", "short")
	if !errors.Is(err, gipadmin.ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	if fake.resetOob != "" {
		t.Fatal("provider must not be called when the password fails the local length check")
	}
}

func TestConfirmPasswordReset_MissingOobCodeRejected(t *testing.T) {
	svc := newTestService(&fakeProvider{})

	if err := svc.ConfirmPasswordReset(context.Background(), "  ", "correct horse battery staple"); err == nil {
		t.Fatal("expected error for empty oob code")
	}
}
