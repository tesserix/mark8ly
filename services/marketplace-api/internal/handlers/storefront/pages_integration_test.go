//go:build integration

package storefront_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// pagesSfTables is the truncate list for storefront pages tests.
var pagesSfTables = []string{
	"pages",
	"store_watermarks",
	"stores",
}

// setupPagesStorefrontRouter builds a minimal storefront router with the
// pages handler registered. It mirrors the pattern in storefront_integration_test.go.
func setupPagesStorefrontRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := testdb.NewDB(t, pagesSfTables...)

	storesRepo := stores.NewRepository(db)
	slugCache := stores.NewSlugCache(storesRepo, fakeStoresClient{}, &singleflight.Group{}, 0)
	pageRepo := page.NewRepository(db)
	pageSvc := page.NewService(pageRepo)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pagesHandler := storefront.NewPagesHandler(pageSvc, logger)

	r := gin.New()
	storefront.RegisterStorefront(r.Group("/api/v1"), storefront.Deps{
		SlugCache:    slugCache,
		PagesHandler: pagesHandler,
	})
	return r, db
}

func seedPageForStorefront(t *testing.T, db *gorm.DB, storeID, tenantID, slug, title string, published bool) {
	t.Helper()
	repo := page.NewRepository(db)
	svc := page.NewService(repo)
	pub := published
	_, err := svc.Create(context.Background(), page.CreateInput{
		TenantID:  uuid.MustParse(tenantID),
		StoreID:   uuid.MustParse(storeID),
		Slug:      slug,
		Title:     title,
		Body:      "page body content",
		Published: &pub,
	})
	if err != nil {
		t.Fatalf("seed page: %v", err)
	}
}

// TestStorefrontPages_Get_HappyPath verifies published page returns 200 with correct fields.
func TestStorefrontPages_Get_HappyPath(t *testing.T) {
	r, db := setupPagesStorefrontRouter(t)
	storeID, tenantID := seedStorefrontStore(t, db, "sf-pages-happy", "USD")

	seedPageForStorefront(t, db, storeID, tenantID, "about-us", "About Us", true)

	w := request(t, r, http.MethodGet, "/api/v1/storefront/stores/sf-pages-happy/pages/about-us", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["slug"] != "about-us" {
		t.Fatalf("slug = %v", resp["slug"])
	}
	if resp["title"] != "About Us" {
		t.Fatalf("title = %v", resp["title"])
	}
	if resp["body"] != "page body content" {
		t.Fatalf("body = %v", resp["body"])
	}
}

// TestStorefrontPages_Get_UnpublishedReturns404 verifies draft pages are hidden.
func TestStorefrontPages_Get_UnpublishedReturns404(t *testing.T) {
	r, db := setupPagesStorefrontRouter(t)
	storeID, tenantID := seedStorefrontStore(t, db, "sf-pages-draft", "USD")

	seedPageForStorefront(t, db, storeID, tenantID, "hidden-page", "Hidden Page", false)

	w := request(t, r, http.MethodGet, "/api/v1/storefront/stores/sf-pages-draft/pages/hidden-page", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unpublished page, got %d: %s", w.Code, w.Body.String())
	}
}

// TestStorefrontPages_Get_WrongStoreReturns404 verifies cross-store isolation.
func TestStorefrontPages_Get_WrongStoreReturns404(t *testing.T) {
	r, db := setupPagesStorefrontRouter(t)
	storeA, tenantA := seedStorefrontStore(t, db, "sf-pages-store-a", "USD")
	seedStorefrontStore(t, db, "sf-pages-store-b", "USD")

	seedPageForStorefront(t, db, storeA, tenantA, "store-a-page", "Store A Page", true)

	// Query store-b for store-a's page slug — must return 404.
	w := request(t, r, http.MethodGet, "/api/v1/storefront/stores/sf-pages-store-b/pages/store-a-page", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 (cross-store), got %d: %s", w.Code, w.Body.String())
	}
}

// TestStorefrontPages_List_OnlyPublished verifies drafts are excluded from list.
func TestStorefrontPages_List_OnlyPublished(t *testing.T) {
	r, db := setupPagesStorefrontRouter(t)
	storeID, tenantID := seedStorefrontStore(t, db, "sf-pages-list", "USD")

	seedPageForStorefront(t, db, storeID, tenantID, "visible-page", "Visible Page", true)
	seedPageForStorefront(t, db, storeID, tenantID, "another-visible", "Another Visible", true)
	seedPageForStorefront(t, db, storeID, tenantID, "draft-page", "Draft Page", false)

	w := request(t, r, http.MethodGet, "/api/v1/storefront/stores/sf-pages-list/pages", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("want 2 published pages, got %d: %s", len(resp), w.Body.String())
	}
	// Verify only slug + title are present (summary shape).
	for _, item := range resp {
		if _, ok := item["slug"]; !ok {
			t.Fatalf("missing slug in summary: %v", item)
		}
		if _, ok := item["title"]; !ok {
			t.Fatalf("missing title in summary: %v", item)
		}
		if _, ok := item["body"]; ok {
			t.Fatalf("body should not appear in list summary: %v", item)
		}
	}
}
