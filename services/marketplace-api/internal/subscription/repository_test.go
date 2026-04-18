//go:build integration

package subscription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestRepository_GetByStoreID_RequiresTenantMatch proves that a foreign tenant
// cannot read another tenant's subscription by guessing its store_id. The repo
// must filter by (tenant_id, store_id) and return NotFound for the mismatch
// case — anything else is an IDOR hole.
func TestRepository_GetByStoreID_RequiresTenantMatch(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	repo := subscription.NewRepository()

	tenantA := uuid.New()
	tenantB := uuid.New()
	store := uuid.New()

	sub := subscription.StoreSubscription{
		TenantID:         tenantA,
		StoreID:          store,
		StripeCustomerID: "cus_A",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}
	require.NoError(t, db.Create(&sub).Error)

	gotA, err := repo.GetByStoreID(context.Background(), db, tenantA, store)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	require.Equal(t, tenantA, gotA.TenantID)

	gotB, err := repo.GetByStoreID(context.Background(), db, tenantB, store)
	var appErr *apperrors.Error
	require.Error(t, err)
	require.True(t, errors.As(err, &appErr) && appErr.Code == apperrors.CodeNotFound,
		"expected NotFound app error when tenant mismatches")
	require.Nil(t, gotB)
}
