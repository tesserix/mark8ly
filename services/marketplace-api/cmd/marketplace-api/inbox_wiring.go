package main

import (
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// newInboxAggregator assembles the GET /admin/inbox aggregator (#280) from
// whichever of its five queues this process can actually reach.
//
// Each provider is registered only when its dependency exists, for the same
// reason platformadmin.Deps guards its optional fields: a provider holding a
// nil dependency does not degrade, it panics on the first unfiltered request
// — and an unfiltered request is the default one the console makes. The
// onboarding queue is the live case, not a hypothetical: its client is left
// nil whenever the platform API URL or secret is unset.
//
// Returns nil, not an empty aggregator, when nothing is reachable. Deps.Inbox
// treats nil as "leave the route unmounted", and an inbox that answers 200
// with an empty list is a worse answer than 404: it tells an operator there
// is no work waiting when the truth is that nothing was asked.
func newInboxAggregator(db *gorm.DB, funnel inbox.SessionLister, idleThresholdHours float64) *inbox.Aggregator {
	var providers []inbox.Provider

	if db != nil {
		providers = append(providers,
			inbox.NewSEAReviewProvider(db, nil),
			inbox.NewMigrationFastPathProvider(db),
			inbox.NewErasureProvider(db),
			inbox.NewArbitrageProvider(db),
		)
	}

	// Typed-nil trap: funnel is an interface, so a nil *onboardingfunnel.Client
	// stored in it is NOT == nil here. Callers pass the same
	// platformadmin.OnboardingFunnel variable they pass to Deps, which is left
	// as an untyped nil interface when unconfigured, so this comparison holds.
	if funnel != nil {
		providers = append(providers, inbox.NewOnboardingProvider(funnel, idleThresholdHours))
	}

	if len(providers) == 0 {
		return nil
	}
	return inbox.NewAggregator(providers...)
}

// inboxDep converts the aggregator to the interface Deps expects, preserving
// a nil aggregator as a nil INTERFACE rather than a non-nil interface holding
// a nil pointer — the latter would pass Register's `deps.Inbox != nil` guard
// and mount a route that panics on first use.
func inboxDep(agg *inbox.Aggregator) platformadmin.InboxAggregator {
	if agg == nil {
		return nil
	}
	return agg
}
