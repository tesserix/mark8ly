package platformadmin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

type fakeReviewer struct {
	approved, rejected int
	gotReviewer        string
	gotNotes           string
	review             *migration.Review
	err                error
}

func (f *fakeReviewer) ApproveAsOperator(_ context.Context, _ uuid.UUID, operatorID, notes string) (*migration.Review, error) {
	f.approved++
	f.gotReviewer, f.gotNotes = operatorID, notes
	return f.review, f.err
}

func (f *fakeReviewer) RejectAsOperator(_ context.Context, _ uuid.UUID, operatorID, notes string) (*migration.Review, error) {
	f.rejected++
	f.gotReviewer, f.gotNotes = operatorID, notes
	return f.review, f.err
}

func reviewFixture(status string) *migration.Review {
	return &migration.Review{
		ID: uuid.New(), TenantID: uuid.New(), StoreID: uuid.New(), Status: status,
	}
}

func TestMigrationExecutor_ApproveAttributesToTheReviewsOwnTenant(t *testing.T) {
	rev := reviewFixture("approved")
	f := &fakeReviewer{review: rev}

	// A real production operator id: opaque, not a uuid. Every operator id
	// this estate has recorded looks like this (op_verify_288 and friends),
	// which is why the executor attributes via reviewer_operator_id.
	res, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: rev.ID.String(), Kind: inbox.KindMigrationFastPath},
		"approve", "op_verify_288", "looks legitimate")

	require.NoError(t, err)
	require.Equal(t, 1, f.approved)
	require.Zero(t, f.rejected)
	require.Equal(t, "op_verify_288", f.gotReviewer)
	require.Equal(t, "looks legitimate", f.gotNotes)

	// Attribution comes from the ROW, not from anything the caller supplied —
	// an operator must not be able to point an audit event at another tenant.
	require.Equal(t, rev.TenantID, res.TenantID)
	require.Equal(t, "approved", res.Status)
}

// A second decision, or a race with another operator, surfaces from the
// repository as ErrNotFound because it matches on `status = 'pending'`. The
// handler answers 409 on inbox.ErrItemNotFound, so the executor must translate
// rather than let a 500 escape for an ordinary race.
func TestMigrationExecutor_AlreadyDecidedMapsToItemNotFound(t *testing.T) {
	f := &fakeReviewer{err: migration.ErrNotFound}

	_, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"reject", "op_verify_288", "not enough evidence")

	require.ErrorIs(t, err, inbox.ErrItemNotFound)
}

// An unattributable decision must not be written. Both the audit row and the
// review row would otherwise record an anonymous approval of a merchant's
// migration, which is precisely the accountability this endpoint exists to
// provide.
func TestMigrationExecutor_MissingOperatorIsRefused(t *testing.T) {
	f := &fakeReviewer{review: reviewFixture("approved")}

	_, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"approve", "   ", "")

	require.ErrorIs(t, err, platformadmin.ErrMissingOperator)
	require.Zero(t, f.approved, "no write may happen when the reviewer cannot be identified")
}

// A non-uuid operator is the NORMAL case on this surface, not an error.
func TestMigrationExecutor_OpaqueOperatorIdIsAccepted(t *testing.T) {
	f := &fakeReviewer{review: reviewFixture("rejected")}

	res, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"reject", "ops@example.com", "insufficient evidence")

	require.NoError(t, err)
	require.Equal(t, 1, f.rejected)
	require.Equal(t, "ops@example.com", f.gotReviewer)
	require.Equal(t, "rejected", res.Status)
}

// The handler validates against the item's declared actions, so this is
// unreachable through HTTP — but an action added to the provider's declaration
// and not here must fail loudly rather than fall through to approve.
func TestMigrationExecutor_UnknownActionDoesNotFallThrough(t *testing.T) {
	f := &fakeReviewer{review: reviewFixture("approved")}

	_, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"escalate", "op_verify_288", "")

	require.Error(t, err)
	require.False(t, errors.Is(err, inbox.ErrItemNotFound))
	require.Zero(t, f.approved)
	require.Zero(t, f.rejected)
}
