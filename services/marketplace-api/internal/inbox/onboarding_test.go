package inbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

type fakeSessions struct {
	res *onboardingfunnel.SessionsResult
	err error
}

func (f fakeSessions) ListSessions(context.Context, onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	return f.res, f.err
}

func TestOnboardingProvider_OnlyStalledSessions(t *testing.T) {
	now := time.Now().UTC()
	c := fakeSessions{res: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{ID: "fresh", Email: "a@example.com", LastActivityAt: now.Add(-time.Hour), IdleHours: 1},
			{ID: "stalled", Email: "b@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80},
		},
		Total: 2,
	}}

	p := inbox.NewOnboardingProvider(c, 48)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1, "only sessions idle beyond the threshold are waiting on a human")
	require.Equal(t, "stalled", items[0].ID)
	require.Equal(t, inbox.KindOnboardingStalled, items[0].Kind)
	require.Equal(t, "b@example.com", items[0].Title)
	require.Nil(t, items[0].DueAt)
}

func TestOnboardingProvider_ErrorPropagatesForTheAggregatorToDegrade(t *testing.T) {
	p := inbox.NewOnboardingProvider(fakeSessions{err: errors.New("platform-api unreachable")}, 48)

	_, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.Error(t, err, "the provider must not swallow the error — the aggregator degrades on it")
}
