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
