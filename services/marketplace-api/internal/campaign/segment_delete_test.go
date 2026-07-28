package campaign

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// fakeSegmentRepo implements only the handful of Repository methods
// DeleteSegment touches. The embedded nil Repository makes any other call
// panic loudly rather than silently returning a zero value.
type fakeSegmentRepo struct {
	Repository

	seg           *CustomerSegment
	getErr        error
	campaignCount int64
	countErr      error

	deleteCalled bool
	deleteErr    error
}

func (f *fakeSegmentRepo) GetSegmentByID(_ context.Context, _ *gorm.DB, _ uuid.UUID) (*CustomerSegment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.seg, nil
}

func (f *fakeSegmentRepo) CountCampaignsBySegment(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (int64, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.campaignCount, nil
}

func (f *fakeSegmentRepo) DeleteSegment(_ *gorm.DB, _ uuid.UUID) error {
	f.deleteCalled = true
	return f.deleteErr
}

func newSegmentFixture(count int64) (*Service, *fakeSegmentRepo, uuid.UUID) {
	segID := uuid.New()
	repo := &fakeSegmentRepo{
		seg: &CustomerSegment{
			ID:       segID,
			TenantID: uuid.New(),
			StoreID:  uuid.New(),
			Name:     "VIPs",
		},
		campaignCount: count,
	}
	return NewService(ServiceConfig{Repo: repo}), repo, segID
}

// TestDeleteSegment_ReferencedByCampaigns is the guard for the 500-on-delete
// bug: a segment a campaign still points at must be refused with the typed
// segment_in_use error (409) and must NOT reach the DELETE.
func TestDeleteSegment_ReferencedByCampaigns(t *testing.T) {
	svc, repo, segID := newSegmentFixture(3)

	err := svc.DeleteSegment(context.Background(), segID)
	if err == nil {
		t.Fatal("expected an error deleting a referenced segment, got nil")
	}
	if !errors.Is(err, apperrors.ErrSegmentInUse) {
		t.Fatalf("expected ErrSegmentInUse, got %v", err)
	}
	if repo.deleteCalled {
		t.Fatal("DeleteSegment reached the repository despite live references")
	}

	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if got := ae.Details["campaign_count"]; got != int64(3) {
		t.Fatalf("campaign_count detail = %v (%T), want int64(3)", got, got)
	}
}

// TestDeleteSegment_Unreferenced proves the refusal is targeted: a segment no
// campaign points at still deletes.
func TestDeleteSegment_Unreferenced(t *testing.T) {
	svc, repo, segID := newSegmentFixture(0)

	if err := svc.DeleteSegment(context.Background(), segID); err != nil {
		t.Fatalf("delete unreferenced segment: %v", err)
	}
	if !repo.deleteCalled {
		t.Fatal("expected the repository DELETE to run")
	}
}

// TestDeleteSegment_NotFound keeps the 404 path intact — the pre-check must
// not turn a missing segment into a conflict.
func TestDeleteSegment_NotFound(t *testing.T) {
	svc, repo, segID := newSegmentFixture(0)
	repo.getErr = apperrors.New(apperrors.CodeSegmentNotFound, "segment not found")

	err := svc.DeleteSegment(context.Background(), segID)
	if !errors.Is(err, apperrors.ErrSegmentNotFound) {
		t.Fatalf("expected ErrSegmentNotFound, got %v", err)
	}
	if repo.deleteCalled {
		t.Fatal("DeleteSegment reached the repository for a missing segment")
	}
}

// TestIsSegmentFKViolation covers the TOCTOU path: when a campaign is created
// between the pre-check and the DELETE, Postgres raises 23503 and the
// repository must map it to segment_in_use rather than let it escape as a 500.
func TestIsSegmentFKViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "gorm-wrapped text form",
			err: errors.New(`ERROR: update or delete on table "customer_segments" violates ` +
				`foreign key constraint "campaigns_segment_id_fkey" on table "campaigns" (SQLSTATE 23503)`),
			want: true,
		},
		{
			name: "unrelated foreign key",
			err: errors.New(`violates foreign key constraint "campaign_recipients_campaign_id_fkey" ` +
				`on table "campaign_recipients" (SQLSTATE 23503)`),
			want: false,
		},
		{
			name: "unique violation",
			err:  errors.New(`duplicate key value violates unique constraint (SQLSTATE 23505)`),
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSegmentFKViolation(tc.err); got != tc.want {
				t.Fatalf("isSegmentFKViolation = %v, want %v", got, tc.want)
			}
		})
	}
}
