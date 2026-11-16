import { describe, expect, it } from "vitest";

import { curvedTextLayout, cylinderGlyphs, fitTextSize, textInk } from "./personalisation-text-fit";

const parse = (css: string) => {
  const m = css.match(/min\(([\d.]+)cqh, ([\d.]+)cqw\)/);
  if (!m) throw new Error(css);
  return { cqh: Number(m[1]), cqw: Number(m[2]) };
};

describe("fitTextSize", () => {
  it("shrinks a long name to fit the width", () => {
    expect(parse(fitTextSize("Alexandria-Rose", "classic")).cqw).toBeLessThan(
      parse(fitTextSize("Asha", "classic")).cqw,
    );
  });

  it("splits the height between the buyer's lines", () => {
    expect(parse(fitTextSize("Happy\nBirthday", "classic")).cqh).toBe(35);
  });

  it("lets a condensed font run larger at the same length", () => {
    expect(parse(fitTextSize("Asha", "block")).cqw).toBeGreaterThan(
      parse(fitTextSize("Asha", "classic")).cqw,
    );
  });

  it("does not divide by zero on an empty value", () => {
    expect(() => parse(fitTextSize("", undefined))).not.toThrow();
  });
});

describe("textInk", () => {
  // Ink, not a sticker: the surface's grain and shading show through.
  it("multiplies a dark or saturated colour into the photo", () => {
    expect(textInk("#6B1F2A").mixBlendMode).toBe("multiply");
    expect(textInk("#111111").mixBlendMode).toBe("multiply");
  });

  // Multiply would make white vanish, so light ink sits on top, slightly sheer.
  it("keeps a light colour visible", () => {
    expect(textInk("#FFFFFF").mixBlendMode).toBe("normal");
    expect(textInk("#F5E6C8").opacity).toBeLessThan(1);
  });

  it("multiplies default ink when no colour is picked", () => {
    expect(textInk(undefined).mixBlendMode).toBe("multiply");
    expect(textInk("not-a-colour").mixBlendMode).toBe("multiply");
  });
});

describe("curvedTextLayout", () => {
  it("fits the same size as the flat preview", () => {
    // 70cqh / 1 line = 70% of 100px; 92 / (4 × 0.56) cqw of 200px ≈ 82px.
    expect(curvedTextLayout("Asha", "classic", 200, 100).fontSize).toBeCloseTo(70, 1);
  });

  // A mug seen from a little above: the line dips in the middle, ends rise.
  it("bends each line into a smile across the print area", () => {
    const { lines } = curvedTextLayout("Grandma", "modern", 200, 100);
    expect(lines).toHaveLength(1);
    const [, x0, y0, , cy, x1, y1] = lines[0]!.match(/M([\d.]+) ([\d.]+) Q([\d.]+) ([\d.]+) ([\d.]+) ([\d.]+)/)!.map(Number);
    expect(x0).toBe(0);
    expect(x1).toBe(200);
    expect(y0).toBe(y1);
    expect(cy).toBeGreaterThan(y0!);
  });

  it("stacks the buyer's lines top to bottom", () => {
    const { lines } = curvedTextLayout("Happy\nBirthday", "classic", 200, 100);
    const ys = lines.map((d) => Number(d.split(" ")[1]));
    expect(ys[1]).toBeGreaterThan(ys[0]!);
  });
});

describe("cylinderGlyphs", () => {
  // Five 10px letters, a 100px print area: the middle letter faces the
  // buyer; letters further out turn away, so they narrow and sit higher.
  const glyphs = cylinderGlyphs([0, 10, 20, 30, 40], [10, 10, 10, 10, 10], 100, 50, 6);

  it("keeps the middle letter full width, centred", () => {
    expect(glyphs[2]!.scaleX).toBeCloseTo(1, 2);
    expect(glyphs[2]!.x).toBeCloseTo(50, 1);
    expect(glyphs[2]!.y).toBeCloseTo(50, 1);
  });

  it("narrows and raises letters symmetrically towards the edges", () => {
    expect(glyphs[0]!.scaleX).toBeLessThan(glyphs[1]!.scaleX);
    expect(glyphs[1]!.scaleX).toBeLessThan(1);
    expect(glyphs[0]!.scaleX).toBeCloseTo(glyphs[4]!.scaleX, 5);
    expect(glyphs[0]!.y).toBeLessThan(glyphs[1]!.y);
    expect(glyphs[0]!.y).toBeCloseTo(glyphs[4]!.y, 5);
    expect(50 - glyphs[0]!.x).toBeCloseTo(glyphs[4]!.x - 50, 5);
  });

  it("brings neighbours closer as they turn away, keeping their order", () => {
    const xs = glyphs.map((g) => g.x);
    expect(xs).toEqual([...xs].sort((a, b) => a - b));
    expect(xs[1]! - xs[0]!).toBeLessThan(xs[2]! - xs[1]!);
  });
});
