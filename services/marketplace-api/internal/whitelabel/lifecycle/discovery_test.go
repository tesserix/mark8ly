package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/appcreds"
	"github.com/mark8ly/marketplace-api/internal/whitelabel/apple"
)

// stubCreds returns a fixed payload (or error) for every credential type
// the discovery path asks for.
type stubCreds struct {
	byType map[appcreds.CredType][]byte
	err    error
	loaded []appcreds.CredType
}

func (s *stubCreds) Load(_ context.Context, in appcreds.LoadInput) ([]byte, error) {
	s.loaded = append(s.loaded, in.CredType)
	if s.err != nil {
		return nil, s.err
	}
	payload, ok := s.byType[in.CredType]
	if !ok {
		return nil, appcreds.ErrNotFound
	}
	return payload, nil
}

const serviceAccountJSON = `{
  "type": "service_account",
  "project_id": "merchant-app-42",
  "private_key": "-----BEGIN PRIVATE KEY-----\nx\n-----END PRIVATE KEY-----\n",
  "client_email": "play@merchant-app-42.iam.gserviceaccount.com"
}`

func discoveryWith(apps []apple.App, listErr error, creds CredentialLoader) *Discovery {
	fake := apple.NewFakeClient()
	fake.Apps = apps
	fake.ListAppsErr = listErr
	return &Discovery{
		Apple: func(context.Context, uuid.UUID, uuid.UUID) (AppleLister, error) {
			return fake, nil
		},
		Creds: creds,
	}
}

func TestDiscover_ExactlyOneApp_Succeeds(t *testing.T) {
	d := discoveryWith(
		[]apple.App{{ID: "6448000111", BundleID: "com.merchant.shop", Name: "Shop"}},
		nil,
		&stubCreds{byType: map[appcreds.CredType][]byte{
			appcreds.CredTypeGooglePlayJSON: []byte(serviceAccountJSON),
		}},
	)

	tenantID, storeID := uuid.New(), uuid.New()
	ev, err := d.discover(context.Background(), tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, "6448000111", ev.AppleAppID)
	require.Equal(t, tenantID, ev.TenantID)
	require.Equal(t, storeID, ev.StoreID)
	// Decision 5: FirebaseProjectID is the service account's project_id.
	require.Equal(t, "merchant-app-42", ev.FirebaseProjectID)
	// Decision 4: no source for a Play package, so none is invented.
	require.Empty(t, ev.GooglePackage)
}

// Zero apps is a conclusion ("this account publishes nothing we can
// retire"), not a licence to seed an empty row.
func TestDiscover_ZeroApps_Refuses(t *testing.T) {
	d := discoveryWith(nil, nil, &stubCreds{})

	_, err := d.discover(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrNoAppleApp)
	require.NotErrorIs(t, err, ErrAmbiguousAppleApp, "zero and many must be distinct errors")
}

// Picking one of several would retire a listing belonging to a merchant
// who did not cancel.
func TestDiscover_MultipleApps_Refuses(t *testing.T) {
	d := discoveryWith([]apple.App{
		{ID: "111", BundleID: "com.merchant.shop"},
		{ID: "222", BundleID: "com.merchant.other"},
	}, nil, &stubCreds{})

	_, err := d.discover(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrAmbiguousAppleApp)
	require.NotErrorIs(t, err, ErrNoAppleApp, "many and zero must be distinct errors")
	// The refusal must name what it saw, or an operator cannot act on it.
	require.Contains(t, err.Error(), "111")
	require.Contains(t, err.Error(), "222")
}

// A rejected key is "we do not know what exists", which must not be
// reported as either of the two conclusions.
func TestDiscover_ListAppsError_IsNeitherRefusal(t *testing.T) {
	d := discoveryWith(nil, apple.ErrUnauthorized, &stubCreds{})

	_, err := d.discover(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, apple.ErrUnauthorized)
	require.NotErrorIs(t, err, ErrNoAppleApp)
	require.NotErrorIs(t, err, ErrAmbiguousAppleApp)
}

// Firebase is best-effort: a missing Play service account narrows the
// teardown, it does not invalidate the Apple half.
func TestDiscover_MissingPlayCredentials_StillDiscoversApple(t *testing.T) {
	d := discoveryWith([]apple.App{{ID: "999"}}, nil, &stubCreds{err: appcreds.ErrNotFound})

	ev, err := d.discover(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	require.Equal(t, "999", ev.AppleAppID)
	require.Empty(t, ev.FirebaseProjectID)
}

func TestDiscover_UnparseablePlayCredentials_StillDiscoversApple(t *testing.T) {
	d := discoveryWith([]apple.App{{ID: "999"}}, nil, &stubCreds{
		byType: map[appcreds.CredType][]byte{
			appcreds.CredTypeGooglePlayJSON: []byte(`{"type":"authorized_user"}`),
		},
	})

	ev, err := d.discover(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	require.Empty(t, ev.FirebaseProjectID)
}

func TestProAppCancelled_WithoutDiscovery_Refuses(t *testing.T) {
	c := NewProAppCancelledConsumer(nil, nil)
	err := c.ProAppCancelled(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrDiscoveryNotConfigured)
}

// A discovery refusal must surface to the caller (the finalize cron
// logs it) and must not reach the database.
func TestProAppCancelled_DiscoveryRefusal_NeverSeeds(t *testing.T) {
	// db is nil: any attempt to seed would panic or error rather than
	// silently succeed.
	c := NewProAppCancelledConsumer(nil, nil).
		WithDiscovery(discoveryWith(nil, nil, &stubCreds{}))

	err := c.ProAppCancelled(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrNoAppleApp)
}

func TestNewAppleListerFactory_PropagatesCredentialErrors(t *testing.T) {
	boom := errors.New("secret manager unavailable")
	factory := NewAppleListerFactory(&stubCreds{err: boom})

	lister, err := factory(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err, "the client is built lazily; credentials are fetched per call")
	_, err = lister.ListApps(context.Background())
	require.ErrorIs(t, err, boom)
}

// ─── Coverage note ───────────────────────────────────────────────────

// With GooglePackage empty by design, the advancer's `if
// r.GooglePackage != ""` guards mean Play is never attempted, never
// errors and never logs. The row must therefore say so itself — and state
// BOTH reasons, because they are of different kinds: the missing package
// identifier is a decision that could be revisited, while the absent
// unpublish API cannot be. This assertion used to require the word
// "stub", which the wired client made false; the note now has to name
// the two real causes instead.
func TestTeardownCoverage_StatesPlayIsNotAttempted(t *testing.T) {
	note := teardownCoverage(ProAppCancelledEvent{
		AppleAppID:        "6448000111",
		FirebaseProjectID: "merchant-app-42",
	})

	require.Contains(t, note, "google_play=NOT_ATTEMPTED")
	require.Contains(t, strings.ToLower(note), "no package identifier")
	require.Contains(t, strings.ToLower(note), "no android publisher api",
		"the permanent half of the reason must be stated, not just the missing identifier")
	require.Contains(t, strings.ToLower(note), "play console",
		"the note must name the manual action, since no code can perform it")
	require.NotContains(t, strings.ToLower(note), "errnotwired",
		"the Play client is wired; a note claiming otherwise is the defect #702 exists to remove")
	require.Contains(t, note, "apple=will_attempt(app_id=6448000111)")
	require.Contains(t, note, "merchant-app-42")
	// A placeholder package would make the guard fire while asserting an
	// identifier nobody has — decision 4 exists to prevent that.
	require.NotContains(t, note, "google_play=will_attempt")
}

func TestTeardownCoverage_NamesPlayPackageWhenOneExists(t *testing.T) {
	note := teardownCoverage(ProAppCancelledEvent{
		AppleAppID:    "1",
		GooglePackage: "com.merchant.shop",
	})
	require.Contains(t, note, "google_play=will_attempt(package=com.merchant.shop")
	require.NotContains(t, note, "NOT_ATTEMPTED")
	// "will_attempt" must not read as "the listing will come down": day 30
	// is all that is attemptable.
	require.Contains(t, strings.ToLower(note), "day-30 download halt only")
	require.Contains(t, strings.ToLower(note), "play console")
}

// GooglePackage — the merchant-supplied Android applicationId (#872).
//
// Before #872 this was empty by design, so these tests are the first
// coverage that the day-30 Play halt can ever become reachable.

func TestDiscover_GooglePackage_WhenSupplied(t *testing.T) {
	creds := &stubCreds{byType: map[appcreds.CredType][]byte{
		appcreds.CredTypeGooglePlayJSON:    []byte(serviceAccountJSON),
		appcreds.CredTypeGooglePackageName: []byte("com.example.storefront"),
	}}
	ev, err := discoveryWith([]apple.App{{ID: "123"}}, nil, creds).
		discover(context.Background(), uuid.New(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, "com.example.storefront", ev.GooglePackage)
}

func TestDiscover_GooglePackage_AbsentIsNotAnError(t *testing.T) {
	// The majority case, and it must stay a first-class state: the upload
	// sits behind an active Pro+App subscription, so every merchant
	// onboarded before #872 has none. Apple's teardown is the larger half
	// and must not be lost with it.
	creds := &stubCreds{byType: map[appcreds.CredType][]byte{
		appcreds.CredTypeGooglePlayJSON: []byte(serviceAccountJSON),
	}}
	ev, err := discoveryWith([]apple.App{{ID: "123"}}, nil, creds).
		discover(context.Background(), uuid.New(), uuid.New())

	require.NoError(t, err)
	require.Empty(t, ev.GooglePackage)
	require.Equal(t, "123", ev.AppleAppID, "apple teardown must survive an absent package")
}

func TestDiscover_GooglePackage_MalformedIsTreatedAsAbsent(t *testing.T) {
	// The value is a Secret Manager payload that an older image or a
	// hand-edit during an incident could have written. A malformed one
	// reaching GooglePackage would pass the advancer's `!= ""` guard and
	// fail against Play inside the nightly cron — months later, visible
	// only as a skipped step. Re-validating on read is what stops that,
	// and dropping to empty is strictly better than seeding a lie.
	creds := &stubCreds{byType: map[appcreds.CredType][]byte{
		appcreds.CredTypeGooglePlayJSON:    []byte(serviceAccountJSON),
		appcreds.CredTypeGooglePackageName: []byte("not a package name"),
	}}
	ev, err := discoveryWith([]apple.App{{ID: "123"}}, nil, creds).
		discover(context.Background(), uuid.New(), uuid.New())

	require.NoError(t, err)
	require.Empty(t, ev.GooglePackage)
}

func TestDiscover_GooglePackage_IsTrimmed(t *testing.T) {
	// Package names are copy-pasted out of build files, and a trailing
	// newline is the usual artefact. It is rejected at upload, but a value
	// written before #872's validator existed would still carry one.
	creds := &stubCreds{byType: map[appcreds.CredType][]byte{
		appcreds.CredTypeGooglePlayJSON:    []byte(serviceAccountJSON),
		appcreds.CredTypeGooglePackageName: []byte("  com.example.app\n"),
	}}
	ev, err := discoveryWith([]apple.App{{ID: "123"}}, nil, creds).
		discover(context.Background(), uuid.New(), uuid.New())

	require.NoError(t, err)
	require.Equal(t, "com.example.app", ev.GooglePackage)
}
