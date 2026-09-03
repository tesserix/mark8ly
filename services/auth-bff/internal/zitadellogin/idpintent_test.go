package zitadellogin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestStartIDPIntentReturnsTheAuthURL(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/idp_intents" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-pat" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"details":{"sequence":"1"},"authUrl":"https://idp.example/authorize?state=abc"}`))
	})
	got, err := c.StartIDPIntent(context.Background(), "idp-1", "https://app.example/success", "https://app.example/failure")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "https://idp.example/authorize?state=abc" {
		t.Fatalf("authURL = %q", got)
	}
}

func TestStartIDPIntentSendsTheIdpIDAndUrls(t *testing.T) {
	var body string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		w.Write([]byte(`{"authUrl":"https://idp.example/authorize"}`))
	})
	_, err := c.StartIDPIntent(context.Background(), "idp-1", "https://app.example/success", "https://app.example/failure")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{`"idpId":"idp-1"`, `"successUrl":"https://app.example/success"`, `"failureUrl":"https://app.example/failure"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request body %q missing %q", body, want)
		}
	}
}

func TestStartIDPIntentErrorsWhenAuthURLIsMissingRatherThanReturningEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"details":{"sequence":"1"}}`))
	})
	got, err := c.StartIDPIntent(context.Background(), "idp-1", "https://app.example/success", "https://app.example/failure")
	if err == nil {
		t.Fatalf("err = nil, want an error for a response with no authUrl")
	}
	if got != "" {
		t.Errorf("authURL = %q on error, want empty", got)
	}
}

func TestStartIDPIntentSurfacesTheZitadelErrorID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":13,"message":"internal (COMMAND-9Ks3f)","details":[{"@type":"type.googleapis.com/zitadel.v1.FailedEvent","id":"COMMAND-9Ks3f"}]}`))
	})
	_, err := c.StartIDPIntent(context.Background(), "idp-missing", "https://app.example/success", "https://app.example/failure")
	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "COMMAND-9Ks3f") {
		t.Errorf("error text = %q, want it to carry the Zitadel error id", err.Error())
	}
}

func TestRetrieveIDPIntentReturnsALinkedIdentity(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/idp_intents/intent-1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{
			"userId": "339070697432875523",
			"idpInformation": {
				"idpId": "idp-1",
				"userId": "108234...google-sub",
				"userName": "person@gmail.com",
				"rawInformation": {"sub":"108234...google-sub","email":"person@gmail.com","email_verified":true,"name":"Person"}
			}
		}`))
	})
	got, err := c.RetrieveIDPIntent(context.Background(), "intent-1", "tok-abc")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.ZitadelUserID != "339070697432875523" {
		t.Errorf("ZitadelUserID = %q", got.ZitadelUserID)
	}
	if got.Email != "person@gmail.com" {
		t.Errorf("Email = %q", got.Email)
	}
	if !got.EmailVerified {
		t.Error("EmailVerified = false, want true")
	}
}

func TestRetrieveIDPIntentReturnsAnUnlinkedIdentityWithEmptyZitadelUserID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Zitadel elides the "userId" field entirely when the intent has not
		// been linked to any existing Zitadel user yet.
		w.Write([]byte(`{
			"idpInformation": {
				"idpId": "idp-1",
				"userId": "108234...google-sub",
				"userName": "new.person@gmail.com",
				"rawInformation": {"sub":"108234...google-sub","email":"new.person@gmail.com","email_verified":false}
			}
		}`))
	})
	got, err := c.RetrieveIDPIntent(context.Background(), "intent-1", "tok-abc")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.ZitadelUserID != "" {
		t.Errorf("ZitadelUserID = %q, want empty for an unlinked intent", got.ZitadelUserID)
	}
	if got.Email != "new.person@gmail.com" {
		t.Errorf("Email = %q", got.Email)
	}
	if got.EmailVerified {
		t.Error("EmailVerified = true, want false")
	}
}

func TestRetrieveIDPIntentSendsTheIntentToken(t *testing.T) {
	var body string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
		w.Write([]byte(`{"idpInformation":{"rawInformation":{}}}`))
	})
	_, err := c.RetrieveIDPIntent(context.Background(), "intent-1", "tok-secret-value")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(body, `"idpIntentToken":"tok-secret-value"`) {
		t.Errorf("request body %q missing idpIntentToken", body)
	}
}

func TestRetrieveIDPIntentSurfacesTheZitadelErrorID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"intent not found (COMMAND-2Ls8f)","details":[{"@type":"type.googleapis.com/zitadel.v1.FailedEvent","id":"COMMAND-2Ls8f"}]}`))
	})
	_, err := c.RetrieveIDPIntent(context.Background(), "intent-missing", "tok-abc")
	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	if !errors.Is(err, ErrIDPIntentInvalid) {
		t.Fatalf("err = %v, want ErrIDPIntentInvalid", err)
	}
	if !strings.Contains(err.Error(), "COMMAND-2Ls8f") {
		t.Errorf("error text = %q, want it to carry the Zitadel error id", err.Error())
	}
}

