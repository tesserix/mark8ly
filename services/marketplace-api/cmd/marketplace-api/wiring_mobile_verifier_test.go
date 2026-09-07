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
// selectMobileTokenVerifier returned without depending on Zitadel
// internals.
type fakeMobileVerifier struct {
	Name string
}

func (f *fakeMobileVerifier) Verify(context.Context, string) (*auth.TokenClaims, error) {
	return nil, errors.New("fakeMobileVerifier: Verify not implemented")
}

// TestSelectMobileTokenVerifier_FlagUnset_DisablesRoutes: with Zitadel
// off there is no second issuer left to fall back to (#786), so the
// mobile admin group must stay unmounted rather than run unauthenticated
// or half-mounted.
func TestSelectMobileTokenVerifier_FlagUnset_DisablesRoutes(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false}
	log, _ := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			t.Fatal("Zitadel factory must not run when ZITADEL_ENABLED is false")
			return nil, nil
		},
	)

	require.Nil(t, got, "no verifier means RegisterAdminMobile mounts nothing")
}

// TestSelectMobileTokenVerifier_FlagSet_BuildsZitadelVerifier proves the
// configured issuer + audience (never constants) are the values handed to
// the factory, and that its result is returned unwrapped — there is no
// composite left to wrap it in.
func TestSelectMobileTokenVerifier_FlagSet_BuildsZitadelVerifier(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled:        true,
		ZitadelIssuer:         "https://auth.tesserix.app",
		ZitadelAdminProjectID: "389070376568619523",
	}
	log, _ := captureLogger()
	zitadelVerifier := &fakeMobileVerifier{Name: "zitadel"}
	var gotIssuer, gotAudience string

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(_ context.Context, issuer, audience string) (auth.TokenVerifier, error) {
			gotIssuer = issuer
			gotAudience = audience
			return zitadelVerifier, nil
		},
	)

	require.Same(t, auth.TokenVerifier(zitadelVerifier), got)
	require.Equal(t, "https://auth.tesserix.app", gotIssuer)
	require.Equal(t, "389070376568619523", gotAudience)
}

// TestSelectMobileTokenVerifier_ZitadelConstructionFails_DisablesRoutes
// is the "fails clearly, never half-mounts" requirement for a runtime
// construction failure (e.g. the issuer's discovery endpoint unreachable
// at boot) that config-time validation (Config.ValidateZitadel, checked
// unconditionally in config.Load) cannot catch ahead of time. This
// behaviour predates #786 and survives it unchanged — only the "and never
// falls back to GIP" half is gone, because there is nothing left to fall
// back to.
func TestSelectMobileTokenVerifier_ZitadelConstructionFails_DisablesRoutes(t *testing.T) {
	cfg := &config.Config{
		ZitadelEnabled:        true,
		ZitadelIssuer:         "https://auth.tesserix.app",
		ZitadelAdminProjectID: "389070376568619523",
	}
	log, capture := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return nil, errors.New("discover https://auth.tesserix.app: connection refused")
		},
	)

	require.Nil(t, got, "a construction failure must disable mobile admin routes (nil verifier), never half-mount")
	_, found := capture.find("zitadel")
	require.True(t, found, "the failure must be logged clearly, not swallowed")
}
