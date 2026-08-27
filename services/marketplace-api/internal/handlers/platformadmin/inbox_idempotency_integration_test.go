//go:build integration

package platformadmin_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #281a: the primary key IS the check. mark8ly runs multiple replicas, so a
// retry routed to another pod must lose the race against the first — an
// in-memory map would let it through.
func TestInboxActionIdempotency_SecondClaimLosesAndSeesTheFirstOutcome(t *testing.T) {
	tx := testdb.NewTx(t)
	store := platformadmin.NewInboxActionIdempotency(tx)
	ctx := context.Background()

	rec := platformadmin.InboxActionRecord{
		Key: "key-1", Kind: "migration_fast_path", ItemID: "item-1",
		ActionID: "reject", OperatorID: "op-1",
	}

	first, existing, err := store.Claim(ctx, rec)
	require.NoError(t, err)
	require.True(t, first, "the first claim must win")
	require.Nil(t, existing)

	require.NoError(t, store.Complete(ctx, "key-1", json.RawMessage(`{"status":"rejected"}`)))

	// A different item under the SAME key still loses. Reusing a key against
	// another item is a client bug, and answering the first call's result is
	// safer than performing a second, different write under a key the client
	// believes it already spent.
	second, prior, err := store.Claim(ctx, platformadmin.InboxActionRecord{
		Key: "key-1", Kind: "migration_fast_path", ItemID: "DIFFERENT-item",
		ActionID: "approve", OperatorID: "op-2",
	})
	require.NoError(t, err)
	require.False(t, second, "a replayed key must not win the claim")
	require.NotNil(t, prior)
	require.Equal(t, "item-1", prior.ItemID, "the ORIGINAL record must be returned")
	require.Equal(t, "reject", prior.ActionID)
	require.JSONEq(t, `{"status":"rejected"}`, string(prior.Outcome))
}

// A distinct key is an independent execution.
func TestInboxActionIdempotency_DistinctKeysAreIndependent(t *testing.T) {
	tx := testdb.NewTx(t)
	store := platformadmin.NewInboxActionIdempotency(tx)
	ctx := context.Background()

	for _, key := range []string{"a", "b"} {
		first, _, err := store.Claim(ctx, platformadmin.InboxActionRecord{
			Key: key, Kind: "migration_fast_path", ItemID: "item-1",
			ActionID: "reject", OperatorID: "op-1",
		})
		require.NoError(t, err)
		require.Truef(t, first, "key %q must be claimable", key)
	}
}
