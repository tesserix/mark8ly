package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

type fakePurgeTenantRepo struct {
	snap      *tenant.TeardownSnapshot
	snapErr   error
	deleted   []string
	deleteErr error
}

func (f *fakePurgeTenantRepo) ListStoreIDs(_ context.Context, _ *gorm.DB, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakePurgeTenantRepo) DeleteInTx(_ context.Context, _ *gorm.DB, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakePurgeTenantRepo) SnapshotForTeardown(_ context.Context, _ *gorm.DB, _ string) (*tenant.TeardownSnapshot, error) {
	return f.snap, f.snapErr
}

type recordingOutbox struct {
	kinds  []string
	delays []time.Duration
}

func (r *recordingOutbox) enqueue(_ *gorm.DB, kind string, _ any, delay time.Duration) error {
	r.kinds = append(r.kinds, kind)
	r.delays = append(r.delays, delay)
	return nil
}

func snapshotWith(slugs ...string) *tenant.TeardownSnapshot {
	refs := make([]tenant.StoreRef, 0, len(slugs))
	for i, s := range slugs {
		refs = append(refs, tenant.StoreRef{ID: "store-" + string(rune('a'+i)), Slug: s})
	}
	return &tenant.TeardownSnapshot{
		TenantID: "t-1", Name: "The Bondi Store", OwnerUserID: "uid-1", Stores: refs,
	}
}

func newTestService(repo TenantRepo, ob outboxEnqueuer) *Service {
	// db nil: teardownTenantTx runs without a real transaction wrapper.
	return NewService(nil, repo, nil, nil, ob, nil)
}

func TestPurgeTenant_MatchingSlugSetTearsDownAndEnqueues(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store", "bondi-outlet")}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	// Deliberately supplied in a DIFFERENT order from the snapshot: the
	// comparison is a set comparison, not a sequence comparison.
	res, err := svc.PurgeTenant(t.Context(), "t-1", []string{"bondi-outlet", "the-bondi-store"})

	require.NoError(t, err)
	require.Equal(t, []string{"t-1"}, repo.deleted)
	require.Equal(t, []string{TenantDeletedOutboxKind}, ob.kinds)
	require.Equal(t, "The Bondi Store", res.TenantName)
	require.ElementsMatch(t, []string{"store-a", "store-b"}, res.StoreIDs)
	require.ElementsMatch(t, []string{"the-bondi-store", "bondi-outlet"}, res.StoreSlugs)
}

// The property under test discriminates between two tenants, so the
// fixture contains two. A check that always passes and a check that
// compares nothing are indistinguishable with one tenant's slugs.
func TestPurgeTenant_AnotherTenantsSlugsAreRefused(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-facade-factory"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "want *MismatchError, got %T", err)
	require.Equal(t, []string{"the-bondi-store"}, me.Expected)
	require.Empty(t, repo.deleted, "nothing may be deleted on a mismatch")
	require.Empty(t, ob.kinds, "nothing may be enqueued on a mismatch")
}

func TestPurgeTenant_SubsetIsRefused(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store", "bondi-outlet")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "a supplied subset must be a mismatch, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_SupersetIsRefused(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store", "bondi-outlet"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "a supplied superset must be a mismatch, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_EmptySetMatchesAStorelessTenant(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith()}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	res, err := svc.PurgeTenant(t.Context(), "t-1", []string{})

	require.NoError(t, err)
	require.Equal(t, []string{"t-1"}, repo.deleted)
	require.Equal(t, []string{}, res.StoreIDs, "must be an empty slice, never nil")
	require.Equal(t, []string{}, res.StoreSlugs)
	require.Equal(t, []string{TenantDeletedOutboxKind}, ob.kinds)
}

func TestPurgeTenant_EmptySetIsRefusedWhenTheTenantHasStores(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "want *MismatchError, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_UnknownTenantPropagatesNotFound(t *testing.T) {
	repo := &fakePurgeTenantRepo{snapErr: apperrors.NotFound("tenant_not_found", "nope")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"x"})

	ae, ok := apperrors.As(err)
	require.True(t, ok, "want an *apperrors.AppError, got %T", err)
	require.Equal(t, "tenant_not_found", ae.Code)
}

// The operator purge's tenant.deleted event must be enqueued as a BACKSTOP,
// not as a competitor to the inline purge this same request performs.
//
// Undelayed, the drainer wins: measured in production, it completed the whole
// marketplace purge 175ms after the teardown committed and 1.6s before the
// inline purge finished, so the inline purge deleted 0 rows and reported
// `total_rows: 0` for a purge that destroyed 5. #288 requires the audit row to
// record "what was destroyed"; 0 is not that.
//
// The merchant self-serve path is the OPPOSITE case and is asserted alongside
// it deliberately — there is no inline purge there, the event IS the purge,
// and delaying it would stall a real teardown for no reason. A single
// assertion on either path alone would pass against a service that used one
// delay everywhere.
func TestPurgeTenant_EnqueuesTheBackstopDelayed(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store"})
	require.NoError(t, err)

	require.Equal(t, []string{TenantDeletedOutboxKind}, ob.kinds)
	require.Equal(t, []time.Duration{PurgeBackstopDelay}, ob.delays,
		"the operator purge purges inline and reports what it destroyed; its outbox event must not race that")
	require.Greater(t, PurgeBackstopDelay, 5*time.Second,
		"the delay must comfortably exceed a real inline purge (53 DELETEs; ~1.8s observed in prod)")
}
