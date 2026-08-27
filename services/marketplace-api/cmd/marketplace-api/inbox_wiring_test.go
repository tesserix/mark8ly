package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

type stubSessionLister struct{}

func (stubSessionLister) ListSessions(context.Context, onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	return &onboardingfunnel.SessionsResult{}, nil
}

// A kind that is registered answers something other than ErrUnknownKind. This
// is how the test proves which providers are present without needing a
// database: the aggregator resolves the kind before it ever touches one.
func kindIsRegistered(t *testing.T, agg *inbox.Aggregator, kind string) bool {
	t.Helper()
	if agg == nil {
		return false
	}
	registered := true
	func() {
		// A registered DB-backed provider panics on a nil *gorm.DB rather
		// than reporting an unknown kind — which still answers the only
		// question here: the aggregator knew the kind.
		defer func() { _ = recover() }()
		_, err := agg.List(context.Background(), inbox.Filter{Kind: kind, Page: 1, Limit: 1})
		registered = !errors.Is(err, inbox.ErrUnknownKind)
	}()
	return registered
}

// #280: the onboarding provider is the one that must NOT be registered
// unconditionally. Its client is nil whenever the platform API URL or secret
// is unset, and a nil client registered as a provider turns every unfiltered
// inbox request into a panic rather than a degraded result.
func TestNewInboxAggregatorOmitsOnboardingWhenClientIsNil(t *testing.T) {
	agg := newInboxAggregator(nil, nil, 0)
	require.Nil(t, agg, "no database and no funnel client means no providers, so no route")

	withFunnel := newInboxAggregator(nil, stubSessionLister{}, 0)
	require.NotNil(t, withFunnel)
	require.True(t, kindIsRegistered(t, withFunnel, inbox.KindOnboardingStalled))
	require.False(t, kindIsRegistered(t, withFunnel, inbox.KindSEAManualReview),
		"a nil database must not register the DB-backed providers")
}

// The five kinds the handler advertises in its unknown_kind error must all be
// registered when the dependencies exist, or the error message promises
// filters that answer "unknown kind".
func TestNewInboxAggregatorCoversEveryAdvertisedKind(t *testing.T) {
	agg := newInboxAggregator(&gorm.DB{}, stubSessionLister{}, 0)
	require.NotNil(t, agg)

	for _, kind := range []string{
		inbox.KindSEAManualReview,
		inbox.KindMigrationFastPath,
		inbox.KindErasureRequest,
		inbox.KindArbitrageAppeal,
		inbox.KindOnboardingStalled,
	} {
		require.Truef(t, kindIsRegistered(t, agg, kind),
			"kind %q is advertised by the handler but not registered by the wiring", kind)
	}
}
