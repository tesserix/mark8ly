package consolecatalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

// The cutover source: it answers from the console's last SUCCESSFUL read and
// says "no" otherwise, letting pricing fall through to the compiled catalog.

func TestSource_AnswersFromAFetchedCatalog(t *testing.T) {
	f := &stubFetcher{catalog: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "k", Plan: "pro", Period: "annual", Tier: "ppp", Currency: "vnd", UnitAmountMinor: 42},
	}}}
	s := consolecatalog.NewSource(f, time.Hour, nil)
	require.NoError(t, s.Refresh(context.Background()))

	amt, ok := s.PPPOption("pro", "annual", "vnd")
	require.True(t, ok)
	require.Equal(t, int64(42), amt.UnitAmountMinor)
	require.Equal(t, "vnd", amt.Currency)
}

// Cold start with a dead console. The source must decline, so pricing serves
// the baked snapshot. Answering with an empty Amount would be silent
// mispricing — money.go reads a zero amount as a real price.
func TestSource_ColdAndFailedDeclinesRatherThanAnsweringZero(t *testing.T) {
	s := consolecatalog.NewSource(&stubFetcher{err: errors.New("dead")}, time.Hour, nil)
	require.Error(t, s.Refresh(context.Background()))

	_, ok := s.PPPOption("pro", "annual", "vnd")
	require.False(t, ok, "a cold source must decline so the compiled catalog answers")

	_, ok = s.DevelopedOptions("pro", "annual")
	require.False(t, ok)
}

// Fail open to last known: once it has data, a later failed refresh must not
// throw it away. The console being down is not a reason to reprice.
func TestSource_KeepsLastKnownWhenARefreshFails(t *testing.T) {
	f := &stubFetcher{catalog: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "k", Plan: "pro", Period: "annual", Tier: "ppp", Currency: "vnd", UnitAmountMinor: 42},
	}}}
	s := consolecatalog.NewSource(f, time.Hour, nil)
	require.NoError(t, s.Refresh(context.Background()))

	f.err = errors.New("console down")
	require.Error(t, s.Refresh(context.Background()))

	amt, ok := s.PPPOption("pro", "annual", "vnd")
	require.True(t, ok, "a failed refresh must not discard a good catalog")
	require.Equal(t, int64(42), amt.UnitAmountMinor)
}

// A currency the console does not carry must decline per lookup, so the
// compiled catalog fills the gap instead of the price vanishing.
func TestSource_DeclinesForACurrencyItDoesNotCarry(t *testing.T) {
	f := &stubFetcher{catalog: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "k", Plan: "pro", Period: "annual", Tier: "ppp", Currency: "vnd", UnitAmountMinor: 42},
	}}}
	s := consolecatalog.NewSource(f, time.Hour, nil)
	require.NoError(t, s.Refresh(context.Background()))

	_, ok := s.PPPOption("pro", "annual", "inr")
	require.False(t, ok)
}

func TestSource_DevelopedOptionsCarryEveryCurrency(t *testing.T) {
	f := &stubFetcher{catalog: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "d", Plan: "starter", Period: "monthly", Tier: "developed", Currency: "usd", UnitAmountMinor: 1900},
		{LookupKey: "d", Plan: "starter", Period: "monthly", Tier: "developed", Currency: "gbp", UnitAmountMinor: 1500},
	}}}
	s := consolecatalog.NewSource(f, time.Hour, nil)
	require.NoError(t, s.Refresh(context.Background()))

	opts, ok := s.DevelopedOptions("starter", "monthly")
	require.True(t, ok)
	require.Len(t, opts, 2)
	require.Equal(t, int64(1500), opts["gbp"].UnitAmountMinor)
}

// Installing it must not change what pricing answers while the two agree —
// which is exactly the state the parallel run has been proving.
func TestSource_InstalledOverAnAgreeingCatalogChangesNothing(t *testing.T) {
	f := &stubFetcher{catalog: matchingCatalog()}
	s := consolecatalog.NewSource(f, time.Hour, nil)
	require.NoError(t, s.Refresh(context.Background()))

	before, okBefore := pricing.LookupPPPOption("pro", "annual", "vnd")
	pricing.UseSource(s)
	t.Cleanup(func() { pricing.UseSource(nil) })
	after, okAfter := pricing.LookupPPPOption("pro", "annual", "vnd")

	require.Equal(t, okBefore, okAfter)
	require.Equal(t, before.UnitAmountMinor, after.UnitAmountMinor,
		"cutting over while the sources agree must be a no-op at the point of use")
}
