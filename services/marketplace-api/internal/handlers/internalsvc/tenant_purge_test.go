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

// newTenantPurgeTestRouter wires a bare gin engine with the purge route
// registered the same way main.go does — no auth secret so tests don't
// need to fight RequireInternalAuth's dev-permissive branch.
func newTenantPurgeTestRouter(h *TenantPurgeHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h.Register(r.Group("/internal"), "")
	return r
}

func doPurgeRequest(t *testing.T, r *gin.Engine, tenantID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/"+tenantID+"/purge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestTenantPurgeHandler_Success(t *testing.T) {
	var gotTenantID string
	var gotStoreIDs []string
	h := NewTenantPurgeHandler(func(_ context.Context, tenantID string, storeIDs []string) error {
		gotTenantID = tenantID
		gotStoreIDs = storeIDs
		return nil
	})
	r := newTenantPurgeTestRouter(h)

	body, err := json.Marshal(map[string]any{"store_ids": []string{"store-1", "store-2"}})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	rec := doPurgeRequest(t, r, "tenant-123", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if gotTenantID != "tenant-123" {
		t.Fatalf("expected purgeFn called with tenantID %q, got %q", "tenant-123", gotTenantID)
	}
	if len(gotStoreIDs) != 2 || gotStoreIDs[0] != "store-1" || gotStoreIDs[1] != "store-2" {
		t.Fatalf("expected purgeFn called with store_ids [store-1 store-2], got %v", gotStoreIDs)
	}
}

// TestTenantPurgeHandler_IdempotentReplay verifies that a second purge call
// for the same tenant (purgeFn returning nil again, mirroring
// tenantpurge.Purge's idempotent no-op-on-replay behavior) also returns 200.
func TestTenantPurgeHandler_IdempotentReplay(t *testing.T) {
	calls := 0
	h := NewTenantPurgeHandler(func(_ context.Context, _ string, _ []string) error {
		calls++
		return nil
	})
	r := newTenantPurgeTestRouter(h)

	body, err := json.Marshal(map[string]any{"store_ids": []string{"store-1"}})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	first := doPurgeRequest(t, r, "tenant-123", body)
	second := doPurgeRequest(t, r, "tenant-123", body)

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("expected both replays to return 200, got %d and %d", first.Code, second.Code)
	}
	if calls != 2 {
		t.Fatalf("expected purgeFn called twice, got %d", calls)
	}
}

func TestTenantPurgeHandler_PurgeErrorReturns500(t *testing.T) {
	h := NewTenantPurgeHandler(func(_ context.Context, _ string, _ []string) error {
		return errors.New("boom")
	})
	r := newTenantPurgeTestRouter(h)

	body, err := json.Marshal(map[string]any{"store_ids": []string{"store-1"}})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	rec := doPurgeRequest(t, r, "tenant-123", body)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTenantPurgeHandler_MalformedBodyReturns400(t *testing.T) {
	h := NewTenantPurgeHandler(func(_ context.Context, _ string, _ []string) error {
		t.Fatal("purgeFn must not be called for a malformed body")
		return nil
	})
	r := newTenantPurgeTestRouter(h)

	rec := doPurgeRequest(t, r, "tenant-123", []byte(`{"store_ids": "not-an-array"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}
