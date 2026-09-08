package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
)

// Discovery resolves the external identifiers a teardown needs from the
// credentials already stored for a tenant (#702, decision 1: discover,
// do not record — there is no provisioning flow that writes them down).
//
// It runs at CANCEL time rather than lazily at day 30/60/90 (decision 2).
// The advancer purges every credential type at day 90, so a row that
// reaches credentials_purged without having discovered anything has
// permanently lost the ability to: the ASC key it would have asked with
// is gone. Discovering early costs one read; discovering late is
// impossible.
type Discovery struct {
	// Apple builds a lister bound to one tenant's stored App Store
	// Connect credentials. Per-tenant because ASC credentials are
	// per-tenant: there is no single client that can answer for
	// everybody.
	Apple AppleListerFactory

	// Creds reads the Google Play service-account JSON, whose
	// project_id is the only source we have for FirebaseProjectID.
	Creds CredentialLoader

	Logger *slog.Logger
}

// AppleLister is the read-only slice of apple.ClientAPI that discovery
// needs. Narrow on purpose: discovery must not be able to call
// BlockDownloads or PullApp — those belong to the advancer, at day 30
// and day 60, not to a cancellation handler.
type AppleLister interface {
	ListApps(ctx context.Context) ([]apple.App, error)
}

// AppleListerFactory returns a lister authenticated as the given tenant.
type AppleListerFactory func(ctx context.Context, tenantID, storeID uuid.UUID) (AppleLister, error)

// CredentialLoader is the read-only slice of *appcreds.Service that
// discovery needs. *appcreds.Service satisfies it.
type CredentialLoader interface {
	Load(ctx context.Context, in appcreds.LoadInput) ([]byte, error)
}

// discoveryActor is the audit actor recorded against every credential
// read discovery performs, so the day-of-cancellation reads are
// distinguishable in the credential-access audit stream from the
// advancer's day-90 purge.
const discoveryActor = "system:cron:pro_app_cancelled_discovery"

// ErrNoAppleApp reports that the store's ASC credentials returned zero
// apps. Distinct from ErrAmbiguousAppleApp and from apple.ErrUnauthorized
// on purpose: "the account has no apps", "the account has several" and
// "the key was rejected so we do not know" are three different facts and
// only the first two are conclusions about the merchant.
var ErrNoAppleApp = errors.New("lifecycle/discovery: App Store Connect returned no apps for this store's credentials")

// ErrAmbiguousAppleApp reports that the credentials see more than one
// app, so there is no single listing this cancellation identifies.
//
// This REFUSES rather than picking one (decision 3). Guessing is not a
// smaller version of the same action: pulling the wrong app id retires a
// listing belonging to a merchant who did not cancel, which is a worse
// outcome than retiring nothing and saying so.
var ErrAmbiguousAppleApp = errors.New("lifecycle/discovery: App Store Connect returned more than one app for this store's credentials")

// ErrDiscoveryNotConfigured reports that ProAppCancelled was called on a
// consumer with no Discovery attached. Returned rather than silently
// seeding an empty row.
var ErrDiscoveryNotConfigured = errors.New("lifecycle/consumer: no Discovery configured; call WithDiscovery before ProAppCancelled")

// discover resolves the identifiers for one store.
//
// Apple is required: without an app id there is nothing this teardown can
// retire, and a row seeded without one would walk the state machine to
// credentials_purged having touched nothing (see ErrNoAppIdentifiers).
// Firebase is best-effort — its absence narrows what gets torn down but
// does not make the row a lie, and the Apple id alone is a real teardown.
func (d *Discovery) discover(ctx context.Context, tenantID, storeID uuid.UUID) (ProAppCancelledEvent, error) {
	log := d.logger()

	appleID, err := d.discoverAppleAppID(ctx, tenantID, storeID)
	if err != nil {
		return ProAppCancelledEvent{}, err
	}

	// GooglePackage is deliberately left empty (decision 4): there is no
	// source for it, and inventing a placeholder to satisfy the
	// advancer's `if r.GooglePackage != ""` guards would seed a row that
	// claims an identifier it does not have. The row's Play coverage is
	// stated explicitly instead — see teardownCoverage.
	ev := ProAppCancelledEvent{
		TenantID:          tenantID,
		StoreID:           storeID,
		AppleAppID:        appleID,
		FirebaseProjectID: d.discoverFirebaseProjectID(ctx, tenantID, storeID, log),
	}
	return ev, nil
}

func (d *Discovery) discoverAppleAppID(ctx context.Context, tenantID, storeID uuid.UUID) (string, error) {
	if d.Apple == nil {
		return "", errors.New("lifecycle/discovery: no Apple lister factory configured")
	}
	lister, err := d.Apple(ctx, tenantID, storeID)
	if err != nil {
		return "", fmt.Errorf("lifecycle/discovery: apple client for store %s: %w", storeID, err)
	}
	apps, err := lister.ListApps(ctx)
	if err != nil {
		return "", fmt.Errorf("lifecycle/discovery: list apps for store %s: %w", storeID, err)
	}
	switch len(apps) {
	case 1:
		return apps[0].ID, nil
	case 0:
		return "", fmt.Errorf("%w (store %s)", ErrNoAppleApp, storeID)
	default:
		return "", fmt.Errorf("%w (store %s saw %d: %s)",
			ErrAmbiguousAppleApp, storeID, len(apps), describeApps(apps))
	}
}

// discoverFirebaseProjectID reads the Play service-account JSON and
// returns its project_id. Failures are logged and yield "" rather than
// failing the whole discovery: a missing Firebase project makes the
// teardown narrower, not wrong, and the row states the narrowing.
func (d *Discovery) discoverFirebaseProjectID(ctx context.Context, tenantID, storeID uuid.UUID, log *slog.Logger) string {
	if d.Creds == nil {
		log.WarnContext(ctx, "lifecycle/discovery: no credential loader; firebase project id not discovered",
			"store_id", storeID)
		return ""
	}
	payload, err := d.Creds.Load(ctx, appcreds.LoadInput{
		TenantID: tenantID,
		StoreID:  storeID,
		CredType: appcreds.CredTypeGooglePlayJSON,
		Actor:    discoveryActor,
	})
	if err != nil {
		log.WarnContext(ctx, "lifecycle/discovery: google play service account unavailable; firebase project id not discovered",
			"store_id", storeID, "err", err)
		return ""
	}
	// The project_id is the GCP project owning the service account —
	// usually, but not necessarily, the Firebase project backing the app.
	// See appcreds.GooglePlayProjectID for why that is an inference.
	projectID, err := appcreds.GooglePlayProjectID(payload)
	if err != nil {
		log.WarnContext(ctx, "lifecycle/discovery: google play service account is not parseable; firebase project id not discovered",
			"store_id", storeID, "err", err)
		return ""
	}
	return projectID
}

func (d *Discovery) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// describeApps renders the ambiguous set for the refusal message. An
// opaque numeric app id alone does not tell an operator which listings
// the credentials saw, and that is the question they will have.
func describeApps(apps []apple.App) string {
	parts := make([]string, 0, len(apps))
	for _, a := range apps {
		parts = append(parts, fmt.Sprintf("%s(%s)", a.ID, a.BundleID))
	}
	return strings.Join(parts, ", ")
}

// NewAppleListerFactory returns a factory that builds a real App Store
// Connect client per tenant, authenticated with that tenant's stored .p8
// key, issuer id and key id.
//
// Credentials are fetched inside the CredsFetcher closure — that is, per
// API call, not once at factory time — so a key rotated or revoked
// between the factory call and the request is picked up, and so every
// read goes through appcreds' audit + metrics choke point.
func NewAppleListerFactory(creds CredentialLoader) AppleListerFactory {
	return func(_ context.Context, tenantID, storeID uuid.UUID) (AppleLister, error) {
		if creds == nil {
			return nil, errors.New("lifecycle/discovery: appcreds service is nil")
		}
		client, err := apple.New(apple.Config{
			CredsFetcher: func(ctx context.Context) (apple.Credentials, error) {
				load := func(ct appcreds.CredType) ([]byte, error) {
					return creds.Load(ctx, appcreds.LoadInput{
						TenantID: tenantID,
						StoreID:  storeID,
						CredType: ct,
						Actor:    discoveryActor,
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
		if err != nil {
			// Return an untyped nil: a typed-nil *Client behind a
			// non-nil AppleLister interface would pass a `!= nil`
			// check and panic later (#288's shape).
			return nil, err
		}
		return client, nil
	}
}
