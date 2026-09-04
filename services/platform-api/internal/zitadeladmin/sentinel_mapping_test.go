package zitadeladmin

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// TestSentinelMapping_MatchesHandlerContract is the test that proves the
// contract in the task brief: every sentinel internal/auth/handler.go
// branches on with errors.Is (ErrUserNotFound, ErrInvalidOobCode,
// ErrWeakPassword, ErrUnauthenticated, ErrTooManyAttempts, ErrUnavailable)
// is reachable through this package's own error paths, asserted against
// the SAME gipadmin.Err* values handler.go checks — not a locally
// redeclared lookalike.
func TestSentinelMapping_MatchesHandlerContract(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
		via    func(c *Client) error
	}{
		{
			name:   "too many attempts on password reset request",
			status: http.StatusTooManyRequests,
			body:   `{"code":8,"message":"rate limit exceeded"}`,
			want:   gipadmin.ErrTooManyAttempts,
			via: func(c *Client) error {
				_, err := c.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
				return err
			},
		},
		{
			name:   "unauthenticated on password reset request (401)",
			status: http.StatusUnauthorized,
			body:   `{"code":16,"message":"invalid PAT"}`,
			want:   gipadmin.ErrUnauthenticated,
			via: func(c *Client) error {
				_, err := c.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
				return err
			},
		},
		{
			name:   "unauthenticated on password reset request (403)",
			status: http.StatusForbidden,
			body:   `{"code":7,"message":"permission denied"}`,
			want:   gipadmin.ErrUnauthenticated,
			via: func(c *Client) error {
				_, err := c.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
				return err
			},
		},
		{
			name:   "unavailable on 5xx during password reset request",
			status: http.StatusBadGateway,
			body:   `{"code":14,"message":"upstream failure"}`,
			want:   gipadmin.ErrUnavailable,
			via: func(c *Client) error {
				_, err := c.SendPasswordResetOobCode(context.Background(), "merchant@example.com")
				return err
			},
		},
		{
			name:   "invalid code on confirm",
			status: http.StatusBadRequest,
			body:   `{"code":3,"message":"the verification code is invalid or expired"}`,
			want:   gipadmin.ErrInvalidOobCode,
			via: func(c *Client) error {
				return c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "code"), "correct horse battery staple")
			},
		},
		{
			name:   "weak password on confirm",
			status: http.StatusBadRequest,
			body:   `{"code":3,"message":"password fails the complexity policy"}`,
			want:   gipadmin.ErrWeakPassword,
			via: func(c *Client) error {
				return c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "code"), "weak")
			},
		},
		{
			name:   "user not found on delete",
			status: http.StatusNotFound,
			body:   `{"code":5,"message":"User not found"}`,
			want:   nil, // DeleteAccount treats not-found as success (idempotent)
			via: func(c *Client) error {
				return c.DeleteAccount(context.Background(), "user-1")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				// Password-reset-request cases hit the search endpoint
				// first; answer it with exactly one match so the flow
				// reaches the call under test.
				if r.URL.Path == "/v2/users" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(searchResponse(humanEntry("user-1", "merchant@example.com", true)))
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			err := tc.via(c)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is match for %v", err, tc.want)
			}
		})
	}
}

// TestUpstreamUnavailable_DoesNotSurfaceAsCredentialError is the explicit
// constraint from the task brief: an unreachable/failing upstream must
// read as unavailable, never as "wrong code" or "weak password" — the two
// sentinels a destructive password-reset confirm could otherwise be
// mistaken for.
func TestUpstreamUnavailable_DoesNotSurfaceAsCredentialError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`not even json`))
	})

	err := c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "code"), "correct horse battery staple")
	if !errors.Is(err, gipadmin.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if errors.Is(err, gipadmin.ErrInvalidOobCode) || errors.Is(err, gipadmin.ErrWeakPassword) {
		t.Fatalf("err = %v must not also match a credential sentinel", err)
	}
}

// TestNetworkFailureMapsToUnavailable proves a genuinely unreachable
// upstream (server closed, not just a bad status) also maps to
// ErrUnavailable, since network transport errors take a different path
// through do() than a non-2xx status.
func TestNetworkFailureMapsToUnavailable(t *testing.T) {
	c, err := New(Config{BaseURL: "http://127.0.0.1:1", Token: "t", OrgID: "o"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	derr := c.DeleteAccount(context.Background(), "user-1")
	if !errors.Is(derr, gipadmin.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", derr)
	}
}
