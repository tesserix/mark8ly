import { describe, expect, it } from "vitest";

import { canEditMember, inviteRoleOptions, memberRoleOptions, roleLabel } from "./roles";

describe("team roles", () => {
  it("offers Cashier for a tenant-wide invite, not a store invite", () => {
    expect(inviteRoleOptions("tenant")).toContain("cashier");
    expect(inviteRoleOptions("store")).not.toContain("cashier");
  });

  it("an admin can move a member to and from cashier", () => {
    expect(memberRoleOptions("admin")).toEqual(["staff", "viewer", "cashier"]);
    expect(canEditMember("admin", { kind: "invited", email: "c@x.test", role: "cashier" }, "me@x.test")).toBe(true);
  });

  it("an owner can set any non-owner role", () => {
    expect(memberRoleOptions("owner")).toEqual(["admin", "staff", "viewer", "cashier"]);
  });

  it("an admin still cannot change another admin", () => {
    expect(canEditMember("admin", { kind: "invited", email: "a@x.test", role: "admin" }, "me@x.test")).toBe(false);
  });

  it("says what a cashier can do", () => {
    expect(roleLabel("cashier")).toBe("Cashier (in-store sales only)");
    expect(roleLabel("staff")).toBe("Staff");
  });
});
