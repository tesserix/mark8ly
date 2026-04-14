//go:build integration

package vendor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestRepository_GetSelfByTenantID_NotFound(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	v, err := repo.GetSelfByTenantID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestRepository_CreateAndGetSelf(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := "11111111-1111-1111-1111-111111111111"
	in := &Vendor{
		TenantID: tenantID,
		Name:     "Acme",
		Slug:     "acme-self",
		IsSelf:   true,
	}
	require.NoError(t, repo.Create(context.Background(), in))
	require.NotEmpty(t, in.ID)

	got, err := repo.GetSelfByTenantID(context.Background(), tenantID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Acme", got.Name)
	require.True(t, got.IsSelf)

	byID, err := repo.GetByID(context.Background(), in.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	require.Equal(t, in.ID, byID.ID)
}

func TestRepository_Create_SelfVendorUniquePerTenant(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := "22222222-2222-2222-2222-222222222222"
	require.NoError(t, repo.Create(context.Background(), &Vendor{
		TenantID: tenantID, Name: "A", Slug: "slug-a-" + tenantID, IsSelf: true,
	}))
	err := repo.Create(context.Background(), &Vendor{
		TenantID: tenantID, Name: "B", Slug: "slug-b-" + tenantID, IsSelf: true,
	})
	require.Error(t, err, "inserting a second self-vendor should violate the partial unique index")
}

func TestRepository_UpdateNameAndSlug(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := "33333333-3333-3333-3333-333333333333"
	v := &Vendor{TenantID: tenantID, Name: "Merchant", Slug: "placeholder-" + tenantID, IsSelf: true}
	require.NoError(t, repo.Create(context.Background(), v))

	require.NoError(t, repo.UpdateNameAndSlug(context.Background(), v.ID, "Acme", "acme-"+tenantID))

	got, err := repo.GetByID(context.Background(), v.ID)
	require.NoError(t, err)
	require.Equal(t, "Acme", got.Name)
	require.Equal(t, "acme-"+tenantID, got.Slug)
}
