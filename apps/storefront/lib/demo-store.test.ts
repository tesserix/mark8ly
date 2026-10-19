import { describe, expect, it } from "vitest";

import { isDemoStore } from "./demo-store";

describe("isDemoStore", () => {
  it("flags the Bondi demo store, whatever the case or padding", () => {
    expect(isDemoStore("the-bondi-store")).toBe(true);
    expect(isDemoStore(" The-Bondi-Store ")).toBe(true);
  });
  it("leaves every other store alone", () => {
    expect(isDemoStore("real-shop")).toBe(false);
    expect(isDemoStore("the-bondi-store-2")).toBe(false);
    expect(isDemoStore("")).toBe(false);
    expect(isDemoStore(null)).toBe(false);
  });
});
