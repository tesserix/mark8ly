package inbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type fakeProvider struct {
	kind  string
	items []inbox.Item
	err   error
}

func (f fakeProvider) Kind() string { return f.kind }
func (f fakeProvider) List(context.Context, inbox.Filter) ([]inbox.Item, error) {
	return f.items, f.err
}
func (f fakeProvider) Count(context.Context, inbox.Filter) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return int64(len(f.items)), nil
}

func at(base time.Time, d time.Duration) *time.Time { t := base.Add(d); return &t }

func TestAggregator_OverdueFirstThenLongestWaitingNullsLast(t *testing.T) {
	now := time.Now().UTC()
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{
			{ID: "no-due-old", WaitingSince: now.Add(-72 * time.Hour)},
			{ID: "due-later", WaitingSince: now.Add(-time.Hour), DueAt: at(now, 48*time.Hour)},
		}},
		fakeProvider{kind: "k2", items: []inbox.Item{
			{ID: "no-due-new", WaitingSince: now.Add(-time.Hour)},
			{ID: "overdue", WaitingSince: now.Add(-2 * time.Hour), DueAt: at(now, -time.Hour)},
		}},
	)

	res, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.NoError(t, err)

	got := make([]string, len(res.Items))
	for i, it := range res.Items {
		got[i] = it.ID
	}
	require.Equal(t, []string{"overdue", "due-later", "no-due-old", "no-due-new"}, got,
		"due dates ascending first, nulls last ordered by longest waiting")
	require.EqualValues(t, 4, res.Total)
	require.Empty(t, res.Degraded)
}

func TestAggregator_DeepPageIsRefusedNotTruncated(t *testing.T) {
	a := inbox.NewAggregator(fakeProvider{kind: "k1"})

	_, err := a.List(context.Background(), inbox.Filter{Page: 11, Limit: 50})
	require.ErrorIs(t, err, inbox.ErrPageTooDeep,
		"page*limit beyond the cap must error, never return a silently truncated page")

	_, err = a.List(context.Background(), inbox.Filter{Page: 10, Limit: 50})
	require.NoError(t, err, "exactly at the cap is allowed")
}

func TestAggregator_SingleKindDelegatesAndBypassesTheCap(t *testing.T) {
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{{ID: "a"}}},
		fakeProvider{kind: "k2", items: []inbox.Item{{ID: "b"}}},
	)

	res, err := a.List(context.Background(), inbox.Filter{Kind: "k2", Page: 100, Limit: 50})
	require.NoError(t, err, "a single-kind request pages natively and is not capped")
	require.Len(t, res.Items, 1)
	require.Equal(t, "b", res.Items[0].ID)
}

func TestAggregator_OneFailingProviderDegradesRatherThanFails(t *testing.T) {
	a := inbox.NewAggregator(
		fakeProvider{kind: "healthy", items: []inbox.Item{{ID: "a"}}},
		fakeProvider{kind: "broken", err: errors.New("platform-api unreachable")},
	)

	res, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	require.Equal(t, []string{"broken"}, res.Degraded,
		"a failed source must be named, so an operator can tell 'none' from 'we could not ask'")
}

func TestAggregator_AllProvidersFailingIsAnError(t *testing.T) {
	boom := errors.New("down")
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", err: boom},
		fakeProvider{kind: "k2", err: boom},
	)

	_, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.Error(t, err, "nothing can be rendered, so this is a real outage")
}
