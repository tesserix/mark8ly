//go:build integration

package dispatch

import (
	"context"

	"gorm.io/gorm"

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
