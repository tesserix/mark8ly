package authbffclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func idpServer(t *testing.T, status int, body string, capture *map[string]any) *MobileLoginClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			raw, _ := io.ReadAll(r.Body)
			var got map[string]any
			_ = json.Unmarshal(raw, &got)
			got["__path"] = r.URL.Path
			got["__internal_auth"] = r.Header.Get("X-Internal-Auth")
			*capture = got
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewMobileLoginClient(srv.URL, "s3cret", srv.Client())
}

func TestIDPStartPostsTheMobileRouteWithTheInternalSecret(t *testing.T) {
	var got map[string]any
	c := idpServer(t, http.StatusOK, `{"auth_url":"https://zitadel.test/go"}`, &got)

	url, err := c.IDPStart(context.Background(), ProviderGoogle, "https://admin.mark8ly.com/auth/idp/mobile")
	if err != nil {
		t.Fatalf("IDPStart: %v", err)
	}
	if url != "https://zitadel.test/go" {
		t.Fatalf("auth url = %q", url)
	}
	if got["__path"] != "/auth/zitadel/mobile/idp/start" {
		t.Fatalf("path = %v; the mobile route is the only one that mints an auth request", got["__path"])
	}
	if got["__internal_auth"] != "s3cret" {
		t.Fatal("the internal-auth secret must be sent; auth-bff refuses without it")
	}
	if got["provider"] != "google" || got["return_url"] != "https://admin.mark8ly.com/auth/idp/mobile" {
		t.Fatalf("body = %v", got)
	}
}

// An empty auth_url would send the app into a blank browser session that
// reads as a user cancellation.
func TestIDPStartErrorsRatherThanReturningAnEmptyAuthURL(t *testing.T) {
	c := idpServer(t, http.StatusOK, `{}`, nil)
	if _, err := c.IDPStart(context.Background(), ProviderGoogle, "https://admin.mark8ly.com/x"); err == nil {
		t.Fatal("want an error for a missing auth_url")
	}
}

// auth-bff's own code must survive: the IDP path needs actionable copy,
// unlike the enumeration-safe password path.
func TestIDPCallsPreserveAuthBFFsErrorCode(t *testing.T) {
	c := idpServer(t, http.StatusForbidden, `{"error":"no_admin_account"}`, nil)

	_, err := c.IDPFinish(context.Background(), ProviderGoogle, "i1", "it1")

	var idpErr *IDPError
	if !errors.As(err, &idpErr) {
		t.Fatalf("err = %v, want an *IDPError", err)
	}
	if idpErr.Code != "no_admin_account" || idpErr.Status != http.StatusForbidden {
		t.Fatalf("idpErr = %+v", idpErr)
	}
	// A 401 must NOT collapse into ErrInvalidCredentials the way the
	// password route's does — that would erase the reason.
	if errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("IDP errors must not flatten into ErrInvalidCredentials")
	}
}

func TestIDPFinishDecodesTheTenantRequiredShape(t *testing.T) {
	var got map[string]any
	c := idpServer(t, http.StatusOK,
		`{"tenant_required":true,"session_id":"s1","session_token":"tok-1","login_name":"a@b.test"}`, &got)

	res, err := c.IDPFinish(context.Background(), ProviderGoogle, "i1", "it1")
	if err != nil {
		t.Fatalf("IDPFinish: %v", err)
	}
	if !res.TenantRequired || res.SessionID != "s1" || res.SessionToken != "tok-1" || res.LoginName != "a@b.test" {
		t.Fatalf("res = %+v", res)
	}
	// No auth_request_id: auth-bff mints one for a native client.
	if _, sent := got["auth_request_id"]; sent {
		t.Fatal("auth_request_id must not be sent; auth-bff mints one for the mobile route")
	}
	if got["intent_id"] != "i1" || got["intent_token"] != "it1" {
		t.Fatalf("body = %v", got)
	}
}

// Complete answers with the identical body a completed password login
// does, so it decodes through the same path.
func TestIDPCompleteDecodesTheLoginShape(t *testing.T) {
	var got map[string]any
	c := idpServer(t, http.StatusOK,
		`{"data":{"uid":"u1","email":"a@b.test","tenant_id":"t-1","access_token":"AT","refresh_token":"RT","token_type":"Bearer","expires_in":3600}}`, &got)

	res, err := c.IDPComplete(context.Background(), "a@b.test", "s1", "tok-1", "t-1")
	if err != nil {
		t.Fatalf("IDPComplete: %v", err)
	}
	if res.AccessToken != "AT" || res.TenantID != "t-1" || res.UID != "u1" {
		t.Fatalf("res = %+v", res)
	}
	if got["__path"] != "/auth/zitadel/mobile/idp/complete" {
		t.Fatalf("path = %v", got["__path"])
	}
}

func TestIDPCompleteDecodesTheStepUpShape(t *testing.T) {
	c := idpServer(t, http.StatusOK,
		`{"data":{"email":"a@b.test","tenant_id":"t-1","email_otp_required":true,"pending_token":"sealed"}}`, nil)

	res, err := c.IDPComplete(context.Background(), "a@b.test", "s1", "tok-1", "t-1")
	if err != nil {
		t.Fatalf("IDPComplete: %v", err)
	}
	if !res.EmailOTPRequired || res.PendingToken != "sealed" || res.AccessToken != "" {
		t.Fatalf("res = %+v", res)
	}
}
