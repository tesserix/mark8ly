package zitadellogin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestIDPIDForProviderResolvesOnlyTheProvidersThisHandlerTrusts walks the
// whole switch. The two supported providers must resolve to DIFFERENT ids,
// the empty string must keep meaning Google (the web callers predate the
// provider field and send nothing), an unknown provider must stay a client
// error, and a supported-but-unconfigured provider must stay a server one —
// never a silent fallback to the other provider's id.
func TestIDPIDForProviderResolvesOnlyTheProvidersThisHandlerTrusts(t *testing.T) {
	h := NewHandler(nil, nil).WithGoogleIDPID(testGoogleIDPID).WithAppleIDPID(testAppleIDPID)

	cases := []struct {
		name     string
		provider string
		wantID   string
		wantErr  error
	}{
		{"empty still means google", "", testGoogleIDPID, nil},
		{"google", "google", testGoogleIDPID, nil},
		{"apple", "apple", testAppleIDPID, nil},
		// The switch trims and lowercases, so these must not be a way to
		// slip past the pin by spelling a provider differently.
		{"apple mixed case", "Apple", testAppleIDPID, nil},
		{"apple padded", "  apple  ", testAppleIDPID, nil},
		{"google mixed case", "GOOGLE", testGoogleIDPID, nil},
		{"unknown provider", "github", "", errUnsupportedIDPProvider},
		{"empty-ish unknown provider", "  facebook ", "", errUnsupportedIDPProvider},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := h.idpIDForProvider(tc.provider)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.wantID {
				t.Fatalf("id = %q, want %q", got, tc.wantID)
			}
		})
	}
}

// TestIDPIDForProviderRefusesASupportedButUnconfiguredProvider proves an
// incomplete deployment is a SERVER error rather than a quiet fallback:
// a handler missing one provider's id must refuse that provider outright,
// not resolve it to the other provider's id (which would hand an intent
// from one provider the other's trust).
func TestIDPIDForProviderRefusesASupportedButUnconfiguredProvider(t *testing.T) {
	t.Run("apple unconfigured", func(t *testing.T) {
		h := NewHandler(nil, nil).WithGoogleIDPID(testGoogleIDPID)
		got, err := h.idpIDForProvider("apple")
		if !errors.Is(err, errIDPProviderNotConfigured) {
			t.Fatalf("err = %v, want errIDPProviderNotConfigured", err)
		}
		if got != "" {
			t.Fatalf("id = %q, want empty (never the google id)", got)
		}
	})
	t.Run("google unconfigured", func(t *testing.T) {
		h := NewHandler(nil, nil).WithAppleIDPID(testAppleIDPID)
		for _, provider := range []string{"", "google"} {
			got, err := h.idpIDForProvider(provider)
			if !errors.Is(err, errIDPProviderNotConfigured) {
				t.Fatalf("provider %q: err = %v, want errIDPProviderNotConfigured", provider, err)
			}
			if got != "" {
				t.Fatalf("provider %q: id = %q, want empty (never the apple id)", provider, got)
			}
		}
	})
}

// intentFromIDP serves a single retrieve-intent response carrying idpID and
// fails the test on any other call — a cross-provider intent must be
// refused before ANY lookup, link, or create runs.
func intentFromIDP(t *testing.T, idpID string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/") {
			w.Write([]byte(`{"idpInformation":{"idpId":"` + idpID + `","userId":"ext-1","userName":"victim@merchant.com","rawInformation":{"email":"victim@merchant.com","email_verified":true}}}`))
			return
		}
		t.Errorf("must not look up, link, or create anything for a cross-provider intent: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestIDPFinishRefusesACrossProviderIntentInBothDirections is the security
// property of Apple support, stated explicitly rather than left implied.
//
// Both providers are configured and both are trusted — but only for the
// provider the REQUEST named. An intent opened against Apple and finished
// as "google" (or the reverse) must be refused, because every trust
// decision downstream, `email_verified` above all, means "the named
// provider asserted this". Accepting either direction would let an
// attacker open an intent against whichever provider is easier to register
// victim@merchant.com at and spend it as the other one — an
// account-takeover primitive, not a convenience.
func TestIDPFinishRefusesACrossProviderIntentInBothDirections(t *testing.T) {
	cases := []struct {
		name        string
		intentIDPID string
		provider    string
	}{
		{"apple intent finished as google", testAppleIDPID, "google"},
		{"google intent finished as apple", testGoogleIDPID, "apple"},
		// The empty provider means Google, so an Apple intent must be
		// refused for a legacy web caller too.
		{"apple intent finished by a legacy caller sending no provider", testAppleIDPID, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := intentFromIDP(t, tc.intentIDPID)
			h := NewHandler(New(srv.URL, "pat", srv.Client()), func(_ context.Context, _ http.ResponseWriter, _ LoginContext) (CompleteResult, error) {
				t.Fatal("the gauntlet must not run for a cross-provider intent")
				return CompleteResult{}, nil
			}).WithGoogleIDPID(testGoogleIDPID).WithAppleIDPID(testAppleIDPID).WithOrgID("org-1")

			rec := httptest.NewRecorder()
			h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
				strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1","provider":"`+tc.provider+`"}`)))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestIDPFinishAcceptsAnAppleIntentFinishedAsApple is the other half of the
// pin: refusing everything would satisfy the test above for the wrong
// reason. An Apple intent named as "apple" must actually get past the pin.
func TestIDPFinishAcceptsAnAppleIntentFinishedAsApple(t *testing.T) {
	var reached atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/idp_intents/"):
			// userId present => already-linked identity, so finish takes
			// the shortest path past the pin.
			w.Write([]byte(`{"userId":"linked-user-1","idpInformation":{"idpId":"` + testAppleIDPID + `","userId":"ext-1","userName":"person@icloud.com","rawInformation":{"email":"person@icloud.com","email_verified":"true"}}}`))
		default:
			reached.Store(true)
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	h := NewHandler(New(srv.URL, "pat", srv.Client()), nil).
		WithGoogleIDPID(testGoogleIDPID).WithAppleIDPID(testAppleIDPID).WithOrgID("org-1")

	rec := httptest.NewRecorder()
	h.idpFinish(rec, httptest.NewRequest(http.MethodPost, "/auth/zitadel/idp/finish",
		strings.NewReader(`{"auth_request_id":"V2_1","intent_id":"i1","intent_token":"tok","workspace_tenant":"t1","provider":"apple"}`)))

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("an apple intent named as apple must get past the pin, got 401: %s", rec.Body.String())
	}
	if !reached.Load() {
		t.Fatalf("the pin refused before any session call ran: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
