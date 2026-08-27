package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// --- doubles -------------------------------------------------------------

type stubItemSource struct {
	item inbox.Item
	err  error
}

func (s stubItemSource) Get(context.Context, string, string) (inbox.Item, error) {
	return s.item, s.err
}

type recordingExecutor struct {
	calls  int
	gotAct string
	res    platformadmin.InboxActionResult
	err    error
}

func (r *recordingExecutor) Kind() string { return inbox.KindMigrationFastPath }
func (r *recordingExecutor) Execute(_ context.Context, _ inbox.Item, actionID, _, _ string) (platformadmin.InboxActionResult, error) {
	r.calls++
	r.gotAct = actionID
	return r.res, r.err
}

type memIdempotency struct {
	claimed map[string]platformadmin.InboxActionRecord
}

func newMemIdempotency() *memIdempotency {
	return &memIdempotency{claimed: map[string]platformadmin.InboxActionRecord{}}
}

func (m *memIdempotency) Claim(_ context.Context, rec platformadmin.InboxActionRecord) (bool, *platformadmin.InboxActionRecord, error) {
	if existing, ok := m.claimed[rec.Key]; ok {
		return false, &existing, nil
	}
	if rec.Outcome == nil {
		rec.Outcome = json.RawMessage(`{}`)
	}
	m.claimed[rec.Key] = rec
	return true, nil, nil
}

func (m *memIdempotency) Complete(_ context.Context, key string, outcome json.RawMessage) error {
	rec := m.claimed[key]
	rec.Outcome = outcome
	m.claimed[key] = rec
	return nil
}

func fastPathItem() inbox.Item {
	return inbox.Item{
		ID:   "11111111-1111-1111-1111-111111111111",
		Kind: inbox.KindMigrationFastPath,
		Actions: []inbox.Action{
			{ID: "approve", Label: "Approve", Destructive: false},
			{ID: "reject", Label: "Reject", Destructive: true},
		},
	}
}

func actionRouter(t *testing.T, src platformadmin.InboxItemSource, ex platformadmin.InboxActionExecutor, idem platformadmin.InboxActionIdempotency) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Stand in for RequirePlatformAuth, which is exercised elsewhere.
	r.Use(func(c *gin.Context) { c.Set(platformadmin.CtxOperatorID, "op-1") })
	var execs []platformadmin.InboxActionExecutor
	if ex != nil {
		execs = append(execs, ex)
	}
	platformadmin.NewInboxActionsHandler(src, execs, idem, nil, nil).Register(r.Group(""))
	return r
}

func post(t *testing.T, r *gin.Engine, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// --- the contract --------------------------------------------------------

// "The action id must be one that item declared. Reject an action absent from
// that item's own actions array, even if mark8ly implements it elsewhere."
//
// `approve` IS implemented for this kind — the executor below would happily
// run it — so a handler that validated against the executor registry rather
// than the item would let this through. The item is the contract.
func TestInboxAction_RejectsActionNotDeclaredByTheItem(t *testing.T) {
	item := fastPathItem()
	item.Actions = []inbox.Action{{ID: "reject", Label: "Reject", Destructive: true}}
	ex := &recordingExecutor{}

	r := actionRouter(t, stubItemSource{item: item}, ex, newMemIdempotency())
	rec := post(t, r, "/admin/inbox/"+inbox.KindMigrationFastPath+"/"+item.ID+"/actions/approve", "k1", `{}`)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "undeclared_action")
	require.Zero(t, ex.calls, "an undeclared action must not reach the executor")
}

// "Destructive actions require an idempotency key."
func TestInboxAction_DestructiveRequiresIdempotencyKey(t *testing.T) {
	ex := &recordingExecutor{}
	r := actionRouter(t, stubItemSource{item: fastPathItem()}, ex, newMemIdempotency())

	rec := post(t, r, "/admin/inbox/"+inbox.KindMigrationFastPath+"/"+fastPathItem().ID+"/actions/reject", "", `{}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "idempotency_key_required")
	require.Zero(t, ex.calls)
}

// A non-destructive action does NOT require one — the requirement exists to
// stop a retried destructive write firing twice, not to add ceremony.
func TestInboxAction_NonDestructiveNeedsNoKey(t *testing.T) {
	ex := &recordingExecutor{res: platformadmin.InboxActionResult{TenantID: uuid.New(), Status: "approved"}}
	r := actionRouter(t, stubItemSource{item: fastPathItem()}, ex, newMemIdempotency())

	rec := post(t, r, "/admin/inbox/"+inbox.KindMigrationFastPath+"/"+fastPathItem().ID+"/actions/approve", "", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, ex.calls)
	require.Equal(t, "approve", ex.gotAct)
}

// "A duplicate submit carrying the same idempotency key is a no-op."
//
// Not merely "does not error" — the executor must not run a second time, and
// the caller must get the SAME answer, because the alternative is showing an
// operator a failure for work that succeeded.
func TestInboxAction_DuplicateKeyIsANoOp(t *testing.T) {
	ex := &recordingExecutor{res: platformadmin.InboxActionResult{TenantID: uuid.New(), Status: "rejected"}}
	idem := newMemIdempotency()
	r := actionRouter(t, stubItemSource{item: fastPathItem()}, ex, idem)

	path := "/admin/inbox/" + inbox.KindMigrationFastPath + "/" + fastPathItem().ID + "/actions/reject"
	first := post(t, r, path, "same-key", `{}`)
	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, 1, ex.calls)

	second := post(t, r, path, "same-key", `{}`)
	require.Equal(t, http.StatusOK, second.Code)
	require.Equal(t, 1, ex.calls, "a replayed key must not execute the action again")

	// The OUTCOME must be identical — a replay that reported a different
	// status would leave the console showing something that never happened.
	// `replayed` is deliberately not part of that: it is how the console
	// distinguishes "already done" from "just done", so it is asserted to
	// DIFFER rather than to match.
	var firstBody, secondBody struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondBody))

	for _, field := range []string{"kind", "item_id", "action_id", "status"} {
		require.Equalf(t, firstBody.Data[field], secondBody.Data[field],
			"a replay must report the same %s as the original execution", field)
	}
	require.Equal(t, "rejected", secondBody.Data["status"],
		"the replay must carry the original outcome, not an empty one")
	require.Equal(t, false, firstBody.Data["replayed"])
	require.Equal(t, true, secondBody.Data["replayed"],
		"a replay must be distinguishable, or the console implies the action just ran")
}

// A kind with no executor is answerable and answerably unsupported. It must
// not 404 (the queue exists) and must not 500 (nothing failed).
func TestInboxAction_KindWithoutExecutorIsNotImplemented(t *testing.T) {
	item := fastPathItem()
	item.Kind = inbox.KindSEAManualReview
	r := actionRouter(t, stubItemSource{item: item}, nil, newMemIdempotency())

	rec := post(t, r, "/admin/inbox/"+inbox.KindSEAManualReview+"/"+item.ID+"/actions/approve", "k9", `{}`)
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "action_not_implemented")
}

func TestInboxAction_UnknownItemIs404(t *testing.T) {
	r := actionRouter(t, stubItemSource{err: inbox.ErrItemNotFound}, &recordingExecutor{}, newMemIdempotency())
	rec := post(t, r, "/admin/inbox/"+inbox.KindMigrationFastPath+"/does-not-exist/actions/approve", "", `{}`)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInboxAction_KindWithoutSingleItemReadIsNotImplemented(t *testing.T) {
	r := actionRouter(t, stubItemSource{err: inbox.ErrGetNotSupported}, &recordingExecutor{}, newMemIdempotency())
	rec := post(t, r, "/admin/inbox/"+inbox.KindErasureRequest+"/x/actions/process", "k2", `{}`)
	require.Equal(t, http.StatusNotImplemented, rec.Code)
}
