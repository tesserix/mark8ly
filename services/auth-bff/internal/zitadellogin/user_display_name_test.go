package zitadellogin

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// userJSON builds a v2 GetUser response with the given human profile.
func userJSON(given, family, email string) string {
	return `{"user":{"human":{"profile":{"givenName":"` + given +
		`","familyName":"` + family + `"},"email":{"email":"` + email + `"}}}}`
}

func TestUserDisplayNameReturnsProviderName(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v2/users/u-1" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(userJSON("Jane", "Roe", "jane@example.com")))
	})

	got, err := c.UserDisplayName(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("UserDisplayName: %v", err)
	}
	if got != "Jane Roe" {
		t.Errorf("got %q, want %q", got, "Jane Roe")
	}
}

// TestUserDisplayNameSuppressesPlaceholders is the whole reason this is
// not a read of profile.displayName. Zitadel refuses an empty human name,
// so this package writes the email's local part and "User" when the
// provider sent no name claim (see boundedProfileName). Handing that back
// would seed a merchant's admin profile with a fabricated "alice User".
func TestUserDisplayNameSuppressesPlaceholders(t *testing.T) {
	cases := []struct {
		name                 string
		given, family, email string
		want                 string
	}{
		{"both halves placeholder", "alice", "User", "alice@example.com", ""},
		{"real given, placeholder family", "Alice", "User", "alice@example.com", "Alice"},
		{"placeholder given, real family", "alice", "Roe", "alice@example.com", "Roe"},
		{"local part cased differently is a real name", "Alice", "Roe", "alice@example.com", "Alice Roe"},
		{"dotted local part placeholder", "alice.roe", "User", "alice.roe@example.com", ""},
		{"whitespace only", "  ", "  ", "alice@example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(userJSON(tc.given, tc.family, tc.email)))
			})
			got, err := c.UserDisplayName(context.Background(), "u-1")
			if err != nil {
				t.Fatalf("UserDisplayName: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A machine user has no human profile. Nothing to seed, nothing wrong.
func TestUserDisplayNameNonHumanIsBlankNotError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"user":{"machine":{"name":"svc"}}}`))
	})

	got, err := c.UserDisplayName(context.Background(), "u-machine")
	if err != nil {
		t.Fatalf("UserDisplayName: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestUserDisplayNameUnknownUser(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"not found"}`))
	})

	if _, err := c.UserDisplayName(context.Background(), "u-gone"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUserDisplayNameEmptyUserID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made for an empty user id")
	})

	if _, err := c.UserDisplayName(context.Background(), ""); err == nil {
		t.Fatal("want an error for an empty user id")
	}
}
