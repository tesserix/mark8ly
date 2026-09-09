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

	// GooglePackage comes from the merchant, via the optional
	// `package_name` field on the Play credential upload (#872). It is
	// still frequently EMPTY and that stays a first-class state: the
	// field is optional because the upload sits behind an active
	// Pro+App subscription, so no earlier point could have required it,
	// and every merchant onboarded before #872 has none.
	//
	// The original reasoning holds for the empty case and is why nothing
	// substitutes a placeholder here: the advancer guards both Google
	// calls on `r.GooglePackage != ""`, so a fabricated value would seed
	// a row claiming an identifier nobody has. The absence is stated in
	// words instead — see teardownCoverage.
	ev := ProAppCancelledEvent{
		TenantID:          tenantID,
		StoreID:           storeID,
		AppleAppID:        appleID,
		GooglePackage:     d.discoverGooglePackage(ctx, tenantID, storeID, log),
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
// discoverGooglePackage reads the merchant-supplied Android applicationId.
//
// Best-effort, exactly like discoverFirebaseProjectID beside it: an absent
// or unreadable package narrows what the teardown can do but does not make
// the row a lie, and teardownCoverage renders the narrowing. Failing the
// whole discovery here would cost the merchant their APPLE teardown too,
// which is the larger half and entirely unrelated.
//
// Re-validated on read rather than trusted from the write. The value is
// stored as a Secret Manager payload and may have been written by an older
// image, or by hand during an incident; a malformed one reaching
// GooglePackage would pass the advancer's non-empty guard and fail against
// Play inside a nightly cron. Cheap check, and the only other place a human
// could notice is months later in a skipped-step counter.
func (d *Discovery) discoverGooglePackage(ctx context.Context, tenantID, storeID uuid.UUID, log *slog.Logger) string {
	if d.Creds == nil {
		log.WarnContext(ctx, "lifecycle/discovery: no credential loader; google package not discovered",
			"store_id", storeID)
		return ""
	}
	payload, err := d.Creds.Load(ctx, appcreds.LoadInput{
		TenantID: tenantID,
		StoreID:  storeID,
		CredType: appcreds.CredTypeGooglePackageName,
		Actor:    discoveryActor,
	})
	if err != nil {
		// Expected for every merchant who never supplied one, so this is
		// Info rather than Warn: at Warn it would fire on the majority of
		// rows and train readers to skip it.
		log.InfoContext(ctx, "lifecycle/discovery: no google package name on file",
			"store_id", storeID, "err", err)
		return ""
	}
	name := strings.TrimSpace(string(payload))
	if err := appcreds.ValidateGooglePackageName(name); err != nil {
		log.WarnContext(ctx, "lifecycle/discovery: stored google package name is not valid; treating as absent",
			"store_id", storeID, "err", err)
		return ""
	}
	return name
}

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
// The client itself is built by newAppleClient (client_factory.go), which
// the advancer's teardown factory shares, so the read path and the write
// path cannot drift in how they load credentials. Only the audit actor
// differs — discoveryActor here, teardownActor there.
func NewAppleListerFactory(creds CredentialLoader) AppleListerFactory {
	return func(_ context.Context, tenantID, storeID uuid.UUID) (AppleLister, error) {
		client, err := newAppleClient(creds, tenantID, storeID, discoveryActor)
		if err != nil {
			// Return an untyped nil: a typed-nil *Client behind a
			// non-nil AppleLister interface would pass a `!= nil`
			// check and panic later (#288's shape).
			return nil, err
		}
		return client, nil
	}
}
