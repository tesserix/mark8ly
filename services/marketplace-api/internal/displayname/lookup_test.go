package displayname

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestLookup(t *testing.T, h http.HandlerFunc) *AuthBFFLookup {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewAuthBFFLookup(srv.URL, "s3cret", srv.Client())
}

func TestDisplayNameReturnsTrimmedName(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/internal/users/uid-google/display-name" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Internal-Auth"); got != "s3cret" {
			t.Errorf("X-Internal-Auth = %q", got)
		}
		w.Write([]byte(`{"data":{"user_id":"uid-google","display_name":"  Jane Roe  "}}`))
	})

	got, err := l.DisplayName(context.Background(), "uid-google")
	if err != nil {
		t.Fatalf("DisplayName: %v", err)
	}
	if got != "Jane Roe" {
		t.Errorf("got %q, want %q", got, "Jane Roe")
	}
}

// The email/password population: the account exists and simply has no
// name. That is a real answer, not an error, and emphatically not a cue
// to invent one from the email.
func TestDisplayNameAbsentIsBlankNotError(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"user_id":"uid-password","display_name":""}}`))
	})

	got, err := l.DisplayName(context.Background(), "uid-password")
	if err != nil {
		t.Fatalf("DisplayName: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want an empty name", got)
	}
}

// A user id with path-hostile characters must not escape the endpoint it
// is interpolated into.
func TestDisplayNameEscapesUserID(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/internal/users/a%2F..%2Fadmin/display-name" {
			t.Errorf("escaped path = %q", r.URL.EscapedPath())
		}
		w.Write([]byte(`{"data":{"display_name":""}}`))
	})

	if _, err := l.DisplayName(context.Background(), "a/../admin"); err != nil {
		t.Fatalf("DisplayName: %v", err)
	}
}

func TestDisplayNameNon200IsError(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"upstream_unavailable"}`))
	})

	if _, err := l.DisplayName(context.Background(), "uid-1"); err == nil {
		t.Fatal("want an error for a 502 from auth-bff")
	}
}

func TestDisplayNameUnparseableBodyIsError(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>not json</html>`))
	})

	if _, err := l.DisplayName(context.Background(), "uid-1"); err == nil {
		t.Fatal("want an error for a non-JSON body")
	}
}

// Local dev with no auth-bff wired: reports ErrNotConfigured rather than
// dialing an empty URL, and the caller turns that into a blank seed.
func TestDisplayNameNotConfigured(t *testing.T) {
	for _, tc := range []struct{ name, baseURL, secret string }{
		{"no base url", "", "s3cret"},
		{"no secret", "http://auth-bff", ""},
		{"neither", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := NewAuthBFFLookup(tc.baseURL, tc.secret, nil)
			if _, err := l.DisplayName(context.Background(), "uid-1"); !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("err = %v, want ErrNotConfigured", err)
			}
		})
	}
}

func TestDisplayNameEmptyUserID(t *testing.T) {
	l := newTestLookup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made for an empty user id")
	})

	if _, err := l.DisplayName(context.Background(), ""); err == nil {
		t.Fatal("want an error for an empty user id")
	}
}

func TestNewAuthBFFLookupTrimsTrailingSlash(t *testing.T) {
	l := NewAuthBFFLookup("http://auth-bff/ ", "s3cret", nil)
	if l.baseURL != "http://auth-bff" {
		t.Errorf("baseURL = %q, want %q", l.baseURL, "http://auth-bff")
	}
}
