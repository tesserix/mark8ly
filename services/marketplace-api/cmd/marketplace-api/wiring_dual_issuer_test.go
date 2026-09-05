package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// loggedContains reports whether any captured log message mentions substr.
func loggedContains(h *captureHandler, substr string) bool {
	for _, r := range h.records {
		if strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}

// acceptingVerifier accepts exactly one token string, so a test can prove
// WHICH issuers the returned verifier actually consults.
type acceptingVerifier struct {
	token  string
	claims *auth.TokenClaims
}

func (a *acceptingVerifier) Verify(_ context.Context, tok string) (*auth.TokenClaims, error) {
	if tok == a.token {
		return a.claims, nil
	}
	return nil, errors.New("acceptingVerifier: not my token")
}

// Dual mode must accept BOTH issuers from one verifier — that is the whole
// point: an installed GIP app and a new Zitadel app authenticate against
// the same deployment during the drain.
func TestSelectMobileTokenVerifier_DualIssuer_AcceptsBothIssuers(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true, ZitadelDualIssuer: true}
	log, _ := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return &acceptingVerifier{token: "zit", claims: &auth.TokenClaims{UserID: "zit-user"}}, nil
		},
		func() (auth.TokenVerifier, error) {
			return &acceptingVerifier{token: "gip", claims: &auth.TokenClaims{UserID: "gip-user", TenantID: "t1"}}, nil
		})
	require.NotNil(t, got)

	zit, err := got.Verify(context.Background(), "zit")
	require.NoError(t, err, "a Zitadel token must be accepted in dual mode")
	require.Equal(t, "zit-user", zit.UserID)

	gip, err := got.Verify(context.Background(), "gip")
	require.NoError(t, err, "an already-installed app's GIP token must still be accepted in dual mode")
	require.Equal(t, "gip-user", gip.UserID)
	require.Equal(t, "t1", gip.TenantID, "the GIP tenant claim must survive the composite unchanged")

	_, err = got.Verify(context.Background(), "garbage")
	require.Error(t, err, "a token from neither issuer must still be rejected")
}

// A Zitadel construction failure must disable mobile routes even in dual
// mode. Falling back to GIP-only would leave a broken Zitadel deployment
// quietly serving the legacy path — masking exactly the misconfiguration
// an operator needs to see, which is the rule the single-issuer path
// already follows.
func TestSelectMobileTokenVerifier_DualIssuer_ZitadelFailureDisablesRoutes(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true, ZitadelDualIssuer: true}
	log, logs := captureLogger()
	gipCalled := false

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return nil, errors.New("discovery unreachable")
		},
		func() (auth.TokenVerifier, error) {
			gipCalled = true
			return &acceptingVerifier{token: "gip"}, nil
		})

	require.Nil(t, got, "a Zitadel construction failure must disable mobile routes, never half-mount on GIP")
	require.False(t, gipCalled, "GIP must not be consulted as a silent fallback for a broken Zitadel config")
	require.True(t, loggedContains(logs, "zitadel"), "the Zitadel construction failure must be logged")
}

// GIP unconfigured (newGIP returns nil, nil — the pre-existing "no GIP
// config" case) must degrade to Zitadel-only rather than nil-panicking or
// disabling the working path. Dual mode is then simply moot.
func TestSelectMobileTokenVerifier_DualIssuer_GIPUnconfiguredFallsBackToZitadelOnly(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true, ZitadelDualIssuer: true}
	log, _ := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return &acceptingVerifier{token: "zit", claims: &auth.TokenClaims{UserID: "zit-user"}}, nil
		},
		func() (auth.TokenVerifier, error) { return nil, nil })
	require.NotNil(t, got)

	claims, err := got.Verify(context.Background(), "zit")
	require.NoError(t, err)
	require.Equal(t, "zit-user", claims.UserID)
}

// A GIP construction ERROR must not take down the Zitadel path with it.
// The legacy issuer is the one being retired; stranding the new app
// because the old one failed to initialise is the worse of the two
// outcomes. It must be logged, because it does silently strand installs
// that still hold GIP tokens.
func TestSelectMobileTokenVerifier_DualIssuer_GIPErrorKeepsZitadelWorking(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: true, ZitadelDualIssuer: true}
	log, logs := captureLogger()

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return &acceptingVerifier{token: "zit", claims: &auth.TokenClaims{UserID: "zit-user"}}, nil
		},
		func() (auth.TokenVerifier, error) { return nil, errors.New("firebase init failed") })
	require.NotNil(t, got, "a GIP failure must not disable the Zitadel path in dual mode")

	claims, err := got.Verify(context.Background(), "zit")
	require.NoError(t, err)
	require.Equal(t, "zit-user", claims.UserID)
	require.True(t, loggedContains(logs, "gip"), "stranding GIP installs must be logged, not silent")
}

// Dual-issuer without Zitadel enabled is meaningless and must behave
// exactly like today's GIP-only deployment.
func TestSelectMobileTokenVerifier_DualIssuerWithoutZitadel_IsGIPOnly(t *testing.T) {
	cfg := &config.Config{ZitadelEnabled: false, ZitadelDualIssuer: true}
	log, _ := captureLogger()
	gipVerifier := &fakeMobileVerifier{Name: "gip"}

	got := selectMobileTokenVerifier(context.Background(), cfg, log,
		func(context.Context, string, string) (auth.TokenVerifier, error) {
			return nil, errors.New("must not be called when Zitadel is disabled")
		},
		func() (auth.TokenVerifier, error) { return gipVerifier, nil })

	require.Same(t, auth.TokenVerifier(gipVerifier), got,
		"with Zitadel off, the GIP verifier must be returned unwrapped, exactly as today")
}
