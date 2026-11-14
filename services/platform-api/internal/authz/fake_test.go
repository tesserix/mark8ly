package authz

import (
	"context"
	"testing"
)

// These tests pin the Phase O derived-permission semantics on the
// FakeClient. The real OpenFGA-backed client is guarded separately by
// the onboarding completion integration test which already boots a
// real container — splitting the fakes here keeps the unit tests
// fast while still catching drift between "what the DSL says" and
// "what the fake returns".

func TestFake_WriteOwnership_GrantsMembership(t *testing.T) {
	ctx := context.Background()
	f := NewFake()

	if err := f.WriteOwnership(ctx, "u1", "t1"); err != nil {
		t.Fatalf("WriteOwnership: %v", err)
	}

	if !f.HasOwnership("u1", "t1") {
		t.Error("HasOwnership should be true after WriteOwnership")
	}
	// Derived via the union — owner implies member.
	if !f.HasMembership("u1", "t1") {
		t.Error("HasMembership should be true because owner ⊂ member")
	}
}

func TestFake_Check_OwnerCanEditSettings(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	if err := f.WriteOwnership(ctx, "u1", "t1"); err != nil {
		t.Fatal(err)
	}

	ok, err := f.Check(ctx, "u1", "can_edit_settings", "t1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !ok {
		t.Error("owner should be allowed can_edit_settings")
	}
}

func TestFake_Check_StaffCannotEditSettings(t *testing.T) {
	ctx := context.Background()
	f := NewFake()
	if err := f.WriteRole(ctx, "u1", RoleStaff, "t1"); err != nil {
		t.Fatal(err)
	}

	editOK, err := f.Check(ctx, "u1", "can_edit_settings", "t1")
	if err != nil {
		t.Fatalf("Check edit: %v", err)
	}
	if editOK {
		t.Error("staff should NOT be allowed can_edit_settings")
	}

	viewOK, err := f.Check(ctx, "u1", "can_view_settings", "t1")
	if err != nil {
		t.Fatalf("Check view: %v", err)
	}
	if !viewOK {
		t.Error("staff should be allowed can_view_settings")
	}
}

func TestFake_Check_StrangerDenied(t *testing.T) {
	ctx := context.Background()
	f := NewFake()

	for _, rel := range []string{"member", "can_view_settings", "can_edit_settings", "owner", "admin", "staff", "viewer"} {
		ok, err := f.Check(ctx, "u-unknown", rel, "t1")
		if err != nil {
			t.Fatalf("Check %s: %v", rel, err)
		}
		if ok {
			t.Errorf("stranger should be denied %s", rel)
		}
	}
}

func TestFake_GetRole_ReturnsHighestRole(t *testing.T) {
	ctx := context.Background()
	f := NewFake()

	// Intentionally write the tuples in priority-inverted order to
	// catch "accidentally returned the last written role" bugs.
	for _, r := range []Role{RoleViewer, RoleStaff, RoleAdmin, RoleOwner} {
		if err := f.WriteRole(ctx, "u1", r, "t1"); err != nil {
			t.Fatalf("WriteRole %s: %v", r, err)
		}
	}
	got, err := f.GetRole(ctx, "u1", "t1")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got != RoleOwner {
		t.Errorf("GetRole = %q, want %q", got, RoleOwner)
	}
}

func TestFake_GetRole_EmptyForStranger(t *testing.T) {
	got, err := NewFake().GetRole(context.Background(), "u-unknown", "t1")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got != "" {
		t.Errorf("GetRole = %q, want empty", got)
	}
}

func TestFake_WriteRole_RejectsUnknownRole(t *testing.T) {
	err := NewFake().WriteRole(context.Background(), "u1", Role("wizard"), "t1")
	if err == nil {
		t.Error("expected error for unknown role")
	}
}

// A cashier (#1104) is a member who can sell in store and nothing else.
func TestFake_Cashier_MemberWhoCanOnlySell(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	if err := f.WriteRole(ctx, "c1", RoleCashier, "t1"); err != nil {
		t.Fatalf("WriteRole(cashier): %v", err)
	}
	want := map[string]bool{
		"cashier": true, "member": true, "can_sell_in_store": true,
		"viewer": false, "staff": false, "can_view_settings": false,
		"can_edit_settings": false, "can_invite_members": false,
	}
	for rel, exp := range want {
		if ok, err := f.Check(ctx, "c1", rel, "t1"); err != nil || ok != exp {
			t.Errorf("Check(%q) = %v, %v; want %v", rel, ok, err, exp)
		}
	}
	if role, _ := f.GetRole(ctx, "c1", "t1"); role != RoleCashier {
		t.Errorf("GetRole = %q, want cashier", role)
	}
}
