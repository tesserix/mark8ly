package main

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customererasure"
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

// inboxItemSource exposes the same aggregator as the single-item reader the
// action endpoint needs (#281a), with the same nil-interface discipline as
// inboxDep below: a non-nil interface holding a nil pointer would mount the
// action route and panic on first use.
func inboxItemSource(agg *inbox.Aggregator) platformadmin.InboxItemSource {
	if agg == nil {
		return nil
	}
	return agg
}

// inboxActionExecutors lists the kinds that can actually be acted on.
//
// migration_fast_path and erasure_request. sea_manual_review and the rest are
// readable but not actionable and answer 501 — deliberately, not as an
// oversight: SEA's underlying support is only partially implemented, and
// wiring a one-click approve into half-built behaviour is worse than an
// honest "not implemented".
//
// erasure_request USED to be in that list, on the grounds that its `process`
// action is irreversible destruction of customer data and should not get a
// one-click path "before the behaviour beneath it is settled". #259 settled
// it: internal/customererasure now runs a reviewed, store-scoped plan in one
// transaction, writes a receipt of what it destroyed AND what it retained,
// and refuses a request another worker holds. The 501 was the honest answer
// while nothing existed underneath; it would be a misleading one now.
//
// Each executor is registered only when its dependency exists, for the same
// reason the providers above are: an executor holding a nil dependency does
// not degrade, it panics on the first click.
func inboxActionExecutors(
	reviews platformadmin.MigrationFastPathReviewer,
	eraser platformadmin.CustomerEraser,
) []platformadmin.InboxActionExecutor {
	var executors []platformadmin.InboxActionExecutor
	if reviews != nil {
		executors = append(executors, platformadmin.NewMigrationFastPathExecutor(reviews))
	}
	// Typed-nil trap, as with funnel above: eraser is an interface, so a nil
	// *customererasure.Executor stored in it would NOT be == nil here.
	// newCustomerEraser returns an untyped nil interface for that reason.
	if eraser != nil {
		executors = append(executors, platformadmin.NewErasureExecutor(eraser))
	}
	if len(executors) == 0 {
		return nil
	}
	return executors
}

// newCustomerEraser builds the erasure executor, or an untyped nil interface
// when there is no database to erase from.
//
// The error from NewExecutor is not swallowed into a nil: a nil db is a
// WIRING bug, and this returning nil silently would turn it into a 501 that
// reads as a deliberate product decision rather than a broken deployment.
func newCustomerEraser(db *gorm.DB, logger *slog.Logger) (platformadmin.CustomerEraser, error) {
	if db == nil {
		return nil, nil
	}
	eraser, err := customererasure.NewExecutor(db, logger)
	if err != nil {
		return nil, err
	}
	return eraser, nil
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
