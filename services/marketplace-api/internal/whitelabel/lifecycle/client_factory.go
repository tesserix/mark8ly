package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
)

// teardownActor is the audit actor recorded against every credential read
// the ADVANCER performs, distinct from discoveryActor so the day-30/60
// writes are separable in the credential-access audit stream from the
// day-of-cancellation discovery reads (#702).
const teardownActor = "system:cron:lifecycle_teardown"

// AppleTeardownClient is the write slice of apple.ClientAPI the advancer
// needs, at day 30 and day 60. Narrow for the same reason AppleLister is
// narrow in the other direction: discovery may only read, teardown may
// only write, and neither can reach the other's verbs by accident.
type AppleTeardownClient interface {
	BlockDownloads(ctx context.Context, appleAppID string) error
	PullApp(ctx context.Context, appleAppID string) error
}

// AppleTeardownFactory returns a teardown client authenticated as one
// tenant.
//
// PER ROW, not per process. apple.Client's CredsFetcher is
// func(ctx) (Credentials, error) — it takes no tenant — so a constructed
// client carries exactly one merchant's App Store Connect key. The
// advancer walks a cohort spanning many tenants, so one shared client
// cannot serve it: it would sign every merchant's teardown with whichever
// key it happened to be built with, and either fail or, worse, act on
// somebody else's account.
type AppleTeardownFactory func(ctx context.Context, tenantID, storeID uuid.UUID) (AppleTeardownClient, error)

// GoogleTeardownFactory is the Play equivalent. Play has the same
// structural constraint as Apple — googleplay.Client's CredsFetcher is
// also tenant-less — so "wire the real client instead of the fake" is not
// a one-line swap on this side either.
//
// The real client returns googleplay.ErrNotWired from every method, after
// fetching credentials. That is what production should see: an honest
// "not implemented", rather than googleplay.FakeClient's silent success.
type GoogleTeardownFactory func(ctx context.Context, tenantID, storeID uuid.UUID) (googleplay.ClientAPI, error)

// NewAppleTeardownFactory builds a real App Store Connect client per
// tenant from that tenant's stored .p8 key, issuer id and key id.
func NewAppleTeardownFactory(creds CredentialLoader) AppleTeardownFactory {
	return func(_ context.Context, tenantID, storeID uuid.UUID) (AppleTeardownClient, error) {
		client, err := newAppleClient(creds, tenantID, storeID, teardownActor)
		if err != nil {
			// Untyped nil: a typed-nil *apple.Client behind a non-nil
			// interface would pass a `!= nil` check and panic later
			// (#288's shape).
			return nil, err
		}
		return client, nil
	}
}

// NewGoogleTeardownFactory builds a real Android Publisher client per
// tenant from that tenant's stored service-account JSON.
func NewGoogleTeardownFactory(creds CredentialLoader) GoogleTeardownFactory {
	return func(_ context.Context, tenantID, storeID uuid.UUID) (googleplay.ClientAPI, error) {
		if creds == nil {
			return nil, errors.New("lifecycle: appcreds service is nil")
		}
		client, err := googleplay.New(googleplay.Config{
			CredsFetcher: func(ctx context.Context) (googleplay.Credentials, error) {
				payload, err := creds.Load(ctx, appcreds.LoadInput{
					TenantID: tenantID,
					StoreID:  storeID,
					CredType: appcreds.CredTypeGooglePlayJSON,
					Actor:    teardownActor,
				})
				if err != nil {
					return googleplay.Credentials{}, fmt.Errorf("load %s: %w", appcreds.CredTypeGooglePlayJSON, err)
				}
				return googleplay.Credentials{ServiceAccountJSON: payload}, nil
			},
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
}

// newAppleClient is the single place an authenticated *apple.Client is
// built, shared by discovery's read-only factory and the advancer's
// teardown factory so the two cannot drift in how they load credentials.
//
// Credentials are fetched inside the CredsFetcher closure — per API call,
// not once at construction — so a key rotated or revoked between the
// factory call and the request is picked up, and so every read goes
// through appcreds' audit + metrics choke point. actor distinguishes
// which caller asked.
func newAppleClient(creds CredentialLoader, tenantID, storeID uuid.UUID, actor string) (*apple.Client, error) {
	if creds == nil {
		return nil, errors.New("lifecycle: appcreds service is nil")
	}
	return apple.New(apple.Config{
		CredsFetcher: func(ctx context.Context) (apple.Credentials, error) {
			load := func(ct appcreds.CredType) ([]byte, error) {
				return creds.Load(ctx, appcreds.LoadInput{
					TenantID: tenantID,
					StoreID:  storeID,
					CredType: ct,
					Actor:    actor,
				})
			}
			p8, err := load(appcreds.CredTypeAppleP8)
			if err != nil {
				return apple.Credentials{}, fmt.Errorf("load %s: %w", appcreds.CredTypeAppleP8, err)
			}
			issuer, err := load(appcreds.CredTypeAppleIssuerID)
			if err != nil {
				return apple.Credentials{}, fmt.Errorf("load %s: %w", appcreds.CredTypeAppleIssuerID, err)
			}
			keyID, err := load(appcreds.CredTypeAppleKeyID)
			if err != nil {
				return apple.Credentials{}, fmt.Errorf("load %s: %w", appcreds.CredTypeAppleKeyID, err)
			}
			return apple.Credentials{
				P8:       p8,
				IssuerID: strings.TrimSpace(string(issuer)),
				KeyID:    strings.TrimSpace(string(keyID)),
			}, nil
		},
	})
}
