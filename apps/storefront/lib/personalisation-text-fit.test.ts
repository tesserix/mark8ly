import { describe, expect, it } from "vitest";

import { fitTextSize } from "./personalisation-text-fit";

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
