//go:build integration

package page

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func newTestService(t *testing.T) *Service {
	tx := testdb.NewTx(t)
	return NewService(NewRepository(tx))
}

func TestService_Create_RejectsInvalidSlug(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Space is invalid.
	_, err := svc.Create(ctx, CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "Foo Bar", Title: "Foo",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))

	// Leading hyphen invalid.
	_, err = svc.Create(ctx, CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "-about", Title: "About",
	})
	require.Error(t, err)

	// Upper case invalid.
	_, err = svc.Create(ctx, CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "About", Title: "About",
	})
	require.Error(t, err)

	// Valid slug works.
	p, err := svc.Create(ctx, CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "about-us", Title: "About",
	})
	require.NoError(t, err)
	require.Equal(t, "about-us", p.Slug)
}

func TestService_Create_RejectsOversizeTitle(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "about", Title: strings.Repeat("a", 201),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))
}

func TestService_Create_DefaultsPublishedToTrue(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.Create(context.Background(), CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "about", Title: "About",
	})
	require.NoError(t, err)
	require.True(t, p.Published)
}

func TestService_Update_PartialPatch(t *testing.T) {
	svc := newTestService(t)
	tenant := uuid.New()
	store := uuid.New()
	p, err := svc.Create(context.Background(), CreateInput{
		TenantID: tenant, StoreID: store,
		Slug: "about", Title: "About", Body: "original body",
	})
	require.NoError(t, err)

	newTitle := "About Us"
	updated, err := svc.Update(context.Background(), p.ID, UpdateInput{Title: &newTitle})
	require.NoError(t, err)
	require.Equal(t, "About Us", updated.Title)
	require.Equal(t, "original body", updated.Body, "body untouched by partial patch")
}

func TestService_Create_RejectsOversizeBody(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "about", Title: "About", Body: strings.Repeat("a", 200_001),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))
}

func TestService_Update_RejectsOversizeBody(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.Create(context.Background(), CreateInput{
		TenantID: uuid.New(), StoreID: uuid.New(),
		Slug: "about", Title: "About",
	})
	require.NoError(t, err)
	big := strings.Repeat("a", 200_001)
	_, err = svc.Update(context.Background(), p.ID, UpdateInput{Body: &big})
	require.Error(t, err)
}

func TestService_GetBySlug_FiltersUnpublishedOnStorefrontRead(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	store := uuid.New()
	draft := false
	p, err := svc.Create(ctx, CreateInput{
		TenantID: uuid.New(), StoreID: store,
		Slug: "coming-soon", Title: "Coming Soon",
		Published: &draft,
	})
	require.NoError(t, err)

	// Storefront read: unpublished → nil.
	got, err := svc.GetBySlugForStorefront(ctx, store, "coming-soon")
	require.NoError(t, err)
	require.Nil(t, got, "unpublished page must not leak to storefront")

	// Admin read: unpublished visible.
	got2, err := svc.GetByID(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got2)
	require.False(t, got2.Published)
}
