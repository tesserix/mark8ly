package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark8ly/auth-bff/internal/deviceguard"
)

// capture records what the fake platform-api received.
type capture struct {
	mu     sync.Mutex
	paths  []string
	auth   []string
	bodies []map[string]any
	status int
}

func (c *capture) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)

		c.mu.Lock()
		c.paths = append(c.paths, r.URL.Path)
		c.auth = append(c.auth, r.Header.Get("X-Internal-Auth"))
		c.bodies = append(c.bodies, body)
		status := c.status
		c.mu.Unlock()

		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"sent":true}`))
	}
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.bodies)
}

func newFake(t *testing.T, cap *capture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(cap.handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestSendLoginCode posts the OTP to platform-api's send endpoint with the
// internal auth header attached.
func TestSendLoginCode(t *testing.T) {
	cap := &capture{}
	srv := newFake(t, cap)
	c := New(Config{BaseURL: srv.URL, AuthSecret: "s3cret", SupportEmail: "help@mark8ly.com"})

	if err := c.SendLoginCode(context.Background(), "user@example.com", "482913", 5*time.Minute); err != nil {
		t.Fatalf("SendLoginCode: %v", err)
	}

	if cap.count() != 1 {
		t.Fatalf("posted %d times, want 1", cap.count())
	}
	if got := cap.paths[0]; got != "/internal/notifications/send" {
		t.Errorf("path = %q", got)
	}
	if got := cap.auth[0]; got != "s3cret" {
		t.Errorf("X-Internal-Auth = %q, want s3cret", got)
	}
	body := cap.bodies[0]
	if body["key"] != "login_otp" {
		t.Errorf("key = %v, want login_otp", body["key"])
	}
	if body["to"] != "user@example.com" {
		t.Errorf("to = %v", body["to"])
	}
	vars, ok := body["vars"].(map[string]any)
	if !ok {
		t.Fatalf("vars is not an object: %#v", body["vars"])
	}
	if vars["Code"] != "482913" {
		t.Errorf("Code = %v, want 482913", vars["Code"])
	}
	if vars["ExpiresIn"] != "5 minutes" {
		t.Errorf("ExpiresIn = %v, want '5 minutes'", vars["ExpiresIn"])
	}
	if vars["SupportEmail"] != "help@mark8ly.com" {
		t.Errorf("SupportEmail = %v", vars["SupportEmail"])
	}
}

// TestSendLoginCode_ErrorIsReturned matters more than it looks: unlike the
// device alert, a failed OTP send must fail the request, because the user
// would otherwise sit waiting for a code that will never arrive.
func TestSendLoginCode_ErrorIsReturned(t *testing.T) {
	cap := &capture{status: http.StatusBadGateway}
	srv := newFake(t, cap)
	c := New(Config{BaseURL: srv.URL, AuthSecret: "s"})

	err := c.SendLoginCode(context.Background(), "user@example.com", "482913", time.Minute)
	if err == nil {
		t.Fatal("expected an error when platform-api returns 502")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should name the status: %v", err)
	}
}

// TestSendLoginCode_NoBaseURL keeps local dev bootable without
// platform-api, but must not silently pretend a code was delivered.
func TestSendLoginCode_NoBaseURL(t *testing.T) {
	c := New(Config{})
	if err := c.SendLoginCode(context.Background(), "u@example.com", "1", time.Minute); err == nil {
		t.Fatal("expected an error when no base URL is configured")
	}
}

// TestNotifyNewDevice checks the alert payload carries every fact the
// account holder needs to judge the sign-in.
func TestNotifyNewDevice(t *testing.T) {
	cap := &capture{}
	srv := newFake(t, cap)
	c := New(Config{
		BaseURL:      srv.URL,
		AuthSecret:   "s",
		SupportEmail: "help@mark8ly.com",
		SecureURL:    "https://admin.mark8ly.com/settings/security",
	})

	at := time.Date(2026, 8, 9, 1, 19, 0, 0, time.UTC)
	err := c.NotifyNewDevice(context.Background(), deviceguard.Alert{
		UserID:      "gip-abc",
		Email:       "user@example.com",
		Device:      "Chrome on macOS",
		IPAddress:   "203.0.113.9",
		Country:     "IN",
		CountryName: "India",
		At:          at,
	})
	if err != nil {
		t.Fatalf("NotifyNewDevice: %v", err)
	}

	if cap.count() != 1 {
		t.Fatalf("posted %d times, want 1", cap.count())
	}
	body := cap.bodies[0]
	if body["key"] != "new_device_login" {
		t.Errorf("key = %v", body["key"])
	}
	if body["to"] != "user@example.com" {
		t.Errorf("to = %v", body["to"])
	}
	vars := body["vars"].(map[string]any)
	for k, want := range map[string]string{
		"Device":       "Chrome on macOS",
		"CountryName":  "India",
		"IPAddress":    "203.0.113.9",
		"SecureURL":    "https://admin.mark8ly.com/settings/security",
		"SupportEmail": "help@mark8ly.com",
	} {
		if vars[k] != want {
			t.Errorf("vars[%s] = %v, want %v", k, vars[k], want)
		}
	}
	if got, _ := vars["At"].(string); !strings.Contains(got, "2026") {
		t.Errorf("At = %q, want a formatted timestamp", got)
	}
}

// TestNotifyNewDevice_NoEmail short-circuits rather than posting a send
// with an empty recipient that platform-api would reject anyway.
func TestNotifyNewDevice_NoEmail(t *testing.T) {
	cap := &capture{}
	srv := newFake(t, cap)
	c := New(Config{BaseURL: srv.URL, AuthSecret: "s"})

	err := c.NotifyNewDevice(context.Background(), deviceguard.Alert{UserID: "u", Device: "d"})
	if err == nil {
		t.Fatal("expected an error for a missing recipient")
	}
	if cap.count() != 0 {
		t.Errorf("posted %d times, want 0", cap.count())
	}
}

// TestClientSatisfiesNotifier is the wiring guarantee — deviceguard takes
// the interface, and this is what gets injected in main.
func TestClientSatisfiesNotifier(t *testing.T) {
	var _ deviceguard.Notifier = (*Client)(nil)
}

// TestHumaniseTTL covers the copy that goes in front of users.
func TestHumaniseTTL(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{5 * time.Minute, "5 minutes"},
		{time.Minute, "1 minute"},
		{90 * time.Second, "2 minutes"},
		{30 * time.Second, "1 minute"},
		{time.Hour, "60 minutes"},
	}
	for _, tt := range tests {
		if got := humaniseTTL(tt.in); got != tt.want {
			t.Errorf("humaniseTTL(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
