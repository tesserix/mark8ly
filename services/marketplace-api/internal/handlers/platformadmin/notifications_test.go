package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/notification"
)

// stubNotificationLister records the filter it was handed and returns a
// canned result, so the tests can assert on parsing without a database.
type stubNotificationLister struct {
	result    notification.ListResult
	err       error
	gotFilter notification.PlatformListFilter
}

func (s *stubNotificationLister) ListPlatform(_ context.Context, _ *gorm.DB, f notification.PlatformListFilter) (notification.ListResult, error) {
	s.gotFilter = f
	if s.err != nil {
		return notification.ListResult{}, s.err
	}
	if s.result.Notifications == nil {
		s.result.Notifications = []notification.Notification{}
	}
	return s.result, nil
}

func getNotifications(t *testing.T, repo platformadmin.NotificationLister) *httptest.ResponseRecorder {
	t.Helper()
	return getNotificationsWithQuery(t, repo, "")
}

func getNotificationsWithQuery(t *testing.T, repo platformadmin.NotificationLister, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewNotificationsHandler(nil, repo, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/notifications"+query, nil))
	return rec
}

// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a zero
// fabricated by a missing field. Two rows, two stores, two tenants, one of
// each audience — the shape this endpoint exists to return.
func notificationsFixture() []notification.Notification {
	body := "MUST NOT APPEAR IN THE RESPONSE"
	otherBody := "MUST NOT APPEAR IN THE RESPONSE EITHER"
	resourceType := "order"
	resourceID := uuid.MustParse("cccccccc-1111-1111-1111-111111111111")
	uid := "gip-uid-customer-7"
	return []notification.Notification{
		{
			ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID:     uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
			StoreID:      uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111"),
			Type:         notification.TypeNewOrder,
			Title:        "New order received",
			Message:      &body,
			ResourceType: &resourceType,
			ResourceID:   &resourceID,
			IsRead:       false,
			CreatedAt:    time.Date(2026, 8, 19, 8, 30, 0, 0, time.UTC),
		},
		{
			ID:              uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID:        uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
			StoreID:         uuid.MustParse("bbbbbbbb-2222-2222-2222-222222222222"),
			RecipientUserID: &uid,
			Type:            notification.TypeOrderShipped,
			Title:           "Order confirmed",
			Message:         &otherBody,
			IsRead:          true,
			CreatedAt:       time.Date(2026, 8, 18, 7, 15, 0, 0, time.UTC),
		},
	}
}

// The golden fixture pins the exact contract shape as bytes, catching a
// rename or an unauthorized addition that a struct-shaped assertion would
// happily accept.
func TestNotificationsMatchesPinnedContract(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/notifications_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// Asserted on the RAW BYTES, not an unmarshalled struct: a struct cannot
// distinguish an absent key from an empty one. `message` is the
// interpolated body #332 exists to keep out; `status` does not exist in
// this estate at all (#348) and must not be faked from is_read.
func TestNotificationsOmitsBodyAndStatus(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	require.NotContains(t, body, `"message"`, "the notification body must never reach the console")
	require.NotContains(t, body, "MUST NOT APPEAR IN THE RESPONSE")
	require.NotContains(t, body, `"status"`, "there is no delivery status in this estate — see #348")
	require.NotContains(t, body, `"source"`, "the platform API stamps source and overwrites the body")
}

// audience is always present, so an absent recipient_user_id reads as
// "went to the store" rather than "the lookup failed". Both values are
// exercised because the fixture carries one row of each kind.
func TestNotificationsAudienceIsAlwaysPresent(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{result: notification.ListResult{
		Notifications: notificationsFixture(), Total: 2,
	}})

	var resp struct {
		Data []struct {
			Audience        string `json:"audience"`
			RecipientUserID string `json:"recipient_user_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	require.Equal(t, "store", resp.Data[0].Audience)
	require.Empty(t, resp.Data[0].RecipientUserID)
	require.Equal(t, "customer", resp.Data[1].Audience)
	require.Equal(t, "gip-uid-customer-7", resp.Data[1].RecipientUserID)
}

// Empty is 200 + [], never null. A nil slice marshals to null and defeats
// the console's `?? []` precisely when there is no data.
func TestNotificationsEmptyIsAnArray(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"data":[]`)
	require.NotContains(t, rec.Body.String(), `"data":null`)
}

// Every filter reaches the repository with the value the caller sent.
// Each value is DISTINCT so a handler that assigned the wrong field would
// be caught.
func TestNotificationsParsesEveryFilter(t *testing.T) {
	stub := &stubNotificationLister{}
	tenantID := uuid.MustParse("aaaaaaaa-3333-3333-3333-333333333333")
	storeID := uuid.MustParse("bbbbbbbb-3333-3333-3333-333333333333")

	rec := getNotificationsWithQuery(t, stub,
		"?type=low_stock&tenant_id="+tenantID.String()+
			"&store_id="+storeID.String()+
			"&audience=customer&recipient_user_id=gip-uid-zzz&read=true"+
			"&from=2026-08-01T00:00:00Z&to=2026-08-31T00:00:00Z&limit=7&page=3")
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "low_stock", stub.gotFilter.Type)
	require.NotNil(t, stub.gotFilter.TenantID)
	require.Equal(t, tenantID, *stub.gotFilter.TenantID)
	require.NotNil(t, stub.gotFilter.StoreID)
	require.Equal(t, storeID, *stub.gotFilter.StoreID)
	require.Equal(t, "customer", stub.gotFilter.Audience)
	require.Equal(t, "gip-uid-zzz", stub.gotFilter.RecipientUserID)
	require.NotNil(t, stub.gotFilter.Read)
	require.True(t, *stub.gotFilter.Read)
	require.NotNil(t, stub.gotFilter.From)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), stub.gotFilter.From.UTC())
	require.NotNil(t, stub.gotFilter.To)
	require.Equal(t, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), stub.gotFilter.To.UTC())
	require.Equal(t, 7, stub.gotFilter.Limit)
	require.Equal(t, 3, stub.gotFilter.Page)
}

// read=false must reach the repository as a non-nil pointer to false, not
// as nil. A handler that only set the pointer for "true" would return read
// AND unread rows for read=false, and a presence-only assertion would miss
// it.
func TestNotificationsReadFalseIsNotTheSameAsAbsent(t *testing.T) {
	stub := &stubNotificationLister{}
	getNotificationsWithQuery(t, stub, "?read=false")
	require.NotNil(t, stub.gotFilter.Read, "read=false must narrow, not be dropped")
	require.False(t, *stub.gotFilter.Read)

	absent := &stubNotificationLister{}
	getNotifications(t, absent)
	require.Nil(t, absent.gotFilter.Read, "an absent read parameter must not narrow")
}

// An oversized limit clamps rather than refusing, a missing one takes the
// default, and pagination.limit reports the EFFECTIVE limit so the console
// can compute total/limit as a page count.
func TestNotificationsLimitClampsAndReportsEffective(t *testing.T) {
	stub := &stubNotificationLister{result: notification.ListResult{Total: 9000}}
	rec := getNotificationsWithQuery(t, stub, "?limit=100000")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, notification.MaxPlatformPageSize, stub.gotFilter.Limit)

	var resp struct {
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, notification.MaxPlatformPageSize, resp.Pagination.Limit,
		"pagination.limit must be the clamped limit, not the requested one")
	require.Equal(t, 1, resp.Pagination.Page)
	require.Equal(t, int64(9000), resp.Pagination.Total)

	def := &stubNotificationLister{}
	getNotifications(t, def)
	require.Equal(t, notification.DefaultPlatformPageSize, def.gotFilter.Limit)
}

// Garbage never errors — it takes the default, matching #276 and #329.
func TestNotificationsMalformedParametersTakeDefaults(t *testing.T) {
	stub := &stubNotificationLister{}
	rec := getNotificationsWithQuery(t, stub,
		"?limit=banana&page=-4&tenant_id=not-a-uuid&store_id=nope&from=yesterday&read=perhaps")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, notification.DefaultPlatformPageSize, stub.gotFilter.Limit)
	require.Equal(t, 1, stub.gotFilter.Page)
	require.Nil(t, stub.gotFilter.TenantID)
	require.Nil(t, stub.gotFilter.StoreID)
	require.Nil(t, stub.gotFilter.From)
	require.Nil(t, stub.gotFilter.Read)
}

// Explicit from/to wins over since_hours when both are supplied, matching
// #276 and #329. The explicit `from` is TEN DAYS back while since_hours
// asks for one hour, so the two cannot coincide.
func TestNotificationsExplicitFromBeatsSinceHours(t *testing.T) {
	stub := &stubNotificationLister{}
	getNotificationsWithQuery(t, stub, "?since_hours=1&from=2026-08-01T00:00:00Z")
	require.NotNil(t, stub.gotFilter.From)
	require.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), stub.gotFilter.From.UTC())
}

// since_hours alone sets a window measured back from now.
func TestNotificationsSinceHoursSetsAWindow(t *testing.T) {
	stub := &stubNotificationLister{}
	before := time.Now()
	getNotificationsWithQuery(t, stub, "?since_hours=24")
	require.NotNil(t, stub.gotFilter.From)
	delta := before.Sub(*stub.gotFilter.From)
	require.InDelta(t, (24 * time.Hour).Seconds(), delta.Seconds(), 60,
		"since_hours=24 must look back roughly 24 hours")
}

// A repository failure is a 500 with a stable code, and the driver's error
// text is never echoed to the caller.
func TestNotificationsRepositoryErrorIs500AndDoesNotLeak(t *testing.T) {
	rec := getNotifications(t, &stubNotificationLister{
		err: errors.New("dial tcp 10.0.0.1:5432: connection refused"),
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "internal_error")
	require.NotContains(t, rec.Body.String(), "connection refused",
		"driver error text must be logged server-side, never echoed")
}
