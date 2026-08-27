//go:build integration

package estateuser

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedTenant inserts a tenant with an owner. Raw SQL rather than the tenant
// package's repository so this test does not couple to that package's
// validation, which is not what is under test here.
func seedTenant(t *testing.T, db *gorm.DB, name, ownerEmail, ownerUserID string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (id, name, owner_user_id, owner_email, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'active', now(), now())`,
		id, name, ownerUserID, ownerEmail).Error)
	return id
}

// token_hash carries a UNIQUE INDEX. Note that pg_constraint does not list
// unique indexes, so a fixture that reuses one fails at INSERT with no hint
// from a constraint audit — each row gets its own.
func seedAcceptedInvite(t *testing.T, db *gorm.DB, tenantID uuid.UUID, email, role string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO invitations (id, tenant_id, email, role, token_hash, expires_at, status, created_at, accepted_at, invited_by_user_id)
		 VALUES (?, ?, ?, ?, ?, now() + interval '7 days', 'accepted', now(), now(), 'uid-inviter')`,
		uuid.New(), tenantID, email, role, uuid.NewString()).Error)
}

// #278: estate identity is DERIVED — tenant owners plus accepted invitations.
// There is no users table, so the directory must union both sources or it
// silently reports only half the staff.
func TestIntegration_EstateUsers_UnionsOwnersAndAcceptedMembers(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tid := seedTenant(t, db, "Zeta Supplies", "owner-278@example.com", "uid-owner-278")
	seedAcceptedInvite(t, db, tid, "staff-278@example.com", "manager")

	res, err := repo.List(context.Background(), Filter{Q: "278@example.com", Limit: 50})
	require.NoError(t, err)

	byEmail := map[string]User{}
	for _, u := range res.Users {
		byEmail[u.Email] = u
	}
	require.Contains(t, byEmail, "owner-278@example.com", "tenant owners are staff")
	require.Contains(t, byEmail, "staff-278@example.com", "accepted invitees are staff")
	require.EqualValues(t, 2, res.Total)

	require.Equal(t, "owner", byEmail["owner-278@example.com"].Roles)
	require.Equal(t, "manager", byEmail["staff-278@example.com"].Roles)
	require.Equal(t, "Zeta Supplies", byEmail["staff-278@example.com"].TenantName)
}

// A pending or revoked invitation is not staff — inviting someone is not the
// same as them being anyone. Conflating the two would put people who never
// accepted into an estate-wide identity lookup.
func TestIntegration_EstateUsers_ExcludesNonAcceptedInvitations(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tid := seedTenant(t, db, "Eta Goods", "owner-278b@example.com", "uid-owner-278b")
	for _, st := range []string{"pending", "revoked", "expired"} {
		require.NoError(t, db.Exec(
			`INSERT INTO invitations (id, tenant_id, email, role, token_hash, expires_at, status, created_at, invited_by_user_id)
			 VALUES (?, ?, ?, 'staff', ?, now() + interval '7 days', ?, now(), 'uid-inviter')`,
			uuid.New(), tid, st+"-278b@example.com", uuid.NewString(), st).Error)
	}

	res, err := repo.List(context.Background(), Filter{Q: "278b@example.com", Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total, "only the owner is staff here")
	require.Equal(t, "owner-278b@example.com", res.Users[0].Email)
}

// One person, one row. Someone who owns one tenant and was invited into
// another is a single identity — the console's global search looks for a
// person, not a membership.
func TestIntegration_EstateUsers_OnePersonOneRowAcrossTenants(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenant(t, db, "Theta One", "multi-278c@example.com", "uid-multi-278c")
	b := seedTenant(t, db, "Iota Two", "other-278c@example.com", "uid-other-278c")
	seedAcceptedInvite(t, db, b, "multi-278c@example.com", "admin")
	_ = a

	res, err := repo.List(context.Background(), Filter{Q: "multi-278c@example.com", Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total, "one person is one row")
	require.EqualValues(t, 2, res.Users[0].TenantCount)
}

// The owner is already listed as owner; an accepted invitation for the same
// email on the SAME tenant must not double-count them. invitation.ListMembers
// de-dupes for exactly this reason.
func TestIntegration_EstateUsers_OwnerWithSelfInviteCountsOnce(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	tid := seedTenant(t, db, "Kappa", "self-278d@example.com", "uid-self-278d")
	seedAcceptedInvite(t, db, tid, "self-278d@example.com", "admin")

	res, err := repo.List(context.Background(), Filter{Q: "self-278d@example.com", Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.EqualValues(t, 1, res.Users[0].TenantCount)
}

// q matches the tenant name too. No person name is recorded anywhere in this
// estate, so the tenant a person belongs to is the only "name" available.
func TestIntegration_EstateUsers_QMatchesTenantName(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	seedTenant(t, db, "Lambda Distinctive Trading", "owner-278e@example.com", "uid-278e")

	res, err := repo.List(context.Background(), Filter{Q: "Lambda Distinctive", Limit: 50})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, "owner-278e@example.com", res.Users[0].Email)
}

// Empty result is an empty slice, never nil — a nil slice marshals to null
// and defeats a caller's `?? []`.
func TestIntegration_EstateUsers_EmptyIsAllocated(t *testing.T) {
	db := testdb.NewTx(t)
	res, err := NewRepository(db).List(context.Background(), Filter{Q: "no-such-person-278z", Limit: 10})
	require.NoError(t, err)
	require.NotNil(t, res.Users)
	require.Empty(t, res.Users)
	require.EqualValues(t, 0, res.Total)
}
