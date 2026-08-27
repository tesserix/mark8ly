//go:build integration

package apikeys_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/ipprivacy"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestLastUsedWorker_PersistsUpdate(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)
	hasher := ipprivacy.New([]byte("test-key"))
	w := apikeys.NewLastUsedWorker(repo, hasher, slog.Default(), 16)

	tenantID, storeID := uuid.New(), uuid.New()
	key := apikeys.APIKey{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		KeyPrefix: "USED1234",
		KeyHash:   "$2a$12$abcdefghijklmnopqrstuvWXYZABCDEFGHIJKLMNOPQRSTUVwxyz0",
		Scopes:    apikeys.ScopeSet{"products:read"},
		Label:     "x", CreatedBy: uuid.New(),
	}
	require.NoError(t, repo.Create(context.Background(), &key))

	w.Submit(key.ID, "203.0.113.5")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Stop(ctx)

	got, err := repo.FindByID(context.Background(), key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.NotNil(t, got.LastUsedIPHash)
	require.Len(t, *got.LastUsedIPHash, 64)
}

func TestLastUsedWorker_DropOnFullBuffer(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)
	w := apikeys.NewLastUsedWorker(repo, nil, slog.Default(), 1)

	// Hammer the buffer; some submits will be dropped without panic.
	for i := 0; i < 100; i++ {
		w.Submit(uuid.New(), "203.0.113.5")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w.Stop(ctx)
}

func TestLastUsedWorker_NilSafe(t *testing.T) {
	var w *apikeys.LastUsedWorker
	w.Submit(uuid.New(), "203.0.113.5")
	w.Stop(context.Background())
}
