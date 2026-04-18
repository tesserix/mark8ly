//go:build integration

package planchange_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestWritePlanChangeAuditRow_InsertsAppendOnly(t *testing.T) {
	db := testdb.NewDB(t, "subscription_plan_change_audit")
	ctx := context.Background()

	tenantID := uuid.New()
	storeID := uuid.New()
	now := time.Now().Truncate(time.Second)

	row := planchange.PlanChangeAuditRow{
		TenantID:             tenantID,
		StoreID:              storeID,
		StripeSubscriptionID: "sub_test123",
		StripeInvoiceID:      "",
		FromPlan:             subscription.PlanStarter,
		ToPlan:               subscription.PlanStudio,
		FromPeriod:           subscription.PeriodMonthly,
		ToPeriod:             subscription.PeriodMonthly,
		Action:               "upgrade_committed",
		BillingCurrency:      "USD",
		ProrationCents:       0,
		Actor:                "user:" + uuid.NewString(),
		Reason:               "merchant_initiated",
		EffectiveAt:          now,
	}

	require.NoError(t, planchange.WritePlanChangeAuditRowTx(ctx, db, row))

	var count int64
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)

	// Verify auto-generated ID is non-nil.
	var got planchange.PlanChangeAuditRow
	require.NoError(t, db.Table("subscription_plan_change_audit").
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		First(&got).Error)
	require.NotEqual(t, uuid.Nil, got.ID)
	require.Equal(t, subscription.PlanStarter, got.FromPlan)
	require.Equal(t, subscription.PlanStudio, got.ToPlan)
	require.Equal(t, "upgrade_committed", got.Action)
	require.Equal(t, "USD", got.BillingCurrency)
}
