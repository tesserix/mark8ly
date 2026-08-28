package platformadmin_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/customererasure"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// fakeEraser records what the inbox action asked for. The real erasure is
// exercised against a live schema in internal/customererasure; what matters
// here is the ROUTING — that "process" destroys and "reject" does not, and
// that neither happens without an operator.
type fakeEraser struct {
	processed []uuid.UUID
	rejected  []uuid.UUID
	notes     string
	req       customererasure.Request
	procErr   error
	rejErr    error
}

func (f *fakeEraser) Process(_ context.Context, id uuid.UUID) (customererasure.Receipt, error) {
	f.processed = append(f.processed, id)
	if f.procErr != nil {
		return customererasure.Receipt{}, f.procErr
	}
	return customererasure.Receipt{RequestID: id}, nil
}

func (f *fakeEraser) Reject(_ context.Context, id uuid.UUID, notes string) (customererasure.Request, error) {
	f.rejected = append(f.rejected, id)
	f.notes = notes
	if f.rejErr != nil {
		return customererasure.Request{}, f.rejErr
	}
	out := f.req
	out.Status = customererasure.StatusRejected
	return out, nil
}

func (f *fakeEraser) Lookup(_ context.Context, id uuid.UUID) (customererasure.Request, error) {
	out := f.req
	out.ID = id
	return out, nil
}

func newFakeEraser() *fakeEraser {
	return &fakeEraser{req: customererasure.Request{
		TenantID: uuid.New(), StoreID: uuid.New(), Status: customererasure.StatusCompleted,
	}}
}

func erasureItem() inbox.Item {
	return inbox.Item{
		ID:   "22222222-2222-2222-2222-222222222222",
		Kind: inbox.KindErasureRequest,
		Actions: []inbox.Action{
			{ID: "process", Label: "Process erasure", Destructive: true},
			{ID: "reject", Label: "Reject", Destructive: false},
		},
	}
}

func TestErasureExecutor_ProcessErasesAndReportsTheTenant(t *testing.T) {
	f := newFakeEraser()
	res, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), erasureItem(), "process", "op-1", "")

	require.NoError(t, err)
	require.Len(t, f.processed, 1)
	require.Equal(t, uuid.MustParse(erasureItem().ID), f.processed[0])
	require.Equal(t, customererasure.StatusCompleted, res.Status)
	require.Equal(t, f.req.TenantID, res.TenantID,
		"the audit row needs a tenant; EmitOperatorAction refuses uuid.Nil (#310)")
	require.NotNil(t, res.StoreID)
	require.Equal(t, f.req.StoreID, *res.StoreID)
}

func TestErasureExecutor_RejectDestroysNothing(t *testing.T) {
	f := newFakeEraser()
	res, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), erasureItem(), "reject", "op-1", "identity unverified")

	require.NoError(t, err)
	require.Empty(t, f.processed, "a rejection must never reach the erasure path")
	require.Len(t, f.rejected, 1)
	require.Equal(t, "identity unverified", f.notes)
	require.Equal(t, customererasure.StatusRejected, res.Status)
}

// An erasure that cannot be attributed to anyone must not run: the audit row
// would record an anonymous, irreversible destruction of customer data.
func TestErasureExecutor_RefusesAnUnattributedErasure(t *testing.T) {
	f := newFakeEraser()
	_, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), erasureItem(), "process", "  ", "")

	require.ErrorIs(t, err, platformadmin.ErrMissingOperator)
	require.Empty(t, f.processed)
}

// An action added to the provider's declaration but not implemented here must
// fail loudly. Falling through to the destructive branch would be the worst
// possible default for this kind.
func TestErasureExecutor_UnknownActionFailsAndErasesNothing(t *testing.T) {
	f := newFakeEraser()
	_, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), erasureItem(), "archive", "op-1", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "archive")
	require.Empty(t, f.processed)
	require.Empty(t, f.rejected)
}

func TestErasureExecutor_NonUUIDItemIDIsRejected(t *testing.T) {
	f := newFakeEraser()
	item := erasureItem()
	item.ID = "not-a-uuid"
	_, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), item, "process", "op-1", "")

	require.Error(t, err)
	require.Empty(t, f.processed)
}

// Two operators on the same queue row is the common race. 409 "already
// actioned" tells them what happened; 500 does not.
func TestErasureExecutor_AlreadyClaimedBecomesItemNotFound(t *testing.T) {
	for name, injected := range map[string]error{
		"another worker holds it": customererasure.ErrAlreadyClaimed,
		"the row is gone":         customererasure.ErrRequestNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeEraser()
			f.procErr = injected
			_, err := platformadmin.NewErasureExecutor(f).
				Execute(context.Background(), erasureItem(), "process", "op-1", "")
			require.ErrorIs(t, err, inbox.ErrItemNotFound)
		})
	}
}

func TestErasureExecutor_OtherFailuresSurfaceUnchanged(t *testing.T) {
	f := newFakeEraser()
	f.procErr = errors.New("customererasure: anonymise step on orders failed with SQLSTATE 23503")
	_, err := platformadmin.NewErasureExecutor(f).
		Execute(context.Background(), erasureItem(), "process", "op-1", "")

	require.Error(t, err)
	require.NotErrorIs(t, err, inbox.ErrItemNotFound,
		"a real failure must not be dressed up as a race — it needs a 500 and a log line")
}

// The 501 is genuinely gone. Before #259 the erasure kind had no executor and
// the handler answered "action_not_implemented"; this asserts the registered
// executor is what now answers, end to end through the handler.
func TestInboxAction_ErasureKindIsNoLongerNotImplemented(t *testing.T) {
	f := newFakeEraser()
	item := erasureItem()
	r := actionRouter(t, stubItemSource{item: item}, platformadmin.NewErasureExecutor(f), newMemIdempotency())

	rec := post(t, r, "/admin/inbox/"+inbox.KindErasureRequest+"/"+item.ID+"/actions/process", "erase-key-1", `{}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "action_not_implemented")
	require.Len(t, f.processed, 1, "the registered executor must be the thing that ran")
}

// The destructive action still requires an idempotency key. A retried erasure
// must replay, not erase a second time.
func TestInboxAction_ErasureProcessStillRequiresAnIdempotencyKey(t *testing.T) {
	f := newFakeEraser()
	item := erasureItem()
	r := actionRouter(t, stubItemSource{item: item}, platformadmin.NewErasureExecutor(f), newMemIdempotency())

	rec := post(t, r, "/admin/inbox/"+inbox.KindErasureRequest+"/"+item.ID+"/actions/process", "", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "idempotency_key_required")
	require.Empty(t, f.processed, "nothing may be erased before the key is checked")
}
