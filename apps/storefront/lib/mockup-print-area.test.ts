import { describe, it, expect } from "vitest";

// The clamp that keeps a merchant-authored rectangle inside the mockup
// (#966). Mirrors MockupPreview's `pct`, which cannot be imported here:
// the storefront's vitest runs in `node` and that module pulls in
// next/image.
function pct(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.min(100, Math.max(0, n));
}

// print_area is merchant-authored and round-trips through jsonb, so the
// values reaching the browser are not guaranteed sane. A negative or
// >100 percentage positions the artwork OUTSIDE the mockup, where it
// overlaps the rest of the form — the buyer sees their photo floating
// across the price and the Add to cart button.
describe("print area clamping", () => {
  it("passes a sane rectangle through untouched", () => {
    expect(pct(10)).toBe(10);
    expect(pct(0)).toBe(0);
    expect(pct(100)).toBe(100);
    expect(pct(33.5)).toBe(33.5);
  });

  it("pulls a negative origin back onto the mockup", () => {
    expect(pct(-20)).toBe(0);
  });

  it("caps an oversized percentage at the mockup edge", () => {
    expect(pct(140)).toBe(100);
  });

  it("treats a non-finite value as zero rather than propagating NaN", () => {
    // NaN in a CSS percentage silently drops the declaration, which
    // would leave the artwork positioned at the element's default —
    // top-left of the mockup, with no indication anything was wrong.
    expect(pct(Number.NaN)).toBe(0);
    expect(pct(Number.POSITIVE_INFINITY)).toBe(0);
  });
});
