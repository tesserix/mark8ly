//go:build integration

package dispatch

import (
	"context"

	"gorm.io/gorm"
)

// HandleCustomerUpdatedForTest exposes the unexported handler to the
// package's external integration tests.
func HandleCustomerUpdatedForTest(ctx context.Context, tx *gorm.DB, raw []byte) error {
	return handleCustomerUpdated(ctx, tx, raw)
}
