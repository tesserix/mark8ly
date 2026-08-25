//go:build integration

package dunning_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingEmail captures which template went to whom, so the assertions can
// be about the VALUES sent rather than merely that a send happened.
type recordingEmail struct{ sent []string }

func (r *recordingEmail) Send(_ context.Context, template email.TemplateID, to string, _ map[string]any) error {
	r.sent = append(r.sent, string(template)+"->"+to)
	return nil
}

func seedTrialSub(t *testing.T, db *gorm.DB, createdAt time.Time, trialEndsAt *time.Time, hasPM bool) subscription.StoreSubscription {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	seedStore(t, db, tenantID, storeID)
	sub := subscription.StoreSubscription{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		StripeCustomerID:        "cus_" + storeID.String()[:8],
		Status:                  subscription.StatusTrialing,
		HasDefaultPaymentMethod: hasPM,
		CreatedAt:               createdAt,
		TrialEndsAt:             trialEndsAt,
	}
	require.NoError(t, db.Create(&sub).Error)
	return sub
}

// An extended trial must be reminded relative to its NEW end. Before #353 the
// cron bucketed on created_at and would have fired T-15 based on the original
// 90-day schedule — a month early — then nothing before the real end.
func TestTrialReminders_FireRelativeToTheExtendedEnd(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Derived end is 15 days away — the OLD schedule would fire T-15 today.
	created := now.Add(-75 * 24 * time.Hour)
	// Real end is 60 days away, so nothing should fire today.
	extended := now.Add(60 * 24 * time.Hour)
	sub := seedTrialSub(t, db, created, &extended, false)

	rec := &recordingEmail{}
	cron := dunning.NewSendTrialReminders(db, rec, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, rec.sent,
		"no reminder is due 60 days before the effective end; bucketing on created_at would have sent T-15")

	var n int64
	require.NoError(t, db.Table("trial_reminders").Where("subscription_id = ?", sub.ID).Count(&n).Error)
	require.Equal(t, int64(0), n, "no idempotency slot should be claimed when nothing is due")
}

// The converse, and the one that proves the cron still works at all: with the
// effective end exactly 15 days out, T-15 fires.
func TestTrialReminders_FireOnTheExtendedEndsT15(t *testing.T) {
	db := testdb.NewDB(t, "trial_reminders", "store_subscriptions", "stores")

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	// Created 200 days ago — the derived end is long past, so ONLY the stored
	// value can put this row in the T-15 bucket.
	created := now.Add(-200 * 24 * time.Hour)
	extended := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC) // 15 days out
	sub := seedTrialSub(t, db, created, &extended, false)

	rec := &recordingEmail{}
	cron := dunning.NewSendTrialReminders(db, rec, nil, nil, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	require.Len(t, rec.sent, 1, "exactly the T-15 reminder is due")

	var keys []string
	require.NoError(t, db.Table("trial_reminders").
		Where("subscription_id = ?", sub.ID).Pluck("offset_key", &keys).Error)
	require.Equal(t, []string{"no_pm_t_minus_15"}, keys)
}
