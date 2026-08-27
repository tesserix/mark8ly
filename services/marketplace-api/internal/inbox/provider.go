package inbox

import "context"

// Kinds mark8ly emits. Each maps to exactly one Provider.
const (
	KindSEAManualReview   = "sea_manual_review"
	KindMigrationFastPath = "migration_fast_path"
	KindErasureRequest    = "erasure_request"
	KindArbitrageAppeal   = "arbitrage_appeal"
	KindOnboardingStalled = "onboarding_stalled"
)

// Provider is one queue's view of the work waiting in it.
//
// List returns items already ordered by the provider's own natural order;
// the aggregator re-sorts across providers, so a provider need only be
// internally consistent. Count answers the same Filter List would.
type Provider interface {
	Kind() string
	List(ctx context.Context, f Filter) ([]Item, error)
	Count(ctx context.Context, f Filter) (int64, error)
}
