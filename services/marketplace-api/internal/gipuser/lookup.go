// Package gipuser reads Google Identity Platform account records through
// the Firebase Admin SDK.
//
// It exists for one reason: GIP is the only place in the platform that
// holds a merchant's actual name. Federated sign-up (Google) makes GIP
// populate `displayName` on the account record server-side, and mobile
// Apple sign-in writes one explicitly. Nothing propagates that name into
// our own databases, and the handler that creates a user_profiles row
// runs behind header-trust auth with no ID token to read a claim from —
// so the name has to be fetched from GIP directly.
package gipuser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	firebaseAuth "firebase.google.com/go/v4/auth"
)

// DisplayNameLookup resolves the display name GIP holds for an account.
// An account with no name returns ("", nil): a real answer, not an error.
type DisplayNameLookup interface {
	DisplayName(ctx context.Context, uid string) (string, error)
}

// ErrNotConfigured is returned when the lookup was constructed without a
// usable Admin SDK client.
var ErrNotConfigured = errors.New("gipuser: admin sdk client not configured")

// AdminSDKLookup reads account records with the Firebase Admin SDK.
//
// GIP multi-tenancy matters here: merchants live in a tenant pool
// (MP-Internal-*), and a project-level lookup does not see tenant users.
// tenantID must therefore be the GIP tenant id, which is a different
// value from a mark8ly tenant id. Leaving it empty falls back to a
// project-level lookup, which is correct only for non-tenanted accounts.
type AdminSDKLookup struct {
	client   *firebaseAuth.Client
	tenantID string
}

var _ DisplayNameLookup = (*AdminSDKLookup)(nil)

// NewAdminSDKLookup constructs a lookup over an Admin SDK auth client.
func NewAdminSDKLookup(client *firebaseAuth.Client, tenantID string) *AdminSDKLookup {
	return &AdminSDKLookup{client: client, tenantID: strings.TrimSpace(tenantID)}
}

// DisplayName returns the GIP account's display name, trimmed. A missing
// user, an unreachable Identity Toolkit, or a service account without
// firebaseauth.users.get all surface as an error — callers are expected
// to treat that as "no name" rather than a failure.
func (l *AdminSDKLookup) DisplayName(ctx context.Context, uid string) (string, error) {
	if l == nil || l.client == nil {
		return "", ErrNotConfigured
	}
	if uid == "" {
		return "", fmt.Errorf("gipuser: empty uid")
	}

	if l.tenantID == "" {
		rec, err := l.client.GetUser(ctx, uid)
		if err != nil {
			return "", fmt.Errorf("gipuser: get user: %w", err)
		}
		return strings.TrimSpace(rec.DisplayName), nil
	}

	tenantClient, err := l.client.TenantManager.AuthForTenant(l.tenantID)
	if err != nil {
		return "", fmt.Errorf("gipuser: auth for tenant: %w", err)
	}
	rec, err := tenantClient.GetUser(ctx, uid)
	if err != nil {
		return "", fmt.Errorf("gipuser: get tenant user: %w", err)
	}
	return strings.TrimSpace(rec.DisplayName), nil
}

// FakeLookup is a test double for DisplayNameLookup.
type FakeLookup struct {
	// Names maps uid → display name. A uid absent from the map yields
	// ("", nil) — the email/password case, where the account exists but
	// has never had a name.
	Names map[string]string
	// Err, when set, is returned instead of consulting Names.
	Err error
	// Calls counts DisplayName invocations, so tests can prove the
	// lookup is first-seed-only rather than per-request.
	Calls int
}

var _ DisplayNameLookup = (*FakeLookup)(nil)

func (f *FakeLookup) DisplayName(_ context.Context, uid string) (string, error) {
	f.Calls++
	if f.Err != nil {
		return "", f.Err
	}
	return f.Names[uid], nil
}
