package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

var discountTenantID = uuid.MustParse("dddddddd-1111-1111-1111-111111111111")
var discountStoreA = uuid.MustParse("dddddddd-2222-2222-2222-222222222222")
var discountStoreB = uuid.MustParse("dddddddd-3333-3333-3333-333333333333")

// stubDiscounter records what the handler asked for and returns a canned
// report. Hand-rolled rather than generated: the package has no mocking
// framework and stubExtender (billing_trial_extend_test.go) is the shape
// every double on this surface follows.
type stubDiscounter struct {
	applyResult  tenantdiscount.Result
	removeResult tenantdiscount.Result
	applyErr     error
	removeErr    error

	applyCalls  int
	removeCalls int
	gotIn       tenantdiscount.Input
}

func (s *stubDiscounter) Apply(_ context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error) {
	s.applyCalls++
	s.gotIn = in
	if s.applyErr != nil {
		return tenantdiscount.Result{}, s.applyErr
	}
	return s.applyResult, nil
}

func (s *stubDiscounter) Remove(_ context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error) {
	s.removeCalls++
	s.gotIn = in
	if s.removeErr != nil {
		return tenantdiscount.Result{}, s.removeErr
	}
	return s.removeResult, nil
}

// twoStoreApplyResult is the fan-out this endpoint exists to report: one
// store where the coupon went on, one card-less trialing store that could
// not carry it. A response that flattened these to a single status would
// throw away exactly the thing the console renders.
func twoStoreApplyResult() tenantdiscount.Result {
	return tenantdiscount.Result{
		TenantID: discountTenantID,
		CouponID: "cpn_test",
		Stores: []tenantdiscount.StoreResult{
			{
				StoreID:              discountStoreA,
				SubscriptionID:       uuid.MustParse("dddddddd-4444-4444-4444-444444444444"),
				StripeCustomerID:     "cus_a",
				StripeSubscriptionID: "sub_a",
				Outcome:              tenantdiscount.OutcomeApplied,
			},
			{
				StoreID:          discountStoreB,
				SubscriptionID:   uuid.MustParse("dddddddd-5555-5555-5555-555555555555"),
				StripeCustomerID: "cus_b",
				Outcome:          tenantdiscount.OutcomePending,
			},
		},
	}
}

const discountBody = `{"coupon_id":"cpn_test","reason":"negotiated renewal discount"}`

func discountPath(tenantID string) string {
	return "/admin/billing/tenants/" + tenantID + "/discount"
}

// doDiscount drives the handler with a nil DB, which is how the trial
// extend unit tests skip the idempotency store: the missing-header check
// still fires, but nothing touches Postgres. Integration coverage of the
// idempotency dance lives in billing_tenant_discount_integration_test.go.
func doDiscount(t *testing.T, svc platformadmin.TenantDiscounter, method, tenantID, body string, withKey bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBillingTenantDiscountHandler(nil, svc, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, discountPath(tenantID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if withKey {
		req.Header.Set("Idempotency-Key", "test-key-"+tenantID)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func decodeDiscount(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())
	return got
}

// Both routes refuse without an Idempotency-Key: a write that cannot be
// retried safely is worse than one that refuses to start, and the DELETE
// is as much a billing change as the POST.
func TestTenantDiscountRequiresIdempotencyKey(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			svc := &stubDiscounter{applyResult: twoStoreApplyResult(), removeResult: twoStoreApplyResult()}
			rec := doDiscount(t, svc, method, discountTenantID.String(), discountBody, false)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, "idempotency_key_required", decodeDiscount(t, rec)["error"])
			require.Zero(t, svc.applyCalls+svc.removeCalls, "the domain must not be called at all")
		})
	}
}

// A blank Idempotency-Key is the same refusal as a missing one — the
// header is trimmed before it is judged.
func TestTenantDiscountRejectsBlankIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &stubDiscounter{applyResult: twoStoreApplyResult()}
	r := gin.New()
	platformadmin.NewBillingTenantDiscountHandler(nil, svc, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, discountPath(discountTenantID.String()),
		bytes.NewBufferString(discountBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "   ")
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "idempotency_key_required", decodeDiscount(t, rec)["error"])
	require.Zero(t, svc.applyCalls)
}

// A malformed tenant id is 400, not 500 — the caller's error, in the shape
// tenant_lifecycle.go already returns so the console handles every write on
// this surface the same way.
func TestTenantDiscountRejectsInvalidTenantID(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			svc := &stubDiscounter{}
			rec := doDiscount(t, svc, method, "not-a-uuid", discountBody, true)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			body := decodeDiscount(t, rec)
			require.Equal(t, "invalid_tenant_id", body["error"])
			require.Equal(t, "tenant_id", body["field"])
			require.Zero(t, svc.applyCalls+svc.removeCalls)
		})
	}
}

// The console's namespaced "<source>:<id>" is NOT accepted here: platform-api
// splits it before it reaches this service (PR 2). Sending one must be a
// clean 400, never a tenant id that half-parses.
func TestTenantDiscountRejectsNamespacedTenantID(t *testing.T) {
	svc := &stubDiscounter{}
	rec := doDiscount(t, svc, http.MethodPost, "mark8ly:"+discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "invalid_tenant_id", decodeDiscount(t, rec)["error"])
	require.Zero(t, svc.applyCalls)
}

func TestTenantDiscountRequiresCouponID(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			svc := &stubDiscounter{}
			rec := doDiscount(t, svc, method, discountTenantID.String(),
				`{"coupon_id":"  ","reason":"why"}`, true)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			body := decodeDiscount(t, rec)
			require.Equal(t, "invalid_coupon_id", body["error"])
			require.Equal(t, "coupon_id", body["field"])
			require.Zero(t, svc.applyCalls+svc.removeCalls)
		})
	}
}

// The reason is mandatory on BOTH routes: tesserix-home#331's rule is that
// removal is as audited as application, and an audit row saying what
// happened without why is the gap this series exists to close.
func TestTenantDiscountRequiresReason(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			svc := &stubDiscounter{}
			rec := doDiscount(t, svc, method, discountTenantID.String(),
				`{"coupon_id":"cpn_test","reason":"   "}`, true)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			body := decodeDiscount(t, rec)
			require.Equal(t, "invalid_reason", body["error"])
			require.Equal(t, "reason", body["field"])
			require.Zero(t, svc.applyCalls+svc.removeCalls)
		})
	}
}

// An omitted body is rejected by the binder (gin returns io.EOF), before
// the field checks above ever run.
func TestTenantDiscountRejectsUnparseableBody(t *testing.T) {
	svc := &stubDiscounter{}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), ``, true)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "invalid_request", decodeDiscount(t, rec)["error"])
	require.Zero(t, svc.applyCalls)
}

// The reason is truncated on a RUNE boundary before it reaches the domain,
// which puts it straight into the audit row's jsonb metadata. Invalid UTF-8
// there fails the marshal, and under EmitTx that failure rolls the store's
// transaction back — so a mis-truncated reason does not mean "succeeded
// unaudited", it means the discount silently did not apply.
func TestTenantDiscountTruncatesReasonOnARuneBoundary(t *testing.T) {
	// "€" is THREE bytes, so a 500-byte cap lands mid-rune (500 = 166*3 + 2)
	// and the truncation has real work to do. A 2-byte rune would divide the
	// cap exactly and the test would pass without exercising anything.
	long := strings.Repeat("€", 300)
	body, err := json.Marshal(map[string]string{"coupon_id": "cpn_test", "reason": long})
	require.NoError(t, err)

	svc := &stubDiscounter{applyResult: twoStoreApplyResult()}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), string(body), true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Equal(t, 1, svc.applyCalls)
	got := svc.gotIn.Reason
	require.LessOrEqual(t, len(got), 500, "reason must be capped before it reaches the domain")
	require.True(t, utf8.ValidString(got), "a truncated reason must still be valid UTF-8")
	require.Equal(t, 498, len(got), "the two bytes of the half rune at the 500-byte cap must be dropped, not kept")
}

// The fan-out's whole point is that outcomes differ per store, so the
// response carries one line per store rather than a single status.
func TestTenantDiscountApplyReportsPerStore(t *testing.T) {
	svc := &stubDiscounter{applyResult: twoStoreApplyResult()}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := decodeDiscount(t, rec)
	require.Equal(t, discountTenantID.String(), body["tenant_id"])
	require.Equal(t, "cpn_test", body["coupon_id"])
	require.Equal(t, "apply", body["operation"])
	require.Equal(t, "ok", body["status"])

	stores, ok := body["stores"].([]any)
	require.True(t, ok, "stores must be an array: %s", rec.Body.String())
	require.Len(t, stores, 2)

	first := stores[0].(map[string]any)
	require.Equal(t, discountStoreA.String(), first["store_id"])
	require.Equal(t, "applied", first["outcome"])
	require.Equal(t, "sub_a", first["stripe_subscription_id"])

	second := stores[1].(map[string]any)
	require.Equal(t, discountStoreB.String(), second["store_id"])
	require.Equal(t, "pending", second["outcome"])
	require.NotContains(t, second, "stripe_subscription_id",
		"a card-less store has no stripe subscription id to report")

	require.Equal(t, 1, svc.applyCalls)
	require.Zero(t, svc.removeCalls)
	require.Equal(t, discountTenantID, svc.gotIn.TenantID)
	require.Equal(t, "cpn_test", svc.gotIn.CouponID)
	require.Equal(t, "negotiated renewal discount", svc.gotIn.Reason)
	require.NotNil(t, svc.gotIn.C, "the gin context must be forwarded so the audit row names the operator")
}

// DELETE reaches Remove, not Apply, and reports the same per-store shape.
func TestTenantDiscountRemoveReportsPerStore(t *testing.T) {
	res := twoStoreApplyResult()
	res.Stores[0].Outcome = tenantdiscount.OutcomeRemoved
	res.Stores[1].Outcome = tenantdiscount.OutcomeNotApplied
	svc := &stubDiscounter{removeResult: res}

	rec := doDiscount(t, svc, http.MethodDelete, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := decodeDiscount(t, rec)
	require.Equal(t, "remove", body["operation"])
	stores := body["stores"].([]any)
	require.Equal(t, "removed", stores[0].(map[string]any)["outcome"])
	require.Equal(t, "not_applied", stores[1].(map[string]any)["outcome"])

	require.Equal(t, 1, svc.removeCalls)
	require.Zero(t, svc.applyCalls, "DELETE must never apply a discount")
}

// Every outcome the domain declares survives the trip to JSON under its own
// name. A response that collapsed two of them would make the console show
// "nothing happened" for a store where something did.
func TestTenantDiscountSurfacesEveryOutcomeVerbatim(t *testing.T) {
	all := []tenantdiscount.Outcome{
		tenantdiscount.OutcomeApplied,
		tenantdiscount.OutcomeAlreadyApplied,
		tenantdiscount.OutcomeRemoved,
		tenantdiscount.OutcomeNotApplied,
		tenantdiscount.OutcomePending,
		tenantdiscount.OutcomeNoSubscription,
		tenantdiscount.OutcomeNoStripeCustomer,
	}
	res := tenantdiscount.Result{TenantID: discountTenantID, CouponID: "cpn_test"}
	for range all {
		res.Stores = append(res.Stores, tenantdiscount.StoreResult{StoreID: uuid.New()})
	}
	for i, o := range all {
		res.Stores[i].Outcome = o
	}

	svc := &stubDiscounter{applyResult: res}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stores := decodeDiscount(t, rec)["stores"].([]any)
	require.Len(t, stores, len(all))
	for i, o := range all {
		require.Equal(t, string(o), stores[i].(map[string]any)["outcome"])
	}
}

// A failed store carries a stage-named failure code and a fixed message —
// never the driver's own error text, which is logged server-side instead.
func TestTenantDiscountFailedStoreNeverEchoesDriverText(t *testing.T) {
	res := twoStoreApplyResult()
	res.Stores[1].Outcome = tenantdiscount.OutcomeFailed
	res.Stores[1].FailureCode = tenantdiscount.FailureLoadSubscription
	res.Stores[1].Err = errors.New(`pq: relation "store_subscriptions" does not exist`)

	svc := &stubDiscounter{applyResult: res}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "does not exist", "driver text must never be echoed")

	body := decodeDiscount(t, rec)
	require.Equal(t, "partial", body["status"], "one applied store and one failed store is a partial result")
	failed := body["stores"].([]any)[1].(map[string]any)
	require.Equal(t, "failed", failed["outcome"])
	require.Equal(t, "load_subscription_failed", failed["failure_code"])
	require.NotEmpty(t, failed["failure_reason"])
}

// The divergence, per store: Stripe holds the discount and no audit row
// explains it. It must get its OWN failure code, not the routine
// audit_write_failed / commit_failed the domain also sets, and the response
// must say reconciliation is required.
func TestTenantDiscountPerStoreDivergenceGetsItsOwnCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		code tenantdiscount.FailureCode
	}{
		{"audit_insert_failed", tenantdiscount.FailureAuditWrite},
		{"commit_failed", tenantdiscount.FailureCommit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := twoStoreApplyResult()
			res.Stores[0].Outcome = tenantdiscount.OutcomeFailed
			res.Stores[0].FailureCode = tc.code
			res.Stores[0].Err = &tenantdiscount.AuditDivergenceError{
				Op:                   "apply",
				StoreID:              discountStoreA,
				CouponID:             "cpn_test",
				StripeSubscriptionID: "sub_a",
				StripeCustomerID:     "cus_a",
				Cause:                errors.New("pq: deadlock detected"),
			}

			svc := &stubDiscounter{applyResult: res}
			rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "deadlock detected")

			body := decodeDiscount(t, rec)
			require.Equal(t, true, body["requires_reconciliation"],
				"a discount stripe holds and no audit row explains must be flagged at the top level")

			failed := body["stores"].([]any)[0].(map[string]any)
			require.Equal(t, "stripe_changed_audit_write_failed", failed["failure_code"],
				"the divergence must not be reported as a routine %s", tc.code)
		})
	}
}

// A per-store Stripe failure is NOT the divergence: nothing was changed and
// the caller may retry. Distinct code, and requires_reconciliation absent.
func TestTenantDiscountPerStoreStripeFailureIsNotTheDivergence(t *testing.T) {
	res := twoStoreApplyResult()
	res.Stores[0].Outcome = tenantdiscount.OutcomeFailed
	res.Stores[0].FailureCode = tenantdiscount.FailureStripeCall
	res.Stores[0].Err = tenantdiscount.ErrStripeCall

	svc := &stubDiscounter{applyResult: res}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	body := decodeDiscount(t, rec)
	require.NotContains(t, body, "requires_reconciliation")
	require.Equal(t, "stripe_call_failed",
		body["stores"].([]any)[0].(map[string]any)["failure_code"])
}

// Every store failing is reported as `failed`, not `partial` — an operator
// reading the top-level status must not be told part of it worked.
func TestTenantDiscountAllStoresFailedIsStatusFailed(t *testing.T) {
	res := twoStoreApplyResult()
	for i := range res.Stores {
		res.Stores[i].Outcome = tenantdiscount.OutcomeFailed
		res.Stores[i].FailureCode = tenantdiscount.FailureStripeCall
		res.Stores[i].Err = tenantdiscount.ErrStripeCall
	}
	svc := &stubDiscounter{applyResult: res}
	rec := doDiscount(t, svc, http.MethodPost, discountTenantID.String(), discountBody, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "failed", decodeDiscount(t, rec)["status"])
}

// Each whole-request sentinel gets its own status and code, so the console
// can tell "this tenant has no stores" from "the lookup broke" rather than
// getting one opaque refusal.
func TestTenantDiscountDomainSentinelsMapToDistinctCodes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"no_stores", tenantdiscount.ErrNoStores, http.StatusNotFound, "no_stores"},
		{"no_tenant", tenantdiscount.ErrNoTenant, http.StatusBadRequest, "invalid_tenant_id"},
		{"no_coupon", tenantdiscount.ErrNoCoupon, http.StatusBadRequest, "invalid_coupon_id"},
		{"stripe_call", tenantdiscount.ErrStripeCall, http.StatusBadGateway, "stripe_unavailable"},
		{
			"divergence",
			&tenantdiscount.AuditDivergenceError{Op: "apply", Cause: errors.New("pq: deadlock detected")},
			http.StatusInternalServerError, "stripe_changed_audit_write_failed",
		},
		{"unknown", errors.New("pq: connection refused"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubDiscounter{applyErr: tc.err, removeErr: tc.err}
			for _, method := range []string{http.MethodPost, http.MethodDelete} {
				rec := doDiscount(t, svc, method, discountTenantID.String(), discountBody, true)
				require.Equal(t, tc.status, rec.Code, rec.Body.String())
				body := decodeDiscount(t, rec)
				require.Equal(t, tc.code, body["error"])
				require.NotContains(t, rec.Body.String(), "connection refused")
				require.NotContains(t, rec.Body.String(), "deadlock detected")
			}
		})
	}
}
