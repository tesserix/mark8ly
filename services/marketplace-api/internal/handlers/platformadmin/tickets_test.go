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
	"github.com/mark8ly/marketplace-api/internal/ticket"
)

// stubTicketLister records the filter it was handed and returns a canned
// result, so the tests can assert on parsing without a database.
type stubTicketLister struct {
	result    ticket.ListResult
	err       error
	gotFilter ticket.PlatformListFilter
}

func (s *stubTicketLister) ListPlatform(_ context.Context, _ *gorm.DB, f ticket.PlatformListFilter) (ticket.ListResult, error) {
	s.gotFilter = f
	if s.err != nil {
		return ticket.ListResult{}, s.err
	}
	if s.result.Tickets == nil {
		s.result.Tickets = []ticket.Ticket{}
	}
	return s.result, nil
}

func ticketsRouter(t *testing.T, repo platformadmin.TicketLister) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewTicketsHandler(nil, repo, nil).Register(r.Group(""))
	return r
}

func getTickets(t *testing.T, repo platformadmin.TicketLister) *httptest.ResponseRecorder {
	t.Helper()
	return getTicketsWithQuery(t, repo, "")
}

func getTicketsWithQuery(t *testing.T, repo platformadmin.TicketLister, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := ticketsRouter(t, repo)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/tickets"+query, nil))
	return rec
}

// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a zero
// fabricated by a missing field. Two tickets, two stores, two tenants — the
// shape this endpoint exists to return.
func ticketsFixture() []ticket.Ticket {
	conv := "conv-abc123"
	resolved := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return []ticket.Ticket{
		{
			ID:               uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID:         uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111"),
			StoreID:          uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111"),
			TicketNumber:     "T-1042",
			Subject:          "Refund not received",
			Description:      "MUST NOT APPEAR IN THE RESPONSE",
			Status:           "open",
			Priority:         "high",
			SubmittedByName:  "Ada Lovelace",
			SubmittedByEmail: "ada@example.com",
			ConversationID:   &conv,
			CreatedAt:        time.Date(2026, 8, 19, 8, 30, 0, 0, time.UTC),
			UpdatedAt:        time.Date(2026, 8, 19, 11, 45, 0, 0, time.UTC),
		},
		{
			ID:               uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID:         uuid.MustParse("aaaaaaaa-2222-2222-2222-222222222222"),
			StoreID:          uuid.MustParse("bbbbbbbb-2222-2222-2222-222222222222"),
			TicketNumber:     "T-2087",
			Subject:          "Wrong size delivered",
			Description:      "MUST NOT APPEAR IN THE RESPONSE EITHER",
			Status:           "resolved",
			Priority:         "low",
			SubmittedByName:  "Grace Hopper",
			SubmittedByEmail: "grace@example.com",
			ResolvedAt:       &resolved,
			CreatedAt:        time.Date(2026, 8, 18, 7, 15, 0, 0, time.UTC),
			UpdatedAt:        time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		},
	}
}

// THE test for the projection: assert against RAW JSON, because an
// unmarshalled struct cannot distinguish an absent key from an empty one.
func TestTickets_OmitsDescriptionAndReplies(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{result: ticket.ListResult{
		Tickets: ticketsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw.Data, 2)
	for i, row := range raw.Data {
		_, hasDesc := row["description"]
		_, hasReplies := row["replies"]
		require.False(t, hasDesc, "row %d must not carry the customer-written description", i)
		require.False(t, hasReplies, "row %d must not carry replies", i)
	}
}

func TestTickets_EmptyIsArrayNotNull(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{result: ticket.ListResult{}})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"data":[]`,
		"a nil slice marshals to null and defeats the caller's ?? []")
}

// An oversized limit clamps and pagination.limit reports the EFFECTIVE value,
// so total/limit is a correct page count.
func TestTickets_LimitClampsAndIsReportedEffective(t *testing.T) {
	stub := &stubTicketLister{result: ticket.ListResult{Total: 1200}}
	rec := getTicketsWithQuery(t, stub, "?limit=100000")
	require.Equal(t, ticket.MaxPlatformPageSize, stub.gotFilter.Limit, "the repo must receive the clamped limit")
	var body struct {
		Pagination struct {
			Limit int `json:"limit"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, ticket.MaxPlatformPageSize, body.Pagination.Limit)
}

// A missing parameter takes the default; it never errors. Assert what the
// REPOSITORY received — asserting only the response would pass even if the
// handler sent 0 and the repo happened to default it downstream.
func TestTickets_MissingLimitTakesDefault(t *testing.T) {
	stub := &stubTicketLister{}
	rec := getTicketsWithQuery(t, stub, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ticket.DefaultPlatformPageSize, stub.gotFilter.Limit)
}

// A non-numeric limit is not an error either: it takes the default, matching
// how #276 treats a malformed parameter.
func TestTickets_MalformedLimitTakesDefaultNotError(t *testing.T) {
	stub := &stubTicketLister{}
	rec := getTicketsWithQuery(t, stub, "?limit=not-a-number")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, ticket.DefaultPlatformPageSize, stub.gotFilter.Limit)
}

// store_id reaches the repository as a NARROWING filter, not a scope.
func TestTickets_StoreIDIsPassedThroughAsNarrowing(t *testing.T) {
	stub := &stubTicketLister{}
	id := uuid.New()
	getTicketsWithQuery(t, stub, "?store_id="+id.String())
	require.NotNil(t, stub.gotFilter.StoreID)
	require.Equal(t, id, *stub.gotFilter.StoreID)

	stub2 := &stubTicketLister{}
	getTicketsWithQuery(t, stub2, "")
	require.Nil(t, stub2.gotFilter.StoreID, "absent store_id must stay nil, meaning every store")
}

// from/to win over since_hours when both are supplied, matching #276. Pin the
// EXACT instant that reaches the repository: asserting merely that From is
// non-nil would pass whichever source won.
func TestTickets_ExplicitRangeWinsOverSinceHours(t *testing.T) {
	stub := &stubTicketLister{}
	from := "2026-08-01T00:00:00Z"
	getTicketsWithQuery(t, stub, "?since_hours=24&from="+from)

	require.NotNil(t, stub.gotFilter.From)
	want, err := time.Parse(time.RFC3339, from)
	require.NoError(t, err)
	require.True(t, stub.gotFilter.From.Equal(want),
		"explicit from must win over since_hours; got %v", stub.gotFilter.From)
}

// And with only since_hours, From is derived from it rather than left unset.
func TestTickets_SinceHoursAppliesWhenNoExplicitRange(t *testing.T) {
	stub := &stubTicketLister{}
	getTicketsWithQuery(t, stub, "?since_hours=24")
	require.NotNil(t, stub.gotFilter.From, "since_hours must produce a From bound")
}

// A repository failure must never render as an empty success — an operator
// reading `data: []` would conclude there are no tickets when the query blew up.
func TestTickets_RepoErrorIs500NotEmptySuccess(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{err: errors.New("pq: connection refused")})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), `"data"`,
		"a failed read must not shape a result at all")
	require.NotContains(t, rec.Body.String(), "connection refused",
		"driver error text must be logged server-side, never echoed")
}

// The golden fixture pins the exact contract shape as bytes, catching a
// rename or an unauthorized addition that a struct-shaped assertion would
// happily accept.
func TestTicketsMatchesPinnedContract(t *testing.T) {
	rec := getTickets(t, &stubTicketLister{result: ticket.ListResult{
		Tickets: ticketsFixture(), Total: 2,
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/tickets_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}
