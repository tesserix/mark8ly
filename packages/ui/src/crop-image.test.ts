import { describe, it, expect } from "vitest";

import { fitScale } from "./crop-image";

// #966. The admin and storefront callers want OPPOSITE things from this
// module, and `fitScale` is where the difference lives:
//
//   admin       the cropped blob IS the product image -> no cap
//   storefront  the cropped blob is a PREVIEW; the buyer's upload is the
//               print source -> cap it
//
// Getting that backwards is a bug nobody catches in review. canvas.toBlob
// re-encodes, so deriving the print source from a capped canvas silently
// turns a 48 MP phone photo into something soft, and the first person to
// notice is the customer holding a figurine with a blurry face.

describe("fitScale", () => {
  it("does not scale when no cap is given — the admin path", () => {
    // The default must stay "full resolution". A cap that crept in as a
    // default would quietly degrade every product image in the catalogue.
    expect(fitScale(8000, 6000, undefined)).toBe(1);
    expect(fitScale(8000, 6000, 0)).toBe(1);
  });

  it("scales a large crop down to the cap on its longest edge", () => {
    // A 48 MP phone photo, capped to a 1024px preview.
    expect(fitScale(8000, 6000, 1024)).toBeCloseTo(1024 / 8000);
    // Portrait: the cap applies to the longest edge either way.
    expect(fitScale(6000, 8000, 1024)).toBeCloseTo(1024 / 8000);
  });

  it("never upscales an image already under the cap", () => {
    // Blowing a small photo up to hit the cap would make it blurrier and
    // bigger, which is worse on both counts.
    expect(fitScale(400, 300, 1024)).toBe(1);
    expect(fitScale(1024, 768, 1024)).toBe(1);
  });

  it("is exactly 1 at the boundary", () => {
    expect(fitScale(1024, 1024, 1024)).toBe(1);
    expect(fitScale(1025, 500, 1024)).toBeCloseTo(1024 / 1025);
  });

  it("handles a square crop", () => {
    expect(fitScale(2048, 2048, 1024)).toBeCloseTo(0.5);
  });
});
