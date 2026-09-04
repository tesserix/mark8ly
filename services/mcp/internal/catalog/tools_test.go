package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/upstream"
)

func TestRegisterTools_RegistersExactlyTheFive(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, &Client{}))

	assert.Equal(t, []string{
		"get_store_branding",
		"get_store_product",
		"list_products_by_category",
		"list_store_categories",
		"list_store_products",
	}, r.Names())
}

// Store scoping is by slug. A tool that accepted an internal id would invite
// cross-store probing, and the id is not a public identifier.
func TestRegisterTools_NoToolAcceptsAnInternalIdentifier(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, &Client{}))

	for _, tool := range r.Tools() {
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for name := range props {
			assert.NotContains(t, name, "store_id", "tool %s", tool.Name)
			assert.NotContains(t, name, "tenant", "tool %s", tool.Name)
			assert.NotContains(t, name, "_uuid", "tool %s", tool.Name)
		}
		assert.Equal(t, false, tool.InputSchema["additionalProperties"], "tool %s input must be closed", tool.Name)
		assert.Equal(t, false, tool.OutputSchema["additionalProperties"], "tool %s output must be closed", tool.Name)
	}
}

// Every input property must carry a description — that's what the model
// reads to know what a parameter means.
func TestRegisterTools_EveryInputPropertyHasADescription(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, &Client{}))

	for _, tool := range r.Tools() {
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for name, raw := range props {
			prop, ok := raw.(map[string]any)
			require.True(t, ok, "tool %s property %s", tool.Name, name)
			desc, _ := prop["description"].(string)
			assert.NotEmpty(t, desc, "tool %s property %s must carry a description", tool.Name, name)
		}
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "sfkey", time.Second)
	require.NoError(t, err)
	return c
}

func TestGetStoreProduct_UnknownHandleReturnsFoundFalseNotError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("get_store_product")
	require.True(t, ok)

	out, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi","handle":"nope"}`))
	require.NoError(t, err)

	p, ok := out.(Product)
	require.True(t, ok)
	assert.False(t, p.Found)
	assert.Empty(t, p.Handle)
}

func TestGetStoreProduct_FoundProjectsTheProduct(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"handle":"mug","title":"Mug","price_range":{"min":"9.5","max":"9.5","currency_code":"AUD"}}`))
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("get_store_product")
	require.True(t, ok)

	out, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi","handle":"mug"}`))
	require.NoError(t, err)

	p, ok := out.(Product)
	require.True(t, ok)
	assert.True(t, p.Found)
	assert.Equal(t, "mug", p.Handle)
}

func TestGetStoreBranding_UnknownStoreReturnsFoundFalseNotError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("get_store_branding")
	require.True(t, ok)

	out, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"nope"}`))
	require.NoError(t, err)

	b, ok := out.(Branding)
	require.True(t, ok)
	assert.False(t, b.Found)
}

// Every other error must propagate unchanged so observe.OutcomeFor can
// classify it — the client's own segment() validation error must not be
// swallowed into a false "not found".
func TestGetStoreProduct_NonNotFoundErrorsPropagate(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("get_store_product")
	require.True(t, ok)

	_, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi","handle":"mug"}`))
	require.Error(t, err)
	require.NotErrorIs(t, err, upstream.ErrNotFound)
}

// A missing store on a list endpoint must not silently read as an empty
// catalogue — the not-found error must propagate for the caller to classify.
func TestListStoreProducts_UnknownStorePropagatesNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("list_store_products")
	require.True(t, ok)

	_, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"nope"}`))
	require.ErrorIs(t, err, upstream.ErrNotFound)
}

func TestListStoreProducts_EmptyStoreIsAnEmptyListNotAnError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("list_store_products")
	require.True(t, ok)

	out, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi"}`))
	require.NoError(t, err)

	list, ok := out.(ProductList)
	require.True(t, ok)
	assert.Empty(t, list.Products)
	assert.NotNil(t, list.Products)
}

func TestListStoreCategories_ProjectsCategories(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"Mugs","slug":"mugs","position":1,"featured":true}]}`))
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("list_store_categories")
	require.True(t, ok)

	out, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi"}`))
	require.NoError(t, err)

	list, ok := out.(CategoryList)
	require.True(t, ok)
	require.Len(t, list.Categories, 1)
	assert.Equal(t, "mugs", list.Categories[0].Slug)
}

func TestListProductsByCategory_SendsPageParams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/storefront/stores/bondi/categories/mugs/products", r.URL.Path)
		assert.Equal(t, "2", r.URL.Query().Get("page"))
		assert.Equal(t, "10", r.URL.Query().Get("page_size"))
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	r := gsmcp.NewRegistry()
	require.NoError(t, RegisterTools(r, c))

	tool, ok := r.Get("list_products_by_category")
	require.True(t, ok)

	_, err := tool.Invoke(context.Background(), []byte(`{"store_slug":"bondi","category_slug":"mugs","page":2,"page_size":10}`))
	require.NoError(t, err)
}
