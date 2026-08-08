package notification

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postSend(t *testing.T, r http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications/send", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestHandler_Send_LoginOTP is the path auth-bff takes on every OTP
// request: a key, a recipient, and typed vars in, one rendered email out.
func TestHandler_Send_LoginOTP(t *testing.T) {
	sender := &captureSender{}
	r := newTestRouter(NewLoader(nil), sender)

	w := postSend(t, r, `{"key":"login_otp","to":"user@example.com","vars":{"Code":"482913","ExpiresIn":"5 minutes","SupportEmail":"help@mark8ly.com"}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(sender.out) != 1 {
		t.Fatalf("sent %d emails, want 1", len(sender.out))
	}
	msg := sender.out[0]
	if msg.To != "user@example.com" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.From != "noreply@mark8ly.com" {
		t.Errorf("From = %q, want the handler's configured from address", msg.From)
	}
	if !strings.Contains(msg.TextBody, "482913") {
		t.Errorf("TextBody missing the code: %q", msg.TextBody)
	}
}

// TestHandler_Send_NewDeviceLogin covers the second caller, including the
// tenant_id passthrough that the loader stamps onto the Email.
func TestHandler_Send_NewDeviceLogin(t *testing.T) {
	sender := &captureSender{}
	r := newTestRouter(NewLoader(nil), sender)

	w := postSend(t, r, `{"key":"new_device_login","to":"user@example.com","tenant_id":"t-9","vars":{"Device":"Chrome on macOS","CountryName":"India","IPAddress":"203.0.113.9","At":"9 Aug 2026","SecureURL":"https://x/","SupportEmail":"help@mark8ly.com"}}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if len(sender.out) != 1 {
		t.Fatalf("sent %d emails, want 1", len(sender.out))
	}
	if got := sender.out[0].TenantID; got != "t-9" {
		t.Errorf("TenantID = %q, want t-9", got)
	}
	if !strings.Contains(sender.out[0].TextBody, "Chrome on macOS") {
		t.Error("TextBody missing the device")
	}
}

// TestHandler_Send_Validation asserts the endpoint rejects bad input
// before it reaches the sender — an unknown key must not fall through to
// a blank email.
func TestHandler_Send_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing key", `{"to":"u@example.com"}`, http.StatusBadRequest},
		{"missing to", `{"key":"login_otp"}`, http.StatusBadRequest},
		{"unknown key", `{"key":"nope","to":"u@example.com"}`, http.StatusBadRequest},
		{"malformed json", `{`, http.StatusBadRequest},
		{"vars wrong shape", `{"key":"login_otp","to":"u@example.com","vars":[1,2,3]}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &captureSender{}
			r := newTestRouter(NewLoader(nil), sender)

			w := postSend(t, r, tt.body)

			if w.Code != tt.want {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tt.want, w.Body.String())
			}
			if len(sender.out) != 0 {
				t.Errorf("sent %d emails, want 0 — invalid input must not reach the sender", len(sender.out))
			}
		})
	}
}

// TestHandler_Send_SenderFailure asserts a provider outage surfaces as
// 502 so the caller can distinguish "your request was wrong" from "we
// could not deliver", and retry only the latter.
func TestHandler_Send_SenderFailure(t *testing.T) {
	sender := &captureSender{err: errors.New("resend: 503")}
	r := newTestRouter(NewLoader(nil), sender)

	w := postSend(t, r, `{"key":"login_otp","to":"user@example.com","vars":{"Code":"111111","ExpiresIn":"5 minutes"}}`)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "send_failed" {
		t.Errorf("error = %v, want send_failed", resp["error"])
	}
}

// TestHandler_Send_DoesNotEchoCode guards against the OTP leaking into a
// response body that gets logged by an intermediary. The caller already
// knows nothing about the code and must not learn it here.
func TestHandler_Send_DoesNotEchoCode(t *testing.T) {
	r := newTestRouter(NewLoader(nil), &captureSender{})

	w := postSend(t, r, `{"key":"login_otp","to":"user@example.com","vars":{"Code":"482913","ExpiresIn":"5 minutes"}}`)

	if strings.Contains(w.Body.String(), "482913") {
		t.Errorf("response echoes the one-time code: %s", w.Body.String())
	}
}
