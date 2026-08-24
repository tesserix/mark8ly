package platformadmin_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/notification"
)

type routeNotificationLister struct{ called bool }

func (s *routeNotificationLister) ListPlatform(_ context.Context, _ *gorm.DB, _ notification.PlatformListFilter) (notification.ListResult, error) {
	s.called = true
	return notification.ListResult{Notifications: []notification.Notification{}}, nil
}

// Register must mount the route when Notifications is supplied. Asserted
// as "not 404" with the secret set, matching TestRegisterTicketsMountsWhenDepPresent
// — this catches a guard that always refuses just as readily as a missing one.
func TestRegisterMountsNotificationsWhenSupplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:          &stubRepo{},
		Notifications: &routeNotificationLister{},
		Secret:        "test-secret",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications", nil))
	require.NotEqual(t, http.StatusNotFound, rec.Code,
		"the route must be mounted when Notifications is set")

	// A bogus path under the SAME prefix must 404. Without this, the
	// assertion above is also satisfied by a router that answers
	// everything under /api/v1/platform — it is what makes the first
	// check mean "this route exists".
	bogus := httptest.NewRecorder()
	r.ServeHTTP(bogus, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications-nope", nil))
	require.Equal(t, http.StatusNotFound, bogus.Code)
}

// A nil Notifications leaves the route unmounted, matching the nil-safe
// pattern every other optional client-backed route uses.
func TestRegisterLeavesNotificationsUnmountedWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo: &stubRepo{},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/notifications", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
