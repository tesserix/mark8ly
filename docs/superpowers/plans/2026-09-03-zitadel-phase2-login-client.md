# Zitadel Migration Phase 2 — auth-bff Login Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `auth-bff` a Zitadel login client that authenticates a user against Zitadel's Session API v2 and finalizes an OIDC auth request — with MFA enforced in our own fail-closed code — reachable only when a disabled-by-default flag is set, leaving GIP as the live path.

**Architecture:** `AutoLogin`'s ten steps are split: step 2 (identity) becomes provider-specific, steps 3–10 (FGA membership, MFA gate, deviceguard, email OTP, session mint, registry, audit) become a shared `completeLogin` both providers call. A new `internal/zitadellogin` package ports hms's `loginclient` + `sufficiency` design. Nothing in `apps/` changes — the new endpoints exist but no frontend calls them until phase 3.

**Tech Stack:** Go 1.26, Gin, `net/http` (no Zitadel SDK — both precedent repos hand-roll deliberately), stdlib `testing` with hand-written fakes and `httptest.Server`.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md`

## Global Constraints

- **Repo is `mark8ly`; the service is `services/auth-bff`.** The sibling checkout `tesserix-new/auth-bff` is a different, stale service — never touch it.
- **GIP stays live.** Every change is additive and gated. `AutoLogin`'s existing behaviour and its existing tests must be unchanged at the end of Task 1.
- **No new test framework.** This service uses hand-written fakes (`gip/fake.go`, `authz/fake.go`) and stdlib `testing`. No gomock, no testify mocks, no cassettes. Fake Zitadel is an `httptest.Server` with inline JSON literals copied from observed responses.
- **No Zitadel SDK dependency.** `net/http` + `encoding/json` only.
- **Config must not be required.** Add fields with no `required:"true"` tag; gate wiring in `main.go` with the existing `if cfg.X != "" { ... } else { log.Warn(...) }` idiom. A required env var that rejects empty crashloops the deployment on merge and no test catches it.
- **Never log a credential.** Session tokens, the login-client PAT, passwords and TOTP codes are all secrets. Error text must not carry Zitadel's `failedAttempts` counter.
- **Zitadel does not enforce MFA for login clients.** Under `forceMfa: true` it still issues an authorization code for a password-only session. Enforcement is entirely this code's job.
- Instance: `https://auth.tesserix.app` (v4.15.3). Projects `mark8ly-admin` = `389070376568619523`, `mark8ly-storefront` = `389070377390703107`.

---

### Task 1: Split `AutoLogin` so the post-identity gauntlet is shared

Provider-agnostic today, but entangled with GIP verification. Extracting it first means the Zitadel path is genuinely additive rather than a second copy of the gating that can drift out of sync.

**Files:**
- Modify: `services/auth-bff/internal/autologin/service.go` (`AutoLogin`, lines ~181-335)
- Test: `services/auth-bff/internal/autologin/service_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func (s *Service) completeLogin(ctx context.Context, w http.ResponseWriter, id Identity, req Request) (*Result, error)` and `type Identity struct { UID, Email, TenantID string }`. Task 5 calls `completeLogin` from the Zitadel handler.

- [ ] **Step 1: Write the characterisation test that pins current behaviour**

Add to `service_test.go`. This must pass BEFORE the refactor — it is the proof the refactor changes nothing.

```go
func TestCompleteLogin_RunsTheSameGauntletAsAutoLogin(t *testing.T) {
	gipFake := gip.NewFakeVerifier()
	gipFake.Add("good-token", gip.VerifiedToken{UID: "user-1", Email: "u@e.com", TenantID: "MP-Internal-test"})
	fgaFake := authz.NewFake()
	fgaFake.SetMembership("user-1", "tenant-uuid-1")

	svc := newTestService(t, gipFake, fgaFake, fastPolicy)

	viaAutoLogin := httptest.NewRecorder()
	got, err := svc.AutoLogin(context.Background(), viaAutoLogin, Request{
		IDToken: "good-token", ExpectedTenantID: "MP-Internal-test", WorkspaceTenant: "tenant-uuid-1",
	})
	if err != nil {
		t.Fatalf("AutoLogin: %v", err)
	}
	if got.UID != "user-1" || got.TenantID != "tenant-uuid-1" {
		t.Fatalf("result = %+v", got)
	}
	if len(viaAutoLogin.Result().Cookies()) == 0 {
		t.Fatal("AutoLogin minted no cookie")
	}
}
```

- [ ] **Step 2: Run it to confirm it passes against today's code**

Run: `cd services/auth-bff && go test ./internal/autologin/ -run TestCompleteLogin_RunsTheSameGauntletAsAutoLogin -v`
Expected: PASS. If it fails, stop — the test is wrong, not the code.

- [ ] **Step 3: Extract `completeLogin`**

In `service.go`, add above `AutoLogin`:

```go
// Identity is the outcome of authenticating a user, independent of which
// provider did it. Everything downstream of this type — membership, MFA,
// device and OTP gating, session minting — is provider-agnostic and shared
// between the GIP and Zitadel paths.
type Identity struct {
	UID      string
	Email    string
	TenantID string
}
```

Then change `AutoLogin` so that everything after the `gip.VerifyToken` call and its error mapping becomes a call to `completeLogin`:

```go
	tok, err := s.gip.VerifyToken(ctx, req.IDToken, req.ExpectedTenantID)
	if err != nil {
		if errors.Is(err, gip.ErrTenantMismatch) {
			return nil, ErrTenantMismatch
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	return s.completeLogin(ctx, w, Identity{
		UID:      tok.UID,
		Email:    tok.Email,
		TenantID: tok.TenantID,
	}, req)
}

// completeLogin runs every gate that stands between a verified identity and a
// minted session: FGA membership (with the outbox-race retry), the MFA gate,
// new-device evaluation, the email-OTP step-up, then the session cookie,
// registry row and audit event.
//
// It is deliberately provider-agnostic. A Zitadel login that has satisfied its
// own credential and factor checks arrives here with the same Identity a
// verified GIP token produces, and is subject to exactly the same gates — so
// the two providers cannot drift apart in what they enforce after login.
func (s *Service) completeLogin(ctx context.Context, w http.ResponseWriter, id Identity, req Request) (*Result, error) {
```

Move the existing body verbatim into `completeLogin`, replacing every `tok.UID` with `id.UID`, `tok.Email` with `id.Email`, and `tok.TenantID` with `id.TenantID`. Change nothing else — no reordering, no new logging, no behaviour change.

- [ ] **Step 4: Run the whole autologin suite**

Run: `cd services/auth-bff && go test ./internal/autologin/ -v`
Expected: every pre-existing test still passes, unchanged. If any needed editing, the refactor was not behaviour-preserving — revert and redo.

- [ ] **Step 5: Vet and build**

Run: `cd services/auth-bff && go vet ./... && go build ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add services/auth-bff/internal/autologin/
git commit -m "refactor(auth-bff): share the post-identity login gauntlet between providers"
```

---

### Task 2: The Zitadel login client

A direct port of hms's `loginclient`, whose every JSON shape was pinned to observed behaviour against a live v4.15.3 instance rather than to documentation.

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/client.go`
- Test: `services/auth-bff/internal/zitadellogin/client_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func New(baseURL, token string, hc *http.Client) *Client`; methods `AuthRequest(ctx, id) (AuthRequest, error)`, `CreatePasswordSession(ctx, loginName, password string) (Session, error)`, `VerifyTOTP(ctx, s Session, code string) (Session, error)`, `SessionFactors(ctx, sessionID string) (Factors, error)`, `LoginPolicyForOrg(ctx, orgID string) (LoginPolicy, error)`, `InstanceLoginPolicyForDisplay(ctx) (LoginPolicy, error)`; types `Session{ID, Token string}`, `LoginPolicy{ForceMFA, ForceMFALocalOnly bool}`; sentinels `ErrBadCredentials`, `ErrUserNotFound`, `ErrAuthRequestInvalid`, `ErrUnavailable`. Task 3 adds `finalize` and the sufficiency entry points to this same package.

- [ ] **Step 1: Write the failing tests**

Create `client_test.go`. The JSON bodies are the literal shapes observed from Zitadel — do not tidy them.

```go
package zitadellogin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-pat", srv.Client())
}

func TestCreatePasswordSessionReturnsSession(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/sessions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-pat" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sessionId":"389070697432875523","sessionToken":"tok-1"}`))
	})
	s, err := c.CreatePasswordSession(context.Background(), "a@b.test", "pw")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s.ID != "389070697432875523" || s.Token != "tok-1" {
		t.Fatalf("session = %+v", s)
	}
}

func TestCreatePasswordSessionMapsWrongPasswordAndHidesAttemptCounter(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"Password is invalid (COMMAND-3M0fs)","details":[{"@type":"type.googleapis.com/zitadel.v1.CredentialsCheckError","id":"COMMAND-3M0fs","message":"Password is invalid","failedAttempts":1}]}`))
	})
	_, err := c.CreatePasswordSession(context.Background(), "a@b.test", "wrong")
	if !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("err = %v, want ErrBadCredentials", err)
	}
	if strings.Contains(err.Error(), "failedAttempts") {
		t.Errorf("error text leaks the attempt counter: %q", err.Error())
	}
}

func TestVerifyTOTPUsesPatchAndReturnsTheRotatedToken(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH (POST returns 405 on this endpoint)", r.Method)
		}
		w.Write([]byte(`{"sessionToken":"tok-ROTATED"}`))
	})
	got, err := c.VerifyTOTP(context.Background(), Session{ID: "s1", Token: "tok-STALE"}, "123456")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Token != "tok-ROTATED" {
		t.Fatalf("token = %q, want the rotated token from the response, not the input", got.Token)
	}
	if got.ID != "s1" {
		t.Fatalf("id = %q", got.ID)
	}
}

func TestLoginPolicyForOrgSetsTheOrgHeader(t *testing.T) {
	var sawOrg string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sawOrg = r.Header.Get("x-zitadel-orgid")
		w.Write([]byte(`{"policy":{"passwordCheckLifetime":"0s","forceMfa":true}}`))
	})
	p, err := c.LoginPolicyForOrg(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if sawOrg != "org-1" {
		t.Errorf("x-zitadel-orgid = %q", sawOrg)
	}
	if !p.ForceMFA {
		t.Error("ForceMFA = false, want true")
	}
}

func TestLoginPolicyForOrgRefusesAnEmptyOrgRatherThanReadingUnscoped(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte(`{"policy":{"passwordCheckLifetime":"0s"}}`))
	})
	_, err := c.LoginPolicyForOrg(context.Background(), "")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if called {
		t.Error("made an unscoped HTTP call; an empty org id must be refused before the request")
	}
}

func TestLoginPolicyRejectsAResponseWithoutTheAnchorField(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"policy":{"someOtherThing":true}}`))
	})
	_, err := c.LoginPolicyForOrg(context.Background(), "org-1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable for an unrecognizable policy object", err)
	}
}

func TestLoginPolicyTreatsAbsentForceMfaAsFalseNotUnknown(t *testing.T) {
	// protojson elides zero-value booleans, so a perfectly healthy org that
	// does not force MFA sends no forceMfa key at all. Treating that as
	// "unrecognized" handed every ordinary login to the hosted UI in hms.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"policy":{"passwordCheckLifetime":"0s"}}`))
	})
	p, err := c.LoginPolicyForOrg(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if p.ForceMFA || p.ForceMFALocalOnly {
		t.Fatalf("policy = %+v, want both false", p)
	}
}

func TestLoginPolicyReadsForceMfaLocalOnlySeparately(t *testing.T) {
	// These are two distinct fields and mark8ly must keep them distinct: it
	// has federated (Google/Apple) users, for whom forceMfaLocalOnly does NOT
	// apply. Folding them together would force MFA on federated logins.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"policy":{"passwordCheckLifetime":"0s","forceMfaLocalOnly":true}}`))
	})
	p, err := c.LoginPolicyForOrg(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if p.ForceMFA {
		t.Error("ForceMFA = true, want false — forceMfa was absent")
	}
	if !p.ForceMFALocalOnly {
		t.Error("ForceMFALocalOnly = false, want true")
	}
}

func TestLoginPolicyRefusesARenamedOrRecasedMfaKey(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"policy":{"passwordCheckLifetime":"0s","force_mfa":true}}`))
	})
	_, err := c.LoginPolicyForOrg(context.Background(), "org-1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v; a renamed key must fail closed, not read as absent-therefore-false", err)
	}
}

func TestTransportFailureMapsToUnavailable(t *testing.T) {
	c := New("http://127.0.0.1:1", "pat", &http.Client{})
	_, err := c.CreatePasswordSession(context.Background(), "a@b.test", "pw")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/`
Expected: build failure — the package does not exist yet.

- [ ] **Step 3: Implement the client**

Create `client.go`. Port hms's design; the comments explaining *why* are as important as the code.

```go
// Package zitadellogin speaks Zitadel's v2 login-client HTTP API so auth-bff
// can host its own login page instead of redirecting to Zitadel's.
//
// Every JSON shape and error mapping here is pinned to what was OBSERVED
// against a live Zitadel v4.15.3 instance, not to what the documentation says
// the API should return. Three separate shapes in this API answer 200 or 201
// while doing something other than what the caller assumes.
//
// This package makes NO decision about whether a session is adequate to
// finalize a login — that is sufficiency.go's job, and Zitadel will happily
// issue an authorization code for a password-only session even under a
// forceMfa policy.
package zitadellogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout      = 10 * time.Second
	maxSuccessBodyBytes = 64 * 1024
	maxErrorBodyBytes   = 4096
)

var (
	ErrBadCredentials     = errors.New("zitadellogin: bad credentials")
	ErrUserNotFound       = errors.New("zitadellogin: user not found")
	ErrAuthRequestInvalid = errors.New("zitadellogin: auth request invalid")
	ErrUnavailable        = errors.New("zitadellogin: zitadel unavailable")
)

type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// New builds a client. token is the login-client PAT: it authenticates every
// call in this package, never the end-user session being established. It is
// instance-level and can mint a session for any user of any product on the
// shared instance — treat it as the most powerful credential this service
// holds and never log it.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: strings.TrimSuffix(baseURL, "/"), token: token, hc: hc}
}

// Session is a Zitadel session. Token is a bearer-equivalent secret for this
// one session and must never be logged.
type Session struct {
	ID    string
	Token string
}

// LoginPolicy carries the two MFA fields as SEPARATE values. They are not
// folded together: forceMfaLocalOnly means "require MFA for local/password
// users, not federated ones", and mark8ly has federated Google and Apple
// users for whom that distinction is load-bearing.
type LoginPolicy struct {
	ForceMFA          bool
	ForceMFALocalOnly bool
}

type Factors struct {
	Password bool
	TOTP     bool
	UserID   string
	OrgID    string
}

type AuthRequest struct {
	ID string
}

// requestOptions accumulates per-request settings. The option type is
// func(*requestOptions), NOT func(*http.Request), deliberately: an option that
// could reach the raw request could set or overwrite any header — including
// the Authorization header do sets from the PAT. This shape makes that class
// of bug unrepresentable rather than something a reviewer must keep checking.
type requestOptions struct {
	orgID string
}

type requestOption func(*requestOptions)

func withOrgID(orgID string) requestOption {
	return func(ro *requestOptions) { ro.orgID = orgID }
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, notFound error, opts ...requestOption) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("zitadellogin: marshal %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("zitadellogin: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	var ro requestOptions
	for _, opt := range opts {
		opt(&ro)
	}
	if ro.orgID != "" {
		req.Header.Set("x-zitadel-orgid", ro.orgID)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("zitadellogin: %s %s: %v: %w", method, path, err, ErrUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		id := readZitadelErrorID(resp.Body)
		switch {
		case resp.StatusCode == http.StatusBadRequest:
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, path, id, ErrBadCredentials)
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, path, id, notFound)
		default:
			return fmt.Errorf("zitadellogin: %s %s: status %d: %s: %w", method, path, resp.StatusCode, id, ErrUnavailable)
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSuccessBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("zitadellogin: decode %s %s: %v: %w", method, path, err, ErrUnavailable)
	}
	return nil
}

// readZitadelErrorID extracts ONLY details[0].id (e.g. "COMMAND-3M0fs"). The
// raw error body is never surfaced, because that is exactly where Zitadel puts
// failedAttempts — a counter that must not reach a caller or a log line.
func readZitadelErrorID(r io.Reader) string {
	var wire struct {
		Details []struct {
			ID string `json:"id"`
		} `json:"details"`
	}
	if err := json.NewDecoder(io.LimitReader(r, maxErrorBodyBytes)).Decode(&wire); err != nil {
		return "unknown"
	}
	if len(wire.Details) == 0 || wire.Details[0].ID == "" {
		return "unknown"
	}
	return wire.Details[0].ID
}

func (c *Client) AuthRequest(ctx context.Context, id string) (AuthRequest, error) {
	var wire struct {
		AuthRequest struct {
			ID string `json:"id"`
		} `json:"authRequest"`
	}
	err := c.do(ctx, http.MethodGet, "/v2/oidc/auth_requests/"+url.PathEscape(id), nil, &wire, ErrAuthRequestInvalid)
	if err != nil {
		return AuthRequest{}, err
	}
	return AuthRequest{ID: wire.AuthRequest.ID}, nil
}

// CreatePasswordSession checks the login name and password in ONE call, so a
// wrong username and a wrong password take the same code path and the same
// time. ErrUserNotFound and ErrBadCredentials stay distinct here for logging;
// collapsing them into one user-facing answer is the handler's job.
func (c *Client) CreatePasswordSession(ctx context.Context, loginName, password string) (Session, error) {
	body := map[string]any{
		"checks": map[string]any{
			"user":     map[string]any{"loginName": loginName},
			"password": map[string]any{"password": password},
		},
	}
	var wire struct {
		SessionID    string `json:"sessionId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/sessions", body, &wire, ErrUserNotFound); err != nil {
		return Session{}, err
	}
	return Session{ID: wire.SessionID, Token: wire.SessionToken}, nil
}

// VerifyTOTP submits a TOTP code.
//
// Two facts, both observed and both easy to get wrong:
//
//  1. The method is PATCH. POST to a session id returns 405.
//  2. The session token ROTATES on every check. The response carries a NEW
//     sessionToken, and finalize needs the newest one. Returning the input
//     session keeps the stale token and makes finalize fail AFTER a correct
//     code, which the user reads as "my code was wrong".
func (c *Client) VerifyTOTP(ctx context.Context, s Session, code string) (Session, error) {
	body := map[string]any{
		"sessionToken": s.Token,
		"checks":       map[string]any{"totp": map[string]any{"code": code}},
	}
	var wire struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := c.do(ctx, http.MethodPatch, "/v2/sessions/"+url.PathEscape(s.ID), body, &wire, ErrBadCredentials); err != nil {
		return Session{}, err
	}
	return Session{ID: s.ID, Token: wire.SessionToken}, nil
}

// SessionFactors re-reads what Zitadel believes was verified. The create
// response omits the factors object entirely, so it cannot be trusted to say
// what was checked — this is the only honest source.
func (c *Client) SessionFactors(ctx context.Context, sessionID string) (Factors, error) {
	var wire struct {
		Session struct {
			Factors struct {
				User *struct {
					ID             string `json:"id"`
					OrganizationID string `json:"organizationId"`
				} `json:"user"`
				Password *struct {
					VerifiedAt string `json:"verifiedAt"`
				} `json:"password"`
				TOTP *struct {
					VerifiedAt string `json:"verifiedAt"`
				} `json:"totp"`
			} `json:"factors"`
		} `json:"session"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/sessions/"+url.PathEscape(sessionID), nil, &wire, ErrUnavailable); err != nil {
		return Factors{}, err
	}
	f := wire.Session.Factors
	out := Factors{Password: f.Password != nil, TOTP: f.TOTP != nil}
	if f.User != nil {
		out.UserID, out.OrgID = f.User.ID, f.User.OrganizationID
	}
	return out, nil
}

// EnrolledMethodTypes lists a user's registered authentication methods.
func (c *Client) EnrolledMethodTypes(ctx context.Context, userID string) ([]string, error) {
	var wire struct {
		AuthMethodTypes []string `json:"authMethodTypes"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(userID)+"/authentication_methods", nil, &wire, ErrUnavailable); err != nil {
		return nil, err
	}
	return wire.AuthMethodTypes, nil
}

var mfaPolicyKeys = []string{"forceMfa", "forceMfaLocalOnly"}

// policyAnchorKey proves a 200 really carried a login-policy object.
//
// It cannot be one of the MFA booleans: protojson elides zero-value fields, so
// a healthy org that does not force MFA sends no forceMfa key at all. Treating
// that absence as "unrecognized" handed every ordinary login to the hosted UI
// in hms. passwordCheckLifetime is a message-typed field observed present on
// every real response.
const policyAnchorKey = "passwordCheckLifetime"

func (c *Client) loginPolicy(ctx context.Context, opts ...requestOption) (LoginPolicy, error) {
	var wire struct {
		Policy map[string]any `json:"policy"`
	}
	if err := c.do(ctx, http.MethodGet, "/management/v1/policies/login", nil, &wire, ErrUnavailable, opts...); err != nil {
		return LoginPolicy{}, err
	}
	if _, ok := wire.Policy[policyAnchorKey]; !ok {
		return LoginPolicy{}, fmt.Errorf("zitadellogin: 200 without a recognizable policy object: %w", ErrUnavailable)
	}
	for _, key := range mfaPolicyKeys {
		if err := refuseIfKeyRenamedOrRecased(wire.Policy, key); err != nil {
			return LoginPolicy{}, err
		}
	}
	forceMFA, err := readMFABool(wire.Policy, "forceMfa")
	if err != nil {
		return LoginPolicy{}, err
	}
	localOnly, err := readMFABool(wire.Policy, "forceMfaLocalOnly")
	if err != nil {
		return LoginPolicy{}, err
	}
	return LoginPolicy{ForceMFA: forceMFA, ForceMFALocalOnly: localOnly}, nil
}

func normalizePolicyKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(key), "_", "")
}

// refuseIfKeyRenamedOrRecased fails closed when the API renames or re-cases an
// MFA field. Without it, force_mfa or ForceMFA would decode as merely absent,
// therefore false — a silent fail-open on the one field that matters most.
func refuseIfKeyRenamedOrRecased(policy map[string]any, wantKey string) error {
	want := normalizePolicyKey(wantKey)
	for k := range policy {
		if k != wantKey && normalizePolicyKey(k) == want {
			return fmt.Errorf("zitadellogin: policy key %q looks like a renamed %q: %w", k, wantKey, ErrUnavailable)
		}
	}
	return nil
}

func readMFABool(policy map[string]any, key string) (bool, error) {
	v, ok := policy[key]
	if !ok {
		return false, nil // elided zero value: genuinely false
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("zitadellogin: policy.%s is %T, not a bool: %w", key, v, ErrUnavailable)
	}
	return b, nil
}

// LoginPolicyForOrg reads the policy of the org the user actually belongs to.
// It refuses an empty org id rather than falling back to an unscoped read: an
// unscoped read judges a user in one org by a different org's policy, which is
// a real, shipped MFA bypass (hms #913).
func (c *Client) LoginPolicyForOrg(ctx context.Context, orgID string) (LoginPolicy, error) {
	if orgID == "" {
		return LoginPolicy{}, fmt.Errorf("zitadellogin: LoginPolicyForOrg with an empty org id, refusing rather than reading unscoped: %w", ErrUnavailable)
	}
	return c.loginPolicy(ctx, withOrgID(orgID))
}

// InstanceLoginPolicyForDisplay reads the unscoped instance policy. It is for
// display before a user is known and MUST NOT be used for enforcement; an
// archtest forbids sufficiency.go from referencing it.
func (c *Client) InstanceLoginPolicyForDisplay(ctx context.Context) (LoginPolicy, error) {
	return c.loginPolicy(ctx)
}
```

- [ ] **Step 4: Run the tests**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/ -v`
Expected: all PASS.

- [ ] **Step 5: Vet and commit**

```bash
cd services/auth-bff && go vet ./... && gofmt -l internal/zitadellogin/
git add services/auth-bff/internal/zitadellogin/
git commit -m "feat(auth-bff): add a Zitadel login client pinned to observed API behaviour"
```

---

### Task 3: Sufficiency — MFA enforced in our code, fail-closed

Zitadel issues an authorization code for a password-only session **even under `forceMfa: true`**. Verified independently by two repos against the live instance. This task is the only thing standing between that and an MFA bypass.

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/sufficiency.go`
- Test: `services/auth-bff/internal/zitadellogin/sufficiency_test.go`
- Test: `services/auth-bff/internal/zitadellogin/arch_test.go`

**Interfaces:**
- Consumes: Task 2's `Client`, `Session`, `LoginPolicy`, `Factors`, sentinels.
- Produces: `type Outcome int` with `OutcomeHandoff` (zero value), `OutcomeComplete`, `OutcomeFactorRequired`; `type Result struct { Outcome Outcome; CallbackURL string; Factors []string }`; `func (c *Client) CompleteIfSufficient(ctx, authRequestID string, s Session, federated bool) (Result, error)`; `func (c *Client) CompleteAfterFactor(ctx, authRequestID string, s Session) (Result, error)`. Task 5's handler consumes both.

- [ ] **Step 1: Write the failing tests**

Create `sufficiency_test.go`:

```go
package zitadellogin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func fakeZitadel(t *testing.T, policyJSON, methodsJSON, factorsJSON string, finalized *atomic.Bool) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/management/v1/policies/login":
			w.Write([]byte(policyJSON))
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			w.Write([]byte(`{"authMethodTypes":` + methodsJSON + `}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/oidc/auth_requests/"):
			finalized.Store(true)
			w.Write([]byte(`{"callbackUrl":"https://admin.mark8ly.com/auth/callback?code=c&state=s"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/sessions/"):
			w.Write([]byte(factorsJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pat", srv.Client())
}

const factorsPasswordOnly = `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"}}}}`
const factorsWithTOTP = `{"session":{"factors":{"user":{"id":"u1","organizationId":"o1"},"password":{"verifiedAt":"2026-09-03T01:00:00Z"},"totp":{"verifiedAt":"2026-09-03T01:01:00Z"}}}}`
const policyNoMFA = `{"policy":{"passwordCheckLifetime":"0s"}}`
const policyForceMFA = `{"policy":{"passwordCheckLifetime":"0s","forceMfa":true}}`
const policyLocalOnly = `{"policy":{"passwordCheckLifetime":"0s","forceMfaLocalOnly":true}}`

func TestPasswordOnlyCompletesWhenNoMFAIsRequired(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyNoMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete || res.CallbackURL == "" {
		t.Fatalf("result = %+v", res)
	}
	if !fin.Load() {
		t.Error("finalize was not called")
	}
}

func TestPasswordOnlyDoesNotCompleteWhenForceMfaIsSet(t *testing.T) {
	// The whole reason this package exists: Zitadel WOULD issue a code here.
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeFactorRequired {
		t.Fatalf("outcome = %v, want OutcomeFactorRequired", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize WAS called for a password-only session under forceMfa — MFA bypass")
	}
}

func TestForceMfaLocalOnlyDoesNotApplyToFederatedUsers(t *testing.T) {
	// forceMfaLocalOnly means "local/password users only". mark8ly has Google
	// and Apple users; forcing MFA on them would be wrong.
	var fin atomic.Bool
	c := fakeZitadel(t, policyLocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, true /* federated */)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete {
		t.Fatalf("outcome = %v, want OutcomeComplete for a federated user", res.Outcome)
	}
	_ = fin
}

func TestForceMfaLocalOnlyDoesApplyToPasswordUsers(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyLocalOnly, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeFactorRequired {
		t.Fatalf("outcome = %v, want OutcomeFactorRequired", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called")
	}
}

func TestAnUncollectibleFactorHandsOff(t *testing.T) {
	// A passkey is a real second factor this login page cannot collect.
	// Completing anyway would silently skip it.
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_PASSKEY"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called with an uncollected factor outstanding")
	}
}

func TestAnUnreadablePolicyHandsOffRatherThanCompleting(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, `{"policy":{"nonsense":true}}`, `["AUTHENTICATION_METHOD_TYPE_PASSWORD"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteIfSufficient(context.Background(), "V2_1", Session{ID: "s1", Token: "t"}, false)
	if err != nil {
		t.Fatalf("err = %v (a handoff is an outcome, not an error)", err)
	}
	if res.Outcome != OutcomeHandoff {
		t.Fatalf("outcome = %v, want OutcomeHandoff", res.Outcome)
	}
	if fin.Load() {
		t.Fatal("finalize was called despite an unreadable policy")
	}
}

func TestZeroValueOutcomeIsHandoff(t *testing.T) {
	var r Result
	if r.Outcome != OutcomeHandoff {
		t.Fatal("the zero value must be OutcomeHandoff so an undecided Result cannot complete a login")
	}
}

func TestCompleteAfterFactorRefusesWhenZitadelDoesNotReportTOTP(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsPasswordOnly, &fin)
	res, err := c.CompleteAfterFactor(context.Background(), "V2_1", Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome == OutcomeComplete {
		t.Fatal("completed without Zitadel confirming the TOTP factor")
	}
	if fin.Load() {
		t.Fatal("finalize was called")
	}
}

func TestCompleteAfterFactorCompletesWhenTOTPIsConfirmed(t *testing.T) {
	var fin atomic.Bool
	c := fakeZitadel(t, policyForceMFA, `["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_TOTP"]`, factorsWithTOTP, &fin)
	res, err := c.CompleteAfterFactor(context.Background(), "V2_1", Session{ID: "s1", Token: "t"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeComplete || !fin.Load() {
		t.Fatalf("result = %+v finalized=%v", res, fin.Load())
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/ -run 'Complete|Force|Uncollectible|ZeroValue|Password' -v`
Expected: build failure — `CompleteIfSufficient` does not exist.

- [ ] **Step 3: Implement sufficiency**

Create `sufficiency.go`:

```go
package zitadellogin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

// Outcome is what to do with a login attempt.
type Outcome int

const (
	// OutcomeHandoff is the zero value ON PURPOSE. Any Result built without a
	// decision lands on "do not complete this login". The opposite default
	// would cost an MFA bypass; that asymmetry decides which value is zero.
	OutcomeHandoff Outcome = iota
	OutcomeComplete
	OutcomeFactorRequired
)

type Result struct {
	Outcome     Outcome
	CallbackURL string
	Factors     []string
}

// sufficient is proof that a session was evaluated by one of this package's
// two classification paths and found adequate to finalize.
//
// It makes "call finalize with no decision at all" a compile error. It does
// NOT stop someone constructing sufficient{} beside a new check-free function
// — Go permits that inside the package — which is why an archtest pins both
// the finalize call site and sufficient{} construction to this file.
type sufficient struct{}

const (
	methodPassword = "AUTHENTICATION_METHOD_TYPE_PASSWORD" // not a second factor
	methodTOTP     = "AUTHENTICATION_METHOD_TYPE_TOTP"     // the one we can collect
)

// finalize exchanges a session for an authorization code. Unexported and
// requiring a sufficient witness, so it is unreachable without a decision.
func (c *Client) finalize(ctx context.Context, authRequestID string, s Session, _ sufficient) (string, error) {
	body := map[string]any{
		"session": map[string]any{"sessionId": s.ID, "sessionToken": s.Token},
	}
	var wire struct {
		CallbackURL string `json:"callbackUrl"`
	}
	err := c.do(ctx, http.MethodPost, "/v2/oidc/auth_requests/"+url.PathEscape(authRequestID), body, &wire, ErrAuthRequestInvalid)
	if err != nil {
		return "", err
	}
	if wire.CallbackURL == "" {
		return "", fmt.Errorf("zitadellogin: finalize returned no callbackUrl: %w", ErrUnavailable)
	}
	return wire.CallbackURL, nil
}

// classifyEnrolledMethods reports whether the user has TOTP enrolled and
// whether they have any factor this login page cannot collect.
//
// Everything that is not PASSWORD or TOTP is uncollectible — an include list,
// so a factor type we have never seen fails closed into a handoff rather than
// being silently skipped.
func (c *Client) classifyEnrolledMethods(ctx context.Context, userID string) (totpEnrolled, uncollectible bool, err error) {
	types, err := c.EnrolledMethodTypes(ctx, userID)
	if err != nil {
		return false, false, err
	}
	for _, t := range types {
		switch t {
		case methodPassword:
		case methodTOTP:
			totpEnrolled = true
		default:
			uncollectible = true
		}
	}
	return totpEnrolled, uncollectible, nil
}

// mfaRequired applies the two policy fields to this user.
//
// forceMfa applies to everyone. forceMfaLocalOnly applies only to users
// authenticating with a local credential — mark8ly has federated Google and
// Apple users, and forcing MFA on them would be wrong. These are kept separate
// rather than OR-ed together for exactly that reason.
func mfaRequired(p LoginPolicy, federated bool) bool {
	if p.ForceMFA {
		return true
	}
	return p.ForceMFALocalOnly && !federated
}

// CompleteIfSufficient decides whether a freshly created session may finalize.
//
// Zitadel does NOT enforce forceMfa for a login client: it issues an
// authorization code for a password-only session and signals nothing. Every
// uncertain input therefore fails closed to OutcomeHandoff, which is a
// legitimate outcome rather than an error.
func (c *Client) CompleteIfSufficient(ctx context.Context, authRequestID string, s Session, federated bool) (Result, error) {
	factors, err := c.SessionFactors(ctx, s.ID)
	if err != nil || factors.UserID == "" || factors.OrgID == "" {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read session subject, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	totpEnrolled, uncollectible, err := c.classifyEnrolledMethods(ctx, factors.UserID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read enrolled methods, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if uncollectible {
		return Result{Outcome: OutcomeHandoff}, nil
	}
	policy, err := c.LoginPolicyForOrg(ctx, factors.OrgID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot read login policy, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if mfaRequired(policy, federated) && !factors.TOTP {
		if !totpEnrolled {
			return Result{Outcome: OutcomeHandoff}, nil
		}
		return Result{Outcome: OutcomeFactorRequired, Factors: []string{methodTOTP}}, nil
	}
	cb, err := c.finalize(ctx, authRequestID, s, sufficient{})
	if err != nil {
		return Result{Outcome: OutcomeHandoff}, err
	}
	return Result{Outcome: OutcomeComplete, CallbackURL: cb}, nil
}

// CompleteAfterFactor finalizes after a TOTP check.
//
// It re-reads the factors from Zitadel rather than trusting that VerifyTOTP
// returned without error, because the caller may be holding a stale session
// value from before the token rotated.
func (c *Client) CompleteAfterFactor(ctx context.Context, authRequestID string, s Session) (Result, error) {
	factors, err := c.SessionFactors(ctx, s.ID)
	if err != nil {
		slog.WarnContext(ctx, "zitadel sufficiency: cannot re-read factors after TOTP, handing off", "err", err)
		return Result{Outcome: OutcomeHandoff}, nil
	}
	if !factors.TOTP {
		return Result{Outcome: OutcomeHandoff}, nil
	}
	cb, err := c.finalize(ctx, authRequestID, s, sufficient{})
	if err != nil {
		return Result{Outcome: OutcomeHandoff}, err
	}
	return Result{Outcome: OutcomeComplete, CallbackURL: cb}, nil
}
```

- [ ] **Step 4: Write the archtests**

Create `arch_test.go`. These are the mechanical backstop for the residual gap the witness type cannot close.

```go
package zitadellogin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = string(b)
	}
	return out
}

func TestFinalizeIsOnlyCalledFromSufficiency(t *testing.T) {
	for name, src := range sourceFiles(t) {
		if name == "sufficiency.go" {
			continue
		}
		if strings.Contains(src, "c.finalize(") || strings.Contains(src, ".finalize(ctx") {
			t.Errorf("%s calls finalize; the OIDC finalize call must stay behind a sufficiency decision, "+
				"because Zitadel does not enforce forceMfa for a login client", name)
		}
	}
}

func TestSufficientWitnessIsOnlyConstructedInSufficiency(t *testing.T) {
	for name, src := range sourceFiles(t) {
		if name == "sufficiency.go" {
			continue
		}
		if strings.Contains(src, "sufficient{") {
			t.Errorf("%s constructs the sufficient witness; only sufficiency.go may", name)
		}
	}
}

func TestSufficiencyNeverUsesTheUnscopedDisplayPolicy(t *testing.T) {
	src := sourceFiles(t)["sufficiency.go"]
	if strings.Contains(src, "InstanceLoginPolicyForDisplay") {
		t.Error("sufficiency.go references InstanceLoginPolicyForDisplay; enforcement must read the " +
			"user's own org policy via LoginPolicyForOrg, or a user in one org is judged by another org's policy")
	}
}
```

- [ ] **Step 5: Run everything**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/ -v`
Expected: all PASS, including the three archtests.

- [ ] **Step 6: Commit**

```bash
git add services/auth-bff/internal/zitadellogin/
git commit -m "feat(auth-bff): enforce MFA in fail-closed sufficiency, since Zitadel does not"
```

---

### Task 4: Config and wiring, disabled by default

**Files:**
- Modify: `services/auth-bff/pkg/config/config.go`
- Modify: `services/auth-bff/cmd/server/main.go`
- Test: `services/auth-bff/pkg/config/config_test.go` (create if absent)

**Interfaces:**
- Consumes: Task 2's `New`.
- Produces: `cfg.ZitadelEnabled`, `cfg.ZitadelIssuer`, `cfg.ZitadelLoginClientToken`, `cfg.ZitadelAdminProjectID`, `cfg.ZitadelStorefrontProjectID`; a `*zitadellogin.Client` in `main.go`, nil when disabled. Task 5 consumes the client.

- [ ] **Step 1: Write the failing config test**

```go
func TestZitadelIsDisabledAndUnrequiredByDefault(t *testing.T) {
	for _, k := range []string{"ZITADEL_ENABLED", "ZITADEL_ISSUER", "ZITADEL_LOGIN_CLIENT_TOKEN"} {
		t.Setenv(k, "")
	}
	// Only the pre-existing required vars are set.
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("GIP_PROJECT_ID", "p")
	t.Setenv("GIP_PROJECT_NUMBER", "1")
	t.Setenv("GIP_WEB_API_KEY", "k")
	t.Setenv("GIP_INTERNAL_TENANT_ID", "t")
	t.Setenv("OAUTH_CLIENT_ID", "c")
	t.Setenv("OAUTH_CLIENT_SECRET", "s")
	t.Setenv("SESSION_ENCRYPT_KEY", "thirtytwo-bytes-for-testing-only")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load must succeed with no Zitadel config at all: %v", err)
	}
	if cfg.ZitadelEnabled {
		t.Error("ZitadelEnabled must default to false")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/auth-bff && go test ./pkg/config/ -run TestZitadelIsDisabled -v`
Expected: FAIL — field does not exist.

- [ ] **Step 3: Add the config fields**

In `config.go`, alongside the existing GIP fields. **No `required:"true"` on any of them** — a required var that rejects empty crashloops the pod on merge.

```go
	// Zitadel (#524 phase 2). All optional and unread unless ZitadelEnabled is
	// set: GIP remains the live provider until the phase 6 cutover.
	ZitadelEnabled             bool   `envconfig:"ZITADEL_ENABLED" default:"false"`
	ZitadelIssuer              string `envconfig:"ZITADEL_ISSUER"`
	ZitadelLoginClientToken    string `envconfig:"ZITADEL_LOGIN_CLIENT_TOKEN"`
	ZitadelAdminProjectID      string `envconfig:"ZITADEL_ADMIN_PROJECT_ID"`
	ZitadelStorefrontProjectID string `envconfig:"ZITADEL_STOREFRONT_PROJECT_ID"`
```

- [ ] **Step 4: Wire it in `main.go`, following the existing optional-collaborator idiom**

After the `autologinSvc` construction block (~line 243):

```go
	// Zitadel login client (#524 phase 2). Constructed only when explicitly
	// enabled AND fully configured; nil otherwise, so no route it backs is
	// mounted. Refusing to boot on partial config is deliberate: a
	// half-configured login path that fails at request time is worse than a
	// loud failure here.
	var zitadelClient *zitadellogin.Client
	switch {
	case !cfg.ZitadelEnabled:
		log.Info("zitadel login disabled; GIP remains the auth provider")
	case cfg.ZitadelIssuer == "" || cfg.ZitadelLoginClientToken == "":
		log.Error("zitadel: ZITADEL_ENABLED is set but ZITADEL_ISSUER or ZITADEL_LOGIN_CLIENT_TOKEN is empty")
		panic("zitadel: enabled but not configured")
	default:
		zitadelClient = zitadellogin.New(cfg.ZitadelIssuer, cfg.ZitadelLoginClientToken, nil)
		log.Info("zitadel login client enabled", "issuer", cfg.ZitadelIssuer)
	}
```

`main()` returns nothing and uses `log.Error(...)` followed by `panic(err)` for every startup failure — there are 11 such sites and zero uses of `log.Fatal`/`os.Exit`. The snippet above matches that idiom; do not introduce a new one, and do not change `main`'s signature.

Refusing to boot on partial config is deliberate and is the one place this phase is allowed to be fatal: the flag is opt-in, so a half-configured Zitadel path can only exist if someone set `ZITADEL_ENABLED` and stopped halfway. Failing loudly at boot beats failing per-request at login. This does not contradict the Global Constraint about crashloops — that constraint is about config being required when the feature is *off*, which it is not here.

- [ ] **Step 5: Run the tests and build**

Run: `cd services/auth-bff && go test ./pkg/config/ -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 6: Commit**

```bash
git add services/auth-bff/pkg/config/ services/auth-bff/cmd/server/main.go
git commit -m "feat(auth-bff): wire the Zitadel login client behind a disabled flag"
```

---

### Task 5: The login handler

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/handler.go`
- Test: `services/auth-bff/internal/zitadellogin/handler_test.go`
- Modify: `services/auth-bff/cmd/server/main.go` (mount, conditional)

**Interfaces:**
- Consumes: Task 1's `completeLogin` (via an injected callback so this package does not import `autologin`), Tasks 2-3's `Client`.
- Produces: `POST /auth/zitadel/login`, `POST /auth/zitadel/totp`.

- [ ] **Step 1: Write the failing handler tests**

```go
func TestLoginCollapsesUnknownUserAndWrongPasswordIntoOneAnswer(t *testing.T) {
	// A different status or message for "no such user" is an account-
	// enumeration oracle. Both must look identical to the browser.
	for _, body := range []string{
		`{"code":3,"message":"Password is invalid (COMMAND-3M0fs)","details":[{"id":"COMMAND-3M0fs","failedAttempts":1}]}`,
		`{"code":5,"message":"User could not be found (QUERY-Dfbg2)","details":[{"id":"QUERY-Dfbg2"}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(body, "QUERY") {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			w.Write([]byte(body))
		}))
		defer srv.Close()
		h := NewHandler(New(srv.URL, "pat", srv.Client()), nil)
		rec := httptest.NewRecorder()
		h.login(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/login",
			strings.NewReader(`{"auth_request_id":"V2_1","login_name":"a@b.test","password":"x"}`)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "failedAttempts") || strings.Contains(rec.Body.String(), "not be found") {
			t.Fatalf("response leaks which half failed: %s", rec.Body.String())
		}
	}
}
```

Add a second test asserting a `OutcomeFactorRequired` result returns a body carrying `totp_required` **and does not mint a session**.

- [ ] **Step 2: Run to verify they fail**

Run: `cd services/auth-bff && go test ./internal/zitadellogin/ -run TestLogin -v`
Expected: FAIL — `NewHandler` does not exist.

- [ ] **Step 3: Implement the handler**

`NewHandler(c *Client, complete CompleteFunc) *Handler` where:

```go
// CompleteFunc runs the shared post-identity gauntlet — membership, MFA gate,
// device and OTP checks, session mint. It is injected rather than imported so
// this package stays a Zitadel client and knows nothing about autologin.
type CompleteFunc func(ctx context.Context, w http.ResponseWriter, uid, email, tenantID string) error
```

`login` reads `{auth_request_id, login_name, password, workspace_tenant}`, calls `CreatePasswordSession`, then `CompleteIfSufficient`, then switches on `Result.Outcome`:
- `OutcomeComplete` → call `complete(...)`, return `{"callback_url": …}`
- `OutcomeFactorRequired` → return `{"totp_required": true, "session_id": …}` with **no session minted**
- `OutcomeHandoff` → return `{"handoff_url": …}` pointing at the Aurora-branded hosted login

Every credential error — `ErrBadCredentials`, `ErrUserNotFound` — maps to a single `401 {"error":"invalid_credentials"}`. Log which one actually happened; never return it.

- [ ] **Step 4: Mount it conditionally in `main.go`**

```go
	if zitadelClient != nil {
		zitadelHandler := zitadellogin.NewHandler(zitadelClient, autologinSvc.CompleteForProvider)
		zitadelHandler.Register(v1)
	}
```

This requires a small exported wrapper on `autologin.Service` around `completeLogin` — add it in this task, not Task 1.

- [ ] **Step 5: Run the full service suite**

Run: `cd services/auth-bff && go test ./... && go vet ./... && go build ./...`
Expected: everything passes, including all pre-existing tests.

- [ ] **Step 6: Commit**

```bash
git add services/auth-bff/
git commit -m "feat(auth-bff): add the Zitadel login and TOTP endpoints behind the flag"
```

---

### Task 6: Record the known gaps

**Files:**
- Create: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1: Write it**

Document, with the reasoning, not just the fact:

1. **Zitadel does not enforce MFA for login clients.** Verified against the live instance by two repos. `sufficiency.go` is the enforcement; the archtests are what keep it that way.
2. **`passwordChangeRequired` is not signalled** to a login client anywhere in this flow — create, read and finalize are byte-identical to a normal user's. Logins that should force a password change silently complete. Open upstream gap; file an issue.
3. **Handoff targets the Aurora-branded hosted login** (`zitadel-login:v4.15.3-aurora.4`) for factors this page cannot collect: passkeys, U2F, SMS OTP, recovery codes.
4. **The login-client PAT is instance-level.** It can mint a session for any user of any product on the shared instance. Zitadel offers no narrower role.
5. **`forceMfaLocalOnly` is NOT folded into `forceMfa`**, unlike hms, because mark8ly has federated Google and Apple users to whom it must not apply.

- [ ] **Step 2: Commit**

```bash
git add services/auth-bff/internal/zitadellogin/README.md
git commit -m "docs(auth-bff): record the Zitadel login client's known gaps"
```

---

## Phase 2 completion criteria

- `AutoLogin`'s behaviour and tests are unchanged; both providers share `completeLogin`.
- `go test ./...` passes in `services/auth-bff`, including three archtests pinning the finalize call site, the witness construction, and the ban on the unscoped policy in enforcement.
- With `ZITADEL_ENABLED` unset the service boots exactly as today, mounts no Zitadel route, and logs that GIP remains the provider.
- A password-only session under `forceMfa` returns `OutcomeFactorRequired` and never finalizes.
- A federated user under `forceMfaLocalOnly` completes.

## Not in this phase

Frontend changes (phase 3), the `marketplace-api` verifier and the `tenant_id` claim (phase 4), `gipadmin` (phase 5), and the cutover itself including deleting `gipkey`, `usermfa` and both GIP verifiers (phase 6).
