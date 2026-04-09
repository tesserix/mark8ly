package storefront_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

type fakeSlugLookup struct {
	store *stores.Store
	err   error
}

func (f fakeSlugLookup) Get(ctx context.Context, slug string) (*stores.Store, error) {
	return f.store, f.err
}

func init() { gin.SetMode(gin.TestMode) }

func runReqWithKey(t *testing.T, secret, header string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.Use(storefront.RequireStorefrontKey(secret))
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if header != "" {
		req.Header.Set("X-Storefront-Key", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireStorefrontKey_EmptySecret_Passthrough(t *testing.T) {
	w := runReqWithKey(t, "", "")
	if w.Code != 200 {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestRequireStorefrontKey_MissingHeader_404(t *testing.T) {
	w := runReqWithKey(t, "abc", "")
	if w.Code != 404 {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestRequireStorefrontKey_MismatchedHeader_404(t *testing.T) {
	w := runReqWithKey(t, "abc", "xyz")
	if w.Code != 404 {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestRequireStorefrontKey_MatchingHeader_Passthrough(t *testing.T) {
	w := runReqWithKey(t, "abc", "abc")
	if w.Code != 200 {
		t.Fatalf("code: %d", w.Code)
	}
}

func runStoreContext(t *testing.T, lookup storefront.SlugLookup) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.GET("/s/:storeSlug", storefront.StoreContext(lookup), func(c *gin.Context) {
		s, ok := c.Get("store")
		if !ok || s == nil {
			c.String(500, "no store")
			return
		}
		c.String(200, s.(*stores.Store).ID)
	})
	req := httptest.NewRequest(http.MethodGet, "/s/acme", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStoreContext_ActiveStore_SetsContext(t *testing.T) {
	w := runStoreContext(t, fakeSlugLookup{store: &stores.Store{ID: "s-1", Status: stores.StatusActive}})
	if w.Code != 200 {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "s-1" {
		t.Fatalf("body: %s", w.Body.String())
	}
}

func TestStoreContext_MissingStore_404(t *testing.T) {
	w := runStoreContext(t, fakeSlugLookup{err: stores.ErrNotFound})
	if w.Code != 404 {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestStoreContext_SuspendedStore_404(t *testing.T) {
	w := runStoreContext(t, fakeSlugLookup{store: &stores.Store{ID: "s-1", Status: stores.StatusSuspended}})
	if w.Code != 404 {
		t.Fatalf("code: %d", w.Code)
	}
}

func TestStoreContext_CacheError_500(t *testing.T) {
	w := runStoreContext(t, fakeSlugLookup{err: errors.New("boom")})
	if w.Code != 500 {
		t.Fatalf("code: %d", w.Code)
	}
}
