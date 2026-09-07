//go:build integration

package platformauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platformauth"
)

func TestClaimAcceptsFirstUseAndRejectsReplay(t *testing.T) {
	db := newTestDB(t)
	store := platformauth.NewNonceStore(db)
	ctx := context.Background()
	nonce := uuid.NewString()
	expires := time.Now().Add(5 * time.Minute)

	first, err := store.Claim(ctx, nonce, expires)
	require.NoError(t, err)
	require.True(t, first, "first use must be accepted")

	second, err := store.Claim(ctx, nonce, expires)
	require.NoError(t, err)
	require.False(t, second, "replayed nonce must be rejected")
}

func TestClaimRejectsMalformedNonce(t *testing.T) {
	db := newTestDB(t)
	store := platformauth.NewNonceStore(db)

	ok, err := store.Claim(context.Background(), "not-a-uuid", time.Now().Add(time.Minute))
	require.Error(t, err)
	require.False(t, ok)
}

// The unique constraint, not application logic, is what makes this safe
// across the 0-5 Knative replicas. Two concurrent claims must yield exactly
// one winner. Each goroutine gets its own *gorm.DB (and therefore its own
// underlying connection) built directly from TEST_DATABASE_URL, so this
// genuinely exercises two separate Postgres connections racing on the
// unique constraint rather than being serialized by a shared transaction.
func TestClaimIsSafeUnderConcurrency(t *testing.T) {
	db := newTestDB(t)
	nonce := uuid.NewString()
	expires := time.Now().Add(5 * time.Minute)

	store1 := platformauth.NewNonceStore(db)
	store2 := platformauth.NewNonceStore(newTestDB(t))

	results := make(chan bool, 2)
	errs := make(chan error, 2)

	claim := func(s platformauth.NonceStore) {
		ok, err := s.Claim(context.Background(), nonce, expires)
		errs <- err
		results <- ok
	}
	go claim(store1)
	go claim(store2)

	won := 0
	for i := 0; i < 2; i++ {
		require.NoError(t, <-errs)
		if <-results {
			won++
		}
	}
	require.Equal(t, 1, won, "exactly one claim must win")
}
