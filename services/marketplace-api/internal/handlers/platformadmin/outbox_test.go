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
	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// stubOutboxLister records the filter it was called with, so the tests can
// assert on PARSING as well as on rendering.
type stubOutboxLister struct {
	gotFilter outbox.PlatformListFilter
	result    outbox.PlatformListResult
	err       error
}

func (s *stubOutboxLister) ListPlatform(_ context.Context, _ *gorm.DB,
	f outbox.PlatformListFilter, _ time.Time) (outbox.PlatformListResult, error) {
	s.gotFilter = f
	return s.result, s.err
}

func outboxRouter(t *testing.T, lister platformadmin.OutboxLister) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewOutboxHandler(nil, lister, nil).Register(r.Group(""))
	return r
}

func getOutbox(t *testing.T, lister platformadmin.OutboxLister, query string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	outboxRouter(t, lister).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/admin/outbox"+query, nil))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

func TestOutboxEmptyIsTwoHundredAndAnArrayNotNull(t *testing.T) {
	rec, _ := getOutbox(t, &stubOutboxLister{}, "")
	require.Equal(t, http.StatusOK, rec.Code)
	// A nil slice marshals to null and defeats a caller's `?? []` exactly
	// when there is no data. The literal matters more than the parsed form.
	require.Contains(t, rec.Body.String(), `"data":[]`)
	require.NotContains(t, rec.Body.String(), `"data":null`)
}

func TestOutboxRendersRowsAndOmitsAgeForPublished(t *testing.T) {
	pubAt := time.Date(2026, 8, 26, 11, 30, 0, 0, time.UTC)
	age := int64(600)
	reason := outbox.ReasonPayloadUnparseable
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 2,
		Rows: []outbox.PlatformRow{
			{
				ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
				EventType: "product.created", Status: outbox.StatusFailed,
				CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
				Error:     &reason, AgeSeconds: &age,
			},
			{
				ID: "44444444-4444-4444-4444-444444444444", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "order", AggregateID: "55555555-5555-5555-5555-555555555555",
				EventType: "order.placed", Status: outbox.StatusPublished,
				CreatedAt:   time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC),
				PublishedAt: &pubAt,
			},
		},
	}}

	rec, _ := getOutbox(t, lister, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Data []struct {
			ID          string  `json:"id"`
			Status      string  `json:"status"`
			CreatedAt   string  `json:"created_at"`
			AgeSeconds  *int64  `json:"age_seconds"`
			PublishedAt *string `json:"published_at"`
			Error       *string `json:"error"`
		} `json:"data"`
		Pagination struct {
			Page, Limit int
			Total       int64
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Data, 2)

	require.Equal(t, outbox.StatusFailed, got.Data[0].Status)
	require.NotNil(t, got.Data[0].AgeSeconds)
	require.Equal(t, int64(600), *got.Data[0].AgeSeconds)
	require.NotNil(t, got.Data[0].Error)
	require.Equal(t, outbox.ReasonPayloadUnparseable, *got.Data[0].Error)
	require.Nil(t, got.Data[0].PublishedAt)
	require.Equal(t, "2026-08-26T11:50:00Z", got.Data[0].CreatedAt, "RFC3339 UTC")

	require.Equal(t, outbox.StatusPublished, got.Data[1].Status)
	require.Nil(t, got.Data[1].AgeSeconds, "a published row must carry no age_seconds")
	require.NotNil(t, got.Data[1].PublishedAt)
	require.Equal(t, "2026-08-26T11:30:00Z", *got.Data[1].PublishedAt)

	require.Equal(t, int64(2), got.Pagination.Total)
}

// payload is excluded by construction. This asserts on the RAW body so it
// fails if the field appears under any name or nesting.
func TestOutboxNeverEmitsPayload(t *testing.T) {
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 1,
		Rows: []outbox.PlatformRow{{
			ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
			Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
			EventType: "product.created", Status: outbox.StatusPending,
			CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
		}},
	}}
	rec, _ := getOutbox(t, lister, "")
	require.NotContains(t, rec.Body.String(), "payload")
}

func TestOutboxParsesFilters(t *testing.T) {
	lister := &stubOutboxLister{}
	tenant := uuid.NewString()
	_, _ = getOutbox(t, lister,
		"?status=failed&event_type=product.created&older_than_minutes=45&limit=10&page=3&tenant_id="+tenant)

	require.Equal(t, outbox.StatusFailed, lister.gotFilter.Status)
	require.Equal(t, "product.created", lister.gotFilter.EventType)
	require.Equal(t, 45, lister.gotFilter.OlderThanMinutes)
	require.Equal(t, 10, lister.gotFilter.Limit)
	require.Equal(t, 3, lister.gotFilter.Page)
	require.NotNil(t, lister.gotFilter.TenantID)
	require.Equal(t, tenant, lister.gotFilter.TenantID.String())
}

// Unknown and malformed values narrow nothing rather than erroring — the
// established contract across this surface.
func TestOutboxUnknownParametersNarrowNothing(t *testing.T) {
	lister := &stubOutboxLister{}
	rec, _ := getOutbox(t, lister, "?status=banana&tenant_id=not-a-uuid&limit=abc&older_than_minutes=-5")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "banana", lister.gotFilter.Status,
		"an unknown status is passed through; the query layer decides it narrows nothing")
	require.Nil(t, lister.gotFilter.TenantID, "an unparseable tenant_id is ignored, not an error")
	require.Zero(t, lister.gotFilter.Limit, "an unparseable limit falls back to the default downstream")
	require.Zero(t, lister.gotFilter.OlderThanMinutes, "a non-positive older_than_minutes is ignored")
}

func TestOutboxRepositoryErrorIsFiveHundred(t *testing.T) {
	lister := &stubOutboxLister{err: errors.New("boom")}
	rec, body := getOutbox(t, lister, "")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal_error", body["error"])
}

// The golden file pins the console's contract. If this fails because the
// shape changed deliberately, update the fixture to match the handler —
// never the handler to match the fixture.
func TestOutboxResponseMatchesGoldenShape(t *testing.T) {
	pubAt := time.Date(2026, 8, 26, 11, 30, 0, 0, time.UTC)
	age := int64(600)
	reason := outbox.ReasonStoreNotFound
	lister := &stubOutboxLister{result: outbox.PlatformListResult{
		Total: 2,
		Rows: []outbox.PlatformRow{
			{
				ID: "11111111-1111-1111-1111-111111111111", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "product", AggregateID: "33333333-3333-3333-3333-333333333333",
				EventType: "product.created", Status: outbox.StatusFailed,
				CreatedAt: time.Date(2026, 8, 26, 11, 50, 0, 0, time.UTC),
				Error:     &reason, AgeSeconds: &age,
			},
			{
				ID: "44444444-4444-4444-4444-444444444444", TenantID: "22222222-2222-2222-2222-222222222222",
				Aggregate: "order", AggregateID: "55555555-5555-5555-5555-555555555555",
				EventType: "order.placed", Status: outbox.StatusPublished,
				CreatedAt:   time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC),
				PublishedAt: &pubAt,
			},
		},
	}}
	rec, _ := getOutbox(t, lister, "")

	want, err := os.ReadFile("testdata/outbox_response.json")
	require.NoError(t, err)

	var gotAny, wantAny any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gotAny))
	require.NoError(t, json.Unmarshal(want, &wantAny))
	require.Equal(t, wantAny, gotAny)
}
