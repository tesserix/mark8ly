package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type routeInboxAggregator struct{ called bool }

func (s *routeInboxAggregator) List(_ context.Context, _ inbox.Filter) (inbox.Result, error) {
	s.called = true
	return inbox.Result{Items: []inbox.Item{}}, nil
}

// Register must mount GET /admin/inbox when Inbox is supplied (#280).
//
// The handler's own tests all construct InboxHandler directly, so they pass
// whether or not Register ever mounts it — which is how this endpoint shipped
// on main and answered 404 in production. This test is the one that fails in
// that state.
func TestRegisterMountsInboxWhenSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Inbox:  &routeInboxAggregator{},
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/inbox", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when Inbox is set")

	// A bogus path under the SAME prefix must still 404, or the assertion
	// above would also pass on a router that answers everything under
	// /api/v1/platform.
	bogus := httptest.NewRecorder()
	r.ServeHTTP(bogus, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/inbox-nope", nil))
	require.Equal(t, http.StatusNotFound, bogus.Code)
}

// A nil Inbox leaves the route unmounted, matching the nil-safe pattern every
// other optional client-backed route uses.
func TestRegisterLeavesInboxUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/inbox", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// #281a: the action route mounts on InboxItems, independently of Inbox. A
// deployment that can read the queue but not act on it is a legitimate state,
// and so is the reverse — neither should silently disable the other.
func TestRegisterMountsInboxActionsWhenItemSourceSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:       &stubRepo{},
		InboxItems: stubRouteItemSource{},
		Secret:     "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/inbox/migration_fast_path/abc/actions/approve", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the action route must be mounted when InboxItems is set")
}

func TestRegisterLeavesInboxActionsUnmountedWhenItemSourceNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Inbox:  &routeInboxAggregator{},
		Secret: "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/platform/admin/inbox/migration_fast_path/abc/actions/approve", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

type stubRouteItemSource struct{}

func (stubRouteItemSource) Get(_ context.Context, _, _ string) (inbox.Item, error) {
	return inbox.Item{}, inbox.ErrItemNotFound
}
