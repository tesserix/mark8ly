import { describe, expect, it } from "vitest";

import { resolveBrandName } from "./brand-name";

describe("resolveBrandName", () => {
  it("prefers the name the page fetched", () => {
    expect(resolveBrandName("The Bondi Store", "Other")).toBe(
      "The Bondi Store",
    );
  });
  it("falls back to the layout's resolved store — the cart page's case", () => {
    expect(resolveBrandName(undefined, "The Bondi Store")).toBe(
      "The Bondi Store",
    );
  });
  it("treats an empty page value as not supplied, so checkout shows the brand rather than nothing", () => {
    expect(resolveBrandName("", "The Bondi Store")).toBe("The Bondi Store");
    expect(resolveBrandName("   ", "The Bondi Store")).toBe("The Bondi Store");
  });
  it('only says "Store" when nothing resolved at all', () => {
    expect(resolveBrandName(undefined, null)).toBe("Store");
    expect(resolveBrandName("", "")).toBe("Store");
  });
});
