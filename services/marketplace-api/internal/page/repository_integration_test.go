//go:build integration

package page

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestRepository_Create_GetByID(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()
	p := &Page{
		TenantID: tenantID,
		StoreID:  storeID,
		Slug:     "about",
		Title:    "About Us",
		Body:     "hello world",
	}
	require.NoError(t, repo.Create(context.Background(), p))
	require.NotEqual(t, uuid.Nil, p.ID)

	got, err := repo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "About Us", got.Title)
	require.Equal(t, "hello world", got.Body)
	require.True(t, got.Published, "default published=true")
}

func TestRepository_GetBySlug_PublishedOnly(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()
	p := &Page{
		TenantID:  tenantID,
		StoreID:   storeID,
		Slug:      "terms",
		Title:     "Terms",
		Body:      "terms body",
		Published: false,
	}
	require.NoError(t, repo.Create(context.Background(), p))

	// publishedOnly=true should return nil for an unpublished page
	got, err := repo.GetBySlug(context.Background(), storeID, "terms", true)
	require.NoError(t, err)
	require.Nil(t, got, "unpublished page should not be returned when publishedOnly=true")

	// publishedOnly=false should return the row
	got, err = repo.GetBySlug(context.Background(), storeID, "terms", false)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Terms", got.Title)
}

func TestRepository_ListByStore(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()
	otherStoreID := uuid.New()

	pages := []*Page{
		{TenantID: tenantID, StoreID: storeID, Slug: "faq", Title: "FAQ", SortOrder: 2},
		{TenantID: tenantID, StoreID: storeID, Slug: "about", Title: "About", SortOrder: 1},
		{TenantID: tenantID, StoreID: storeID, Slug: "contact", Title: "Contact", SortOrder: 2},
		{TenantID: tenantID, StoreID: otherStoreID, Slug: "other", Title: "Other Store Page", SortOrder: 0},
	}
	for _, pg := range pages {
		require.NoError(t, repo.Create(context.Background(), pg))
	}

	list, err := repo.ListByStore(context.Background(), storeID, false)
	require.NoError(t, err)
	require.Len(t, list, 3, "other store page should be excluded")

	// First must be sort_order=1 (About)
	require.Equal(t, "About", list[0].Title)
	// sort_order=2 rows sorted by title: Contact before FAQ
	require.Equal(t, "Contact", list[1].Title)
	require.Equal(t, "FAQ", list[2].Title)

	// Unpublished filtering: mark FAQ as unpublished
	require.NoError(t, repo.Update(context.Background(), pages[0].ID, map[string]any{"published": false}))
	pubList, err := repo.ListByStore(context.Background(), storeID, true)
	require.NoError(t, err)
	require.Len(t, pubList, 2, "unpublished FAQ should be excluded with publishedOnly=true")
}

func TestRepository_Update(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()
	p := &Page{
		TenantID: tenantID,
		StoreID:  storeID,
		Slug:     "privacy",
		Title:    "Privacy Policy",
		Body:     "original body",
	}
	require.NoError(t, repo.Create(context.Background(), p))

	require.NoError(t, repo.Update(context.Background(), p.ID, map[string]any{"title": "Updated Privacy Policy"}))

	got, err := repo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Privacy Policy", got.Title)
	require.Equal(t, "original body", got.Body, "body should be unchanged after patch")
}

func TestRepository_Delete(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()
	p := &Page{
		TenantID: tenantID,
		StoreID:  storeID,
		Slug:     "to-delete",
		Title:    "Delete Me",
	}
	require.NoError(t, repo.Create(context.Background(), p))

	require.NoError(t, repo.Delete(context.Background(), p.ID))

	got, err := repo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Nil(t, got, "page should not exist after delete")
}

func TestRepository_Create_DuplicateSlug_Errors(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository(tx)

	tenantID := uuid.New()
	storeID := uuid.New()

	require.NoError(t, repo.Create(context.Background(), &Page{
		TenantID: tenantID,
		StoreID:  storeID,
		Slug:     "dup-slug",
		Title:    "First",
	}))

	// Same store_id + same slug → must error (unique index violation)
	require.NoError(t, tx.Exec("SAVEPOINT sp").Error)
	err := repo.Create(context.Background(), &Page{
		TenantID: tenantID,
		StoreID:  storeID,
		Slug:     "dup-slug",
		Title:    "Second",
	})
	require.Error(t, err, "inserting duplicate (store_id, slug) should violate the unique index")
	require.True(t, errors.Is(err, apperrors.ErrSlugTaken),
		"expected SlugTaken, got %v", err)
	require.NoError(t, tx.Exec("ROLLBACK TO SAVEPOINT sp").Error)

	// Different store_id with same slug → must succeed
	otherStoreID := uuid.New()
	require.NoError(t, repo.Create(context.Background(), &Page{
		TenantID: tenantID,
		StoreID:  otherStoreID,
		Slug:     "dup-slug",
		Title:    "Other Store Same Slug",
	}), "same slug in a different store should be allowed")
}
