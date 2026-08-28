package main

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/customererasure"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
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

// stubEraser stands in for *customererasure.Executor. inboxActionExecutors
// only needs to know whether it is nil.
type stubEraser struct{}

func (stubEraser) Process(context.Context, uuid.UUID) (customererasure.Receipt, error) {
	return customererasure.Receipt{}, nil
}
func (stubEraser) Reject(context.Context, uuid.UUID, string) (customererasure.Request, error) {
	return customererasure.Request{}, nil
}
func (stubEraser) Lookup(context.Context, uuid.UUID) (customererasure.Request, error) {
	return customererasure.Request{}, nil
}

func executorKinds(executors []platformadmin.InboxActionExecutor) []string {
	kinds := make([]string, 0, len(executors))
	for _, e := range executors {
		kinds = append(kinds, e.Kind())
	}
	return kinds
}

// #259: erasure_request used to answer 501 because no executor was wired for
// it. This is the assertion that the 501 is genuinely replaced rather than
// merely implemented somewhere unreachable — an executor that exists but is
// never registered leaves the endpoint exactly as it was.
func TestInboxActionExecutorsRegistersErasure(t *testing.T) {
	executors := inboxActionExecutors(&migration.Repository{}, stubEraser{})
	require.Contains(t, executorKinds(executors), inbox.KindErasureRequest,
		"the erasure executor must be registered, or /admin/inbox/erasure_request/... still answers 501")
	require.Contains(t, executorKinds(executors), inbox.KindMigrationFastPath,
		"adding erasure must not displace the migration fast path")
}

// Each executor is registered only when its own dependency exists. A missing
// eraser must not take the migration action down with it, and vice versa.
func TestInboxActionExecutorsRegistersOnlyWhatItCanReach(t *testing.T) {
	require.Equal(t, []string{inbox.KindMigrationFastPath},
		executorKinds(inboxActionExecutors(&migration.Repository{}, nil)))
	require.Equal(t, []string{inbox.KindErasureRequest},
		executorKinds(inboxActionExecutors(nil, stubEraser{})))
	require.Nil(t, inboxActionExecutors(nil, nil),
		"nothing reachable means no executors, so every kind keeps its honest 501")
}

// newCustomerEraser must not hand back a non-nil INTERFACE holding a nil
// pointer: that would pass inboxActionExecutors' nil check and register an
// executor that panics on the first click.
func TestNewCustomerEraserReturnsAnUntypedNilWithoutADatabase(t *testing.T) {
	eraser, err := newCustomerEraser(nil, nil)
	require.NoError(t, err)
	require.Nil(t, eraser)
	require.Nil(t, inboxActionExecutors(nil, eraser))
}
