package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/outbox"
)

// stubOutboxWriter is a fully-controllable double for
// platformadmin.OutboxWriter: each method call is recorded and its result
// is whatever the test configured, defaulting to a generic success so a
// test that only cares about routing/wiring does not have to configure
// every field.
type stubOutboxWriter struct {
	requeueOneResult outbox.RequeueResult
	requeueOneErr    error
	requeueOneCalls  []string

	requeueBatchOutcomes  []outbox.RequeueOutcome
	requeueBatchCalledIDs []string

	deadLetterResult outbox.DeadLetterResult
	deadLetterErr    error
	deadLetterCalls  []struct{ id, reason string }
}

func (s *stubOutboxWriter) RequeueOne(_ context.Context, _ *gorm.DB, id string) (outbox.RequeueResult, error) {
	s.requeueOneCalls = append(s.requeueOneCalls, id)
	if s.requeueOneErr != nil {
		return outbox.RequeueResult{}, s.requeueOneErr
	}
	res := s.requeueOneResult
	if res.ID == "" {
		res = outbox.RequeueResult{ID: id, TenantID: uuid.NewString(), OriginalCreatedAt: time.Now().UTC()}
	}
	return res, nil
}

func (s *stubOutboxWriter) RequeueBatch(_ context.Context, _ *gorm.DB, ids []string) []outbox.RequeueOutcome {
	s.requeueBatchCalledIDs = ids
	if s.requeueBatchOutcomes != nil {
		return s.requeueBatchOutcomes
	}
	out := make([]outbox.RequeueOutcome, 0, len(ids))
	for _, id := range ids {
		out = append(out, outbox.RequeueOutcome{ID: id, OK: true, TenantID: uuid.NewString(), OriginalCreatedAt: time.Now().UTC()})
	}
	return out
}

func (s *stubOutboxWriter) DeadLetterOne(_ context.Context, _ *gorm.DB, id, reason string) (outbox.DeadLetterResult, error) {
	s.deadLetterCalls = append(s.deadLetterCalls, struct{ id, reason string }{id, reason})
	if s.deadLetterErr != nil {
		return outbox.DeadLetterResult{}, s.deadLetterErr
	}
	res := s.deadLetterResult
	if res.ID == "" {
		res = outbox.DeadLetterResult{ID: id, TenantID: uuid.NewString(), DeadLetteredAt: time.Now().UTC()}
	}
	return res, nil
}

func outboxWriteRouter(writer platformadmin.OutboxWriter, aud *capturedAudit) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewOutboxHandler(nil, &stubOutboxLister{}, writer, aud.fn, nil).Register(r.Group(""))
	return r
}

func doOutboxWrite(method, path, body string, writer platformadmin.OutboxWriter, aud *capturedAudit) *httptest.ResponseRecorder {
	r := outboxWriteRouter(writer, aud)
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	return rec
}

const testOutboxID = "11111111-1111-1111-1111-111111111111"

// TestOutboxRequeueRefusesPublishedRow_DoublePublishGuard is the
// handler-level counterpart to the double-publish guard: the handler must
// map outbox.ErrAlreadyPublished to a refusal (409), not a silent success,
// and must NOT emit an audit event for a refused requeue.
func TestOutboxRequeueRefusesPublishedRow_DoublePublishGuard(t *testing.T) {
	writer := &stubOutboxWriter{requeueOneErr: outbox.ErrAlreadyPublished}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/requeue", "", writer, aud)

	require.Equal(t, http.StatusConflict, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "already_published", body["error"])
	require.Empty(t, aud.events, "a refused requeue must not be audited")
}

func TestOutboxRequeueSingle_NotFoundIsFourOhFour(t *testing.T) {
	writer := &stubOutboxWriter{requeueOneErr: outbox.ErrNotFound}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/requeue", "", writer, aud)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, aud.events)
}

func TestOutboxRequeueSingle_InvalidIDIsFourHundred(t *testing.T) {
	writer := &stubOutboxWriter{}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/not-a-uuid/requeue", "", writer, aud)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, writer.requeueOneCalls, "an invalid id must never reach the writer")
}

// The audit event MUST carry the row's ORIGINAL created_at — the only
// place it survives, since requeue overwrites the column itself.
func TestOutboxRequeueSingle_AuditCarriesOriginalCreatedAt(t *testing.T) {
	original := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tenantID := uuid.NewString()
	writer := &stubOutboxWriter{requeueOneResult: outbox.RequeueResult{
		ID: testOutboxID, TenantID: tenantID, OriginalCreatedAt: original,
	}}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/requeue", "", writer, aud)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, aud.events, 1)
	ev := aud.events[0]
	require.Equal(t, "outbox.requeued", ev.Action)
	require.Equal(t, testOutboxID, ev.ResourceID)
	require.Equal(t, original.Format(time.RFC3339), ev.Metadata["original_created_at"])

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, original.Format(time.RFC3339), body["original_created_at"])
}

// Batch requeue returns a per-row outcome; one bad id must not fail the
// rest of the set, and only the OK rows are audited.
func TestOutboxRequeueBatch_OneInvalidIDDoesNotFailTheRest(t *testing.T) {
	goodID := "22222222-2222-2222-2222-222222222222"
	badID := "33333333-3333-3333-3333-333333333333"
	writer := &stubOutboxWriter{requeueBatchOutcomes: []outbox.RequeueOutcome{
		{ID: goodID, OK: true, TenantID: uuid.NewString(), OriginalCreatedAt: time.Now().UTC()},
		{ID: badID, OK: false, Err: "already_published"},
	}}
	aud := &capturedAudit{}
	body := `{"ids":["` + goodID + `","` + badID + `"]}`
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/requeue", body, writer, aud)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Results []struct {
			ID    string `json:"id"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Results, 2)
	require.True(t, got.Results[0].OK)
	require.False(t, got.Results[1].OK)
	require.Equal(t, "already_published", got.Results[1].Error)
	require.Len(t, aud.events, 1, "only the successfully-requeued row is audited")
}

func TestOutboxRequeueBatch_EmptyIDsIsFourHundred(t *testing.T) {
	writer := &stubOutboxWriter{}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/requeue", `{"ids":[]}`, writer, aud)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// Dead-letter rejects an empty reason before the writer is ever called.
func TestOutboxDeadLetter_RejectsEmptyReason(t *testing.T) {
	writer := &stubOutboxWriter{}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/dead-letter", `{"reason":"  "}`, writer, aud)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "reason_required", body["error"])
	require.Empty(t, writer.deadLetterCalls, "an empty reason must never reach the writer")
	require.Empty(t, aud.events)
}

// The dead-letter guard mirrors requeue's: a published row cannot be
// dead-lettered.
func TestOutboxDeadLetter_RefusesPublishedRow(t *testing.T) {
	writer := &stubOutboxWriter{deadLetterErr: outbox.ErrAlreadyPublished}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/dead-letter", `{"reason":"operator says so"}`, writer, aud)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Empty(t, aud.events)
}

func TestOutboxDeadLetter_SuccessEmitsAuditWithReason(t *testing.T) {
	tenantID := uuid.NewString()
	deadAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	writer := &stubOutboxWriter{deadLetterResult: outbox.DeadLetterResult{
		ID: testOutboxID, TenantID: tenantID, DeadLetteredAt: deadAt,
	}}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/dead-letter", `{"reason":"confirmed duplicate"}`, writer, aud)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, aud.events, 1)
	ev := aud.events[0]
	require.Equal(t, "outbox.dead_lettered", ev.Action)
	require.Equal(t, testOutboxID, ev.ResourceID)
	require.Equal(t, "confirmed duplicate", ev.Metadata["reason"])

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, deadAt.Format(time.RFC3339), body["dead_lettered_at"])
}

// payload is never accepted or returned by any write endpoint — same
// governance-surface reasoning as List's outboxRow.
func TestOutboxWriteEndpoints_NeverEchoPayload(t *testing.T) {
	writer := &stubOutboxWriter{}
	aud := &capturedAudit{}
	rec := doOutboxWrite(http.MethodPost, "/admin/outbox/"+testOutboxID+"/dead-letter", `{"reason":"x","payload":{"secret":"leak"}}`, writer, aud)
	require.NotContains(t, rec.Body.String(), "leak")
	require.NotContains(t, rec.Body.String(), "payload")
}

// Write routes must not mount at all when writer is nil — matching every
// other nil-safe optional dependency on this surface.
func TestOutboxWriteRoutesUnmountedWhenWriterNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewOutboxHandler(nil, &stubOutboxLister{}, nil, nil, nil).Register(r.Group(""))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/outbox/"+testOutboxID+"/requeue", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
