import { describe, expect, it } from "vitest";

import { normalizeCustomerEmail } from "./normalize-customer-email";

describe("normalizeCustomerEmail", () => {
  it("trims surrounding whitespace and lowercases", () => {
    expect(normalizeCustomerEmail("  Shopper@Example.COM  ")).toBe("shopper@example.com");
  });

  it("is idempotent", () => {
    const once = normalizeCustomerEmail("Shopper@Example.com");
    expect(normalizeCustomerEmail(once)).toBe(once);
  });

  it("matches marketplace-api's TrimSpace(ToLower(email)) keying for the cases that matter here", () => {
    expect(normalizeCustomerEmail("A@B.com")).toBe(normalizeCustomerEmail(" a@b.COM "));
  });
});
