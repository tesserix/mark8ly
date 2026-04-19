//go:build integration

package apikeys_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newTestService(t *testing.T) (*apikeys.Service, *gorm.DB) {
	t.Helper()
	db := testdb.NewDB(t, "enterprise_api_keys")
	return apikeys.NewService(db, apikeys.NewRepo(db), apikeys.NewCache(60*time.Second), apikeys.EnvLive), db
}

func validProInput() apikeys.CreateInput {
	return apikeys.CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(), CreatedBy: uuid.New(),
		Scopes: []string{"products:read", "products:write"},
		RateLimitPerMin: 500, Label: "Integration A",
		Plan: subscription.PlanPro,
	}
}

func TestService_Create_PersistsHashOnly_PlaintextReturnedOnce(t *testing.T) {
	svc, db := newTestService(t)
	in := validProInput()

	out, err := svc.Create(context.Background(), in)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out.Plaintext, "mk8_live_"))
	require.NotEqual(t, uuid.Nil, out.ID)

	var row apikeys.APIKey
	require.NoError(t, db.First(&row, "id = ?", out.ID).Error)
	require.NotEqual(t, out.Plaintext, row.KeyHash)
	require.NoError(t, apikeys.Verify(row.KeyHash, out.Plaintext))
}

func TestService_Create_StudioCannotCreateWriteScope(t *testing.T) {
	svc, _ := newTestService(t)
	in := apikeys.CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(), CreatedBy: uuid.New(),
		Scopes: []string{"products:read", "products:write"},
		RateLimitPerMin: 100, Label: "X",
		Plan: subscription.PlanStudio,
	}
	_, err := svc.Create(context.Background(), in)
	require.ErrorIs(t, err, apikeys.ErrWriteScopeRequiresPro)
}

func TestService_Create_StarterRejected(t *testing.T) {
	svc, _ := newTestService(t)
	in := apikeys.CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(), CreatedBy: uuid.New(),
		Scopes: []string{"products:read"}, RateLimitPerMin: 100, Label: "X",
		Plan: subscription.PlanStarter,
	}
	_, err := svc.Create(context.Background(), in)
	require.ErrorIs(t, err, apikeys.ErrPlanDoesNotAllowAPI)
}

func TestService_Create_RateLimitExceedsCeiling(t *testing.T) {
	svc, _ := newTestService(t)
	in := apikeys.CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(), CreatedBy: uuid.New(),
		Scopes: []string{"products:read"}, RateLimitPerMin: 99999, Label: "X",
		Plan: subscription.PlanStudio,
	}
	_, err := svc.Create(context.Background(), in)
	require.ErrorIs(t, err, apikeys.ErrRateLimitExceedsPlanCeiling)
}

func TestService_Rotate_OldKeyValidFor24h(t *testing.T) {
	svc, db := newTestService(t)
	in := validProInput()
	orig, err := svc.Create(context.Background(), in)
	require.NoError(t, err)

	rot, err := svc.Rotate(context.Background(), in.TenantID, orig.ID, "scheduled_rotation")
	require.NoError(t, err)
	require.NotEqual(t, orig.Plaintext, rot.NewPlaintext)

	var oldRow apikeys.APIKey
	require.NoError(t, db.First(&oldRow, "id = ?", orig.ID).Error)
	require.NotNil(t, oldRow.RevokedAt)
	require.True(t, oldRow.RevokedAt.After(time.Now().Add(23*time.Hour)))
	require.True(t, oldRow.RevokedAt.Before(time.Now().Add(25*time.Hour)))

	var newRow apikeys.APIKey
	require.NoError(t, db.First(&newRow, "id = ?", rot.NewID).Error)
	require.NotNil(t, newRow.RotationReplaces)
	require.Equal(t, orig.ID, *newRow.RotationReplaces)
}

func TestService_Rotate_RejectsRevoked(t *testing.T) {
	svc, _ := newTestService(t)
	in := validProInput()
	orig, err := svc.Create(context.Background(), in)
	require.NoError(t, err)
	require.NoError(t, svc.Revoke(context.Background(), in.TenantID, orig.ID, "compromised"))

	_, err = svc.Rotate(context.Background(), in.TenantID, orig.ID, "scheduled_rotation")
	require.ErrorIs(t, err, apikeys.ErrCannotRotateRevoked)
}

func TestService_Revoke_SetsRevokedAtImmediately(t *testing.T) {
	svc, db := newTestService(t)
	in := validProInput()
	k, err := svc.Create(context.Background(), in)
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(context.Background(), in.TenantID, k.ID, "compromised"))

	var row apikeys.APIKey
	require.NoError(t, db.First(&row, "id = ?", k.ID).Error)
	require.NotNil(t, row.RevokedAt)
	require.False(t, row.IsUsable(time.Now()))
}

func TestService_Revoke_OtherTenantNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	in := validProInput()
	k, err := svc.Create(context.Background(), in)
	require.NoError(t, err)

	other := uuid.New()
	err = svc.Revoke(context.Background(), other, k.ID, "x")
	require.ErrorIs(t, err, apikeys.ErrNotFound)
}

func TestService_List_FiltersByTenantStore(t *testing.T) {
	svc, _ := newTestService(t)
	in := validProInput()
	_, err := svc.Create(context.Background(), in)
	require.NoError(t, err)

	got, err := svc.List(context.Background(), in.TenantID, in.StoreID)
	require.NoError(t, err)
	require.Len(t, got, 1)

	none, err := svc.List(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	require.Empty(t, none)
}
