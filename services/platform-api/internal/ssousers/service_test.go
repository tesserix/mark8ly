package ssousers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/idperr"
	"github.com/mark8ly/platform-api/internal/ssousers"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
)

const testTenant = "11111111-1111-1111-1111-111111111111"

// stubDirectory stands in for *zitadeladmin.StaffProvisioner, which supplies
// both the lookup and the create.
type stubDirectory struct {
	resolveID  string
	resolveErr error

	provisionID  string
	provisionErr error
	calls        []struct{ email, first, last string }
}

func (s *stubDirectory) ResolveUserIDByEmail(context.Context, string) (string, error) {
	return s.resolveID, s.resolveErr
}

func (s *stubDirectory) ProvisionSSOStaff(_ context.Context, email, first, last string) (string, error) {
	s.calls = append(s.calls, struct{ email, first, last string }{email, first, last})
	return s.provisionID, s.provisionErr
}

type stubFGA struct {
	authz.Client
	member    bool
	memberErr error
	writes    []struct {
		userID, tenantID string
		role             authz.Role
	}
	writeErr error
}

func (s *stubFGA) CheckMembership(_ context.Context, _, _ string) (bool, error) {
	return s.member, s.memberErr
}

func (s *stubFGA) WriteRole(_ context.Context, userID string, role authz.Role, tenantID string) error {
	s.writes = append(s.writes, struct {
		userID, tenantID string
		role             authz.Role
	}{userID, tenantID, role})
	return s.writeErr
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestLookup_AMemberIsFound(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveID: "zid-1"}, &stubFGA{member: true})

	res, err := svc.Lookup(context.Background(), testTenant, "someone@example.com")
	require.NoError(t, err)
	require.True(t, res.Found)
	require.Equal(t, "zid-1", res.UserID)
}

// The subtle one. An account that exists but holds no role here must NOT be
// reported as found: the caller would bind the SSO login to an identity with
// no access. Answering false sends it to Provision, which resolves that same
// account and grants the role — one identity, with access.
func TestLookup_AnAccountWithNoRoleHereIsNotFound(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveID: "zid-1"}, &stubFGA{member: false})

	res, err := svc.Lookup(context.Background(), testTenant, "someone@example.com")
	require.NoError(t, err)
	require.False(t, res.Found)
	require.Equal(t, "zid-1", res.UserID, "the id is still reported, for logging")
}

func TestLookup_AnUnknownEmailIsNotFound(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveErr: idperr.ErrUserNotFound}, &stubFGA{})

	res, err := svc.Lookup(context.Background(), testTenant, "nobody@example.com")
	require.NoError(t, err)
	require.False(t, res.Found)
	require.Empty(t, res.UserID)
}

// "Zitadel is unreachable" reported as "not found" would provision a
// duplicate account for every login for the duration of the outage.
func TestLookup_AResolutionFailureIsNotANegativeAnswer(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveErr: errors.New("zitadel down")}, &stubFGA{})

	_, err := svc.Lookup(context.Background(), testTenant, "someone@example.com")
	require.Error(t, err)
}

// Two accounts sharing an address is not a question this service may settle
// by picking one — it decides whose identity an IdP assertion binds to.
func TestLookup_AnAmbiguousEmailIsAnError(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveErr: zitadeladmin.ErrAmbiguousEmail}, &stubFGA{})

	_, err := svc.Lookup(context.Background(), testTenant, "shared@example.com")
	require.ErrorIs(t, err, zitadeladmin.ErrAmbiguousEmail)
}

func TestLookup_AMembershipFailureIsNotANegativeAnswer(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{resolveID: "zid-1"},
		&stubFGA{memberErr: errors.New("openfga down")})

	_, err := svc.Lookup(context.Background(), testTenant, "someone@example.com")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Provision
// ---------------------------------------------------------------------------

func TestProvision_EnsuresTheAccountThenGrantsTheRole(t *testing.T) {
	prov := &stubDirectory{provisionID: "zid-new"}
	fga := &stubFGA{}
	svc := ssousers.NewService(prov, fga)

	id, err := svc.Provision(context.Background(), ssousers.ProvisionInput{
		TenantID: testTenant, Email: "New.Person@example.com",
		FirstName: "New", LastName: "Person", Role: "staff",
	})
	require.NoError(t, err)
	require.Equal(t, "zid-new", id)
	require.Len(t, prov.calls, 1)
	require.Len(t, fga.writes, 1)
	require.Equal(t, authz.RoleStaff, fga.writes[0].role)
	require.Equal(t, "zid-new", fga.writes[0].userID)
	require.Equal(t, testTenant, fga.writes[0].tenantID)
}

// Owner is the one role that can remove the real owner, and the assertion
// driving this call comes from a CUSTOMER-controlled IdP.
func TestProvision_RefusesToGrantOwner(t *testing.T) {
	prov := &stubDirectory{provisionID: "zid-new"}
	fga := &stubFGA{}
	svc := ssousers.NewService(prov, fga)

	_, err := svc.Provision(context.Background(), ssousers.ProvisionInput{
		TenantID: testTenant, Email: "attacker@example.com", Role: "owner",
	})
	require.Error(t, err)
	require.Empty(t, prov.calls, "an account was created for a refused role")
	require.Empty(t, fga.writes)
}

func TestProvision_RefusesAnUnknownRole(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{}, &stubFGA{})

	_, err := svc.Provision(context.Background(), ssousers.ProvisionInput{
		TenantID: testTenant, Email: "someone@example.com", Role: "superuser",
	})
	require.Error(t, err)
}

// Zitadel rejects an empty givenName/familyName outright, so an assertion
// carrying no name would otherwise fail the login rather than provision.
func TestProvision_DerivesNamesFromTheEmailWhenTheIdPSendsNone(t *testing.T) {
	prov := &stubDirectory{provisionID: "zid-new"}
	svc := ssousers.NewService(prov, &stubFGA{})

	_, err := svc.Provision(context.Background(), ssousers.ProvisionInput{
		TenantID: testTenant, Email: "jordan@example.com", Role: "viewer",
	})
	require.NoError(t, err)
	require.Len(t, prov.calls, 1)
	require.Equal(t, "jordan", prov.calls[0].first)
	require.Equal(t, "jordan", prov.calls[0].last)
}

// A role that could not be written means the user has an account and no
// access. Reporting success would hand the caller a user id it would then
// bind an SSO identity to, silently.
func TestProvision_AFailedRoleWriteIsAFailure(t *testing.T) {
	svc := ssousers.NewService(&stubDirectory{provisionID: "zid-new"},
		&stubFGA{writeErr: errors.New("openfga down")})

	_, err := svc.Provision(context.Background(), ssousers.ProvisionInput{
		TenantID: testTenant, Email: "someone@example.com", Role: "staff",
	})
	require.Error(t, err)
}
