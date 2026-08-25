//go:build integration

package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Idempotency is the acceptance criterion this proves: the SAME key
// replays the stored body and writes NO second audit row, while a
// DIFFERENT key is a new extension. Asserting the audit-row count is what
// distinguishes real idempotency from a coincidentally identical response.
func TestTrialExtendIsIdempotentPerKey(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	ex := &stubExtender{result: okResult()}
	aud := &capturedAudit{}
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(db, ex, aud.fn, nil).Register(r.Group(""))

	do := func(key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost,
			"/admin/billing/trials/"+extendStoreID.String()+"/extend",
			bytes.NewBufferString(goodBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		r.ServeHTTP(rec, req)
		return rec
	}

	first := do("key-alpha")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := do("key-alpha")
	require.Equal(t, http.StatusOK, second.Code)
	require.JSONEq(t, first.Body.String(), second.Body.String(), "same key must replay the same body")
	require.Equal(t, 1, ex.calls, "same key must NOT perform a second extension")
	require.Len(t, aud.events, 1, "same key must NOT write a second audit row")

	third := do("key-beta")
	require.Equal(t, http.StatusOK, third.Code)
	require.Equal(t, 2, ex.calls, "a DIFFERENT key is a new extension")
	require.Len(t, aud.events, 2)
}

// The bug this closes (#286 final review, F1): idempotency_keys.key is a
// bare primary key shared by the whole service — nothing bound a key to
// the store it was used against. Reusing the SAME Idempotency-Key against
// a DIFFERENT store must perform TWO extensions and return each store's
// own response, not replay the first store's body onto the second. The
// stub echoes the requested store id into StoreID so a replay is
// detectable directly in the response bytes.
func TestTrialExtendKeyDoesNotReplayAcrossStores(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	storeA := uuid.New()
	storeB := uuid.New()

	var calledWith []uuid.UUID
	ex := platformadmin.TrialExtenderFunc(func(_ context.Context, _ *gorm.DB, storeID uuid.UUID, newEnd, _ time.Time) (trial.ExtendResult, error) {
		calledWith = append(calledWith, storeID)
		return trial.ExtendResult{
			SubscriptionID:   uuid.New(),
			TenantID:         uuid.New(),
			StoreID:          storeID,
			PreviousEndsAt:   time.Date(2026, 9, 14, 10, 22, 31, 0, time.UTC),
			NewEndsAt:        newEnd,
			RemindersCleared: 0,
		}, nil
	})
	aud := &capturedAudit{}
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(db, ex, aud.fn, nil).Register(r.Group(""))

	do := func(storeID uuid.UUID) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost,
			"/admin/billing/trials/"+storeID.String()+"/extend",
			bytes.NewBufferString(goodBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "shared-key") // SAME key, both requests
		r.ServeHTTP(rec, req)
		return rec
	}

	first := do(storeA)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := do(storeB)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())

	require.Len(t, calledWith, 2, "the same key against two different stores must perform TWO extensions")
	require.Equal(t, []uuid.UUID{storeA, storeB}, calledWith)
	require.Len(t, aud.events, 2, "each store's extension must be independently audited")

	var firstResp, secondResp struct {
		StoreID string `json:"store_id"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))
	require.Equal(t, storeA.String(), firstResp.StoreID)
	require.Equal(t, storeB.String(), secondResp.StoreID,
		"a global (unscoped) key would have replayed store A's response here")
}

// The bug this closes (#286 final review, F3): Lookup-then-Save is
// check-then-act, so two concurrent callers with the same key can both
// miss the lookup before either saves. The winner must do the work; the
// loser must see 409 in_progress and must NEVER call Extend while the
// winner is still running.
func TestTrialExtendSecondCallerDoesNotExecuteWhileFirstInFlight(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	gin.SetMode(gin.TestMode)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls int32

	ex := platformadmin.TrialExtenderFunc(func(_ context.Context, _ *gorm.DB, storeID uuid.UUID, newEnd, _ time.Time) (trial.ExtendResult, error) {
		atomic.AddInt32(&calls, 1)
		close(entered)
		<-release // held open until the test allows it to finish
		res := okResult()
		res.StoreID = storeID
		res.NewEndsAt = newEnd
		return res, nil
	})
	aud := &capturedAudit{}
	r := gin.New()
	platformadmin.NewBillingTrialExtendHandler(db, ex, aud.fn, nil).Register(r.Group(""))

	do := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost,
			"/admin/billing/trials/"+extendStoreID.String()+"/extend",
			bytes.NewBufferString(goodBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "in-flight-key")
		r.ServeHTTP(rec, req)
		return rec
	}

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- do() }()

	<-entered // the first caller has reserved the key and is now doing the work

	second := do()
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())
	require.Contains(t, second.Body.String(), "in_progress")

	close(release)
	first := <-firstDone
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	require.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"the second caller must not execute Extend while the first is still in flight")
}
