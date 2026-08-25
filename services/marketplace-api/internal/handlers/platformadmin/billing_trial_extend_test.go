package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
//
// extended_at is necessarily dynamic — it is the wall-clock instant the
// extension was performed, set from time.Now() in the handler — so it is
// verified separately (present, RFC3339, recent) and then normalized to
// the fixture's placeholder value before the byte-for-byte comparison.
func TestTrialExtendMatchesPinnedContract(t *testing.T) {
	aud := &capturedAudit{}
	before := time.Now().UTC()
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	extendedAt, ok := got["extended_at"].(string)
	require.True(t, ok, "extended_at must be present in the response")
	parsed, err := time.Parse(time.RFC3339, extendedAt)
	require.NoError(t, err, "extended_at must be RFC3339")
	require.WithinDuration(t, before, parsed, 5*time.Second,
		"extended_at must be the instant the extension was performed")

	got["extended_at"] = "2026-08-25T00:00:00Z"
	normalized, err := json.Marshal(got)
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/trial_extend_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(normalized))
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
// happening on another internal route. The error shape matches
// tenant_lifecycle.go's invalid_tenant_id (field: "id") — invalid_store_id
// with field: "store_id" — so the console handles both writes the same way.
func TestTrialExtendMalformedStoreIDIs400(t *testing.T) {
	ex := &stubExtender{result: okResult()}
	rec := postExtend(t, ex, &capturedAudit{}, "not-a-uuid", goodBody)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, ex.calls, "the domain call must not be reached with an unparsed id")

	var resp struct {
		Error string `json:"error"`
		Field string `json:"field"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "invalid_store_id", resp.Error)
	require.Equal(t, "store_id", resp.Field)
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

// A reason with a multibyte rune sitting on the 500-byte truncation
// boundary must not be cut mid-rune: a raw byte-slice truncation there
// produces invalid UTF-8, which fails to marshal into the audit row's
// jsonb Metadata column, silently drops the audit emit, and leaves the
// extension unaudited — the exact gap this series exists to close.
func TestTrialExtendTruncatesReasonOnARuneBoundary(t *testing.T) {
	longReason := strings.Repeat("日", 400) // 3 bytes/rune * 400 = 1200 bytes, well past the 500 cap

	type reqBody struct {
		ReasonCode  string `json:"reason_code"`
		Reason      string `json:"reason"`
		TrialEndsAt string `json:"trial_ends_at"`
	}
	raw, err := json.Marshal(reqBody{ReasonCode: "goodwill", Reason: longReason, TrialEndsAt: "2026-12-01T00:00:00Z"})
	require.NoError(t, err)

	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), string(raw))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, aud.events, 1)
	reason, _ := aud.events[0].Metadata["reason"].(string)
	require.NotEmpty(t, reason)
	require.LessOrEqual(t, len(reason), 500)
	require.True(t, utf8.ValidString(reason), "a truncated multibyte reason must remain valid UTF-8")

	var resp struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, utf8.ValidString(resp.Reason))
}

// stripeOKResult is okResult() plus the Stripe-side facts a card-backed
// extension produces. Every value is DISTINCT and non-zero, AND
// StripeTrialEnd is deliberately a DIFFERENT instant from goodBody's
// trial_ends_at ("2026-12-01T00:00:00Z"): the response and audit row must
// report what STRIPE actually stored, not an echo of the request, and a
// fixture where the two happened to match would let a handler that echoes
// the request pass every assertion here undetected.
func stripeOKResult() trial.ExtendResult {
	r := okResult()
	r.StripeApplied = true
	r.StripeSubscriptionID = "sub_verify_358"
	r.StripeTrialEnd = time.Date(2026, 12, 3, 8, 15, 0, 0, time.UTC).Unix()
	r.PreviousStripeTrialEnd = time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC).Unix()
	r.PreviousBillingAnchor = time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC).Unix()
	return r
}

// The new refusals must be distinguishable by the console. 502 is
// deliberately NOT the handler's existing 503 `unavailable`: 503 means our
// own idempotency store is unreachable, 502 means the dependency refused —
// and, critically, that no local write happened.
func TestExtend_StripeRefusalsMapToDistinctStatuses(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"unconfigured", trial.ErrStripeManaged, http.StatusConflict, "stripe_managed"},
		{"state conflict", trial.ErrStripeStateConflict, http.StatusConflict, "stripe_state_conflict"},
		{"too far", trial.ErrTrialEndTooFar, http.StatusBadRequest, "trial_end_too_far"},
		{"call failed", fmt.Errorf("%w: update trial end: boom", trial.ErrStripeCall), http.StatusBadGateway, "stripe_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aud := &capturedAudit{}
			rec := postExtend(t, &stubExtender{err: tc.err}, aud, extendStoreID.String(), goodBody)
			require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())

			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tc.wantCode, got["error"])

			// A refusal is not an operator action: nothing was extended, so
			// nothing may be audited as extended.
			require.Empty(t, aud.events, "a refused extension must emit no audit event")

			// The driver's own text must never be echoed to the caller.
			msg, _ := got["message"].(string)
			require.NotContains(t, msg, "boom")
		})
	}
}

// A card-backed extension must disclose that the billing anchor moved, and
// must echo STRIPE's value rather than the request's.
func TestExtend_CardBacked_ResponseDisclosesStripeFacts(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: stripeOKResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "sub_verify_358", got["stripe_subscription_id"])
	require.Equal(t, "2026-12-03T08:15:00Z", got["stripe_trial_end"])
	require.Equal(t, true, got["billing_anchor_moved"])

	// The request asked for a DIFFERENT instant ("2026-12-01T00:00:00Z" in
	// goodBody). A handler that echoed the request's parsed trial_ends_at
	// instead of res.StripeTrialEnd would still satisfy the Equal above by
	// coincidence in a collision fixture; this guards against exactly that.
	require.NotEqual(t, "2026-12-01T00:00:00Z", got["stripe_trial_end"],
		"stripe_trial_end must be Stripe's reply, not the request's trial_ends_at")
}

// A card-less extension must carry NONE of those keys — not null, not false,
// absent. Asserted on the RAW BYTES: a decoded map cannot tell an absent key
// from a false one, which is exactly the distinction being made here.
func TestExtend_CardLess_OmitsStripeFields(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := rec.Body.String()
	require.NotContains(t, body, "stripe_subscription_id")
	require.NotContains(t, body, "stripe_trial_end")
	require.NotContains(t, body, "billing_anchor_moved")
}

// The audit row must carry the exact integer sent to Stripe. An audit that
// records only "extended" cannot answer "extended to what, in Stripe?" —
// which is the question this whole series exists to be able to answer.
func TestExtend_CardBacked_AuditCarriesExactUnixSecond(t *testing.T) {
	aud := &capturedAudit{}
	res := stripeOKResult()
	rec := postExtend(t, &stubExtender{result: res}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Len(t, aud.events, 1)
	md := aud.events[0].Metadata
	require.Equal(t, "sub_verify_358", md["stripe_subscription_id"])
	require.Equal(t, res.StripeTrialEnd, md["stripe_trial_end_unix"])
	require.Equal(t, res.PreviousStripeTrialEnd, md["previous_stripe_trial_end_unix"])
	require.Equal(t, res.PreviousBillingAnchor, md["previous_billing_anchor_unix"])

	// The two anchors are DIFFERENT values in this fixture, so a mapping
	// that swapped them would be caught. Identical fixtures prove nothing.
	require.NotEqual(t, md["stripe_trial_end_unix"], md["previous_billing_anchor_unix"])

	// The request's parsed trial_ends_at ("2026-12-01T00:00:00Z") is a
	// different instant from res.StripeTrialEnd. A handler that audited the
	// REQUEST's end instead of Stripe's reply would still pass the Equal
	// above by coincidence in a collision fixture; this guards against
	// exactly that.
	requestEnd := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).Unix()
	require.NotEqual(t, requestEnd, md["stripe_trial_end_unix"],
		"stripe_trial_end_unix must be Stripe's reply, not the request's trial_ends_at")
}

// A card-less extension must add none of the Stripe keys to the audit row
// either — the metadata says what happened, and nothing Stripe-shaped did.
func TestExtend_CardLess_AuditHasNoStripeKeys(t *testing.T) {
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{result: okResult()}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, aud.events, 1)
	for _, k := range []string{"stripe_subscription_id", "stripe_trial_end_unix",
		"previous_stripe_trial_end_unix", "previous_billing_anchor_unix"} {
		_, present := aud.events[0].Metadata[k]
		require.False(t, present, "card-less audit must not carry %s", k)
	}
}

// ErrStripeAppliedLocalWriteFailed is the one divergence #358's design
// accepts: Stripe already moved a real merchant's billing_cycle_anchor and
// the local commit then failed, so this service recorded nothing. It must
// never collapse into the ordinary 500 or the 502 used for ErrStripeCall —
// both of those imply nothing happened, and here something very much did,
// to a real billing date. No audit emit is attempted on this path: it would
// write to the same database whose write just failed, and a failed emit is
// swallowed by design, so the loud Error log is the only signal.
func TestExtend_StripeAppliedLocalWriteFailed_Maps500NoAudit(t *testing.T) {
	err := fmt.Errorf("%w: stripe subscription sub_verify_358 now holds trial_end=1798761600 (previously trial_end=1757845351, billing_cycle_anchor=1757845351): write trial_ends_at: boom",
		trial.ErrStripeAppliedLocalWriteFailed)
	aud := &capturedAudit{}
	rec := postExtend(t, &stubExtender{err: err}, aud, extendStoreID.String(), goodBody)
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "stripe_applied_local_write_failed", got["error"])

	require.Empty(t, aud.events, "no audit emit must be attempted when the local write itself failed")

	// The driver's own text must never be echoed to the caller.
	msg, _ := got["message"].(string)
	require.NotContains(t, msg, "boom")
}
