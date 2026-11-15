import { describe, expect, it } from "vitest";

import { cashierPath } from "./cashier";

describe("cashierPath", () => {
  it("allows the sale screen, its actions and sign-out", () => {
    for (const p of ["/sell", "/sell/", "/logout"]) expect(cashierPath(p), p).toBe("allow");
  });

  it("lets a cashier who works for several businesses pick one", () => {
    expect(cashierPath("/pick-tenant")).toBe("allow");
  });

  it("redirects every other admin page", () => {
    for (const p of ["/", "/dashboard", "/orders", "/orders/abc", "/settings/team", "/products", "/customers", "/seller", "/api/notifications"]) {
      expect(cashierPath(p), p).toBe("redirect");
    }
  });
});
