package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// fakeMobileVerifier is a trivial auth.TokenVerifier double distinguishable
// by identity (via its Name field), so tests can assert WHICH verifier
// selectMobileTokenVerifier returned without depending on GIP or Zitadel
// internals.
type fakeMobileVerifier struct {
	Name string
}

func (f *fakeMobileVerifier) Verify(context.Context, string) (*auth.TokenClaims, error) {
	return nil, errors.New("fakeMobileVerifier: Verify not implemented")
}

// TestSelectMobileTokenVerifier_FlagUnset_SelectsGIP is the "nothing
// changes without opting in" guarantee: with ZitadelEnabled false (the
// config default), the Zitadel factory must never even be called, and the
// GIP factory's result is returned unchanged.
func TestSelectMobileTokenVerifier_FlagUnset_SelectsGIP(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false}
	log, _ := captureLogger()
	gipVerifier := &fakeMobileVerifier{Name: "gip"}
	zitadelCalled := false

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			zitadelCalled = true
			return nil, errors.New("must not be called when Zitadel is disabled")
		},
		func() (auth.TokenVerifier, error) {
			return gipVerifier, nil
		},
	)

	require.False(t, zitadelCalled, "Zitadel factory must not run when ZITADEL_ENABLED is false")
	require.Same(t, auth.TokenVerifier(gipVerifier), got)
}

// TestSelectMobileTokenVerifier_FlagUnset_NoGIPConfig_DisablesSilently
// preserves the pre-existing byte-identical behaviour for an
// unconfigured dev environment: no GIP project id means newGIP returns
// (nil, nil) — not an error — and nothing is logged.
func TestSelectMobileTokenVerifier_FlagUnset_NoGIPConfig_DisablesSilently(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false}
	log, capture := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			t.Fatal("Zitadel factory must not run when ZITADEL_ENABLED is false")
			return nil, nil
		},
		func() (auth.TokenVerifier, error) {
			return nil, nil
		},
	)

	require.Nil(t, got)
	require.Empty(t, capture.all(), "an unconfigured GIP provider must disable silently, matching today's behaviour")
}

// TestSelectMobileTokenVerifier_FlagSet_SelectsZitadel proves the flag
// actually switches providers, and that the configured issuer + audience
// (never constants) are the values handed to the factory.
func TestSelectMobileTokenVerifier_FlagSet_SelectsZitadel(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled:        true,
		ZitadelIssuer:         "https://auth.tesserix.app",
		ZitadelAdminProjectID: "389070376568619523",
	}
	log, _ := captureLogger()
	zitadelVerifier := &fakeMobileVerifier{Name: "zitadel"}
	gipCalled := false
	var gotIssuer, gotAudience string

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(_ context.Context, issuer, audience string) (auth.TokenVerifier, error) {
			gotIssuer = issuer
			gotAudience = audience
			return zitadelVerifier, nil
		},
		func() (auth.TokenVerifier, error) {
			gipCalled = true
			return nil, errors.New("must not be called when Zitadel is selected")
		},
	)

	require.False(t, gipCalled, "GIP factory must not run when ZITADEL_ENABLED is true")
	require.Same(t, auth.TokenVerifier(zitadelVerifier), got)
	require.Equal(t, "https://auth.tesserix.app", gotIssuer)
	require.Equal(t, "389070376568619523", gotAudience)
}

// TestSelectMobileTokenVerifier_ZitadelConstructionFails_DisablesWithoutFallback
// is the "fails clearly, never silently falls back to GIP, never
// half-mounts" requirement for a runtime construction failure (e.g. the
// issuer's discovery endpoint unreachable at boot) that config-time
// validation (Config.ValidateZitadel, checked unconditionally in
// config.Load) cannot catch ahead of time.
func TestSelectMobileTokenVerifier_ZitadelConstructionFails_DisablesWithoutFallback(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled:        true,
		ZitadelIssuer:         "https://auth.tesserix.app",
		ZitadelAdminProjectID: "389070376568619523",
	}
	log, capture := captureLogger()
	gipCalled := false

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return nil, errors.New("discover https://auth.tesserix.app: connection refused")
		},
		func() (auth.TokenVerifier, error) {
			gipCalled = true
			return &fakeMobileVerifier{Name: "gip"}, nil
		},
	)

	require.Nil(t, got, "a construction failure must disable mobile admin routes (nil verifier), never half-mount")
	require.False(t, gipCalled, "a broken Zitadel deployment must not silently fall back to GIP")
	_, found := capture.find("zitadel")
	require.True(t, found, "the failure must be logged clearly, not swallowed")
}
