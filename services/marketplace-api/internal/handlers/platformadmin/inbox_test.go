package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type fakeAgg struct {
	res inbox.Result
	err error
}

func (f fakeAgg) List(context.Context, inbox.Filter) (inbox.Result, error) { return f.res, f.err }

func TestInboxHandler_RendersEnvelopeWithDegraded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{
		Items:    []inbox.Item{{ID: "a", Kind: "erasure_request"}},
		Total:    1,
		Degraded: []string{"onboarding_stalled"},
	}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data       []inbox.Item `json:"data"`
		Pagination struct {
			Page, Limit int
			Total       int64
		} `json:"pagination"`
		Degraded []string `json:"degraded"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.EqualValues(t, 1, body.Pagination.Total)
	require.Equal(t, []string{"onboarding_stalled"}, body.Degraded,
		"a degraded source must reach the console, not be swallowed")
}

func TestInboxHandler_EmptyIsTwoHundredWithEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`,
		"empty must serialise as [] not null — the console renders an array")
}

func TestInboxHandler_DeepPageIsFourHundred(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{err: inbox.ErrPageTooDeep}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox?page=99&limit=50", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "kind", "the error must tell the caller how to narrow")
}
