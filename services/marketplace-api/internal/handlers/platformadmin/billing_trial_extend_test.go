package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

var extendStoreID = uuid.MustParse("bbbbbbbb-1111-1111-1111-111111111111")
var extendTenantID = uuid.MustParse("aaaaaaaa-1111-1111-1111-111111111111")

type stubExtender struct {
	result  trial.ExtendResult
	err     error
	calls   int
	gotEnd  time.Time
	gotStor uuid.UUID
}

func (s *stubExtender) Extend(_ context.Context, _ *gorm.DB, storeID uuid.UUID, newEnd, _ time.Time) (trial.ExtendResult, error) {
	s.calls++
	s.gotStor = storeID
	s.gotEnd = newEnd
	if s.err != nil {
		return trial.ExtendResult{}, s.err
	}
	return s.result, nil
}

type capturedAudit struct{ events []audit.Event }

func (c *capturedAudit) fn(_ *gin.Context, _ uuid.UUID, ev audit.Event) error {
	c.events = append(c.events, ev)
	return nil
}

func okResult() trial.ExtendResult {
	return trial.ExtendResult{
		SubscriptionID:   uuid.MustParse("cccccccc-1111-1111-1111-111111111111"),
		TenantID:         extendTenantID,
		StoreID:          extendStoreID,
		PreviousEndsAt:   time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC),
		NewEndsAt:        time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		RemindersCleared: 2,
	}
}

// postExtend sends the Idempotency-Key header by default — required since
// this handler makes the endpoint safe to retry. Tests that specifically
// exercise the missing-header case use postExtendNoIdempotencyKey instead.
func postExtend(t *testing.T, ex platformadmin.TrialExtender, aud *capturedAudit, storeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postExtendWithHeaders(t, ex, aud, storeID, body, true)
}

// postExtendNoIdempotencyKey omits the Idempotency-Key header, for the test
// asserting that its absence is refused.
func postExtendNoIdempotencyKey(t *testing.T, ex platformadmin.TrialExtender, aud *capturedAudit, storeID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postExtendWithHeaders(t, ex, aud, storeID, body, false)
}

func postExtendWithHeaders(t *testing.T, ex platformadmin.TrialExtender, aud *capturedAudit, storeID, body string, withIdempotencyKey bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(nil, ex, aud.fn, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/admin/billing/trials/"+storeID+"/extend", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if withIdempotencyKey {
		req.Header.Set("Idempotency-Key", "test-key-"+storeID)
	}
	r.ServeHTTP(rec, req)
	return rec
}

const goodBody = `{"reason_code":"support_escalation","reason":"migration slipped two weeks","trial_ends_at":"2026-12-01T00:00:00Z"}`

// The golden fixture pins the contract as bytes, catching a rename or an
// unauthorized addition that a struct-shaped assertion would accept.
func TestTrialExtendMatchesPinnedContract(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	want, err := os.ReadFile("testdata/trial_extend_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

// Every declared reason code is accepted, and one outside the set is
// refused with #287's exact error shape. Both directions asserted: a check
// that always passes and one that always fails look identical otherwise.
func TestTrialExtendReasonCodes(t *testing.T) {
	for _, code := range platformadmin.ExtendReasonCodes {
		t.Run("accepts_"+code, func(t *testing.T) {
			body := `{"reason_code":"` + code + `","trial_ends_at":"2026-12-01T00:00:00Z"}`
			rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(), body)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}

	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(),
		`{"reason_code":"because_i_said_so","trial_ends_at":"2026-12-01T00:00:00Z"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp struct {
		Error   string   `json:"error"`
		Field   string   `json:"field"`
		Allowed []string `json:"allowed"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "invalid_reason_code", resp.Error)
	require.Equal(t, "reason_code", resp.Field)
	require.Equal(t, platformadmin.ExtendReasonCodes, resp.Allowed,
		"the response must publish the allowed set, as #287 does")
}

// An absent reason_code is refused — `{}` binds successfully to the zero
// value, so this is the case the check exists to catch.
func TestTrialExtendRequiresReasonCode(t *testing.T) {
	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(),
		`{"trial_ends_at":"2026-12-01T00:00:00Z"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_reason_code")
}

// Each domain refusal maps to its own status and code, so the console can
// tell them apart. Every row asserted — a mapping is exactly the kind of
// table where one wrong entry hides behind the others.
func TestTrialExtendRefusalMapping(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{trial.ErrAlreadyConverted, http.StatusConflict, "already_converted"},
		{trial.ErrStripeManaged, http.StatusConflict, "stripe_managed"},
		{trial.ErrNotTrialing, http.StatusConflict, "not_trialing"},
		{trial.ErrEndNotInFuture, http.StatusBadRequest, "invalid_request"},
		{trial.ErrNoSubscription, http.StatusNotFound, "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.wantCode, func(t *testing.T) {
			aud := &capturedAudit{}
			rec := postExtend(t, &stubExtender{err: tc.err}, aud, extendStoreID.String(), goodBody)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())

			var resp struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, tc.wantCode, resp.Error)
			require.Empty(t, aud.events, "a refused extension must not write an audit row")
		})
	}
}

// A malformed store id is a 400, not a 500 — #343 records the opposite
// happening on another internal route.
func TestTrialExtendMalformedStoreIDIs400(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, "not-a-uuid", goodBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, ex.calls, "the domain call must not be reached with an unparsed id")
}

// The audit row carries the action, the reason code, the free text and both
// dates. Asserting the VALUES, not merely that an event was emitted: a
// payload built by map lookup returns the zero value for a missing key.
func TestTrialExtendEmitsAnAttributedAuditRow(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, aud.events, 1)
	ev := aud.events[0]
	require.Equal(t, "trial.extended", ev.Action)

	raw, err := json.Marshal(ev.Metadata)
	require.NoError(t, err)
	body := string(raw)
	require.Contains(t, body, "support_escalation")
	require.Contains(t, body, "migration slipped two weeks")
	require.Contains(t, body, "2026-12-01T00:00:00Z")
	require.Contains(t, body, "2026-09-14T10:22:31Z")
}

// A body that is not JSON at all is refused before the reason-code check,
// matching #287's binder behaviour.
func TestTrialExtendUnparseableBody(t *testing.T) {
	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(), `not json`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid_request")
}

// An unparseable trial_ends_at is a 400 and never reaches the domain call.
func TestTrialExtendUnparseableDate(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, extendStoreID.String(),
		`{"reason_code":"goodwill","trial_ends_at":"next tuesday"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, ex.calls)
}

// The Idempotency-Key header is REQUIRED. A write that cannot be retried
// safely is worse than one that refuses to start.
func TestTrialExtendRequiresIdempotencyKey(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtendNoIdempotencyKey(t, ex, &capturedAudit{}, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "idempotency_key_required")
	require.Equal(t, 0, ex.calls)
}

// `reason` must be OMITTED from the response JSON when empty, not present
// with an empty value — asserted on the raw body bytes, which is the only
// way to distinguish an absent key from an empty one. An unmarshalled
// struct assertion cannot tell those apart.
func TestTrialExtendOmitsEmptyReasonFromResponse(t *testing.T) {
	body := `{"reason_code":"goodwill","trial_ends_at":"2026-12-01T00:00:00Z"}`
	rec := postExtend(t, &stubExtender{result: okResult()}, &capturedAudit{}, extendStoreID.String(), body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), `"reason"`,
		"an empty reason must be omitted from the response, not sent as an empty string")
}
