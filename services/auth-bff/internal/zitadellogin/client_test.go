package zitadellogin

import (
	"context"
	"errors"
	"io"
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

func TestUserEmailReadsTheHumanEmail(t *testing.T) {
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"user":{"human":{"email":{"email":"real-owner@mark8ly.com","isVerified":true}}}}`))
	})
	email, err := c.UserEmail(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if email != "real-owner@mark8ly.com" {
		t.Fatalf("email = %q", email)
	}
	if gotPath != "/v2/users/u1" {
		t.Fatalf("path = %q, want /v2/users/u1", gotPath)
	}
}

func TestUserEmailEscapesTheUserID(t *testing.T) {
	var gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"user":{"human":{"email":{"email":"a@b.test"}}}}`))
	})
	if _, err := c.UserEmail(context.Background(), "u/1"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/v2/users/u%2F1" {
		t.Fatalf("path = %q, want the id path-escaped", gotPath)
	}
}

func TestUserEmailMapsNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"User could not be found (QUERY-Dfbg2)","details":[{"id":"QUERY-Dfbg2"}]}`))
	})
	_, err := c.UserEmail(context.Background(), "missing")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUserEmailRefusesAMachineUserWithNoHumanProfile(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"user":{"machine":{"name":"svc-account"}}}`))
	})
	_, err := c.UserEmail(context.Background(), "svc-1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable for a user with no human.email", err)
	}
}

func TestFindUserByVerifiedEmailSendsTheOrgIDHeader(t *testing.T) {
	var gotOrgHeader, gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader = r.Header.Get("x-zitadel-orgid")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Write([]byte(`{"result":[]}`))
	})
	if _, err := c.FindUserByVerifiedEmail(context.Background(), "org-1", "person@gmail.com"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotOrgHeader != "org-1" {
		t.Fatalf("x-zitadel-orgid = %q, want org-1 — an unscoped search could match a verified email in an unrelated org", gotOrgHeader)
	}
	if !strings.Contains(gotBody, "TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE") {
		t.Fatalf("request body %q missing the case-insensitive query method", gotBody)
	}
}

func TestFindUserByVerifiedEmailRefusesAnEmptyOrgID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not search instance-wide with an empty org id")
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.FindUserByVerifiedEmail(context.Background(), "", "person@gmail.com")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestFindUserByVerifiedEmailMatchesCaseInsensitively is the fix for the
// functional bug where a stored "Person@x.com" never matched Google's
// "person@x.com": Zitadel's own account uniqueness is case-insensitive, so
// this search must be too.
func TestFindUserByVerifiedEmailMatchesCaseInsensitively(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"Person@Gmail.com","isVerified":true}}}]}`))
	})
	got, err := c.FindUserByVerifiedEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "existing-1" {
		t.Fatalf("userID = %q, want existing-1 to match despite the case difference", got)
	}
}

// TestFindUserByVerifiedEmailRefusesAmbiguousMatches is the fix for the
// review finding that this function took the first match: two orgs on a
// shared instance (or, within a single scoped search, any duplicate) can
// each hold a verified copy of the same email — picking one is an
// unspecified-ordering decision this code must not make.
func TestFindUserByVerifiedEmailRefusesAmbiguousMatches(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[
			{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}},
			{"userId":"existing-2","human":{"email":{"email":"person@gmail.com","isVerified":true}}}
		]}`))
	})
	_, err := c.FindUserByVerifiedEmail(context.Background(), "org-1", "person@gmail.com")
	if !errors.Is(err, ErrAmbiguousEmailMatch) {
		t.Fatalf("err = %v, want ErrAmbiguousEmailMatch", err)
	}
}

func TestFindUserByVerifiedEmailIgnoresAnUnverifiedMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":false}}}]}`))
	})
	got, err := c.FindUserByVerifiedEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "" {
		t.Fatalf("userID = %q, want empty for an unverified match", got)
	}
}

// TestFindUserByEmailSendsTheOrgIDHeader mirrors
// TestFindUserByVerifiedEmailSendsTheOrgIDHeader: FindUserByEmail must be
// just as org-scoped as its verified-only sibling, for the identical
// cross-org leak reason.
func TestFindUserByEmailSendsTheOrgIDHeader(t *testing.T) {
	var gotOrgHeader, gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotOrgHeader = r.Header.Get("x-zitadel-orgid")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Write([]byte(`{"result":[]}`))
	})
	if _, _, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotOrgHeader != "org-1" {
		t.Fatalf("x-zitadel-orgid = %q, want org-1", gotOrgHeader)
	}
	if !strings.Contains(gotBody, "TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE") {
		t.Fatalf("request body %q missing the case-insensitive query method", gotBody)
	}
}

func TestFindUserByEmailRefusesAnEmptyOrgID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not search instance-wide with an empty org id")
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, _, err := c.FindUserByEmail(context.Background(), "", "person@gmail.com")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestFindUserByEmailReportsAnUnverifiedMatch is the property
// FindUserByVerifiedEmail cannot provide: register needs to know an
// unverified account exists, not just that no verified one does.
func TestFindUserByEmailReportsAnUnverifiedMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":false}}}]}`))
	})
	got, verified, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "existing-1" || verified {
		t.Fatalf("userID = %q, verified = %v, want existing-1/false", got, verified)
	}
}

func TestFindUserByEmailReportsAVerifiedMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":true}}}]}`))
	})
	got, verified, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "existing-1" || !verified {
		t.Fatalf("userID = %q, verified = %v, want existing-1/true", got, verified)
	}
}

func TestFindUserByEmailMatchesCaseInsensitively(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[{"userId":"existing-1","human":{"email":{"email":"Person@Gmail.com","isVerified":false}}}]}`))
	})
	got, _, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "existing-1" {
		t.Fatalf("userID = %q, want existing-1 to match despite the case difference", got)
	}
}

// TestFindUserByEmailRefusesAmbiguousMatchesRegardlessOfVerification pins
// the same refusal FindUserByVerifiedEmail follows, but here it must fire
// even when the two matches disagree on verification state — there is no
// safe way to guess which of two accounts holding the same address is the
// "right" one to treat as existing.
func TestFindUserByEmailRefusesAmbiguousMatchesRegardlessOfVerification(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":[
			{"userId":"existing-1","human":{"email":{"email":"person@gmail.com","isVerified":false}}},
			{"userId":"existing-2","human":{"email":{"email":"person@gmail.com","isVerified":true}}}
		]}`))
	})
	_, _, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com")
	if !errors.Is(err, ErrAmbiguousEmailMatch) {
		t.Fatalf("err = %v, want ErrAmbiguousEmailMatch", err)
	}
}

func TestFindUserByEmailNoMatchReturnsEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	})
	got, verified, err := c.FindUserByEmail(context.Background(), "org-1", "person@gmail.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "" || verified {
		t.Fatalf("userID = %q, verified = %v, want empty/false for a response that omits result entirely", got, verified)
	}
}

func TestCreateHumanUserWithIDPLinkSendsProfileEmailAndIDPLinks(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/users/human" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Write([]byte(`{"userId":"new-user-1"}`))
	})
	identity := IDPIdentity{
		Email:            "person@gmail.com",
		EmailVerified:    true,
		IDPID:            "idp-1",
		ExternalUserID:   "google-sub-1",
		ExternalUserName: "person@gmail.com",
	}
	got, err := c.CreateHumanUserWithIDPLink(context.Background(), identity)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "new-user-1" {
		t.Fatalf("userID = %q", got)
	}
	for _, want := range []string{
		`"email":"person@gmail.com"`, `"isVerified":true`,
		`"idpId":"idp-1"`, `"userId":"google-sub-1"`, `"userName":"person@gmail.com"`,
	} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body %q missing %q", gotBody, want)
		}
	}
}

func TestCreateHumanUserWithIDPLinkRefusesAnUnverifiedEmailWithoutCallingZitadel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not create a user from an unverified email")
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.CreateHumanUserWithIDPLink(context.Background(), IDPIdentity{Email: "person@gmail.com", EmailVerified: false})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestCreateHumanUserWithIDPLinkMapsADuplicateEmailDistinctly is the fix for
// the functional bug where a 400 here (most often a duplicate email in a
// different case) mapped to the generic ErrBadCredentials, which reads as
// "wrong password" for something that is neither.
func TestCreateHumanUserWithIDPLinkMapsADuplicateEmailDistinctly(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":6,"message":"user already exists (COMMAND-oR9nS)","details":[{"id":"COMMAND-oR9nS"}]}`))
	})
	_, err := c.CreateHumanUserWithIDPLink(context.Background(), IDPIdentity{
		Email: "person@gmail.com", EmailVerified: true, IDPID: "idp-1", ExternalUserID: "google-sub-1",
	})
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, want ErrEmailAlreadyExists", err)
	}
	if errors.Is(err, ErrBadCredentials) {
		t.Fatal("must not also read as ErrBadCredentials — that reads as a wrong password to a caller checking the wrong sentinel")
	}
}

// TestCreateHumanUserWithIDPLinkDoesNotMapAnUnrelated400ToEmailAlreadyExists
// is the fix for review Finding 3: the ErrEmailAlreadyExists mapping is
// narrowed to grpc code 6 (ALREADY_EXISTS). A 400 from AddHumanUser for any
// OTHER reason — a malformed profile field, a future validation rule, any
// policy rejection — must never surface as "email already exists": that
// reads as a retryable race to a caller when it might be a permanent,
// unrelated failure.
func TestCreateHumanUserWithIDPLinkDoesNotMapAnUnrelated400ToEmailAlreadyExists(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"invalid profile (COMMAND-9zK2p)","details":[{"id":"COMMAND-9zK2p"}]}`))
	})
	_, err := c.CreateHumanUserWithIDPLink(context.Background(), IDPIdentity{
		Email: "person@gmail.com", EmailVerified: true, IDPID: "idp-1", ExternalUserID: "google-sub-1",
	})
	if errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, must NOT read as ErrEmailAlreadyExists for an unrelated 400 (code 3, not 6)", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable as the generic fallback", err)
	}
}

// TestCreateHumanUserWithIDPLinkUsesGoogleGivenAndFamilyNameWhenPresent is
// the fix for review Finding 6: a merchant-flavoured "Member" placeholder
// must not land on a shopper account when Google already sent a real name.
func TestCreateHumanUserWithIDPLinkUsesGoogleGivenAndFamilyNameWhenPresent(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Write([]byte(`{"userId":"new-user-1"}`))
	})
	_, err := c.CreateHumanUserWithIDPLink(context.Background(), IDPIdentity{
		Email: "person@gmail.com", EmailVerified: true, IDPID: "idp-1", ExternalUserID: "google-sub-1",
		GivenName: "Priya", FamilyName: "Shah",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(gotBody, `"givenName":"Priya"`) || !strings.Contains(gotBody, `"familyName":"Shah"`) {
		t.Fatalf("request body = %q, want Google's given/family name used", gotBody)
	}
	if strings.Contains(gotBody, "Member") {
		t.Fatalf("request body = %q, must not fall back to the placeholder when Google sent a real name", gotBody)
	}
}

// TestCreateHumanUserWithIDPLinkFallsBackToNeutralNamesWhenAbsent pins the
// fallback shape when Google sends no given_name/family_name at all: the
// email's local part for the given name, and a neutral (not
// merchant-flavoured) placeholder for the family name.
func TestCreateHumanUserWithIDPLinkFallsBackToNeutralNamesWhenAbsent(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Write([]byte(`{"userId":"new-user-1"}`))
	})
	_, err := c.CreateHumanUserWithIDPLink(context.Background(), IDPIdentity{
		Email: "person@gmail.com", EmailVerified: true, IDPID: "idp-1", ExternalUserID: "google-sub-1",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(gotBody, `"givenName":"person"`) {
		t.Fatalf("request body = %q, want the email local part as the given name fallback", gotBody)
	}
	if strings.Contains(gotBody, `"familyName":"Member"`) {
		t.Fatalf("request body = %q, must not use the merchant-flavoured \"Member\" placeholder", gotBody)
	}
}

func TestLinkIDPToUserSendsTheIDPLink(t *testing.T) {
	var gotPath, gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	})
	identity := IDPIdentity{
		Email:            "person@gmail.com",
		EmailVerified:    true,
		IDPID:            "idp-1",
		ExternalUserID:   "google-sub-1",
		ExternalUserName: "person@gmail.com",
	}
	if err := c.LinkIDPToUser(context.Background(), "existing-1", identity); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/v2/users/existing-1/links" {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{`"idpId":"idp-1"`, `"userId":"google-sub-1"`, `"userName":"person@gmail.com"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body %q missing %q", gotBody, want)
		}
	}
}

func TestLinkIDPToUserRefusesAnUnverifiedEmailWithoutCallingZitadel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not link an unverified email to an existing account")
		w.WriteHeader(http.StatusInternalServerError)
	})
	err := c.LinkIDPToUser(context.Background(), "existing-1", IDPIdentity{Email: "person@gmail.com", EmailVerified: false})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestCreateHumanUserWithPasswordSendsReturnCodeDirectlyUnderEmail(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/users/human" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Write([]byte(`{"userId":"new-user-1","emailCode":"123456"}`))
	})
	userID, emailCode, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if userID != "new-user-1" {
		t.Fatalf("userID = %q", userID)
	}
	if emailCode != "123456" {
		t.Fatalf("emailCode = %q", emailCode)
	}
	// returnCode must sit DIRECTLY under email, never wrapped in a "medium"
	// key — the phase 5 defect this package's doc warns about returns 200
	// for the wrapped shape too, so only inspecting the request body proves
	// this test would have caught it.
	if !strings.Contains(gotBody, `"email":{"email":"shopper@example.com","returnCode":{}}`) {
		t.Fatalf("request body = %q, want returnCode directly under email, unwrapped", gotBody)
	}
	if strings.Contains(gotBody, `"medium"`) {
		t.Fatalf("request body = %q, must not wrap returnCode in a medium key", gotBody)
	}
	if !strings.Contains(gotBody, `"password":{"password":"test-password-not-real"}`) {
		t.Fatalf("request body = %q, missing password", gotBody)
	}
}

func TestCreateHumanUserWithPasswordFailsClosedWhenZitadelOmitsTheEmailCode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// A 200 with no emailCode: the wrapped-oneof defect shape (Zitadel
		// mailed the account itself and gave this caller nothing to send).
		w.Write([]byte(`{"userId":"new-user-1"}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable when emailCode is missing from a 200", err)
	}
}

func TestCreateHumanUserWithPasswordMapsAWeakPasswordDistinctlyFromDuplicateEmail(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"Password is too short (DOMAIN-HuJf6)","details":[{"id":"DOMAIN-HuJf6"}]}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "x", "", "")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
	if errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatal("must not also read as ErrEmailAlreadyExists")
	}
}

func TestCreateHumanUserWithPasswordMapsADuplicateEmailDistinctlyFromWeakPassword(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":6,"message":"user already exists (COMMAND-oR9nS)","details":[{"id":"COMMAND-oR9nS"}]}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, want ErrEmailAlreadyExists", err)
	}
	if errors.Is(err, ErrWeakPassword) {
		t.Fatal("must not also read as ErrWeakPassword")
	}
}

func TestCreateHumanUserWithPasswordDoesNotMapAnUnrelated400ToEitherSentinel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":3,"message":"invalid profile (COMMAND-9zK2p)","details":[{"id":"COMMAND-9zK2p"}]}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if errors.Is(err, ErrWeakPassword) || errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, must not mislabel an unrelated 400 (code 3, unrecognized id)", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable as the generic fallback", err)
	}
}

func TestCreateHumanUserWithPasswordFallsBackToNeutralNamesWhenAbsent(t *testing.T) {
	var gotBody string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Write([]byte(`{"userId":"new-user-1","emailCode":"123456"}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(gotBody, `"givenName":"shopper"`) {
		t.Fatalf("request body = %q, want the email local part as the given name fallback", gotBody)
	}
	// "User" is the actual documented fallback for this path (see
	// boundedProfileName's callers above) — asserting its presence, not
	// merely the absence of the merchant-flavoured "Member" placeholder
	// (which never appears anywhere in this call), is what would actually
	// catch a regressed fallback value.
	if !strings.Contains(gotBody, `"familyName":"User"`) {
		t.Fatalf("request body = %q, want the neutral \"User\" family name fallback", gotBody)
	}
}

// TestCreateHumanUserWithPasswordMatchesDuplicateEmailByIDEvenWithAnUnexpectedCode
// is the fix for review Finding 3: the brief asked for id-keying, not
// code-only narrowing. Zitadel's grpc code for a given error id is not
// something this package controls; keying primarily off details[0].id (with
// code 6 kept only as a fallback) means a duplicate-email 400 that, for
// whatever reason, arrives with a code other than 6 still maps correctly,
// as long as the id matches.
func TestCreateHumanUserWithPasswordMatchesDuplicateEmailByIDEvenWithAnUnexpectedCode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":9,"message":"user already exists (COMMAND-oR9nS)","details":[{"id":"COMMAND-oR9nS"}]}`))
	})
	_, _, err := c.CreateHumanUserWithPassword(context.Background(), "shopper@example.com", "test-password-not-real", "", "")
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("err = %v, want ErrEmailAlreadyExists — the id must be matched even when the code is unexpected", err)
	}
}

func TestDeleteUserSendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	if err := c.DeleteUser(context.Background(), "user-1"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v2/users/user-1" {
		t.Fatalf("got %s %s, want DELETE /v2/users/user-1", gotMethod, gotPath)
	}
}

func TestDeleteUserTreatsNotFoundAsSuccess(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"User could not be found (QUERY-Dfbg2)","details":[{"id":"QUERY-Dfbg2"}]}`))
	})
	if err := c.DeleteUser(context.Background(), "user-1"); err != nil {
		t.Fatalf("err = %v, want nil — a 404 on delete is idempotent success", err)
	}
}

func TestUserEmailVerifiedReadsIsVerified(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"user":{"human":{"email":{"email":"a@b.test","isVerified":false}}}}`))
	})
	verified, err := c.UserEmailVerified(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if verified {
		t.Fatal("verified = true, want false")
	}
}
