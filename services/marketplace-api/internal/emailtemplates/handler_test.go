package emailtemplates

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

// captureSender is a TestSender double that records every send for
// later assertion.
type captureSender struct {
	mu  sync.Mutex
	out []capturedSend
	err error
}

type capturedSend struct {
	To       string
	Rendered Rendered
}

func (s *captureSender) SendTest(_ context.Context, to string, r Rendered) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.out = append(s.out, capturedSend{To: to, Rendered: r})
	return nil
}

func newTestRouter(loader *Loader, sender TestSender) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	internal := r.Group("/internal")
	NewHandler(loader, sender).Register(internal)
	return r
}

func TestHandler_Refresh_NoBody(t *testing.T) {
	r := newTestRouter(NewLoader(nil), &captureSender{})
	req := httptest.NewRequest(http.MethodPost, "/internal/templates/refresh", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"refreshed":true`) {
		t.Errorf("body=%s", w.Body.String())
	}
}

func TestHandler_Refresh_SpecificKey(t *testing.T) {
	r := newTestRouter(NewLoader(nil), &captureSender{})
	req := httptest.NewRequest(http.MethodPost, "/internal/templates/refresh",
		strings.NewReader(`{"key":"refund_email"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"key":"refund_email"`) {
		t.Errorf("body=%s", w.Body.String())
	}
}

func TestHandler_TestSend_RoundTrip(t *testing.T) {
	loader := NewLoader(nil)
	loader.Register("welcome", EmbeddedFallback{
		Subject:  "Hello {{.Name}}",
		HTMLBody: "<p>Hi {{.Name}}</p>",
		TextBody: "Hi {{.Name}}",
	})
	sender := &captureSender{}
	r := newTestRouter(loader, sender)

	body, _ := json.Marshal(map[string]interface{}{
		"to":   "ops@mark8ly.com",
		"vars": map[string]string{"Name": "Acme"},
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/templates/welcome/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if len(sender.out) != 1 {
		t.Fatalf("want 1 capture, got %d", len(sender.out))
	}
	got := sender.out[0]
	if got.To != "ops@mark8ly.com" {
		t.Errorf("To = %q", got.To)
	}
	if got.Rendered.Subject != "Hello Acme" {
		t.Errorf("Subject = %q", got.Rendered.Subject)
	}
}

func TestHandler_TestSend_UnknownKey(t *testing.T) {
	r := newTestRouter(NewLoader(nil), &captureSender{})
	req := httptest.NewRequest(http.MethodPost, "/internal/templates/no-such/test",
		strings.NewReader(`{"to":"x@y.com","vars":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", w.Code, w.Body.String())
	}
}

func TestHandler_TestSend_MissingTo(t *testing.T) {
	r := newTestRouter(NewLoader(nil), &captureSender{})
	req := httptest.NewRequest(http.MethodPost, "/internal/templates/welcome/test",
		strings.NewReader(`{"to":"","vars":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandler_TestSend_NoSender(t *testing.T) {
	loader := NewLoader(nil)
	loader.Register("welcome", EmbeddedFallback{
		Subject: "s", HTMLBody: "<p>h</p>", TextBody: "t",
	})
	r := newTestRouter(loader, nil)

	req := httptest.NewRequest(http.MethodPost, "/internal/templates/welcome/test",
		strings.NewReader(`{"to":"x@y.com","vars":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
