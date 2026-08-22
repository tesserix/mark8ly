//go:build integration

package platformadmin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestSweepExpiredNoncesDeletesOnlyExpired(t *testing.T) {
	db := testdb.NewDB(t, "platform_request_nonces")
	store := platformadmin.NewNonceStore(db)
	ctx := context.Background()

	expiredNonce := uuid.NewString()
	liveNonce := uuid.NewString()

	// Expired: TTL already in the past.
	_, err := store.Claim(ctx, expiredNonce, time.Now().Add(-time.Minute))
	require.NoError(t, err)

	// Still valid: TTL in the future.
	_, err = store.Claim(ctx, liveNonce, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	deleted, err := platformadmin.SweepExpiredNonces(ctx, db)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	// The expired row is gone: claiming the same nonce again succeeds.
	freshAgain, err := store.Claim(ctx, expiredNonce, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, freshAgain, "expired nonce row should have been swept, allowing reclaim")

	// The still-valid row survives: claiming it again is a replay, not a fresh use.
	stillReplay, err := store.Claim(ctx, liveNonce, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	require.False(t, stillReplay, "unexpired nonce row must survive the sweep")
}
