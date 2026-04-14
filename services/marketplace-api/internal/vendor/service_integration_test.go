//go:build integration

package vendor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newTestService(t *testing.T) *Service {
	tx := testdb.NewTx(t)
	return NewService(NewRepository(tx))
}

func TestService_EnsureSelfVendor_CreatesOnFirstCall(t *testing.T) {
	svc := newTestService(t)
	tenantID := "33333333-3333-3333-3333-333333333333"

	v, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Acme", "acme-"+tenantID)
	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "Acme", v.Name)
	require.Equal(t, "acme-"+tenantID, v.Slug)
	require.True(t, v.IsSelf)
	require.Equal(t, "active", string(v.Status))
}

func TestService_EnsureSelfVendor_IdempotentOnSecondCall(t *testing.T) {
	svc := newTestService(t)
	tenantID := "44444444-4444-4444-4444-444444444444"
	slug := "acme-" + tenantID

	first, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Acme", slug)
	require.NoError(t, err)

	second, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Renamed", "new-slug-"+tenantID)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "idempotent — same vendor row returned")
	require.Equal(t, "Acme", second.Name, "existing name is NOT overwritten by Ensure")
	require.Equal(t, slug, second.Slug, "existing slug is NOT overwritten by Ensure")
}

func TestService_EnsureSelfVendor_RejectsEmptyInput(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.EnsureSelfVendor(ctx, "", "Acme", "acme")
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))

	_, err = svc.EnsureSelfVendor(ctx, "tenant", "", "acme")
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))

	_, err = svc.EnsureSelfVendor(ctx, "tenant", "Acme", "")
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))
}

func TestService_UpdateSelfVendor_OverwritesNameAndSlug(t *testing.T) {
	svc := newTestService(t)
	tenantID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	_, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Merchant", "placeholder-"+tenantID)
	require.NoError(t, err)

	updated, err := svc.UpdateSelfVendor(context.Background(), tenantID, "Acme", "acme-"+tenantID)
	require.NoError(t, err)
	require.Equal(t, "Acme", updated.Name)
	require.Equal(t, "acme-"+tenantID, updated.Slug)
}

func TestService_UpdateSelfVendor_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.UpdateSelfVendor(context.Background(), "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "Acme", "acme")
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrNotFound))
}

func TestService_UpdateSelfVendor_RejectsEmptyInput(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.UpdateSelfVendor(ctx, "", "Acme", "acme")
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))

	_, err = svc.UpdateSelfVendor(ctx, "tenant", "", "acme")
	require.Error(t, err)

	_, err = svc.UpdateSelfVendor(ctx, "tenant", "Acme", "")
	require.Error(t, err)
}
