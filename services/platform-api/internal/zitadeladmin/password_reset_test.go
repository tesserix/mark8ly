package zitadeladmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// TestSendPasswordResetOobCode_HappyPath drives the full email->id->code
// flow: the search hit resolves an id, the password_reset call is made
// against that id's path with the "returnCode" medium (so Zitadel does not
// send its own notification — see D7), and the returned code is opaque but
// decodes back to the same id and code.
func TestSendPasswordResetOobCode_HappyPath(t *testing.T) {
	var resetPath string
	var resetBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(searchResponse(humanEntry("user-42", "merchant@example.com", true)))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/password_reset"):
			resetPath = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &resetBody)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"verificationCode":"code-abc"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	oobCode, err := c.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
	if err != nil {
		t.Fatalf("SendPasswordResetOobCode: %v", err)
	}
	if oobCode == "" {
		t.Fatal("expected non-empty oobCode")
	}
	if resetPath != "/v2/users/user-42/password_reset" {
		t.Errorf("resetPath = %q", resetPath)
	}
	medium, ok := resetBody["medium"].(map[string]any)
	if !ok {
		t.Fatalf("medium = %v", resetBody["medium"])
	}
	if _, ok := medium["returnCode"]; !ok {
		t.Errorf("expected medium.returnCode, got %v", medium)
	}

	userID, code, err := decodeCompositeCode(oobCode)
	if err != nil {
		t.Fatalf("decodeCompositeCode: %v", err)
	}
	if userID != "user-42" || code != "code-abc" {
		t.Errorf("userID=%q code=%q, want user-42/code-abc", userID, code)
	}
}

func TestSendPasswordResetOobCode_UnknownEmailMapsToUserNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(searchResponse())
	})

	_, err := c.SendPasswordResetOobCode(context.Background(), "nobody@example.com")
	if !errors.Is(err, gipadmin.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

// TestResetPassword_HappyPath proves the composite code round-trips: it
// decodes back to the id embedded in the URL path and the code embedded in
// the request body, alongside the new password.
func TestResetPassword_HappyPath(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	oobCode := encodeCompositeCode("user-42", "code-abc")
	if err := c.ResetPassword(context.Background(), oobCode, "correct horse battery staple"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if gotPath != "/v2/users/user-42/password" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["verificationCode"] != "code-abc" {
		t.Errorf("verificationCode = %v", gotBody["verificationCode"])
	}
	np, ok := gotBody["newPassword"].(map[string]any)
	if !ok || np["password"] != "correct horse battery staple" {
		t.Errorf("newPassword = %v", gotBody["newPassword"])
	}
}

func TestResetPassword_MalformedCodeNeverHitsNetwork(t *testing.T) {
	called := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	err := c.ResetPassword(context.Background(), "not-a-real-code", "correct horse battery staple")
	if !errors.Is(err, gipadmin.ErrInvalidOobCode) {
		t.Fatalf("err = %v, want ErrInvalidOobCode", err)
	}
	if called {
		t.Error("expected no network call for an undecodable oobCode")
	}
}

func TestResetPassword_UpstreamInvalidCodeMapsToSentinel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":3,"message":"verification code is invalid or expired"}`))
	})

	oobCode := encodeCompositeCode("user-42", "stale-code")
	err := c.ResetPassword(context.Background(), oobCode, "correct horse battery staple")
	if !errors.Is(err, gipadmin.ErrInvalidOobCode) {
		t.Fatalf("err = %v, want ErrInvalidOobCode", err)
	}
}

func TestResetPassword_UpstreamWeakPasswordMapsToSentinel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":3,"message":"password does not meet the complexity policy"}`))
	})

	oobCode := encodeCompositeCode("user-42", "code-abc")
	err := c.ResetPassword(context.Background(), oobCode, "weak")
	if !errors.Is(err, gipadmin.ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}
