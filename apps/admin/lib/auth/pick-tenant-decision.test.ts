import { describe, expect, it } from "vitest";

import {
  decidePickTenantOutcome,
  type PickTenantTenant,
} from "./pick-tenant-decision";

const A: PickTenantTenant = { tenant_id: "tenant-a", name: "A", role: "owner" };
const B: PickTenantTenant = { tenant_id: "tenant-b", name: "B", role: "staff" };
const FOREIGN = "tenant-foreign";

describe("decidePickTenantOutcome", () => {
  it("redirects to /login for a user with zero tenants", () => {
    expect(
      decidePickTenantOutcome({ tenants: [], hostTenantId: null }),
    ).toEqual({ kind: "redirect_login" });
    expect(
      decidePickTenantOutcome({ tenants: [], hostTenantId: "tenant-a" }),
    ).toEqual({ kind: "redirect_login" });
  });

  it("auto-skips a single-tenant user to /dashboard on the canonical host", () => {
    expect(
      decidePickTenantOutcome({ tenants: [A], hostTenantId: null }),
    ).toEqual({ kind: "redirect_dashboard" });
  });

  it("auto-skips a single-tenant user to /dashboard on their own slug subdomain", () => {
    expect(
      decidePickTenantOutcome({ tenants: [A], hostTenantId: A.tenant_id }),
    ).toEqual({ kind: "redirect_dashboard" });
  });

  it("renders the picker when a single-tenant user lands on a wrong slug subdomain — REGRESSION GUARD", () => {
    // This is the case that produced the original /dashboard ↔ /pick-tenant
    // loop on playwrite-test-admin.mark8ly.com. If this test ever flips back
    // to redirect_dashboard, the loop is back. Do not change this without
    // also rethinking apps/admin/middleware.ts auto-switch failure path.
    expect(
      decidePickTenantOutcome({ tenants: [A], hostTenantId: FOREIGN }),
    ).toEqual({ kind: "render_picker", wrongStore: true });
  });

  it("renders the picker for multi-tenant users on the canonical host", () => {
    expect(
      decidePickTenantOutcome({ tenants: [A, B], hostTenantId: null }),
    ).toEqual({ kind: "render_picker", wrongStore: false });
  });

  it("renders the picker without wrong-store header when a multi-tenant user is on one of their slugs", () => {
    expect(
      decidePickTenantOutcome({ tenants: [A, B], hostTenantId: A.tenant_id }),
    ).toEqual({ kind: "render_picker", wrongStore: false });
    expect(
      decidePickTenantOutcome({ tenants: [A, B], hostTenantId: B.tenant_id }),
    ).toEqual({ kind: "render_picker", wrongStore: false });
  });

  it("renders the picker WITH wrong-store header when a multi-tenant user is on a foreign slug", () => {
    expect(
      decidePickTenantOutcome({ tenants: [A, B], hostTenantId: FOREIGN }),
    ).toEqual({ kind: "render_picker", wrongStore: true });
  });

  it("never returns redirect_dashboard for a host slug not in the user's tenant list", () => {
    // Property check — covers any future shape change. The loop only
    // appears when we redirect to /dashboard from a host whose slug
    // resolves to a tenant the user doesn't own.
    const cases = [
      { tenants: [A], hostTenantId: FOREIGN },
      { tenants: [A, B], hostTenantId: FOREIGN },
      { tenants: [A, B, { tenant_id: "tenant-c", name: "C", role: "viewer" }], hostTenantId: FOREIGN },
    ];
    for (const c of cases) {
      const out = decidePickTenantOutcome(c);
      expect(out.kind).not.toBe("redirect_dashboard");
    }
  });
});
