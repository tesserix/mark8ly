package platformadmin_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestInboxHandler_EnvelopeIsExactlyItemsTotal is the point of this task: the
// Product Admin Integration Contract's "items-total" envelope requires the
// body to have EXACTLY the keys items and total — no pagination, no
// degraded, nothing else. Decoding into a map so a future added field fails
// here rather than overnight against production.
func TestInboxHandler_EnvelopeIsExactlyItemsTotal(t *testing.T) {
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

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.ElementsMatch(t, []string{"items", "total"}, keysOf(body),
		"the contract requires exactly { items, total } — no pagination, no degraded, nothing else")

	items, ok := body["items"].([]any)
	require.True(t, ok, "items must be an array")
	require.Len(t, items, 1)
	require.EqualValues(t, 1, body["total"])

	require.Equal(t, "onboarding_stalled", w.Header().Get(platformadmin.InboxDegradedHeader),
		"a degraded source must reach the console via the header, not be swallowed")
}

func TestInboxHandler_EmptyIsTwoHundredWithEmptyArrayAndNoDegradedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"items":[]`,
		"empty must serialise as [] not null — the console renders an array")

	_, present := w.Result().Header[http.CanonicalHeaderKey(platformadmin.InboxDegradedHeader)]
	require.False(t, present, "the degraded header must be absent, not empty-string, when nothing is degraded")
}

func TestInboxHandler_DegradedHeaderHasCommaSeparatedKinds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{res: inbox.Result{
		Items:    []inbox.Item{},
		Total:    0,
		Degraded: []string{"onboarding_stalled", "erasure_request"},
	}}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "onboarding_stalled,erasure_request", w.Header().Get(platformadmin.InboxDegradedHeader))
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

func TestInboxHandler_UnknownKindIsFourHundred(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	wrapped := fmt.Errorf("%w: %q", inbox.ErrUnknownKind, "bogus")
	platformadmin.NewInboxHandler(fakeAgg{err: wrapped}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox?kind=bogus", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "kind", "the response must be actionable — name a valid kind or the word kind")
}

func TestInboxHandler_AllSourcesFailedIsFiveHundred(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewInboxHandler(fakeAgg{err: inbox.ErrAllSourcesFailed}, nil).Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/inbox", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}
