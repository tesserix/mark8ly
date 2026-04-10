package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStoresHandler_List_MissingTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewStoresHandler(nil, nil)

	r := gin.New()
	r.GET("/admin/stores", h.List)

	req := httptest.NewRequest(http.MethodGet, "/admin/stores", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Errorf("expected error=unauthorized, got %v", body["error"])
	}
}
