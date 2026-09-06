//go:build integration

package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// discountRouter mounts the handler against a real database, which is what
// makes the Reserve/Lookup/Complete/Release dance in the handler reachable
// at all — the unit tests pass a nil DB and skip it entirely.
func discountRouter(db *gorm.DB, svc platformadmin.TenantDiscounter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewBillingTenantDiscountHandler(db, svc, nil).Register(r.Group(""))
	return r
}

// sendDiscount drives one operation. It takes an OPERATION, not a method:
// apply and remove are two distinct routes (POST .../discount and POST
// .../discount/remove), and discountRoute is the single place that pairs a
// method with its path — see its doc comment.
func sendDiscount(r *gin.Engine, op, tenantID, key, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	method, path := discountRoute(op, tenantID)
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	r.ServeHTTP(rec, req)
	return rec
}

// The same key replays the stored body without calling the domain a second
// time — no second Stripe round-trip and no second audit row, since both
// live inside Apply. A different key is a new request.
func TestTenantDiscountIsIdempotentPerKey(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")
	svc := &stubDiscounter{applyResult: twoStoreApplyResult()}
	r := discountRouter(db, svc)

	first := sendDiscount(r, opApply, discountTenantID.String(), "key-alpha", discountBody)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := sendDiscount(r, opApply, discountTenantID.String(), "key-alpha", discountBody)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.JSONEq(t, first.Body.String(), second.Body.String(), "same key must replay the same body")
	require.Equal(t, 1, svc.applyCalls, "same key must NOT apply the discount a second time")

	third := sendDiscount(r, opApply, discountTenantID.String(), "key-beta", discountBody)
	require.Equal(t, http.StatusOK, third.Code, third.Body.String())
	require.Equal(t, 2, svc.applyCalls, "a DIFFERENT key is a new request")
}

// idempotency_keys.key is a bare primary key shared by the whole service, so
// the handler scopes it. Reusing one key against two tenants must perform
// TWO fan-outs and return each tenant's own report, not replay the first
// tenant's body onto the second.
func TestTenantDiscountKeyDoesNotReplayAcrossTenants(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	tenantA := uuid.New()
	tenantB := uuid.New()

	var seen []uuid.UUID
	svc := &platformadmin.TenantDiscounterFuncs{
		ApplyFunc: func(_ context.Context, in tenantdiscount.Input) (tenantdiscount.Result, error) {
			seen = append(seen, in.TenantID)
			return tenantdiscount.Result{
				TenantID: in.TenantID,
				CouponID: in.CouponID,
				Stores: []tenantdiscount.StoreResult{
					{StoreID: uuid.New(), Outcome: tenantdiscount.OutcomeApplied, StripeSubscriptionID: "sub_x"},
				},
			}, nil
		},
	}
	r := discountRouter(db, svc)

	first := sendDiscount(r, opApply, tenantA.String(), "shared-key", discountBody)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	second := sendDiscount(r, opApply, tenantB.String(), "shared-key", discountBody)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())

	require.Equal(t, []uuid.UUID{tenantA, tenantB},
		seen, "the same key against two tenants must perform TWO fan-outs")

	var firstResp, secondResp struct {
		TenantID string `json:"tenant_id"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))
	require.Equal(t, tenantA.String(), firstResp.TenantID)
	require.Equal(t, tenantB.String(), secondResp.TenantID)
}

// Apply and Remove are opposite billing changes, so they must not share one
// idempotency namespace: an operator reusing a key to REVOKE a discount they
// just applied would otherwise get the apply's stored report back and the
// discount would silently stay on.
//
// The separate routes do NOT provide this on their own, so this assertion
// still bites: the scoped key is "tenant_discount:<op>:<tenant>:<key>" and
// the request PATH never enters it. Drop the operation from that scope and
// the two calls below collide again, whatever paths they arrived on.
func TestTenantDiscountApplyAndRemoveDoNotShareAnIdempotencyScope(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")
	removed := twoStoreApplyResult()
	removed.Stores[0].Outcome = tenantdiscount.OutcomeRemoved
	svc := &stubDiscounter{applyResult: twoStoreApplyResult(), removeResult: removed}
	r := discountRouter(db, svc)

	applied := sendDiscount(r, opApply, discountTenantID.String(), "same-key", discountBody)
	require.Equal(t, http.StatusOK, applied.Code, applied.Body.String())

	revoked := sendDiscount(r, opRemove, discountTenantID.String(), "same-key", discountBody)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())

	require.Equal(t, 1, svc.removeCalls, "the remove call must run, not replay the apply's stored report")
	body := map[string]any{}
	require.NoError(t, json.Unmarshal(revoked.Body.Bytes(), &body))
	require.Equal(t, "remove", body["operation"])
}

// A domain refusal releases the reservation, so a corrected retry with the
// SAME key proceeds instead of answering 409 in_progress for the full TTL.
func TestTenantDiscountCorrectedRetryAfterDomainFailureProceeds(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")
	svc := &stubDiscounter{applyErr: tenantdiscount.ErrNoStores}
	r := discountRouter(db, svc)

	refused := sendDiscount(r, opApply, discountTenantID.String(), "key-retry", discountBody)
	require.Equal(t, http.StatusNotFound, refused.Code, refused.Body.String())

	// The operator fixes whatever was wrong and retries the same request.
	svc.applyErr = nil
	svc.applyResult = twoStoreApplyResult()
	retried := sendDiscount(r, opApply, discountTenantID.String(), "key-retry", discountBody)
	require.Equal(t, http.StatusOK, retried.Code, retried.Body.String())
	require.Equal(t, 2, svc.applyCalls, "the corrected retry must reach the domain")
}

// A partial result — one store applied, one failed — still COMPLETES the
// key. The stores that committed must not be re-applied by a retry of the
// same key; a retry aimed at the failed store is a new operator decision and
// takes a new key.
func TestTenantDiscountPartialResultCompletesTheKey(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")
	res := twoStoreApplyResult()
	res.Stores[1].Outcome = tenantdiscount.OutcomeFailed
	res.Stores[1].FailureCode = tenantdiscount.FailureStripeCall
	res.Stores[1].Err = tenantdiscount.ErrStripeCall
	svc := &stubDiscounter{applyResult: res}
	r := discountRouter(db, svc)

	first := sendDiscount(r, opApply, discountTenantID.String(), "key-partial", discountBody)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := sendDiscount(r, opApply, discountTenantID.String(), "key-partial", discountBody)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.JSONEq(t, first.Body.String(), second.Body.String())
	require.Equal(t, 1, svc.applyCalls, "a partial result is a completed request, not a retryable one")
}
