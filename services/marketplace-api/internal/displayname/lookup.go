// Package displayname resolves the human name the identity provider
// holds for a merchant, so a freshly created user_profiles row seeds with
// the name they already gave Google or Apple instead of a blank.
//
// It exists for one reason: nothing downstream of sign-up writes a
// person's name into our own databases, and the handler that creates the
// profile row runs behind header-trust auth — there is no bearer token on
// that request to read a `name` claim from, so the name has to be asked
// for.
//
// The ask goes through auth-bff, not to Zitadel directly. auth-bff is the
// only service that holds the Zitadel login-client PAT, and that is
// deliberate: it is an instance-level credential that can mint a session
// for any user of any product on the shared instance. Giving
// marketplace-api its own copy to read one profile field would widen that
// blast radius for no gain, so this package speaks the same
// X-Internal-Auth service-to-service protocol AccountHandler already uses
// for the MFA, session, and profile-reset cascades.
//
// Seeding is first-create only, never a per-request refresh. Both IDPs are
// registered with isAutoUpdate:false on purpose — Apple returns a name
// only on the very first authorization, and Google would otherwise
// overwrite a name the merchant has since edited at every sign-in.
package displayname

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Lookup resolves the display name the identity provider holds for a user.
//
// An account with no name a person actually supplied returns ("", nil): a
// real answer, not an error. Every email/password merchant is that case.
type Lookup interface {
	DisplayName(ctx context.Context, userID string) (string, error)
}

// ErrNotConfigured is returned when the lookup was built without an
// auth-bff URL or internal secret — local dev, mostly.
var ErrNotConfigured = errors.New("displayname: auth-bff is not configured")

// maxResponseBytes bounds the JSON we are willing to read back. The reply
// is one short object; anything larger is a misrouted response, not a name.
const maxResponseBytes = 4 << 10

// defaultTimeout caps a single lookup. Callers apply their own deadline
// too; this is the floor for a client built without one.
const defaultTimeout = 5 * time.Second

// AuthBFFLookup reads the name over auth-bff's internal user surface.
type AuthBFFLookup struct {
	baseURL string
	secret  string
	hc      *http.Client
}

var _ Lookup = (*AuthBFFLookup)(nil)

// NewAuthBFFLookup constructs a lookup over auth-bff's
// GET /internal/users/:id/display-name. baseURL and secret are
// AUTH_BFF_URL and MARKETPLACE_INTERNAL_AUTH_SECRET; either being empty
// yields a lookup that always reports ErrNotConfigured, which callers
// treat as "no name". hc may be nil.
func NewAuthBFFLookup(baseURL, secret string, hc *http.Client) *AuthBFFLookup {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &AuthBFFLookup{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		secret:  strings.TrimSpace(secret),
		hc:      hc,
	}
}

// DisplayName returns the name auth-bff resolved, trimmed. An unreachable
// auth-bff, a non-200 answer, or an unparseable body all surface as an
// error — callers are expected to treat that as "no name" rather than as
// a failure, because a profile must still be creatable when the identity
// provider is having a bad day.
func (l *AuthBFFLookup) DisplayName(ctx context.Context, userID string) (string, error) {
	if l == nil || l.baseURL == "" || l.secret == "" {
		return "", ErrNotConfigured
	}
	if userID == "" {
		return "", fmt.Errorf("displayname: empty user id")
	}

	endpoint := fmt.Sprintf("%s/internal/users/%s/display-name", l.baseURL, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("displayname: build request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", l.secret)

	resp, err := l.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("displayname: call auth-bff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The status alone. auth-bff's error bodies are safe, but there is
		// no reason to widen what a failed name lookup can put in a log.
		return "", fmt.Errorf("displayname: auth-bff returned %d", resp.StatusCode)
	}

	var wire struct {
		Data struct {
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&wire); err != nil {
		return "", fmt.Errorf("displayname: decode response: %w", err)
	}
	return strings.TrimSpace(wire.Data.DisplayName), nil
}

// FakeLookup is a test double for Lookup.
type FakeLookup struct {
	// Names maps user id → display name. A user id absent from the map
	// yields ("", nil) — the email/password case, where the account exists
	// but has never had a name.
	Names map[string]string
	// Err, when set, is returned instead of consulting Names.
	Err error
	// Calls counts DisplayName invocations, so tests can prove the lookup
	// is first-seed-only rather than per-request.
	Calls int
}

var _ Lookup = (*FakeLookup)(nil)

func (f *FakeLookup) DisplayName(_ context.Context, userID string) (string, error) {
	f.Calls++
	if f.Err != nil {
		return "", f.Err
	}
	return f.Names[userID], nil
}
