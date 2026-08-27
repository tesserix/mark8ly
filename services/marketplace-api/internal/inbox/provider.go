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

// Compile-time proof that every provider satisfies Provider. Without these, a
// signature drift surfaces only where a provider is registered, far from the
// change that caused it.
var (
	_ Provider = (*SEAReviewProvider)(nil)
	_ Provider = (*ErasureProvider)(nil)
	_ Provider = (*ArbitrageProvider)(nil)
	_ Provider = (*MigrationFastPathProvider)(nil)
	_ Provider = (*OnboardingProvider)(nil)

	// Only this kind can be acted on today (#281a). The assertion is here so
	// that if the Get signature drifts, it breaks next to the interface it
	// belongs to rather than at the action endpoint's type switch.
	_ ItemGetter = (*MigrationFastPathProvider)(nil)
)
