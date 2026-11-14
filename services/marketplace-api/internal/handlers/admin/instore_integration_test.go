//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func inStoreDB(t *testing.T) (*gorm.DB, uuid.UUID, uuid.UUID, *stores.Store) {
	t.Helper()
	db := testdb.NewTx(t)
	tid, sid := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tid, sid)
	var st stores.Store
	require.NoError(t, db.First(&st, "id = ?", sid).Error)
	return db, tid, sid, &st
}

// publishedAt is SQL: "now()", "NULL", or a future time for a scheduled product.
func seedProduct(t *testing.T, db *gorm.DB, tid, sid uuid.UUID, title, status, publishedAt string) {
	t.Helper()
	pid := uuid.NewString()
	pub := publishedAt
	require.NoError(t, db.Exec(
		`INSERT INTO products (id, tenant_id, store_id, title, handle, status, vendor_id, published_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, `+pub+`)`,
		pid, tid, sid, title, "h-"+pid[:8], status, testdb.SeedVendor(t, db, tid)).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO product_variants (id, product_id, store_id, sku, price, cost_price, currency_code)
		 VALUES (?, ?, ?, ?, 20.00, 7.00, 'EUR')`,
		uuid.NewString(), pid, sid, "S-"+pid[:8]).Error)
}

func inStoreGET(t *testing.T, db *gorm.DB, tid uuid.UUID, st *stores.Store, userID, path string, mount func(*gin.RouterGroup, *admin.InStoreHandler)) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := admin.NewInStoreHandler(db, nil, authz.NewFakeClient(), nil)
	e := gin.New()
	g := e.Group("/admin/stores/:storeId", func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("tenant_id", tid.String())
		c.Set("store", st)
	})
	mount(g, h)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/stores/"+st.ID+path, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestInStoreCatalog_OnlyActivePublished(t *testing.T) {
	db, tid, sid, st := inStoreDB(t)
	seedProduct(t, db, tid, sid, "Live skin", "active", "now()")
	seedProduct(t, db, tid, sid, "Draft skin", "draft", "NULL")
	seedProduct(t, db, tid, sid, "Archived skin", "archived", "now()")
	seedProduct(t, db, tid, sid, "Scheduled skin", "active", "now() + interval '1 day'")

	body := inStoreGET(t, db, tid, st, "u-1", "/in-store/catalog", func(g *gin.RouterGroup, h *admin.InStoreHandler) {
		g.GET("/in-store/catalog", h.Catalog)
	})
	data := body["data"].([]any)
	require.Len(t, data, 1)
	p := data[0].(map[string]any)
	require.Equal(t, "Live skin", p["title"])
	v := p["variants"].([]any)[0].(map[string]any)
	require.NotContains(t, v, "cost_price", "a cashier must not see cost")
}

func TestInStoreSalesMine_OnlyCallersToday(t *testing.T) {
	db, tid, sid, st := inStoreDB(t)
	insert := func(soldBy, channel, placed string) {
		require.NoError(t, db.Exec(
			`INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key, customer_email,
			                     subtotal, grand_total, currency_code, channel, sold_by_user_id, placed_at)
			 VALUES (?, ?, ?, ?, ?, 'buyer@example.com', 10, 10, 'EUR', ?, ?, `+placed+`)`,
			uuid.New(), tid, sid, "N-"+uuid.NewString()[:8], uuid.NewString(), channel, soldBy).Error)
	}
	insert("u-1", "in_store", "now()")
	insert("u-2", "in_store", "now()")                     // someone else's
	insert("u-1", "online", "now()")                       // not in store
	insert("u-1", "in_store", "now() - interval '2 days'") // not today

	body := inStoreGET(t, db, tid, st, "u-1", "/in-store/sales", func(g *gin.RouterGroup, h *admin.InStoreHandler) {
		g.GET("/in-store/sales", h.MySales)
	})
	data := body["data"].([]any)
	require.Len(t, data, 1)
	require.Equal(t, "b***@example.com", data[0].(map[string]any)["receipt_email"])
}
