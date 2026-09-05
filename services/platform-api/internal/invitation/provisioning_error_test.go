package invitation

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/authz"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakePolicyErr stands in for *zitadeladmin.policyError: an error that
// names the password rule the identity provider rejected. It is
// deliberately built from the interface this package declares rather
// than by importing zitadeladmin — if that import ever becomes necessary
// the layering has slipped, and this test would keep compiling only
// because it never had one.
type fakePolicyErr struct {
	rule    string
	message string
	// cause is what the provider's own error string looked like. The
	// production shape carries no credential; a test can therefore only
	// prove redaction works by planting one here.
	cause string
}

func (e *fakePolicyErr) Error() string { return e.cause }
func (e *fakePolicyErr) PasswordPolicyRule() (string, string) {
	return e.rule, e.message
}

// captureLog redirects the standard logger for the duration of fn and
// returns everything written to it.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	fn()
	return buf.String()
}

// TestProvisioningError_PolicyViolationIsA400WithTheRuleNamed is the
// user-visible half of the fix. A password the merchant can correct must
// not be reported as a server fault, and the message must say which rule
// they broke — the old copy ("try the invitation link again") sent the
// invitee round the same loop with the same password.
func TestProvisioningError_PolicyViolationIsA400WithTheRuleNamed(t *testing.T) {
	err := &fakePolicyErr{
		rule:    "too_short",
		message: "That password is too short — it needs at least 12 characters.",
		cause:   "zitadeladmin: password policy violation too_short (COMMA-HuJf6)",
	}

	var got error
	_ = captureLog(t, func() {
		got = provisioningError(err, "inv-1", "tid-1", "Test@123_01")
	})

	ae, ok := apperrors.As(got)
	if !ok {
		t.Fatalf("provisioningError = %v, want an AppError", got)
	}
	if ae.Status != 400 {
		t.Errorf("status = %d, want 400 — a policy violation is the caller's input, not a server fault", ae.Status)
	}
	if ae.Code != "password_policy" {
		t.Errorf("code = %q, want password_policy (distinct from provisioning_failed)", ae.Code)
	}
	if ae.Message != err.message {
		t.Errorf("message = %q, want the rule-specific text %q", ae.Message, err.message)
	}
	if !errors.Is(got, err) {
		t.Error("the underlying provider error must stay in the chain")
	}
}

// TestProvisioningError_NonPolicyFailureStaysA500 pins that everything
// that is genuinely a server fault keeps its old code, status and copy.
func TestProvisioningError_NonPolicyFailureStaysA500(t *testing.T) {
	var got error
	out := captureLog(t, func() {
		got = provisioningError(errors.New("zitadeladmin: POST /v2/users/human: status 503"), "inv-1", "tid-1", "Test@123_01")
	})

	ae, ok := apperrors.As(got)
	if !ok {
		t.Fatalf("provisioningError = %v, want an AppError", got)
	}
	if ae.Status != 500 || ae.Code != "provisioning_failed" {
		t.Errorf("got %d/%s, want 500/provisioning_failed", ae.Status, ae.Code)
	}
	if !strings.Contains(out, "status 503") {
		t.Errorf("log = %q, want it to carry the underlying cause — the incident's second half was that this logged nothing", out)
	}
}

// TestProvisioningError_LogsTheCauseAtError is the operability half of
// the fix: the reason diagnosing this took twenty minutes is that
// provisioning_failed logged nothing at all.
func TestProvisioningError_LogsTheCauseAtError(t *testing.T) {
	err := &fakePolicyErr{
		rule:    "no_symbol",
		message: "That password needs a symbol.",
		cause:   "zitadeladmin: password policy violation no_symbol (COMMA-ZDLwA)",
	}
	out := captureLog(t, func() {
		_ = provisioningError(err, "inv-42", "tid-7", "Test@123_01")
	})

	for _, want := range []string{"ERROR", "COMMA-ZDLwA", "no_symbol", "inv-42", "tid-7"} {
		if !strings.Contains(out, want) {
			t.Errorf("log = %q, want it to contain %q", out, want)
		}
	}
}

// TestProvisioningError_NeverLogsThePassword is the non-negotiable one.
// Both branches are exercised with a cause string that embeds the
// credential, which is what a careless upstream %v of a request body
// would produce.
func TestProvisioningError_NeverLogsThePassword(t *testing.T) {
	const password = "Test@123_01"

	cases := []struct {
		name string
		err  error
	}{
		{
			name: "policy violation",
			err: &fakePolicyErr{
				rule:    "too_short",
				message: "That password is too short.",
				cause:   `create human user: body {"password":{"password":"` + password + `"}}`,
			},
		},
		{
			name: "generic failure",
			err:  errors.New(`POST /v2/users/human body {"password":{"password":"` + password + `"}}: status 400`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureLog(t, func() {
				_ = provisioningError(tc.err, "inv-1", "tid-1", password)
			})
			if strings.Contains(out, password) {
				t.Fatalf("the chosen password leaked into the log: %q", out)
			}
			if !strings.Contains(out, "[redacted]") {
				t.Errorf("log = %q, want the credential replaced by [redacted]", out)
			}
		})
	}
}

// TestRedactPassword_EmptyPasswordIsANoOp guards the sharp edge in
// strings.ReplaceAll: an empty old value splices the replacement between
// every character, which would turn a clean log line into noise.
func TestRedactPassword_EmptyPasswordIsANoOp(t *testing.T) {
	const s = "status 503"
	if got := redactPassword(s, ""); got != s {
		t.Errorf("redactPassword(%q, \"\") = %q, want it unchanged", s, got)
	}
}

// TestAccept_Zitadel_PasswordPolicyFailureSurfacesAs400 drives the whole
// thing through Accept, proving the classification survives the real
// call path and that nothing is half-provisioned on the way out.
func TestAccept_Zitadel_PasswordPolicyFailureSurfacesAs400(t *testing.T) {
	repo := &fakeRepo{inv: pendingInvitation()}
	fga := authz.NewFake()
	prov := &fakeProvisioner{t: t, err: &fakePolicyErr{
		rule:    "too_short",
		message: "That password is too short — it needs at least 12 characters.",
		cause:   "zitadeladmin: password policy violation too_short (COMMA-HuJf6)",
	}}
	svc := NewService(Config{Repo: repo, FGA: fga, Provisioner: prov})

	var err error
	out := captureLog(t, func() {
		_, err = svc.Accept(context.Background(), zitadelAcceptInput())
	})

	ae, ok := apperrors.As(err)
	if !ok || ae.Status != 400 || ae.Code != "password_policy" {
		t.Fatalf("err = %v, want a 400 password_policy AppError", err)
	}
	if !strings.Contains(ae.Message, "12 characters") {
		t.Errorf("message = %q, want it to name the rule the invitee broke", ae.Message)
	}
	if strings.Contains(out, zitadelAcceptInput().Password) {
		t.Error("the submitted password reached the log")
	}
	if fga.WriteCallCount() != 0 || repo.acceptedID != "" {
		t.Error("a rejected password must leave the invitation pending and write no tuples")
	}
}
