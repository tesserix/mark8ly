import { describe, expect, it } from "vitest";

import { isUsableWeight } from "./usable-weight";

describe("isUsableWeight", () => {
  it("accepts a positive decimal", () => {
    expect(isUsableWeight("1.20")).toBe(true);
    expect(isUsableWeight("0.2")).toBe(true);
  });

  it("rejects empty and whitespace", () => {
    expect(isUsableWeight("")).toBe(false);
    expect(isUsableWeight("   ")).toBe(false);
  });

  it("rejects null and undefined", () => {
    expect(isUsableWeight(null)).toBe(false);
    expect(isUsableWeight(undefined)).toBe(false);
  });

  // A zero weight is exactly what made ShipEngine refuse every rate and
  // 500 the order — it must not read as "set".
  it("rejects zero and negatives", () => {
    expect(isUsableWeight("0")).toBe(false);
    expect(isUsableWeight("0.0")).toBe(false);
    expect(isUsableWeight("-1")).toBe(false);
  });

  it("rejects text that is not a number", () => {
    expect(isUsableWeight("heavy")).toBe(false);
    expect(isUsableWeight("1.2kg")).toBe(false);
  });
});
