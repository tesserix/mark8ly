import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({ usePathname: () => "/sell", useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

import { navigationFor } from "./AdminShell";

const hrefs = (role: Parameters<typeof navigationFor>[0], inStore = true) =>
  navigationFor(role, inStore).flatMap((s) => [s.href, ...(s.children ?? []).map((c) => c.href)]).filter(Boolean);

describe("navigationFor", () => {
  it("shows only In-store sale to a cashier", () => {
    expect(navigationFor("cashier", true)).toEqual([
      expect.objectContaining({ label: "In-store sale", href: "/sell" }),
    ]);
  });

  it("shows the full navigation to everyone else", () => {
    expect(hrefs("owner")).toEqual(expect.arrayContaining(["/dashboard", "/orders", "/sell", "/products"]));
  });

  it("hides In-store sale when the store has it turned off, but keeps its setting", () => {
    expect(hrefs("owner", false)).not.toContain("/sell");
    expect(hrefs("owner", false)).toContain("/settings/in-store");
    expect(hrefs("owner", true)).toContain("/sell");
  });

  // A viewer cannot sell (can_sell_in_store is staff and up, or cashier).
  it("hides In-store sale from a viewer", () => {
    expect(hrefs("viewer")).not.toContain("/sell");
    expect(hrefs("staff")).toContain("/sell");
  });
});
