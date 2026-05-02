package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// captureSender is a Sender double that records whatever Email passes
// through, so tests can assert the rendered output without burning a
// real SendGrid call.
type captureSender struct {
	mu  sync.Mutex
	out []Email
	err error
}

func (s *captureSender) Send(_ context.Context, msg Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.out = append(s.out, msg)
	return nil
}

func newTestRouter(loader *Loader, sender Sender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	internal := r.Group("/internal")
	NewHandler(loader, sender, "noreply@mark8ly.com").Register(internal)
	return r
}

// TestHandler_Refresh_NoBody evicts all entries when called without
// a body — the cache becomes a fresh map regardless of prior state.
func TestHandler_Refresh_NoBody(t *testing.T) {
	loader := NewLoader(nil)
	r := newTestRouter(loader, &captureSender{})

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"refreshed":true`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestHandler_Refresh_SpecificKey accepts the targeted form. Both
// shapes (empty body and explicit key) need to succeed because
// tesserix-home may not always know which template changed.
func TestHandler_Refresh_SpecificKey(t *testing.T) {
	loader := NewLoader(nil)
	r := newTestRouter(loader, &captureSender{})

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/refresh",
		strings.NewReader(`{"key":"welcome"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"key":"welcome"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestHandler_TestSend_RoundTrip renders + sends a welcome email via
// the captureSender. Asserts the rendered Email carries the right
// subject + interpolated vars.
func TestHandler_TestSend_RoundTrip(t *testing.T) {
	loader := NewLoader(nil)
	sender := &captureSender{}
	r := newTestRouter(loader, sender)

	body := map[string]any{
		"to": "ops@mark8ly.com",
		"vars": map[string]string{
			"BusinessName":  "Acme Co",
			"OwnerName":     "Pat",
			"AdminURL":      "https://acme-admin.mark8ly.com",
			"StorefrontURL": "https://acme.mark8ly.com",
			"SupportEmail":  "help@mark8ly.com",
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/welcome/test", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if len(sender.out) != 1 {
		t.Fatalf("want 1 sent message, got %d", len(sender.out))
	}
	got := sender.out[0]
	if got.To != "ops@mark8ly.com" {
		t.Errorf("To = %q, want ops@mark8ly.com", got.To)
	}
	if !strings.Contains(got.Subject, "Acme Co") {
		t.Errorf("Subject = %q, want to contain 'Acme Co'", got.Subject)
	}
	if !strings.Contains(got.HTMLBody, "https://acme-admin.mark8ly.com") {
		t.Errorf("HTMLBody missing AdminURL")
	}
}

// TestHandler_TestSend_UnknownKey returns a 400 with a useful error
// rather than panicking on the type-assertion.
func TestHandler_TestSend_UnknownKey(t *testing.T) {
	loader := NewLoader(nil)
	r := newTestRouter(loader, &captureSender{})

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/no-such/test",
		strings.NewReader(`{"to":"x@y.com","vars":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// TestHandler_TestSend_MissingTo returns 400 instead of sending to
// an empty address (which would error inside the sender anyway).
func TestHandler_TestSend_MissingTo(t *testing.T) {
	loader := NewLoader(nil)
	r := newTestRouter(loader, &captureSender{})

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/welcome/test",
		strings.NewReader(`{"to":"","vars":{"BusinessName":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}
