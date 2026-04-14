//go:build integration

package admin_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/page"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// pagesTables is the dependency-ordered truncate list for pages tests.
var pagesTables = []string{
	"pages",
	"stores",
}

// pagesTestEnv bundles the test router, db, and fga fake for pages tests.
type pagesTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
	fga    *authz.FakeClient
}

func setupPagesRouter(t *testing.T) *pagesTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, pagesTables...)

	storesRepo := stores.NewRepository(db)
	pageRepo := page.NewRepository(db)
	pageSvc := page.NewService(pageRepo)

	fga := authz.NewFakeClient()
	storeMW := stores.StoreMiddleware(stores.MiddlewareConfig{
		Repo:   storesRepo,
		Client: stubClient{},
		Flight: &singleflight.Group{},
	})
	authzMW := authz.NewMiddleware(fga, nil)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	pagesHandler := admin.NewPagesHandler(pageSvc, logger)

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		PagesHandler:     pagesHandler,
		StoresMiddleware: storeMW,
		AuthzMiddleware:  authzMW,
		InternalSecret:   "",
	})

	return &pagesTestEnv{router: r, db: db, fga: fga}
}

func seedPageRow(t *testing.T, db *gorm.DB, storeID, tenantID, slug, title string, published bool) string {
	t.Helper()
	repo := page.NewRepository(db)
	svc := page.NewService(repo)
	pub := published
	p, err := svc.Create(context.Background(), page.CreateInput{
		TenantID:  uuid.MustParse(tenantID),
		StoreID:   uuid.MustParse(storeID),
		Slug:      slug,
		Title:     title,
		Body:      "test body",
		Published: &pub,
	})
	if err != nil {
		t.Fatalf("seed page: %v", err)
	}
	return p.ID.String()
}

func seedPagesStore(t *testing.T, db *gorm.DB) (storeID, tenantID string) {
	t.Helper()
	tenantID = uuid.NewString()
	storeID = uuid.NewString()
	s := &stores.Store{
		ID:           storeID,
		TenantID:     tenantID,
		Slug:         "pages-" + storeID[:8],
		Name:         "Pages Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     time.Now(),
	}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return
}

func pagesAdminURL(storeID string) string {
	return "/api/v1/admin/stores/" + storeID + "/pages"
}

// TestAdminPages_Create_Get verifies POST creates a page and GET returns it.
func TestAdminPages_Create_Get(t *testing.T) {
	env := setupPagesRouter(t)
	storeID, tenantID := seedPagesStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	body := map[string]any{
		"slug":  "about-us",
		"title": "About Us",
		"body":  "We are a test store.",
	}
	w := request(t, env.router, http.MethodPost, pagesAdminURL(storeID), body, authHeaders(userID, tenantID))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id, _ := createResp["id"].(string)
	if id == "" {
		t.Fatalf("missing id in response: %s", w.Body.String())
	}
	if createResp["slug"] != "about-us" {
		t.Fatalf("slug = %v", createResp["slug"])
	}

	w2 := request(t, env.router, http.MethodGet, pagesAdminURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w2.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", w2.Code, w2.Body.String())
	}
	var getResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &getResp)
	if getResp["title"] != "About Us" {
		t.Fatalf("title = %v", getResp["title"])
	}
}

// TestAdminPages_List_IncludesDrafts verifies admin list sees unpublished pages.
func TestAdminPages_List_IncludesDrafts(t *testing.T) {
	env := setupPagesRouter(t)
	storeID, tenantID := seedPagesStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleStaff, tenantID)

	seedPageRow(t, env.db, storeID, tenantID, "published-page", "Published Page", true)
	seedPageRow(t, env.db, storeID, tenantID, "draft-page", "Draft Page", false)

	w := request(t, env.router, http.MethodGet, pagesAdminURL(storeID), nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Fatalf("expected 2 pages (including draft), got %d: %s", len(resp), w.Body.String())
	}
}

// TestAdminPages_Update_PartialPatch verifies PATCH only title keeps body.
func TestAdminPages_Update_PartialPatch(t *testing.T) {
	env := setupPagesRouter(t)
	storeID, tenantID := seedPagesStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := seedPageRow(t, env.db, storeID, tenantID, "privacy-policy", "Privacy Policy", true)

	patch := map[string]any{"title": "Privacy & Policy"}
	w := request(t, env.router, http.MethodPatch, pagesAdminURL(storeID)+"/"+id, patch, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["title"] != "Privacy & Policy" {
		t.Fatalf("title = %v", resp["title"])
	}
	if resp["body"] != "test body" {
		t.Fatalf("body changed unexpectedly: %v", resp["body"])
	}
}

// TestAdminPages_Delete_Returns204 verifies DELETE returns 204 and page is gone.
func TestAdminPages_Delete_Returns204(t *testing.T) {
	env := setupPagesRouter(t)
	storeID, tenantID := seedPagesStore(t, env.db)
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)

	id := seedPageRow(t, env.db, storeID, tenantID, "contact-us", "Contact Us", true)

	w := request(t, env.router, http.MethodDelete, pagesAdminURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}

	w2 := request(t, env.router, http.MethodGet, pagesAdminURL(storeID)+"/"+id, nil, authHeaders(userID, tenantID))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("post-delete get status = %d, expected 404", w2.Code)
	}
}
