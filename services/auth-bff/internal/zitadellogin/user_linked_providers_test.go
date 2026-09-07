package zitadellogin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// routeUser answers the three endpoints UserLinkedProviders may touch:
// the authentication-methods read, the IDP-link search, and the user read
// that supplies the password entry's email.
func routeUser(t *testing.T, methods, links, user string) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/authentication_methods"):
			if r.Method != http.MethodGet {
				t.Errorf("authentication_methods method = %s, want GET", r.Method)
			}
			w.Write([]byte(methods))
		case strings.HasSuffix(r.URL.Path, "/links/_search"):
			if r.Method != http.MethodPost {
				t.Errorf("links/_search method = %s, want POST", r.Method)
			}
			w.Write([]byte(links))
		default:
			w.Write([]byte(user))
		}
	})
}

const (
	userJane   = `{"user":{"human":{"profile":{"givenName":"Jane","familyName":"Roe"},"email":{"email":"jane@example.com"}}}}`
	noLinks    = `{"result":[]}`
	googleLink = `{"result":[{"idpId":"idp-google","userId":"g-1","userName":"jane@example.com"}]}`
)

func TestUserLinkedProvidersPasswordOnly(t *testing.T) {
	c := routeUser(t, `{"authMethodTypes":["PASSWORD"]}`, noLinks, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	want := []LinkedProvider{{ProviderID: "password", Email: "jane@example.com"}}
	assertProviders(t, got, want)
}

// The federated case, and the whole reason a second upstream read exists:
// authentication_methods says only "IDP", so the Google name comes from
// the link search matched against the configured IDP id.
func TestUserLinkedProvidersMapsIDPToGoogle(t *testing.T) {
	c := routeUser(t, `{"authMethodTypes":["PASSWORD","IDP"]}`, googleLink, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{
		{ProviderID: "password", Email: "jane@example.com"},
		{ProviderID: "google.com", Email: "jane@example.com"},
	})
}

func TestUserLinkedProvidersMapsIDPToApple(t *testing.T) {
	links := `{"result":[{"idpId":"idp-apple","userName":"jane@privaterelay.appleid.com"}]}`
	c := routeUser(t, `{"authMethodTypes":["IDP"]}`, links, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{
		{ProviderID: "apple.com", Email: "jane@privaterelay.appleid.com"},
	})
}

// An IDP the deployment has not named still renders, under its raw id.
// Dropping it would tell a customer auditing their account that a live
// sign-in route does not exist.
func TestUserLinkedProvidersUnknownIDPKeepsRawID(t *testing.T) {
	links := `{"result":[{"idpId":"idp-github","userName":"jane"}]}`
	c := routeUser(t, `{"authMethodTypes":["IDP"]}`, links, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{{ProviderID: "idp-github", Email: "jane"}})
}

// Second factors are not sign-in providers. Identity Toolkit never
// reported them and the panel would offer an "Unlink" button for the
// customer's TOTP if they appeared here.
func TestUserLinkedProvidersIgnoresSecondFactors(t *testing.T) {
	c := routeUser(t, `{"authMethodTypes":["PASSWORD","TOTP","U2F","OTP_SMS"]}`, noLinks, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{{ProviderID: "password", Email: "jane@example.com"}})
}

// Zitadel has returned both the bare and the fully-qualified enum names
// across versions; a version bump must not silently empty the list.
func TestUserLinkedProvidersAcceptsQualifiedEnumNames(t *testing.T) {
	methods := `{"authMethodTypes":["AUTHENTICATION_METHOD_TYPE_PASSWORD","AUTHENTICATION_METHOD_TYPE_IDP"]}`
	c := routeUser(t, methods, googleLink, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{
		{ProviderID: "password", Email: "jane@example.com"},
		{ProviderID: "google.com", Email: "jane@example.com"},
	})
}

// A real account with nothing enrolled: an empty list, not an error.
func TestUserLinkedProvidersNoMethodsIsEmpty(t *testing.T) {
	c := routeUser(t, `{"authMethodTypes":[]}`, noLinks, userJane)

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want no providers", got)
	}
}

// A failed email read must not lose the password entry — the label is the
// answer, the address beneath it is decoration.
func TestUserLinkedProvidersKeepsPasswordWhenEmailReadFails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/authentication_methods") {
			w.Write([]byte(`{"authMethodTypes":["PASSWORD"]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	got, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple")
	if err != nil {
		t.Fatalf("UserLinkedProviders: %v", err)
	}
	assertProviders(t, got, []LinkedProvider{{ProviderID: "password", Email: ""}})
}

func TestUserLinkedProvidersUpstreamFailureIsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := c.UserLinkedProviders(context.Background(), "u-1", "idp-google", "idp-apple"); err == nil {
		t.Fatal("want an error when authentication_methods fails")
	}
}

func TestUserLinkedProvidersUnknownUser(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/authentication_methods") {
			w.Write([]byte(`{"authMethodTypes":["IDP"]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":5,"message":"not found"}`))
	})

	if _, err := c.UserLinkedProviders(context.Background(), "u-gone", "idp-google", "idp-apple"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestUserLinkedProvidersEmptyUserID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made for an empty user id")
	})

	if _, err := c.UserLinkedProviders(context.Background(), "", "idp-google", "idp-apple"); err == nil {
		t.Fatal("want an error for an empty user id")
	}
}

func assertProviders(t *testing.T, got, want []LinkedProvider) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("provider %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
