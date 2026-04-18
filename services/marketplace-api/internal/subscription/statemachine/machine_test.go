//go:build integration

package statemachine_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestTransition_HappyPath(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusActive,
		To:       subscription.StatusCancelScheduled,
		Actor:    "user:" + uuid.New().String(),
		Reason:   "merchant_cancelled",
	})
	require.NoError(t, err)

	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusCancelScheduled, sub.Status)
}

func TestTransition_CASConflict_WhenStatusAlreadyMoved(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	// Caller expects From=trialing but the row is Active — CAS will find
	// zero matching rows and must return ErrCASConflict.
	err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusTrialing,
		To:       subscription.StatusActive,
		Actor:    "system:webhook",
		Reason:   "invoice.paid",
	})
	require.True(t, errors.Is(err, statemachine.ErrCASConflict))

	// The row must remain unchanged.
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusActive, sub.Status)
}

func TestTransition_InvalidTransition_Rejected(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusExpired,
	}).Error)

	// The spec forbids a direct expired → pending_hard_delete — must go
	// through store_closed first (§17.2).
	err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusExpired,
		To:       subscription.StatusPendingHardDelete,
		Actor:    "system:cron",
		Reason:   "day_90",
	})
	require.True(t, errors.Is(err, statemachine.ErrInvalidTransition))

	// The transition table check must short-circuit before touching the DB.
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusExpired, sub.Status)
}

func TestTransition_EmitsAuditEvent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "audit_logs")

	em := audit.NewEmitter(audit.EmitterConfig{
		DB:     db,
		Repo:   audit.NewRepository(),
		Logger: testLogger(t),
	})

	tenantID, storeID := uuid.New(), uuid.New()
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_x",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	require.NoError(t, statemachine.Transition(context.Background(), statemachine.TransitionInput{
		DB:       db,
		Emitter:  em,
		TenantID: tenantID,
		StoreID:  storeID,
		From:     subscription.StatusActive,
		To:       subscription.StatusCancelScheduled,
		Actor:    "user:" + uuid.New().String(),
		Reason:   "merchant_cancelled",
	}))

	// Drain the async queue before asserting.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	em.Stop(ctx)

	var entry audit.Entry
	require.NoError(t, db.Where("tenant_id = ? AND store_id = ?", tenantID, storeID).First(&entry).Error)
	require.Equal(t, "subscription.state_transition", entry.Action)

	md := entry.Metadata
	require.Equal(t, "active", md["from_status"])
	require.Equal(t, "cancel_scheduled", md["to_status"])
	require.Equal(t, "merchant_cancelled", md["reason"])
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(&machineTestWriter{t: t}, nil))
}

type machineTestWriter struct{ t *testing.T }

func (w *machineTestWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
