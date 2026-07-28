package admin_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func init() { gin.SetMode(gin.TestMode) }

func runRespondErr(t *testing.T, err error) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := gin.New()
	r.GET("/x", func(c *gin.Context) { admin.RespondErr(c, err, nil) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if jerr := json.Unmarshal(w.Body.Bytes(), &body); jerr != nil {
		t.Fatalf("json: %v", jerr)
	}
	return w, body
}

func TestRespondErr_TypedError_RendersEnvelope(t *testing.T) {
	w, body := runRespondErr(t, apperrors.HandleTaken("foo", "foo-2"))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if body["error"] != "handle_taken" {
		t.Fatalf("error = %v", body["error"])
	}
	det, ok := body["details"].(map[string]any)
	if !ok || det["suggested"] != "foo-2" {
		t.Fatalf("details = %v", body["details"])
	}
}

func TestRespondErr_NotFound_Returns404(t *testing.T) {
	w, body := runRespondErr(t, apperrors.NotFound("product"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
	if body["error"] != "not_found" {
		t.Fatalf("body = %v", body)
	}
}

func TestRespondErr_ValidationFailed_Returns400(t *testing.T) {
	w, body := runRespondErr(t, apperrors.ValidationFailed("title", "required"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
	det, _ := body["details"].(map[string]any)
	if det["field"] != "title" {
		t.Fatalf("details = %v", body["details"])
	}
}

func TestRespondErr_PayloadTooLarge_Returns413(t *testing.T) {
	w, _ := runRespondErr(t, apperrors.PayloadTooLarge("k", 11<<20))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRespondErr_UntypedError_Returns500_GenericBody(t *testing.T) {
	w, body := runRespondErr(t, errors.New("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if body["error"] != "internal" {
		t.Fatalf("error = %v", body["error"])
	}
	if strings.Contains(string(w.Body.Bytes()), "boom") {
		t.Fatalf("leaked error: %s", w.Body.String())
	}
}

// TestCodeStatus_CoversAllCodes enumerates from apperrors.AllCodes() — the
// same authoritative list backing apperrors.IsKnownCode — rather than a
// second hand-copied slice here. A hand-copied list drifts silently: it
// missed CodeGiftCardNotRedeemable when it was added, so RespondErr fell
// through to the 500 fallback for that code with nothing catching it.
// Reading straight from AllCodes() means any newly added code is exercised
// automatically, with no second place to remember to update.
func TestCodeStatus_CoversAllCodes(t *testing.T) {
	for _, code := range apperrors.AllCodes() {
		if !apperrors.IsKnownCode(string(code)) {
			t.Errorf("code %q not in IsKnownCode", code)
		}
		// Render through RespondErr and assert status != 500 (unknown fallback).
		err := apperrors.New(code, "x")
		w, _ := runRespondErr(t, err)
		if w.Code == http.StatusInternalServerError {
			t.Errorf("code %q maps to 500 (missing from codeStatus)", code)
		}
	}
}

func TestRespondErr_RefundUnavailable_Returns422(t *testing.T) {
	w, _ := runRespondErr(t, apperrors.RefundUnavailable("no captured payment"))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}
