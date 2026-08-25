//go:build integration

package platformadmin_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Idempotency is the acceptance criterion this proves: the SAME key
// replays the stored body and writes NO second audit row, while a
// DIFFERENT key is a new extension. Asserting the audit-row count is what
// distinguishes real idempotency from a coincidentally identical response.
func TestTrialExtendIsIdempotentPerKey(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	ex := &stubExtender{result: okResult()}
	aud := &capturedAudit{}
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(db, ex, aud.fn, nil).Register(r.Group(""))

	do := func(key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost,
			"/admin/billing/trials/"+extendStoreID.String()+"/extend",
			bytes.NewBufferString(goodBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		r.ServeHTTP(rec, req)
		return rec
	}

	first := do("key-alpha")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := do("key-alpha")
	require.Equal(t, http.StatusOK, second.Code)
	require.JSONEq(t, first.Body.String(), second.Body.String(), "same key must replay the same body")
	require.Equal(t, 1, ex.calls, "same key must NOT perform a second extension")
	require.Len(t, aud.events, 1, "same key must NOT write a second audit row")

	third := do("key-beta")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, 2, ex.calls, "a DIFFERENT key is a new extension")
	require.Len(t, aud.events, 2)
}
