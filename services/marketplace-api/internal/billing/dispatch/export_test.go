//go:build integration

package dispatch

import (
	"context"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// HandleCustomerUpdatedForTest exposes the unexported handler to the
// package's external integration tests.
func HandleCustomerUpdatedForTest(ctx context.Context, tx *gorm.DB, raw []byte) error {
	return handleCustomerUpdated(ctx, tx, raw)
}

// ReportArbitrageFailureForTest exposes the non-fatal arbitrage failure
// reporter (log + metric) to the package's external integration tests.
func ReportArbitrageFailureForTest(sub subscription.StoreSubscription, err error) {
	reportArbitrageFailure(sub, err)
}

// RecordArbitrageForTest exposes the post-commit arbitrage registration to
// the package's external integration tests.
//
// It exists because the only production caller hard-codes IPCountry: "", and
// arbitrage.Evaluate never flags without an IP country — so the recorder's
// writes are unreachable through handleCheckoutSessionCompleted and the
// cross-connection stall in #438 cannot be provoked through it. Tests build
// the flagging RecordInput directly instead.
func RecordArbitrageForTest(ctx context.Context, d *Dispatcher, sub subscription.StoreSubscription, in arbitrage.RecordInput) {
	d.recordArbitrage(ctx, sub, in)
}
