//go:build integration

package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/authz"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/order"
)

func strp(s string) *string { return &s }

// seedChannelOrder writes an order row directly: the list and metrics only read.
func seedChannelOrder(t *testing.T, db *gorm.DB, storeID, tenantID, channel, email string) *order.Order {
	t.Helper()
	cash := decimal.NewFromInt(50)
	o := &order.Order{
		TenantID: uuid.MustParse(tenantID), StoreID: uuid.MustParse(storeID),
		OrderNumber: "M-" + uuid.NewString()[:8], IdempotencyKey: uuid.NewString(),
		CustomerEmail: email, Status: "fulfilled", PaymentStatus: "paid", FulfillmentStatus: "fulfilled",
		Subtotal: decimal.NewFromInt(25), GrandTotal: decimal.NewFromInt(28), CurrencyCode: "AUD",
		Channel: channel,
	}
	if channel == "in_store" {
		o.PaymentMethod = strp("cash")
		o.CashTendered = &cash
		o.SoldByUserID = strp("u-1")
		o.SoldByEmail = strp("sam@shop.test")
		o.Notes = strp("gift wrap")
	}
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

func TestOrdersList_ChannelFilter(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	online := seedChannelOrder(t, env.db, storeID, tenantID, "online", "a@example.com")
	inStore := seedChannelOrder(t, env.db, storeID, tenantID, "in_store", "")
	base := "/api/v1/admin/stores/" + storeID + "/orders"

	list := func(q string) (int, []map[string]any) {
		w := request(t, env.router, http.MethodGet, base+q, nil, authHeaders(userID, tenantID))
		var body struct{ Data []map[string]any }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body.Data
	}

	code, rows := list("?channel=in_store")
	if code != http.StatusOK || len(rows) != 1 || rows[0]["id"] != inStore.ID.String() {
		t.Fatalf("in_store filter: code %d rows %v", code, rows)
	}
	r := rows[0]
	if r["channel"] != "in_store" || r["payment_method"] != "cash" || r["sold_by_email"] != "sam@shop.test" ||
		r["cash_tendered"] != "50" || r["notes"] != "gift wrap" {
		t.Errorf("in-store fields missing: %v", r)
	}

	code, rows = list("?channel=online")
	if code != http.StatusOK || len(rows) != 1 || rows[0]["id"] != online.ID.String() || rows[0]["channel"] != "online" {
		t.Fatalf("online filter: code %d rows %v", code, rows)
	}
	if _, ok := rows[0]["payment_method"]; ok {
		t.Errorf("online order should omit payment_method: %v", rows[0])
	}

	if code, _ = list("?channel=pos"); code != http.StatusBadRequest {
		t.Errorf("unknown channel: want 400, got %d", code)
	}
}

func TestCustomersMetrics_IgnoresEmptyEmail(t *testing.T) {
	env := setupOrdersRouter(t)
	storeID, tenantID := seedStoreRow(t, env.db, "")
	userID := uuid.NewString()
	env.fga.Grant(userID, authz.RoleAdmin, tenantID)
	for range 3 {
		seedChannelOrder(t, env.db, storeID, tenantID, "in_store", "")
	}
	seedChannelOrder(t, env.db, storeID, tenantID, "online", "a@example.com")

	r := gin.New()
	admin.RegisterAdmin(r.Group("/api/v1"), admin.Deps{
		DashboardHandler: admin.NewDashboardHandler(env.db, nil),
		AuthzMiddleware:  authz.NewMiddleware(env.fga, nil),
	})
	w := request(t, r, http.MethodGet, "/api/v1/admin/stores/"+storeID+"/dashboard/metrics/customers?range=7d", nil, authHeaders(userID, tenantID))
	if w.Code != http.StatusOK {
		t.Fatalf("metrics: %d %s", w.Code, w.Body.String())
	}
	var m struct {
		UniqueBuyers   int64 `json:"unique_buyers"`
		NewTotal       int64 `json:"new_total"`
		ReturningTotal int64 `json:"returning_total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m.UniqueBuyers != 1 || m.ReturningTotal != 0 || m.NewTotal != 1 {
		t.Errorf("anonymous counter sales counted as customers: %+v", m)
	}
}
