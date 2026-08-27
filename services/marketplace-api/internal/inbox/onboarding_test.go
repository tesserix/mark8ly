package inbox_test

import (
	"context"
	"fmt"
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

// recordingSessions captures the SessionsParams it was called with, so tests
// can assert on the outgoing request rather than relying on the fake to
// filter.
type recordingSessions struct {
	res  *onboardingfunnel.SessionsResult
	err  error
	gotP onboardingfunnel.SessionsParams
}

func (r *recordingSessions) ListSessions(_ context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	r.gotP = p
	return r.res, r.err
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
	wrapped := fmt.Errorf("%w: platform-api unreachable", onboardingfunnel.ErrUnavailable)
	p := inbox.NewOnboardingProvider(fakeSessions{err: wrapped}, 48)

	_, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.ErrorIs(t, err, onboardingfunnel.ErrUnavailable,
		"the provider must not swallow the error — the aggregator degrades on it")
}

func TestOnboardingProvider_TenantFilterExcludesOtherTenantsAndUnlinkedSessions(t *testing.T) {
	now := time.Now().UTC()
	tenantA, tenantB := "tenant-a", "tenant-b"
	c := fakeSessions{res: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{ID: "mine", Email: "a@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80, TenantID: &tenantA},
			{ID: "theirs", Email: "b@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80, TenantID: &tenantB},
			{ID: "unlinked", Email: "c@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80},
		},
	}}

	p := inbox.NewOnboardingProvider(c, 48)
	items, err := p.List(context.Background(), inbox.Filter{TenantID: tenantA, Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1, "a tenant-filtered inbox must not leak other tenants' or unlinked sessions")
	require.Equal(t, "mine", items[0].ID)

	n, err := p.Count(context.Background(), inbox.Filter{TenantID: tenantA, Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "Count must answer the same filter as List")
}

func TestOnboardingProvider_RequestsAbandonedSessionsOnly(t *testing.T) {
	now := time.Now().UTC()
	c := &recordingSessions{res: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{ID: "huge-idle", Email: "a@example.com", LastActivityAt: now.Add(-3000 * time.Hour), IdleHours: 3000},
		},
	}}

	p := inbox.NewOnboardingProvider(c, 48)
	_, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.NotNil(t, c.gotP.Abandoned, "List must request abandoned sessions only")
	require.True(t, *c.gotP.Abandoned)
}

func TestOnboardingProvider_ListOrdersByWaitingSinceAscending(t *testing.T) {
	now := time.Now().UTC()
	c := fakeSessions{res: &onboardingfunnel.SessionsResult{
		Sessions: []onboardingfunnel.Session{
			{ID: "middle", Email: "b@example.com", LastActivityAt: now.Add(-80 * time.Hour), IdleHours: 80},
			{ID: "oldest", Email: "c@example.com", LastActivityAt: now.Add(-200 * time.Hour), IdleHours: 200},
			{ID: "newest", Email: "a@example.com", LastActivityAt: now.Add(-50 * time.Hour), IdleHours: 50},
		},
	}}

	p := inbox.NewOnboardingProvider(c, 48)
	items, err := p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 3)

	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	require.Equal(t, []string{"oldest", "middle", "newest"}, ids,
		"items must be sorted by WaitingSince ascending")
}

// limitHonoringSessions mimics upstream's behavior of truncating the
// response to the requested Limit, unlike fakeSessions which always returns
// its full fixed set regardless of what was asked for.
type limitHonoringSessions struct {
	sessions []onboardingfunnel.Session
}

func (l limitHonoringSessions) ListSessions(_ context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error) {
	limit := p.Limit
	if limit <= 0 || limit > len(l.sessions) {
		limit = len(l.sessions)
	}
	return &onboardingfunnel.SessionsResult{Sessions: l.sessions[:limit], Total: int64(len(l.sessions))}, nil
}

func TestOnboardingProvider_CountIsStableAcrossPages(t *testing.T) {
	now := time.Now().UTC()
	sessions := make([]onboardingfunnel.Session, 0, 30)
	for i := 0; i < 30; i++ {
		sessions = append(sessions, onboardingfunnel.Session{
			ID:             "s" + string(rune('a'+i)),
			Email:          "s@example.com",
			LastActivityAt: now.Add(-time.Duration(100+i) * time.Hour),
			IdleHours:      float64(100 + i),
		})
	}
	c := limitHonoringSessions{sessions: sessions}

	p := inbox.NewOnboardingProvider(c, 48)

	// These mirror the aggregator's fanout filters for page=1,limit=25 and
	// page=2,limit=25 (fanout.Limit = page*limit).
	n1, err := p.Count(context.Background(), inbox.Filter{Page: 1, Limit: 25})
	require.NoError(t, err)
	n2, err := p.Count(context.Background(), inbox.Filter{Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, n1, n2, "Count must not change as the aggregator pages forward")
	// Pinned to the fixture size (30), not just to each other: two Counts that
	// silently agreed on the wrong number (e.g. both broken to 0) would still
	// pass the equality check above. All 30 seeded sessions are well past the
	// idle threshold, so 30 is the correct count, not an incidental fixture
	// detail — pinning it here does not make the test brittle to unrelated
	// fixture edits elsewhere in the file.
	require.EqualValues(t, 30, n1, "Count must report the true number of stalled sessions, not just agree with itself")
}

func TestOnboardingProvider_ListForwardsPage(t *testing.T) {
	c := &recordingSessions{res: &onboardingfunnel.SessionsResult{}}
	p := inbox.NewOnboardingProvider(c, 48)

	_, err := p.List(context.Background(), inbox.Filter{Page: 3, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 3, c.gotP.Page, "List must forward the caller's page")

	_, err = p.List(context.Background(), inbox.Filter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, c.gotP.Page, "a zero page must default to 1")
}
