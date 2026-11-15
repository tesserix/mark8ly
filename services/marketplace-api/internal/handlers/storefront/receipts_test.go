package storefront

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type fakeReceiptReader struct {
	storeID  uuid.UUID
	token    string
	resent   int
	notFound bool
}

func (f *fakeReceiptReader) Load(_ context.Context, storeID uuid.UUID, token string) (*instore.Receipt, error) {
	f.storeID, f.token = storeID, token
	if f.notFound {
		return nil, apperrors.NotFound("receipt")
	}
	return &instore.Receipt{OrderNumber: "M-1", TaxLabel: "GST included", Lines: []instore.ReceiptLine{}}, nil
}

func (f *fakeReceiptReader) Resend(_ context.Context, storeID uuid.UUID, token string) error {
	f.storeID, f.token = storeID, token
	if f.notFound {
		return apperrors.NotFound("receipt")
	}
	f.resent++
	return nil
}

func receiptEngine(r ReceiptReader, storeID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	g := e.Group("/s", func(c *gin.Context) { c.Set("store", &stores.Store{ID: storeID}) })
	h := NewReceiptsHandler(r, nil)
	g.GET("/receipts/:token", h.Get)
	g.POST("/receipts/:token/resend", h.Resend)
	return e
}

func TestReceiptsHandler_GetScopesToStore(t *testing.T) {
	sid := uuid.New()
	r := &fakeReceiptReader{}
	w := httptest.NewRecorder()
	receiptEngine(r, sid.String()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/s/receipts/tok123", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"order_number":"M-1"`) {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}
	if r.storeID != sid || r.token != "tok123" {
		t.Errorf("looked up %s/%s", r.storeID, r.token)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("receipt may be cached: %q", w.Header().Get("Cache-Control"))
	}
}

func TestReceiptsHandler_Unknown404(t *testing.T) {
	w := httptest.NewRecorder()
	receiptEngine(&fakeReceiptReader{notFound: true}, uuid.NewString()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/s/receipts/x", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown token: %d", w.Code)
	}
}

func TestReceiptsHandler_ResendIs202WithNoDetails(t *testing.T) {
	r := &fakeReceiptReader{}
	w := httptest.NewRecorder()
	receiptEngine(r, uuid.NewString()).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/s/receipts/tok/resend", nil))
	if w.Code != http.StatusAccepted || r.resent != 1 || strings.Contains(w.Body.String(), "@") {
		t.Errorf("resend: %d %s resent=%d", w.Code, w.Body, r.resent)
	}
}
