package zitadeladmin

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// TestErrorIDMapping_VerifiedLiveIDs proves classifyError keys off the
// stable details[0].id first, using the exact error bodies observed live
// against the TESSERIX Zitadel instance on 2026-09-04 (see the package
// doc). This is the discrimination message-text matching alone cannot
// make reliably: COMMAND-2M9fs and COMMAND-G8dh3 share both HTTP status
// (400) and grpc code (9), and their messages differ by one word.
func TestErrorIDMapping_VerifiedLiveIDs(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{
			name:   "COMMAND-SAF4f user not found",
			status: http.StatusNotFound,
			body:   `{"code":5,"message":"User could not be found","details":[{"@type":"...","id":"COMMAND-SAF4f"}]}`,
			want:   gipadmin.ErrUserNotFound,
		},
		{
			name:   "COMMAND-2M9fs wrong or expired verification code",
			status: http.StatusBadRequest,
			body:   `{"code":9,"message":"Code not found","details":[{"@type":"...","id":"COMMAND-2M9fs"}]}`,
			want:   gipadmin.ErrInvalidOobCode,
		},
		{
			name:   "COMMAND-G8dh3 missing password field is NOT an invalid code",
			status: http.StatusBadRequest,
			body:   `{"code":9,"message":"Password not found","details":[{"@type":"...","id":"COMMAND-G8dh3"}]}`,
			want:   gipadmin.ErrUnavailable,
		},
		{
			// Different id prefix ("DOMAIN-" not "COMMAND-") and a message
			// ("Password is too short") that hits none of classifyError's
			// substring fallbacks (no "weak"/"polic"/"complexity") — this
			// case only works via the id table, proving the fallback text
			// match alone would have missed it.
			name:   "DOMAIN-HuJf6 too-short password maps to ErrWeakPassword",
			status: http.StatusBadRequest,
			body:   `{"code":3,"message":"Password is too short","details":[{"@type":"...","id":"DOMAIN-HuJf6"}]}`,
			want:   gipadmin.ErrWeakPassword,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := readZitadelErrorID([]byte(tc.body))
			got := classifyError(tc.status, []byte(tc.body), id)
			if !errors.Is(got, tc.want) {
				t.Fatalf("classifyError = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestErrorIDMapping_CommandG8dh3NeverMapsToInvalidOobCode is the specific
// discrimination the task called out: a malformed-request error, despite
// sharing status/grpc-code with a genuine bad verification code, must
// never be mistaken for one.
func TestErrorIDMapping_CommandG8dh3NeverMapsToInvalidOobCode(t *testing.T) {
	body := []byte(`{"code":9,"message":"Password not found","details":[{"id":"COMMAND-G8dh3"}]}`)
	id := readZitadelErrorID(body)
	got := classifyError(http.StatusBadRequest, body, id)
	if errors.Is(got, gipadmin.ErrInvalidOobCode) {
		t.Fatalf("classifyError = %v, must not match ErrInvalidOobCode", got)
	}
	if !errors.Is(got, gipadmin.ErrUnavailable) {
		t.Fatalf("classifyError = %v, want ErrUnavailable", got)
	}
}

// TestErrorIDMapping_EndToEndThroughResetPassword drives the same
// COMMAND-2M9fs / COMMAND-G8dh3 distinction through the real
// ResetPassword call (not just classifyError directly), proving the id is
// actually read off the wire and threaded through do().
func TestErrorIDMapping_EndToEndThroughResetPassword(t *testing.T) {
	t.Run("wrong code maps to ErrInvalidOobCode", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":9,"message":"Code not found","details":[{"id":"COMMAND-2M9fs"}]}`))
		})
		err := c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "wrong-code"), "correct horse battery staple")
		if !errors.Is(err, gipadmin.ErrInvalidOobCode) {
			t.Fatalf("err = %v, want ErrInvalidOobCode", err)
		}
	})

	t.Run("malformed request maps to ErrUnavailable, not ErrInvalidOobCode", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":9,"message":"Password not found","details":[{"id":"COMMAND-G8dh3"}]}`))
		})
		err := c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "code"), "correct horse battery staple")
		if errors.Is(err, gipadmin.ErrInvalidOobCode) {
			t.Fatalf("err = %v, must not match ErrInvalidOobCode", err)
		}
		if !errors.Is(err, gipadmin.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("too-short password maps to ErrWeakPassword via id, not message text", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":3,"message":"Password is too short","details":[{"id":"DOMAIN-HuJf6"}]}`))
		})
		err := c.ResetPassword(context.Background(), encodeCompositeCode("user-1", "code"), "a")
		if !errors.Is(err, gipadmin.ErrWeakPassword) {
			t.Fatalf("err = %v, want ErrWeakPassword", err)
		}
	})
}

// TestReadZitadelErrorID covers the extractor directly: present, absent,
// and undecodable bodies.
func TestReadZitadelErrorID(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"present", `{"code":5,"message":"x","details":[{"id":"COMMAND-SAF4f"}]}`, "COMMAND-SAF4f"},
		{"no details", `{"code":5,"message":"x"}`, ""},
		{"not json", `not even json`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readZitadelErrorID([]byte(tc.body)); got != tc.want {
				t.Errorf("readZitadelErrorID = %q, want %q", got, tc.want)
			}
		})
	}
}
