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
	var got inbox.Filter
	rec := recordingProvider{
		kind:   "k2",
		items:  []inbox.Item{{ID: "b"}},
		onList: func(f inbox.Filter) { got = f },
	}
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{{ID: "a"}}},
		rec,
	)

	res, err := a.List(context.Background(), inbox.Filter{Kind: "k2", Page: 100, Limit: 50})
	require.NoError(t, err, "a single-kind request pages natively and is not capped")
	require.Len(t, res.Items, 1)
	require.Equal(t, "b", res.Items[0].ID)
	require.Equal(t, 100, got.Page,
		"the delegated provider must receive the caller's exact page, unmodified")
	require.Equal(t, 50, got.Limit,
		"the delegated provider must receive the caller's exact limit, unmodified")
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
	require.EqualValues(t, 1, res.Total,
		"a failed provider's count must be excluded from the total, not summed as 0 or otherwise")
}

func TestAggregator_AllProvidersFailingIsAnError(t *testing.T) {
	boom := errors.New("down")
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", err: boom},
		fakeProvider{kind: "k2", err: boom},
	)

	_, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 10})
	require.ErrorIs(t, err, inbox.ErrAllSourcesFailed, "nothing can be rendered, so this is a real outage")
}

type recordingProvider struct {
	kind   string
	items  []inbox.Item
	onList func(inbox.Filter)
}

func (r recordingProvider) Kind() string { return r.kind }
func (r recordingProvider) List(_ context.Context, f inbox.Filter) ([]inbox.Item, error) {
	if r.onList != nil {
		r.onList(f)
	}
	return r.items, nil
}
func (r recordingProvider) Count(context.Context, inbox.Filter) (int64, error) {
	return int64(len(r.items)), nil
}

func TestAggregator_SingleKindAppliesDefaultPagination(t *testing.T) {
	var got inbox.Filter
	rec := recordingProvider{kind: "k1", onList: func(f inbox.Filter) { got = f }}

	a := inbox.NewAggregator(rec)
	_, err := a.List(context.Background(), inbox.Filter{Kind: "k1"})
	require.NoError(t, err)

	require.Equal(t, 1, got.Page, "a zero page must default to 1 on the delegated path")
	require.Equal(t, 25, got.Limit,
		"a zero limit must default, or the provider's `if f.Limit > 0` guard returns every row unbounded")
}

func TestAggregator_AggregatePageTwoWindowIsNonOverlapping(t *testing.T) {
	now := time.Now().UTC()
	a := inbox.NewAggregator(
		fakeProvider{kind: "k1", items: []inbox.Item{
			{ID: "a1", WaitingSince: now.Add(-6 * time.Hour)},
			{ID: "a2", WaitingSince: now.Add(-4 * time.Hour)},
			{ID: "a3", WaitingSince: now.Add(-2 * time.Hour)},
		}},
		fakeProvider{kind: "k2", items: []inbox.Item{
			{ID: "b1", WaitingSince: now.Add(-5 * time.Hour)},
			{ID: "b2", WaitingSince: now.Add(-3 * time.Hour)},
			{ID: "b3", WaitingSince: now.Add(-1 * time.Hour)},
		}},
	)

	// Global ascending-WaitingSince order: a1, b1, a2, b2, a3, b3.
	page1, err := a.List(context.Background(), inbox.Filter{Page: 1, Limit: 2})
	require.NoError(t, err)
	page2, err := a.List(context.Background(), inbox.Filter{Page: 2, Limit: 2})
	require.NoError(t, err)

	ids1 := make([]string, len(page1.Items))
	for i, it := range page1.Items {
		ids1[i] = it.ID
	}
	ids2 := make([]string, len(page2.Items))
	for i, it := range page2.Items {
		ids2[i] = it.ID
	}

	require.Equal(t, []string{"a1", "b1"}, ids1, "page 1 is the first window of the globally sorted order")
	require.Equal(t, []string{"a2", "b2"}, ids2, "page 2 is the next, adjacent, non-overlapping window")
}

// #281a: executing an action requires reading back the item it belongs to, so
// the declared `actions` array can be checked. Provider is Kind/List/Count and
// has no Get, so the aggregator resolves one through the optional ItemGetter
// interface — a kind whose provider does not implement it is answerable, and
// answerably NOT supported, rather than silently pretending.
func TestAggregatorGetUnknownKind(t *testing.T) {
	agg := inbox.NewAggregator(fakeProvider{kind: "a"})
	_, err := agg.Get(context.Background(), "nope", "id-1")
	require.ErrorIs(t, err, inbox.ErrUnknownKind)
}

func TestAggregatorGetKindWithoutGetterIsNotSupported(t *testing.T) {
	agg := inbox.NewAggregator(fakeProvider{kind: "a"})
	_, err := agg.Get(context.Background(), "a", "id-1")
	require.ErrorIs(t, err, inbox.ErrGetNotSupported)
}

type gettableProvider struct {
	fakeProvider
	item inbox.Item
	err  error
}

func (g *gettableProvider) Get(context.Context, string) (inbox.Item, error) {
	return g.item, g.err
}

func TestAggregatorGetDelegatesToTheKindsProvider(t *testing.T) {
	want := inbox.Item{ID: "i1", Kind: "a", Title: "one",
		Actions: []inbox.Action{{ID: "approve", Label: "Approve"}}}
	agg := inbox.NewAggregator(&gettableProvider{fakeProvider: fakeProvider{kind: "a"}, item: want})

	got, err := agg.Get(context.Background(), "a", "i1")
	require.NoError(t, err)
	require.Equal(t, want.ID, got.ID)
	require.Len(t, got.Actions, 1)
}
