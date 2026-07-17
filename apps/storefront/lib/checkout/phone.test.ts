/**
 * Regression: on an India-only store a customer entered an Australian
 * number (+61469044601). isAddressFilled() returned false, so no
 * shipping rates were fetched, Shipping showed "--", Tax showed "Enter
 * address" and Place order stayed disabled — with nothing anywhere
 * pointing at the phone field.
 *
 * Two behaviours are locked in:
 *   1. Commonly-pasted Indian formats (+91, 0 trunk, spaces) are accepted.
 *   2. Genuinely non-Indian numbers are still rejected — normalization
 *      must not launder a wrong-country number into a passing one.
 */

import { describe, it, expect } from "vitest";
import { normalizeInPhone, isInPhoneValid, isIndia } from "./phone";

describe("normalizeInPhone", () => {
  it("passes through a bare 10-digit mobile", () => {
    expect(normalizeInPhone("9876543210")).toBe("9876543210");
  });

  it("strips a +91 country code", () => {
    expect(normalizeInPhone("+919876543210")).toBe("9876543210");
  });

  it("strips spaces and dashes alongside +91", () => {
    expect(normalizeInPhone("+91 98765-43210")).toBe("9876543210");
  });

  it("strips a leading 0 trunk prefix", () => {
    expect(normalizeInPhone("09876543210")).toBe("9876543210");
  });

  it("leaves a non-Indian number recognisably wrong", () => {
    expect(normalizeInPhone("+61469044601")).toBe("61469044601");
  });
});

describe("isInPhoneValid", () => {
  it.each(["9876543210", "+919876543210", "+91 98765 43210", "09876543210", "8123456789"])(
    "accepts %s",
    (input) => {
      expect(isInPhoneValid(input)).toBe(true);
    },
  );

  it.each([
    ["+61469044601", "the AU number from the original report"],
    ["5876543210", "must start 6-9"],
    ["987654321", "too short"],
    ["98765432101", "too long"],
    ["", "empty"],
    ["not-a-number", "non-numeric"],
  ])("rejects %s (%s)", (input) => {
    expect(isInPhoneValid(input)).toBe(false);
  });
});

describe("isIndia", () => {
  it.each(["IN", "in", " In "])("recognises %s", (code) => {
    expect(isIndia(code)).toBe(true);
  });

  it.each(["AU", "US", ""])("rejects %s", (code) => {
    expect(isIndia(code)).toBe(false);
  });
});
