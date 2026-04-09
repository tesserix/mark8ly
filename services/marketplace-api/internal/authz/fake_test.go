package authz

import (
	"context"
	"testing"
)

func TestFakeClient_GrantOwner_ImpliesLower(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleOwner, "t1")
	ctx := context.Background()

	for _, rel := range []string{"owner", "admin", "staff", "viewer"} {
		ok, err := f.Check(ctx, "u1", rel, "t1")
		if err != nil || !ok {
			t.Fatalf("Check(%q) = %v, %v; want true, nil", rel, ok, err)
		}
	}
	mem, err := f.CheckMembership(ctx, "u1", "t1")
	if err != nil || !mem {
		t.Fatalf("CheckMembership = %v, %v; want true, nil", mem, err)
	}
}

func TestFakeClient_GrantAdmin(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleAdmin, "t1")
	ctx := context.Background()

	if ok, _ := f.Check(ctx, "u1", "owner", "t1"); ok {
		t.Fatal("admin should not imply owner")
	}
	for _, rel := range []string{"admin", "staff", "viewer"} {
		if ok, _ := f.Check(ctx, "u1", rel, "t1"); !ok {
			t.Fatalf("admin should imply %q", rel)
		}
	}
	if ok, _ := f.CheckMembership(ctx, "u1", "t1"); !ok {
		t.Fatal("admin should imply membership")
	}
}

func TestFakeClient_GrantStaff(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleStaff, "t1")
	ctx := context.Background()

	if ok, _ := f.Check(ctx, "u1", "owner", "t1"); ok {
		t.Fatal("staff should not imply owner")
	}
	if ok, _ := f.Check(ctx, "u1", "admin", "t1"); ok {
		t.Fatal("staff should not imply admin")
	}
	if ok, _ := f.Check(ctx, "u1", "staff", "t1"); !ok {
		t.Fatal("staff should satisfy staff")
	}
	if ok, _ := f.CheckMembership(ctx, "u1", "t1"); !ok {
		t.Fatal("staff should imply membership")
	}
}

func TestFakeClient_NoGrant(t *testing.T) {
	f := NewFakeClient()
	ctx := context.Background()
	for _, rel := range []string{"owner", "admin", "staff", "viewer"} {
		if ok, _ := f.Check(ctx, "u1", rel, "t1"); ok {
			t.Fatalf("ungranted user should fail Check(%q)", rel)
		}
	}
	if ok, _ := f.CheckMembership(ctx, "u1", "t1"); ok {
		t.Fatal("ungranted user should fail membership")
	}
}

func TestFakeClient_CrossTenantIsolation(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleOwner, "tA")
	ctx := context.Background()
	if ok, _ := f.Check(ctx, "u1", "owner", "tB"); ok {
		t.Fatal("grant on tA should not leak to tB")
	}
	if ok, _ := f.CheckMembership(ctx, "u1", "tB"); ok {
		t.Fatal("membership on tA should not leak to tB")
	}
}

func TestFakeClient_GetRole_HighestWins(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleStaff, "t1")
	f.Grant("u1", RoleAdmin, "t1")
	ctx := context.Background()
	role, err := f.GetRole(ctx, "u1", "t1")
	if err != nil {
		t.Fatalf("GetRole err: %v", err)
	}
	if role != RoleAdmin {
		t.Fatalf("GetRole = %q; want admin", role)
	}
	// Grant a lower role after admin — should not downgrade.
	f.Grant("u1", RoleViewer, "t1")
	role, _ = f.GetRole(ctx, "u1", "t1")
	if role != RoleAdmin {
		t.Fatalf("GetRole after lower grant = %q; want admin", role)
	}
}

func TestFakeClient_Revoke(t *testing.T) {
	f := NewFakeClient()
	f.Grant("u1", RoleOwner, "t1")
	f.Revoke("u1", "t1")
	ctx := context.Background()
	if ok, _ := f.Check(ctx, "u1", "owner", "t1"); ok {
		t.Fatal("revoked user should fail Check")
	}
	role, _ := f.GetRole(ctx, "u1", "t1")
	if role != "" {
		t.Fatalf("GetRole after revoke = %q; want empty", role)
	}
}
