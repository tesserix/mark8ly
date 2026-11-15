import { describe, expect, it } from "vitest";

import { fitTextSize, textInk } from "./personalisation-text-fit";

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
