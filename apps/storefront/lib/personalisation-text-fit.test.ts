import { describe, expect, it } from "vitest";

import { fitTextSize, textHalo } from "./personalisation-text-fit";

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

describe("textHalo", () => {
  it("rings a dark or saturated colour in white", () => {
    expect(textHalo("#B030C0")).toBe("#fff");
    expect(textHalo("#111111")).toBe("#fff");
  });

  it("rings a light colour in near-black", () => {
    expect(textHalo("#FFFFFF")).toBe("#111");
    expect(textHalo("#F5D90A")).toBe("#111");
  });

  it("leaves text with no picked colour alone", () => {
    expect(textHalo(undefined)).toBeUndefined();
    expect(textHalo("not-a-colour")).toBeUndefined();
  });
});
