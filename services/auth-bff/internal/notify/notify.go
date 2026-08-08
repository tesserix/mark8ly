// Package notify is auth-bff's client for platform-api's notification
// send endpoint. It owns the two security emails on the login path: the
// one-time sign-in code and the unrecognised-device alert.
//
// The two have opposite failure semantics on purpose. A failed OTP send
// must fail the request — the user is about to wait for a code that will
// never arrive, so telling them now is strictly better. A failed device
// alert must not, because the sign-in itself already succeeded and
// refusing it after the fact helps nobody; deviceguard swallows and logs.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mark8ly/auth-bff/internal/deviceguard"
)

// Config holds the client's dependencies. BaseURL empty disables the
// client, and every call then returns an error rather than succeeding
// silently — a security email that quietly vanishes is worse than a
// loud failure.
type Config struct {
	BaseURL      string
	AuthSecret   string
	SupportEmail string
	SecureURL    string
	HTTPClient   *http.Client
}

// Client posts rendered-template send requests to platform-api.
type Client struct {
	baseURL      string
	authSecret   string
	supportEmail string
	secureURL    string
	http         *http.Client
}

// New constructs a Client.
func New(cfg Config) *Client {
	hc := cfg.HTTPClient
	if hc == nil {
		// Bounded: this sits on the login request path.
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:      cfg.BaseURL,
		authSecret:   cfg.AuthSecret,
		supportEmail: cfg.SupportEmail,
		secureURL:    cfg.SecureURL,
		http:         hc,
	}
}

// SendLoginCode delivers the one-time sign-in code.
func (c *Client) SendLoginCode(ctx context.Context, email, code string, ttl time.Duration) error {
	return c.post(ctx, "login_otp", email, map[string]string{
		"Code":         code,
		"ExpiresIn":    humaniseTTL(ttl),
		"SupportEmail": c.supportEmail,
	})
}

// NotifyNewDevice implements deviceguard.Notifier.
func (c *Client) NotifyNewDevice(ctx context.Context, a deviceguard.Alert) error {
	if a.Email == "" {
		return fmt.Errorf("notify: no recipient for new-device alert (user %s)", a.UserID)
	}
	return c.post(ctx, "new_device_login", a.Email, map[string]string{
		"Device":       a.Device,
		"CountryName":  a.CountryName,
		"IPAddress":    a.IPAddress,
		"At":           a.At.UTC().Format("2 Jan 2006 at 15:04 MST"),
		"SecureURL":    c.secureURL,
		"SupportEmail": c.supportEmail,
	})
}

type sendRequest struct {
	Key  string            `json:"key"`
	To   string            `json:"to"`
	Vars map[string]string `json:"vars"`
}

func (c *Client) post(ctx context.Context, key, to string, vars map[string]string) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("notify: no platform-api base URL configured, cannot send %q", key)
	}

	body, err := json.Marshal(sendRequest{Key: key, To: to, Vars: vars})
	if err != nil {
		return fmt.Errorf("notify: marshal %s: %w", key, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/internal/notifications/send", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build request %s: %w", key, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authSecret != "" {
		req.Header.Set("X-Internal-Auth", c.authSecret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("notify: post %s: %w", key, err)
	}
	defer resp.Body.Close()
	// Drain so the connection returns to the pool.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("notify: platform-api returned %d for %s", resp.StatusCode, key)
	}
	return nil
}

// humaniseTTL renders a duration as whole minutes for email copy,
// rounding up so the stated expiry is never later than the real one.
func humaniseTTL(d time.Duration) string {
	mins := max(int((d+time.Minute-1)/time.Minute), 1)
	if mins == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", mins)
}
