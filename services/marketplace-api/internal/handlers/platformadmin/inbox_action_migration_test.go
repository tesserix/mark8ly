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
	gotReviewer        uuid.UUID
	gotNotes           string
	review             *migration.Review
	err                error
}

func (f *fakeReviewer) Approve(_ context.Context, _ uuid.UUID, reviewerID uuid.UUID, notes string) (*migration.Review, error) {
	f.approved++
	f.gotReviewer, f.gotNotes = reviewerID, notes
	return f.review, f.err
}

func (f *fakeReviewer) Reject(_ context.Context, _ uuid.UUID, reviewerID uuid.UUID, notes string) (*migration.Review, error) {
	f.rejected++
	f.gotReviewer, f.gotNotes = reviewerID, notes
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
	op := uuid.New()

	res, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: rev.ID.String(), Kind: inbox.KindMigrationFastPath},
		"approve", op.String(), "looks legitimate")

	require.NoError(t, err)
	require.Equal(t, 1, f.approved)
	require.Zero(t, f.rejected)
	require.Equal(t, op, f.gotReviewer)
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
		"reject", uuid.NewString(), "not enough evidence")

	require.ErrorIs(t, err, inbox.ErrItemNotFound)
}

// reviewer_id is a uuid column with NO foreign key, so a synthesised id would
// be accepted by the database and read as a real user forever after. Refusing
// is the honest answer; the audit row still carries the readable operator.
func TestMigrationExecutor_NonUUIDOperatorIsRefusedNotSynthesised(t *testing.T) {
	f := &fakeReviewer{review: reviewFixture("approved")}

	_, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"approve", "ops@example.com", "")

	require.ErrorIs(t, err, platformadmin.ErrOperatorNotAddressable)
	require.Zero(t, f.approved, "no write may happen when the reviewer cannot be identified")
}

// The handler validates against the item's declared actions, so this is
// unreachable through HTTP — but an action added to the provider's declaration
// and not here must fail loudly rather than fall through to approve.
func TestMigrationExecutor_UnknownActionDoesNotFallThrough(t *testing.T) {
	f := &fakeReviewer{review: reviewFixture("approved")}

	_, err := platformadmin.NewMigrationFastPathExecutor(f).Execute(
		context.Background(),
		inbox.Item{ID: uuid.NewString(), Kind: inbox.KindMigrationFastPath},
		"escalate", uuid.NewString(), "")

	require.Error(t, err)
	require.False(t, errors.Is(err, inbox.ErrItemNotFound))
	require.Zero(t, f.approved)
	require.Zero(t, f.rejected)
}
