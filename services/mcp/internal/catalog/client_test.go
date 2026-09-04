package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tesserix/go-shared/mcp/upstream"
)

// The OpenAPI document this replaces claimed limit/offset. The handler reads
// page/page_size. Sending the wrong names does not error — it silently returns
// page 1 forever, which is why this test asserts the WIRE, not the result.
func TestListProducts_SendsPageParamsNotLimitOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/products", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("page"))
		assert.Equal(t, "50", r.URL.Query().Get("page_size"))
		assert.Empty(t, r.URL.Query().Get("limit"), "limit is not a parameter this API reads")
		assert.Empty(t, r.URL.Query().Get("offset"), "offset is not a parameter this API reads")
		assert.Equal(t, "sfkey", r.Header.Get("X-Storefront-Key"))
		_, _ = w.Write([]byte(`{"data":[{"handle":"mug","title":"Mug","price_range":{"min":"9.5","max":"9.5","currency_code":"AUD"}}],"meta":{"page":2,"page_size":50}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.ListProducts(context.Background(), "bondi", 2, 50)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mug", got[0].Handle)
}

// The list endpoints wrap results in {"data": …}. Decoding into a bare slice
// yields zero products and no error — a wrong answer that looks like an empty
// catalogue.
func TestListCategories_UnwrapsTheDataEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"Mugs","slug":"mugs","position":1,"featured":true}]}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.ListCategories(context.Background(), "bondi")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mugs", got[0].Slug)
}

func TestGetProduct_UnknownHandleIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), "bondi", "nope")
	require.ErrorIs(t, err, upstream.ErrNotFound)
}

// page_size above the handler's cap must be rejected here rather than sent.
// An agent told it received 500 products when it received 100 will summarise
// a catalogue it never saw.
func TestListProducts_RejectsPageSizeAboveCap(t *testing.T) {
	c, err := NewClient("http://example.invalid", "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.ListProducts(context.Background(), "bondi", 1, 500)
	require.Error(t, err)
	require.Contains(t, err.Error(), "100")
}

// ListByCategory shares the cap with ListProducts — same handler-side bind.
func TestListByCategory_RejectsPageSizeAboveCap(t *testing.T) {
	c, err := NewClient("http://example.invalid", "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.ListByCategory(context.Background(), "bondi", "mugs", 1, 500)
	require.Error(t, err)
	require.Contains(t, err.Error(), "100")
}

// GetProduct must not send page/page_size at all — it is a single-resource
// fetch by handle, not a list.
func TestGetProduct_HitsHandlePathAndDecodesBareObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/products/mug", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"p1","handle":"mug","title":"Mug","price_range":{"min":"9.5","max":"9.5","currency_code":"AUD"}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.GetProduct(context.Background(), "bondi", "mug")
	require.NoError(t, err)
	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "9.5", got.PriceRange.Min)
}

// Store slug and product handle come from an agent. A raw path separator
// inside either must be rejected outright, not escaped-and-forwarded — a
// slug/handle legitimately never contains "/", and rejecting keeps the
// segment validator's contract simple: nothing containing "/" survives it,
// escaped or not.
func TestGetProduct_RejectsSlashInSegments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should have been sent for a slug/handle containing a path separator; got %s", r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), "bon/di", "mug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")

	_, err = c.GetProduct(context.Background(), "bondi", "mu/g")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handle")
}

// A normal slug and handle still produce the correct escaped path once they
// contain no separator or dot-segment to reject.
func TestGetProduct_NormalSlugAndHandleProduceCorrectPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/products/mug", r.URL.Path)
		_, _ = w.Write([]byte(`{"handle":"mug","price_range":{"min":"1","max":"1","currency_code":"AUD"}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), "bondi", "mug")
	require.NoError(t, err)
}

func TestListByCategory_HitsCategoryPathWithPageParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/categories/mugs/products", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		assert.Equal(t, "20", r.URL.Query().Get("page_size"))
		_, _ = w.Write([]byte(`{"data":[{"handle":"mug","price_range":{"min":"1","max":"1","currency_code":"AUD"}}],"meta":{"page":1,"page_size":20}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.ListByCategory(context.Background(), "bondi", "mugs", 1, 20)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mug", got[0].Handle)
}

// GetBranding: the real handler (branding.go's Get) nests branding fields
// under a "branding" key and reports at most one active promotion as a
// singular "active_promotion" object sibling key — not an
// "active_promotions" array inside branding, as the replaced OpenAPI
// document and an earlier draft of this brief assumed. This test pins the
// actual wire shape.
func TestGetBranding_UnwrapsRealResponseShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/branding", r.URL.Path)
		_, _ = w.Write([]byte(`{
			"branding": {
				"logo_url": "https://example.com/logo.png",
				"tagline": "Handmade goods",
				"color_accent": "#2D4A2B",
				"announcement_text": "Free shipping over $50"
			},
			"store": {"name": "Bondi", "slug": "bondi", "currency_code": "AUD", "country_code": "AU"},
			"active_promotion": {"label": "Winter sale", "coupon_code": "WINTER10"}
		}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.GetBranding(context.Background(), "bondi")
	require.NoError(t, err)
	require.NotNil(t, got.Tagline)
	assert.Equal(t, "Handmade goods", *got.Tagline)
	assert.Equal(t, "#2D4A2B", got.ColorAccent)
	require.Len(t, got.Promotions, 1)
	assert.Equal(t, "Winter sale", got.Promotions[0].Label)
	require.NotNil(t, got.Promotions[0].CouponCode)
	assert.Equal(t, "WINTER10", *got.Promotions[0].CouponCode)
}

// No active promotion: the sibling key is absent from the response
// entirely. Promotions must decode as empty, not error or panic.
// A store slug of ".." must never reach path.Clean inside upstream.Get,
// which would otherwise collapse "/stores/../products/mug" into
// "/products/mug" — a different, unscoped route. The base URL here has no
// path component, so upstream.Get's own traversal guard is inert; this
// client must reject the segment itself before a request is built.
func TestListProducts_RejectsDotDotSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should have been sent for a %q slug; got %s", "..", r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.ListProducts(context.Background(), "..", 1, 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

func TestGetProduct_RejectsDotDotHandle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should have been sent for a %q handle; got %s", "..", r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), "bondi", "..")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handle")
}

func TestGetProduct_RejectsDotSlug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should have been sent for a %q slug; got %s", ".", r.URL.Path)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	_, err = c.GetProduct(context.Background(), ".", "mug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

func TestGetBranding_NoActivePromotionYieldsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"branding": {"color_accent": "#2D4A2B"},
			"store": {"name": "Bondi", "slug": "bondi", "currency_code": "AUD", "country_code": "AU"}
		}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)

	got, err := c.GetBranding(context.Background(), "bondi")
	require.NoError(t, err)
	assert.Empty(t, got.Promotions)
}
