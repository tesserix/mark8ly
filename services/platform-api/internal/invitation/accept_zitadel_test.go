package invitation

import (
	"context"
	"errors"
	"testing"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeProvisioner stands in for *zitadeladmin.StaffProvisioner. It is a
// TRIPWIRE: it fails the test from inside ProvisionStaff when called
// with anything other than the invitation's own email, because "we
// provisioned the wrong address" is precisely the class of bug that
// produced the production incident behind this code.
type fakeProvisioner struct {
	t          *testing.T
	wantEmail  string
	returnUID  string
	err        error
	calls      int
	gotEmail   string
	gotFirst   string
	gotLast    string
	gotPasswrd string
}

func (f *fakeProvisioner) ProvisionStaff(_ context.Context, email, firstName, lastName, password string) (string, error) {
	f.calls++
	f.gotEmail, f.gotFirst, f.gotLast, f.gotPasswrd = email, firstName, lastName, password
	if f.wantEmail != "" && email != f.wantEmail {
		f.t.Errorf("ProvisionStaff called with email %q, want the INVITATION's email %q — "+
			"provisioning must never trust a caller-supplied address", email, f.wantEmail)
	}
	if f.err != nil {
		return "", f.err
	}
	return f.returnUID, nil
}

func zitadelAcceptInput() AcceptInput {
	return AcceptInput{
		Token:         "tok",
		VerifiedEmail: "staff@example.com",
		Password:      "correct-horse-battery-staple",
		FirstName:     "Sam",
		LastName:      "Staff",
	}
}

// TestAccept_Zitadel_ProvisionsAndWritesBothIdentityKeys is the core
// regression test for #679. The admin login path resolves membership by
// EMAIL (apps/admin/app/login/actions.ts resolveWorkspaceTenant, which
// runs BEFORE authentication) while the bearer-token API path resolves
// by the Zitadel uid. Writing only one of them breaks the other reader.
func TestAccept_Zitadel_ProvisionsAndWritesBothIdentityKeys(t *testing.T) {
	repo := &fakeRepo{inv: pendingInvitation()}
	fga := authz.NewFake()
	prov := &fakeProvisioner{t: t, wantEmail: "staff@example.com", returnUID: "zid-1"}
	claims := &fakeClaimSetter{}
	svc := NewService(Config{Repo: repo, FGA: fga, Provisioner: prov, Claims: claims})

	res, err := svc.Accept(context.Background(), zitadelAcceptInput())
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.TenantID != "tid-1" || res.Role != "staff" {
		t.Errorf("result = %+v, want tenant tid-1 role staff", res)
	}
	if prov.calls != 1 {
		t.Fatalf("ProvisionStaff called %d times, want 1", prov.calls)
	}
	if prov.gotFirst != "Sam" || prov.gotLast != "Staff" || prov.gotPasswrd != "correct-horse-battery-staple" {
		t.Errorf("provisioned with (%q, %q, %q), want the submitted profile and password",
			prov.gotFirst, prov.gotLast, prov.gotPasswrd)
	}
	if !fga.HasRole("staff@example.com", authz.RoleStaff, "tid-1") {
		t.Error("no EMAIL-keyed tuple: the admin login path resolves membership by email and " +
			"would show \"we couldn't find a store for this account\"")
	}
	if !fga.HasRole("zid-1", authz.RoleStaff, "tid-1") {
		t.Error("no ZITADEL-UID-keyed tuple: the bearer-token API path resolves by the token's sub")
	}
	// The Zitadel uid, not the (absent) caller uid, is what gets recorded.
	if repo.acceptedUID != "zid-1" {
		t.Errorf("acceptedUID = %q, want the Zitadel uid zid-1", repo.acceptedUID)
	}
	// EnsureTenantClaim writes a GIP claim keyed by GIP uid; there is no
	// GIP uid on this path, so calling it would only log a failure.
	if claims.calls != 0 {
		t.Errorf("EnsureTenantClaim called %d times on the Zitadel path, want 0", claims.calls)
	}
}

// TestAccept_Zitadel_NoUIDRequired pins that the Zitadel path does not
// demand a caller-supplied uid: the invitee has no provider account at
// this point, which is exactly what Accept is about to create.
func TestAccept_Zitadel_NoUIDRequired(t *testing.T) {
	svc := NewService(Config{
		Repo:        &fakeRepo{inv: pendingInvitation()},
		FGA:         authz.NewFake(),
		Provisioner: &fakeProvisioner{t: t, returnUID: "zid-1"},
	})
	in := zitadelAcceptInput()
	in.UID = ""
	if _, err := svc.Accept(context.Background(), in); err != nil {
		t.Fatalf("Accept without a uid on the Zitadel path: %v", err)
	}
}

// TestAccept_Zitadel_ProvisioningFailureAbortsEverything is the
// half-provisioned-teammate guard. A tuple without a Zitadel account (or
// without the admin-project grant, which ProvisionStaff also ensures)
// looks like a working member and cannot sign in.
func TestAccept_Zitadel_ProvisioningFailureAbortsEverything(t *testing.T) {
	repo := &fakeRepo{inv: pendingInvitation()}
	fga := authz.NewFake()
	prov := &fakeProvisioner{t: t, err: errors.New("zitadel: 503")}
	svc := NewService(Config{Repo: repo, FGA: fga, Provisioner: prov})

	_, err := svc.Accept(context.Background(), zitadelAcceptInput())
	if err == nil {
		t.Fatal("Accept = nil error, want the accept to fail loudly rather than half-provision")
	}
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "provisioning_failed" {
		t.Errorf("err = %v, want an apperror coded provisioning_failed the accept form can show", err)
	}
	if fga.WriteCallCount() != 0 {
		t.Errorf("%d FGA writes happened after provisioning failed, want 0 — "+
			"a tuple with no provider account is the half-provisioned state this guards", fga.WriteCallCount())
	}
	if repo.acceptedID != "" {
		t.Error("invitation was marked accepted despite provisioning failing; the link must stay usable")
	}
}

// TestAccept_Zitadel_StoreScopedWritesBothKeys pins that the Phase R
// store-scoped branch got the same treatment — it writes a store role
// plus a tenant viewer back-fill, and both need both identity keys.
func TestAccept_Zitadel_StoreScopedWritesBothKeys(t *testing.T) {
	inv := pendingInvitation()
	storeID := "store-1"
	inv.StoreID = &storeID
	inv.Role = "manager"

	fga := authz.NewFake()
	svc := NewService(Config{
		Repo:        &fakeRepo{inv: inv},
		FGA:         fga,
		Provisioner: &fakeProvisioner{t: t, returnUID: "zid-1"},
	})
	if _, err := svc.Accept(context.Background(), zitadelAcceptInput()); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	for _, subject := range []string{"staff@example.com", "zid-1"} {
		if !fga.HasStoreRole(subject, "manager", "store-1") {
			t.Errorf("no store manager tuple for %q", subject)
		}
		if !fga.HasRole(subject, authz.RoleViewer, "tid-1") {
			t.Errorf("no tenant viewer back-fill for %q — session mint checks tenant membership", subject)
		}
	}
}

// TestAccept_Zitadel_DerivesProfileNamesFromEmail pins that a missing
// name does not fail the accept: Zitadel rejects an empty givenName /
// familyName, and a cosmetic field must never cost a teammate their
// invitation.
func TestAccept_Zitadel_DerivesProfileNamesFromEmail(t *testing.T) {
	prov := &fakeProvisioner{t: t, returnUID: "zid-1"}
	svc := NewService(Config{
		Repo:        &fakeRepo{inv: pendingInvitation()},
		FGA:         authz.NewFake(),
		Provisioner: prov,
	})
	in := zitadelAcceptInput()
	in.FirstName, in.LastName = "", ""
	if _, err := svc.Accept(context.Background(), in); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if prov.gotFirst == "" || prov.gotLast == "" {
		t.Errorf("derived names = (%q, %q), want both non-empty — Zitadel rejects an empty profile name",
			prov.gotFirst, prov.gotLast)
	}
}

// --- The GIP path must be untouched -----------------------------------

// TestAccept_GIP_UnchangedSingleUIDTuple pins flag-off behaviour: one
// tuple, keyed by the caller-supplied GIP uid, and the tenant claim
// still stamped. No email-keyed tuple appears.
func TestAccept_GIP_UnchangedSingleUIDTuple(t *testing.T) {
	repo := &fakeRepo{inv: pendingInvitation()}
	fga := authz.NewFake()
	claims := &fakeClaimSetter{}
	svc := NewService(Config{Repo: repo, FGA: fga, Claims: claims})

	if _, err := svc.Accept(context.Background(), acceptInput()); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !fga.HasRole("uid-staff", authz.RoleStaff, "tid-1") {
		t.Error("the GIP uid tuple is missing")
	}
	if fga.HasRole("staff@example.com", authz.RoleStaff, "tid-1") {
		t.Error("the GIP path must NOT start writing an email-keyed tuple")
	}
	if fga.WriteCallCount() != 1 {
		t.Errorf("%d FGA writes, want exactly 1 on the GIP path", fga.WriteCallCount())
	}
	if claims.calls != 1 || claims.uid != "uid-staff" {
		t.Errorf("EnsureTenantClaim called %d times with uid %q, want 1 with uid-staff", claims.calls, claims.uid)
	}
	if repo.acceptedUID != "uid-staff" {
		t.Errorf("acceptedUID = %q, want uid-staff", repo.acceptedUID)
	}
}

// TestAccept_GIP_StillRequiresUID pins that relaxing the uid requirement
// for Zitadel did not relax it for GIP, where a missing uid means an
// unusable tuple.
func TestAccept_GIP_StillRequiresUID(t *testing.T) {
	svc := NewService(Config{Repo: &fakeRepo{inv: pendingInvitation()}, FGA: authz.NewFake()})
	in := acceptInput()
	in.UID = ""

	_, err := svc.Accept(context.Background(), in)
	ae, ok := apperrors.As(err)
	if !ok || ae.Code != "invalid_input" {
		t.Fatalf("err = %v, want the unchanged invalid_input rejection", err)
	}
}

func TestProfileNames(t *testing.T) {
	cases := []struct{ first, last, email, wantFirst, wantLast string }{
		{"Sam", "Staff", "a@b.com", "Sam", "Staff"},
		{"", "", "jane.doe@example.com", "Jane", "Doe"},
		{"", "", "jane@example.com", "Jane", "Jane"},
		{"", "", "jane.van.doe@example.com", "Jane", "Van doe"},
	}
	for _, tc := range cases {
		gotFirst, gotLast := profileNames(tc.first, tc.last, tc.email)
		if gotFirst != tc.wantFirst || gotLast != tc.wantLast {
			t.Errorf("profileNames(%q,%q,%q) = (%q,%q), want (%q,%q)",
				tc.first, tc.last, tc.email, gotFirst, gotLast, tc.wantFirst, tc.wantLast)
		}
	}
}

// --- UpdateMemberRole keeps both identity keys in step ----------------

type roleChangeRepo struct {
	Repository
	inv       *Invitation
	updated   string
	updateErr error
}

func (r *roleChangeRepo) FindAcceptedByEmail(_ context.Context, _, _ string) (*Invitation, error) {
	return r.inv, nil
}

func (r *roleChangeRepo) UpdateRoleByEmail(_ context.Context, _, _, role string) error {
	r.updated = role
	return r.updateErr
}

type roleChangeTenantRepo struct {
	tenant.Repository
}

func (roleChangeTenantRepo) GetByID(_ context.Context, id string) (*tenant.Tenant, error) {
	return &tenant.Tenant{ID: id, Name: "Bondi", OwnerEmail: "owner@example.com"}, nil
}

func acceptedStaffInvitation() *Invitation {
	uid := "zid-1"
	return &Invitation{
		ID:               "inv-1",
		TenantID:         "tid-1",
		Email:            "staff@example.com",
		Role:             "staff",
		Status:           StatusAccepted,
		AcceptedByUserID: &uid,
	}
}

// TestUpdateMemberRole_Zitadel_WritesBothKeys pins that a role change
// reaches the EMAIL-keyed tuple too. Updating only the uid-keyed one
// would look correct in the team list and change nothing at sign-in,
// because the login path resolves membership by email.
func TestUpdateMemberRole_Zitadel_WritesBothKeys(t *testing.T) {
	fga := authz.NewFake()
	ctx := context.Background()
	// The actor must be an owner to be allowed to change roles.
	if err := fga.WriteRole(ctx, "owner-uid", authz.RoleOwner, "tid-1"); err != nil {
		t.Fatalf("seed actor role: %v", err)
	}
	svc := NewService(Config{
		Repo:        &roleChangeRepo{inv: acceptedStaffInvitation()},
		TenantRepo:  roleChangeTenantRepo{},
		FGA:         fga,
		Provisioner: &fakeProvisioner{t: t, returnUID: "zid-1"},
	})

	if _, err := svc.UpdateMemberRole(ctx, UpdateMemberRoleInput{
		TenantID:    "tid-1",
		TargetEmail: "staff@example.com",
		NewRole:     "admin",
		ActorUID:    "owner-uid",
	}); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if !fga.HasRole("zid-1", authz.RoleAdmin, "tid-1") {
		t.Error("the uid-keyed tuple was not updated")
	}
	if !fga.HasRole("staff@example.com", authz.RoleAdmin, "tid-1") {
		t.Error("the email-keyed tuple was not updated — the login path would keep reading the old role")
	}
}

// TestUpdateMemberRole_GIP_WritesOnlyTheUIDKey pins that the GIP path
// still writes exactly one tuple.
func TestUpdateMemberRole_GIP_WritesOnlyTheUIDKey(t *testing.T) {
	fga := authz.NewFake()
	ctx := context.Background()
	if err := fga.WriteRole(ctx, "owner-uid", authz.RoleOwner, "tid-1"); err != nil {
		t.Fatalf("seed actor role: %v", err)
	}
	svc := NewService(Config{
		Repo:       &roleChangeRepo{inv: acceptedStaffInvitation()},
		TenantRepo: roleChangeTenantRepo{},
		FGA:        fga,
	})

	if _, err := svc.UpdateMemberRole(ctx, UpdateMemberRoleInput{
		TenantID:    "tid-1",
		TargetEmail: "staff@example.com",
		NewRole:     "admin",
		ActorUID:    "owner-uid",
	}); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if fga.HasRole("staff@example.com", authz.RoleAdmin, "tid-1") {
		t.Error("the GIP path must not start writing an email-keyed tuple")
	}
}
