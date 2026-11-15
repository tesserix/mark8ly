import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({ usePathname: () => "/sell", useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

import { navigationFor } from "./AdminShell";

const hrefs = (role: Parameters<typeof navigationFor>[0]) =>
  navigationFor(role).flatMap((s) => [s.href, ...(s.children ?? []).map((c) => c.href)]).filter(Boolean);

describe("navigationFor", () => {
  it("shows only In-store sale to a cashier", () => {
    expect(navigationFor("cashier")).toEqual([
      expect.objectContaining({ label: "In-store sale", href: "/sell" }),
    ]);
  });

  it("shows the full navigation to everyone else", () => {
    expect(hrefs("owner")).toEqual(expect.arrayContaining(["/dashboard", "/orders", "/sell", "/products"]));
  });
});
