//go:build integration

package apikeys_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func sampleKey(tenantID, storeID, createdBy uuid.UUID, prefix string) apikeys.APIKey {
	return apikeys.APIKey{
		ID:              uuid.New(),
		TenantID:        tenantID,
		StoreID:         storeID,
		KeyPrefix:       prefix,
		KeyHash:         "$2a$12$abcdefghijklmnopqrstuvWXYZABCDEFGHIJKLMNOPQRSTUVwxyz0",
		Scopes:          apikeys.ScopeSet{"products:read", "orders:read"},
		RateLimitPerMin: 100,
		Label:           "Integration X",
		CreatedBy:       createdBy,
	}
}

func TestRepo_Create_AndLookupByTenantPrefix(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)

	tenantID, storeID, by := uuid.New(), uuid.New(), uuid.New()
	rec := sampleKey(tenantID, storeID, by, "ABCD1234")
	require.NoError(t, repo.Create(context.Background(), &rec))

	got, err := repo.FindByTenantPrefix(context.Background(), tenantID, "ABCD1234")
	require.NoError(t, err)
	require.Equal(t, rec.ID, got.ID)
	require.True(t, got.IsUsable(time.Now()))
	require.True(t, got.Scopes.Has("products:read"))
}

func TestRepo_FindByTenantPrefix_NotFound(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)
	_, err := repo.FindByTenantPrefix(context.Background(), uuid.New(), "ZZZZZZZZ")
	require.ErrorIs(t, err, apikeys.ErrNotFound)
}

func TestRepo_Revoke_FlipsRow(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)

	tenantID, storeID, by := uuid.New(), uuid.New(), uuid.New()
	rec := sampleKey(tenantID, storeID, by, "REVK1234")
	require.NoError(t, repo.Create(context.Background(), &rec))

	require.NoError(t, repo.Revoke(context.Background(), rec.ID, time.Now(), "compromised"))

	got, err := repo.FindByTenantPrefix(context.Background(), tenantID, "REVK1234")
	require.NoError(t, err)
	require.False(t, got.IsUsable(time.Now()))
	require.NotNil(t, got.RevokedAt)
	require.NotNil(t, got.RevokedReason)
	require.Equal(t, "compromised", *got.RevokedReason)
}

func TestRepo_RotationOverlap_StaysUsableUntilExpiry(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)

	tenantID, storeID, by := uuid.New(), uuid.New(), uuid.New()
	rec := sampleKey(tenantID, storeID, by, "ROTA1234")
	require.NoError(t, repo.Create(context.Background(), &rec))

	in24h := time.Now().Add(24 * time.Hour)
	require.NoError(t, repo.Revoke(context.Background(), rec.ID, in24h, "rotated"))

	got, err := repo.FindByTenantPrefix(context.Background(), tenantID, "ROTA1234")
	require.NoError(t, err)
	require.True(t, got.IsUsable(time.Now()), "still usable inside the 24h window")
	require.False(t, got.IsUsable(in24h.Add(time.Second)), "no longer usable past expiry")
}

func TestRepo_CountActiveForStore_ExcludesRevoked(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)

	tenantID, storeID, by := uuid.New(), uuid.New(), uuid.New()
	a := sampleKey(tenantID, storeID, by, "AAAA1111")
	b := sampleKey(tenantID, storeID, by, "BBBB2222")
	require.NoError(t, repo.Create(context.Background(), &a))
	require.NoError(t, repo.Create(context.Background(), &b))
	require.NoError(t, repo.Revoke(context.Background(), b.ID, time.Now(), "test"))

	n, err := repo.CountActiveForStore(context.Background(), tenantID, storeID)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func TestRepo_UpdateLastUsed(t *testing.T) {
	db := testdb.NewDB(t, "enterprise_api_keys")
	repo := apikeys.NewRepo(db)

	tenantID, storeID, by := uuid.New(), uuid.New(), uuid.New()
	rec := sampleKey(tenantID, storeID, by, "USED1234")
	require.NoError(t, repo.Create(context.Background(), &rec))

	now := time.Now()
	require.NoError(t, repo.UpdateLastUsed(context.Background(), rec.ID, now, "abcd1234"))

	got, err := repo.FindByID(context.Background(), rec.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.NotNil(t, got.LastUsedIPHash)
	require.Equal(t, "abcd1234", *got.LastUsedIPHash)
}
