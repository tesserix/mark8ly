package account

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/tenant"
)

// failingListMembersFGA wraps a real authz.Client (normally a
// *authz.FakeClient) and forces ListTenantMembers to fail, leaving every
// other method to the embedded client. Used to prove that an enumeration
// failure is logged and cleanup continues rather than propagating an
// error or aborting the teardown — authz.FakeClient's blanket
// FailNextChecks counter can't target ListTenantMembers alone once
// DeleteAccount's own GetRole call is in the mix.
type failingListMembersFGA struct {
	authz.Client
	err error
}

func (f *failingListMembersFGA) ListTenantMembers(_ context.Context, _ string) ([]authz.Member, error) {
	return nil, f.err
}

// TestDeleteAccount_Owner_TearsDownAllMembersNotJustOwner is the #361
// regression test for the merchant self-serve path: before the fix, only
// the owner's role tuple was removed on teardown, leaving staff/admin/
// viewer tuples pointing at a tenant that no longer exists.
func TestDeleteAccount_Owner_TearsDownAllMembersNotJustOwner(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteOwnership(ctx, "owner-1", "t1"))
	require.NoError(t, fga.WriteRole(ctx, "admin-1", authz.RoleAdmin, "t1"))
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"))
	require.NoError(t, fga.WriteRole(ctx, "viewer-1", authz.RoleViewer, "t1"))
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "owner-1"))

	require.False(t, fga.HasRole("owner-1", authz.RoleOwner, "t1"), "owner tuple should be deleted")
	require.False(t, fga.HasRole("admin-1", authz.RoleAdmin, "t1"), "admin tuple should be deleted")
	require.False(t, fga.HasRole("staff-1", authz.RoleStaff, "t1"), "staff tuple should be deleted")
	require.False(t, fga.HasRole("viewer-1", authz.RoleViewer, "t1"), "viewer tuple should be deleted")

	// None of them belong to any other tenant, so all four identities
	// should be gone.
	require.True(t, gip.deleted["owner-1"])
	require.True(t, gip.deleted["admin-1"])
	require.True(t, gip.deleted["staff-1"])
	require.True(t, gip.deleted["viewer-1"])
}

// TestDeleteAccount_Owner_MemberOnAnotherTenantKeepsIdentity pins the
// CRITICAL safety rule from #361: a user with a role on more than one
// tenant must keep their identity when only one of those tenants is torn
// down. Deleting it unconditionally (the issue's own "suggested fix
// direction") would sever their access to every other tenant they belong
// to. Applies the rule to the owner too, since an owner can (in
// principle) own more than one tenant.
func TestDeleteAccount_Owner_MemberOnAnotherTenantKeepsIdentity(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	// owner-1 owns t1 only.
	require.NoError(t, fga.WriteOwnership(ctx, "owner-1", "t1"))
	// staff-1 is staff on t1 AND owns a second, unrelated tenant t2.
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"))
	require.NoError(t, fga.WriteOwnership(ctx, "staff-1", "t2"))
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "owner-1"))

	// t1's tuples are gone for both users.
	require.False(t, fga.HasRole("owner-1", authz.RoleOwner, "t1"))
	require.False(t, fga.HasRole("staff-1", authz.RoleStaff, "t1"))
	// staff-1's membership on t2 is untouched.
	require.True(t, fga.HasOwnership("staff-1", "t2"), "unrelated tenant membership must survive")

	// owner-1 has no remaining tenant → identity deleted.
	require.True(t, gip.deleted["owner-1"])
	// staff-1 still belongs to t2 → identity must be KEPT.
	require.False(t, gip.deleted["staff-1"], "identity must survive while the user still belongs to another tenant")
}

// TestDeleteAccount_Owner_ListTenantMembersFailure_LogsAndContinues
// verifies an enumeration failure doesn't fail DeleteAccount or block the
// store-parent cleanup that follows it — everything here is best-effort
// post-commit cleanup, per the package doc.
func TestDeleteAccount_Owner_ListTenantMembersFailure_LogsAndContinues(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteOwnership(ctx, "owner-1", "t1"))
	require.NoError(t, fga.WriteStoreParent(ctx, "s1", "t1"))
	failing := &failingListMembersFGA{Client: fga, err: errors.New("fga read unavailable")}
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, failing, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "owner-1"), "an enumeration failure must not fail the call")

	// The tenant row is still gone (that part already committed).
	require.True(t, repo.deleted["t1"])
	// Store-parent cleanup, which doesn't depend on member enumeration,
	// still ran.
	require.False(t, fga.HasStoreParent("s1", "t1"))
	// No identity was deleted: enumeration never returned any members.
	require.False(t, gip.deleted["owner-1"])
}

// TestDeleteAccount_Owner_IdentityDeleteFailure_LogsAndContinues verifies
// a GIP failure for one member doesn't stop the others from being
// processed, and doesn't fail the call.
func TestDeleteAccount_Owner_IdentityDeleteFailure_LogsAndContinues(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteOwnership(ctx, "owner-1", "t1"))
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"))
	gip := &fakeGIP{err: errors.New("gip unavailable")}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "owner-1"), "identity delete failures are best-effort")

	// FGA tuples are still cleaned up regardless of the GIP failure.
	require.False(t, fga.HasRole("owner-1", authz.RoleOwner, "t1"))
	require.False(t, fga.HasRole("staff-1", authz.RoleStaff, "t1"))
}

// TestPurgeTenant_CleansUpAllMembersNotJustOwner is the #361 regression
// test for the operator-initiated purge path: cleanupAfterTeardown used
// to only remove snap.OwnerUserID's tuple.
func TestPurgeTenant_CleansUpAllMembersNotJustOwner(t *testing.T) {
	ctx := t.Context()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteOwnership(ctx, "uid-1", "t-1"))
	require.NoError(t, fga.WriteRole(ctx, "admin-1", authz.RoleAdmin, "t-1"))
	require.NoError(t, fga.WriteStoreParent(ctx, "store-a", "t-1"))
	gip := &fakeGIP{}
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	ob := &recordingOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.enqueue, testLogger())

	_, err := svc.PurgeTenant(ctx, "t-1", []string{"the-bondi-store"})
	require.NoError(t, err)

	require.False(t, fga.HasOwnership("uid-1", "t-1"))
	require.False(t, fga.HasRole("admin-1", authz.RoleAdmin, "t-1"))
	require.False(t, fga.HasStoreParent("store-a", "t-1"))
	require.True(t, gip.deleted["uid-1"])
	require.True(t, gip.deleted["admin-1"])
}

// TestPurgeTenant_MemberOnAnotherTenantKeepsIdentity applies the same
// CRITICAL multi-tenant-membership safety rule to the operator purge
// path: snap.OwnerUserID's identity used to be deleted unconditionally,
// which would be a latent instance of the #361 bug for any owner who
// owns a second tenant.
func TestPurgeTenant_MemberOnAnotherTenantKeepsIdentity(t *testing.T) {
	ctx := t.Context()
	fga := authz.NewFake()
	// uid-1 owns t-1 AND a second tenant t-2.
	require.NoError(t, fga.WriteOwnership(ctx, "uid-1", "t-1"))
	require.NoError(t, fga.WriteOwnership(ctx, "uid-1", "t-2"))
	gip := &fakeGIP{}
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	ob := &recordingOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.enqueue, testLogger())

	_, err := svc.PurgeTenant(ctx, "t-1", []string{"the-bondi-store"})
	require.NoError(t, err)

	require.False(t, fga.HasOwnership("uid-1", "t-1"), "t-1's tuple must be gone")
	require.True(t, fga.HasOwnership("uid-1", "t-2"), "t-2's tuple is unrelated and must survive")
	require.False(t, gip.deleted["uid-1"], "identity must survive while the user still owns t-2")
}

// TestPurgeTenant_NilFGAAndGIPSkipsCleanupWithoutPanicking pins the
// documented nil-tolerance: PurgeTenant's route is mounted unconditionally
// and fga/gip may be true nil interfaces (see purge.go's doc comment).
func TestPurgeTenant_NilFGAAndGIPSkipsCleanupWithoutPanicking(t *testing.T) {
	repo := &fakePurgeTenantRepo{snap: snapshotWith("the-bondi-store")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	res, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store"})

	require.NoError(t, err)
	require.Equal(t, "t-1", res.TenantID)
}

// TestPurgeTenant_ListTenantMembersFailure_LogsAndContinues mirrors the
// owner-path enumeration-failure test for the operator purge: an FGA read
// failure must not fail PurgeTenant, which has already committed the DB
// teardown by the time cleanup runs.
func TestPurgeTenant_ListTenantMembersFailure_LogsAndContinues(t *testing.T) {
	ctx := t.Context()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteOwnership(ctx, "uid-1", "t-1"))
	require.NoError(t, fga.WriteStoreParent(ctx, "store-a", "t-1"))
	failing := &failingListMembersFGA{Client: fga, err: errors.New("fga read unavailable")}
	gip := &fakeGIP{}
	repo := &fakePurgeTenantRepo{snap: &tenant.TeardownSnapshot{
		TenantID: "t-1", Name: "The Bondi Store", OwnerUserID: "uid-1",
		Stores: []tenant.StoreRef{{ID: "store-a", Slug: "the-bondi-store"}},
	}}
	ob := &recordingOutbox{}

	svc := NewService(nil, repo, failing, gip, ob.enqueue, testLogger())

	_, err := svc.PurgeTenant(ctx, "t-1", []string{"the-bondi-store"})
	require.NoError(t, err, "an enumeration failure must not fail the call")

	// Store-parent cleanup, independent of member enumeration, still ran.
	require.False(t, fga.HasStoreParent("store-a", "t-1"))
	require.False(t, gip.deleted["uid-1"])
}

// A staff member leaving ONE of their tenants must keep their identity —
// this is the likeliest instance of #361's bug to occur in practice, since
// self-serve "delete my account" is a user-facing action and multi-tenant
// staff are ordinary. Deleting the identity here would silently revoke their
// access to every other tenant they belong to.
func TestDeleteAccount_Staff_MemberOnAnotherTenantKeepsIdentity(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"))
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t2"))
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "staff-1"))

	require.False(t, fga.HasRole("staff-1", authz.RoleStaff, "t1"), "t1 tuple should be deleted")
	require.True(t, fga.HasRole("staff-1", authz.RoleStaff, "t2"), "t2 tuple must survive")
	require.False(t, gip.deleted["staff-1"], "identity must survive: still a member of t2")
}

// The counterpart: their last membership, so the identity does go.
func TestDeleteAccount_Staff_LastMembershipDeletesIdentity(t *testing.T) {
	ctx := context.Background()
	fga := authz.NewFake()
	require.NoError(t, fga.WriteRole(ctx, "staff-1", authz.RoleStaff, "t1"))
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}

	svc := NewService(nil, repo, fga, gip, ob.Enqueue, testLogger())

	require.NoError(t, svc.DeleteAccount(ctx, "t1", "staff-1"))

	require.True(t, gip.deleted["staff-1"], "identity should be deleted: no memberships remain")
}
