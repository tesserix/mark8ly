package stripe

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// #459: the lookup_key format lived in TWO packages. catalog.go derived it
// canonically; lookupKeyFor here re-derived it with its own fmt.Sprintf,
// having called MustGetDescriptor purely to validate and then thrown the
// descriptor away with descriptor.LookupKey sitting unused.
//
// Nothing compared the two. Change the catalog to _v2 and this path keeps
// writing _v1, pointing the subscription UPDATE path at prices that are
// stale or absent — no compile error, no failing test, and it rots longest
// in the unattended downgrade cron.
//
// This is the guard the issue asked for. It walks the WHOLE catalog rather
// than sampling, so a divergence cannot hide in a plan or currency nobody
// thought to spot-check.
func TestLookupKeyFor_AgreesWithEveryCatalogDescriptor(t *testing.T) {
	descriptors := pricing.AllDescriptors()
	require.NotEmpty(t, descriptors, "a vacuous loop would prove nothing")

	var checked int
	for _, d := range descriptors {
		var tier subscription.PriceTier
		switch d.Tier {
		case pricing.TierDeveloped:
			tier = subscription.PriceTierDeveloped
		case pricing.TierPPP:
			tier = subscription.PriceTierPPP
		default:
			t.Fatalf("unknown tier %q in catalog — this test must be taught about it", d.Tier)
		}

		got, err := lookupKeyFor(
			subscription.SubscriptionPlan(d.Plan),
			subscription.SubscriptionPeriod(d.Period),
			d.Currency,
			tier,
		)
		require.NoError(t, err, "descriptor %s must be resolvable from the subscription layer", d.LookupKey)
		require.Equal(t, d.LookupKey, got,
			"the subscription path must return the catalog's own LookupKey, not a "+
				"second derivation of the format — see #459")
		checked++
	}
	require.Equal(t, len(descriptors), checked)
}
