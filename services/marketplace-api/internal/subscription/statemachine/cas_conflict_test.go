//go:build integration

package statemachine_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestConcurrentTransition_CASConflict races two goroutines that both attempt
// to move the same subscription from active → cancel_scheduled. The advisory
// lock serializes the two transactions; the second writer's CAS UPDATE finds
// status != active (the first already moved it) and must return ErrCASConflict.
//
// Invariants pinned:
//   - exactly one goroutine succeeds (successes == 1)
//   - exactly one goroutine gets ErrCASConflict (conflicts == 1)
//   - any other error causes the test to fail
//   - the final persisted status is cancel_scheduled
func TestConcurrentTransition_CASConflict(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)
	require.NoError(t, db.Create(&subscription.StoreSubscription{
		TenantID:         tenantID,
		StoreID:          storeID,
		StripeCustomerID: "cus_race",
		Plan:             subscription.PlanStarter,
		Status:           subscription.StatusActive,
	}).Error)

	var successes int32
	var conflicts int32
	var wg sync.WaitGroup

	run := func() {
		defer wg.Done()
		err := statemachine.Transition(context.Background(), statemachine.TransitionInput{
			DB:       db,
			TenantID: tenantID,
			StoreID:  storeID,
			From:     subscription.StatusActive,
			To:       subscription.StatusCancelScheduled,
			Actor:    "user:" + uuid.New().String(),
			Reason:   "race",
		})
		if err == nil {
			atomic.AddInt32(&successes, 1)
			return
		}
		if errors.Is(err, statemachine.ErrCASConflict) {
			atomic.AddInt32(&conflicts, 1)
			return
		}
		t.Errorf("unexpected error from concurrent Transition: %v", err)
	}

	wg.Add(2)
	go run()
	go run()
	wg.Wait()

	require.EqualValues(t, 1, successes, "exactly one concurrent transition must win")
	require.EqualValues(t, 1, conflicts, "the losing goroutine must get ErrCASConflict")

	// Final state must reflect the winner's target.
	var sub subscription.StoreSubscription
	require.NoError(t, db.Where("store_id = ?", storeID).First(&sub).Error)
	require.Equal(t, subscription.StatusCancelScheduled, sub.Status)
}
