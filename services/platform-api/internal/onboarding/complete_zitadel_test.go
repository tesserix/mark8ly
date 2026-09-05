package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Issue #685 — the halves of onboarding completion that need no
// database. Everything here exercises the code that runs BEFORE the
// completion transaction, which is exactly where the Zitadel
// provisioning step was added and exactly where its failures must be
// contained. The full commit path (both FGA tuples, the GIP claim) is
// covered against a real database in complete_zitadel_integration_test.go.

// fakeSessionRepo serves one verified session and is a tripwire on
// everything else: the tests below must never reach a write.
type fakeSessionRepo struct {
	sess *Session
	t    *testing.T
}

func (f *fakeSessionRepo) Create(context.Context, *Session) error {
	f.t.Fatal("Create called")
	return nil
}
func (f *fakeSessionRepo) GetByID(_ context.Context, id string) (*Session, error) {
	if f.sess == nil || f.sess.ID != id {
		return nil, apperrors.NotFound("session_not_found", "no such session")
	}
	return f.sess, nil
}
func (f *fakeSessionRepo) UpdateDraft(context.Context, string, json.RawMessage) error {
	f.t.Fatal("UpdateDraft called")
	return nil
}
func (f *fakeSessionRepo) MarkEmailVerified(context.Context, string) error {
	f.t.Fatal("MarkEmailVerified called")
	return nil
}
func (f *fakeSessionRepo) CompleteInTx(context.Context, *gorm.DB, string, string) error {
	f.t.Fatal("CompleteInTx called — completion should have aborted before the transaction")
	return nil
}
func (f *fakeSessionRepo) GetFunnel(context.Context, FunnelFilter) (*FunnelStats, error) {
	f.t.Fatal("GetFunnel called")
	return nil, nil
}
func (f *fakeSessionRepo) ListSessions(context.Context, FunnelFilter) ([]SessionRow, int64, error) {
	f.t.Fatal("ListSessions called")
	return nil, 0, nil
}

// recordingProvisioner captures the arguments Complete passes and
// answers with a fixed result.
type recordingProvisioner struct {
	uid   string
	err   error
	calls int
	email string
	first string
	last  string
	pass  string
}

func (p *recordingProvisioner) ProvisionStaff(_ context.Context, email, first, last, password string) (string, error) {
	p.calls++
	p.email, p.first, p.last, p.pass = email, first, last, password
	return p.uid, p.err
}

// policyErr satisfies the passwordPolicyViolation interface the same way
// *zitadeladmin.policyError does, without importing that package (which
// this one deliberately does not know about).
type policyErr struct{}

func (policyErr) Error() string { return "zitadeladmin: password is shorter than the policy minimum" }
func (policyErr) PasswordPolicyRule() (string, string) {
	return "too_short", "Password must be at least 12 characters, with an uppercase letter, a lowercase letter, a number, and a symbol."
}

func verifiedSession(t *testing.T) *Session {
	t.Helper()
	now := time.Now()
	return &Session{
		ID:              "sess-685",
		Email:           "founder@example.test",
		Draft:           json.RawMessage(`{}`),
		Status:          StatusInProgress,
		EmailVerifiedAt: &now,
	}
}

func zitadelRequest() CompleteRequest {
	return CompleteRequest{
		SessionID:    "sess-685",
		BusinessName: "Bondi Surf Co",
		Slug:         "bondi-surf",
		OwnerEmail:   "Founder@Example.test",
		CountryCode:  "AU",
		CurrencyCode: "AUD",
		Timezone:     "Australia/Sydney",
		FirstName:    "Ada",
		LastName:     "Lovelace",
		// Obviously fake, and still satisfies the live policy (12+
		// chars, upper, lower, number, symbol) so it exercises the real
		// validation rather than tripping it.
		Password: "Not-A-Real-Password-1!",
	}
}

// The Zitadel path must abort the whole completion when provisioning
// fails — no tenant, no store, no outbox row. The fake repository above
// fails the test from inside CompleteInTx if the transaction is ever
// entered, so this asserts the ordering and not merely the error.
func TestComplete_ZitadelProvisioningFailureAborts(t *testing.T) {
	prov := &recordingProvisioner{err: errors.New("zitadel unreachable")}
	svc := NewService(Config{
		Repo:        &fakeSessionRepo{sess: verifiedSession(t), t: t},
		Provisioner: prov,
	})

	_, err := svc.Complete(context.Background(), zitadelRequest())
	if err == nil {
		t.Fatal("expected Complete to fail when provisioning fails")
	}
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected an AppError, got %T", err)
	}
	if ae.Status != 500 || ae.Code != "provisioning_failed" {
		t.Fatalf("got %d %s, want 500 provisioning_failed", ae.Status, ae.Code)
	}
	if prov.calls != 1 {
		t.Fatalf("provisioner called %d times, want 1", prov.calls)
	}
}

// A password the identity provider rejects is the MERCHANT'S input being
// wrong, so it must come back as 400 with the specific rule named —
// not the opaque 500 that had an invitee retyping the same password.
func TestComplete_ZitadelPasswordPolicyRejectionNamesTheRule(t *testing.T) {
	prov := &recordingProvisioner{err: policyErr{}}
	svc := NewService(Config{
		Repo:        &fakeSessionRepo{sess: verifiedSession(t), t: t},
		Provisioner: prov,
	})

	_, err := svc.Complete(context.Background(), zitadelRequest())
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("expected an AppError, got %T (%v)", err, err)
	}
	if ae.Status != 400 {
		t.Fatalf("status = %d, want 400 — a bad password is not a server fault", ae.Status)
	}
	if ae.Code != "password_policy" {
		t.Fatalf("code = %q, want password_policy (the code apps/onboarding branches on)", ae.Code)
	}
	if !strings.Contains(ae.Message, "12 characters") {
		t.Fatalf("message %q does not name the rule that was broken", ae.Message)
	}
	if strings.Contains(ae.Message, "Not-A-Real-Password-1!") {
		t.Fatal("the rejected password was echoed back to the caller")
	}
}

// Complete must hand the provisioner a LOWERCASED email. Every
// email-keyed FGA tuple is lowercase and the login path folds to lower
// server-side, so a merchant who typed Founder@Example.test would
// otherwise miss their own membership.
func TestComplete_ZitadelNormalisesTheEmailItProvisions(t *testing.T) {
	prov := &recordingProvisioner{err: errors.New("stop here")}
	svc := NewService(Config{
		Repo:        &fakeSessionRepo{sess: verifiedSession(t), t: t},
		Provisioner: prov,
	})

	_, _ = svc.Complete(context.Background(), zitadelRequest())

	if prov.email != "founder@example.test" {
		t.Fatalf("provisioned email = %q, want the lowercased address", prov.email)
	}
	if prov.first != "Ada" || prov.last != "Lovelace" {
		t.Fatalf("profile names = %q/%q, want Ada/Lovelace", prov.first, prov.last)
	}
	if prov.pass != "Not-A-Real-Password-1!" {
		t.Fatalf("password was not passed through verbatim: %q", prov.pass)
	}
}

// Zitadel rejects an empty givenName/familyName outright, so a merchant
// who typed no name must still get a derived one rather than a failed
// signup.
func TestComplete_ZitadelDerivesNamesFromTheEmail(t *testing.T) {
	prov := &recordingProvisioner{err: errors.New("stop here")}
	svc := NewService(Config{
		Repo:        &fakeSessionRepo{sess: verifiedSession(t), t: t},
		Provisioner: prov,
	})
	req := zitadelRequest()
	req.FirstName, req.LastName = "", ""

	_, _ = svc.Complete(context.Background(), req)

	if prov.first == "" || prov.last == "" {
		t.Fatalf("derived names were empty (%q/%q) — Zitadel would reject the create", prov.first, prov.last)
	}
}

// owner_user_id is required on the GIP path and must NOT be on the
// Zitadel path, where the account does not exist yet. The GIP branch's
// error code and message are pinned because the GIP path is the
// fallback and its responses must stay byte-identical.
func TestValidateCompleteRequest_OwnerUserIDRequiredOnlyUnderGIP(t *testing.T) {
	req := zitadelRequest()

	if err := validateCompleteRequest(req, false); err != nil {
		t.Fatalf("Zitadel path rejected a request with no owner_user_id: %v", err)
	}

	err := validateCompleteRequest(req, true)
	ae, ok := apperrors.As(err)
	if !ok {
		t.Fatalf("GIP path accepted a request with no owner_user_id (%v)", err)
	}
	if ae.Status != 400 || ae.Code != "invalid_owner" || ae.Message != "owner_user_id is required" {
		t.Fatalf("GIP-path error changed: %d %s %q", ae.Status, ae.Code, ae.Message)
	}
}

// The GIP path must not construct a provisioner call at all. Asserted
// via the service rather than by reading the code: a nil provisioner is
// what selects it, and a typed-nil interface would silently pass a
// `!= nil` guard and panic here instead.
func TestComplete_GIPPathNeverProvisions(t *testing.T) {
	svc := NewService(Config{Repo: &fakeSessionRepo{sess: verifiedSession(t), t: t}})

	req := zitadelRequest()
	req.OwnerUserID = "" // GIP path: the form supplies one; this is the reject case
	_, err := svc.Complete(context.Background(), req)

	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_owner" {
		t.Fatalf("expected the unchanged GIP validation error, got %v", err)
	}
}
