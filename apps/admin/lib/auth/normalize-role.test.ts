import { describe, expect, it, vi } from "vitest";

vi.mock("next/headers", () => ({ headers: vi.fn(), cookies: vi.fn() }));

import { normalizeRole } from "./serverSession";

describe("normalizeRole", () => {
  // Coercing a cashier to viewer would show read-only pages the API refuses.
  it("keeps cashier", () => {
    expect(normalizeRole("cashier")).toBe("cashier");
  });

  it("falls back to viewer for anything unknown", () => {
    expect(normalizeRole("superuser")).toBe("viewer");
    expect(normalizeRole(null)).toBe("viewer");
  });
});
