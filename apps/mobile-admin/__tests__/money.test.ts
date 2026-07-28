import { formatMoney, formatWholeMoney } from "@/lib/money";

describe("formatMoney — the exact-amount default", () => {
  it("keeps two decimal places in the store's currency", () => {
    expect(formatMoney(8400.5, "AUD")).toBe("$8,400.50");
  });

  it("falls back to a plain 2dp number with no currency code", () => {
    expect(formatMoney(8400.5)).toBe("8,400.50");
  });
});

describe("formatWholeMoney — the display formatter", () => {
  // The defect: `maximumFractionDigits: 0` rounds HALF-UP, so an $8,400.50
  // order rendered "$8,401" — a display column overstating money on the
  // screen a merchant signs off their day from. The doc promises dropped
  // cents, so it drops them.
  it("truncates the cents instead of rounding them up", () => {
    expect(formatWholeMoney(8400.5, "AUD")).toBe("$8,400");
    expect(formatWholeMoney(8400.99, "AUD")).toBe("$8,400");
  });

  it("never renders more than the amount actually is", () => {
    for (const cents of [0.01, 0.49, 0.5, 0.51, 0.99]) {
      expect(formatWholeMoney(189 + cents, "AUD")).toBe("$189");
    }
  });

  it("rounds toward zero on a negative amount, not away from it", () => {
    // A -$8,400.50 credit is not a -$8,401 credit.
    expect(formatWholeMoney(-8400.5, "AUD")).toBe("-$8,400");
  });

  it("leaves a whole amount alone", () => {
    expect(formatWholeMoney(189, "AUD")).toBe("$189");
    expect(formatWholeMoney(0, "AUD")).toBe("$0");
  });

  it("truncates the no-currency fallback too", () => {
    expect(formatWholeMoney(8400.5)).toBe("8,400");
  });
});

// `Intl.NumberFormat.format(NaN)` returns the literal string "NaN" and
// `Math.trunc(NaN)` is NaN, so neither formatter caught it on its own. The
// SAME class was fixed one commit earlier in `MetricsCard.changeBadge` (which
// returns an em dash for a non-finite percentage) and both of these siblings
// were missed — which is how a merchant's monthly revenue could have rendered
// as "NaN" on the Dashboard hero, and been spoken as that by VoiceOver
// through `RevenueChart`'s accessibilityLabel.
describe("money — non-finite input never reaches the screen", () => {
  const NON_FINITE = [NaN, Infinity, -Infinity];

  it("renders an em dash rather than NaN or ∞ from formatWholeMoney", () => {
    for (const value of NON_FINITE) {
      expect(formatWholeMoney(value, "AUD")).toBe("—");
      expect(formatWholeMoney(value)).toBe("—");
    }
  });

  it("renders an em dash rather than NaN or ∞ from formatMoney", () => {
    for (const value of NON_FINITE) {
      expect(formatMoney(value, "AUD")).toBe("—");
      expect(formatMoney(value)).toBe("—");
    }
  });

  // Stated as the property, not as three examples: nothing either formatter
  // produces may contain the substrings a raw Intl passthrough would.
  it("never emits the literal NaN or ∞ from either formatter", () => {
    for (const value of NON_FINITE) {
      for (const rendered of [
        formatMoney(value, "AUD"),
        formatMoney(value),
        formatWholeMoney(value, "AUD"),
        formatWholeMoney(value),
      ]) {
        expect(rendered).not.toMatch(/NaN|∞/);
      }
    }
  });

  it("leaves every finite amount, including zero and negatives, untouched", () => {
    expect(formatWholeMoney(0, "AUD")).toBe("$0");
    expect(formatWholeMoney(-12.5, "AUD")).toBe("-$12");
    expect(formatMoney(0, "AUD")).toBe("$0.00");
  });
});
