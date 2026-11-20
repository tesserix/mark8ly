package internalsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func doCountRequest(t *testing.T, h *TenantCountHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/internal"), "")
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/purge-preview", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestTenantCountHandler_ReturnsPerTableCounts(t *testing.T) {
	var gotTenant string
	var gotStores []string
	h := NewTenantCountHandler(func(_ context.Context, tenantID string, storeIDs []string) (map[string]int64, error) {
		gotTenant, gotStores = tenantID, storeIDs
		return map[string]int64{"products": 3, "orders": 0}, nil
	})
	rec := doCountRequest(t, h, `{"store_ids":["s-1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var body struct{ Tables map[string]int64 }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if gotTenant != "t-1" || len(gotStores) != 1 || gotStores[0] != "s-1" || body.Tables["products"] != 3 {
		t.Fatalf("tenant %q stores %v tables %v", gotTenant, gotStores, body.Tables)
	}
}

func TestTenantCountHandler_CountErrorReturns500(t *testing.T) {
	h := NewTenantCountHandler(func(context.Context, string, []string) (map[string]int64, error) {
		return nil, errors.New("db down")
	})
	if rec := doCountRequest(t, h, `{"store_ids":[]}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTenantCountHandler_MalformedBodyReturns400(t *testing.T) {
	h := NewTenantCountHandler(func(context.Context, string, []string) (map[string]int64, error) {
		t.Fatal("count must not run on a malformed body")
		return nil, nil
	})
	if rec := doCountRequest(t, h, `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
