import { describe, expect, it } from "vitest";
import {
  countryToCurrency,
  COUNTRY_TO_CURRENCY,
  SUPPORTED_CURRENCIES,
} from "../../../lib/geo/countryToCurrency";

describe("countryToCurrency", () => {
  describe("core 18 billing currency mappings", () => {
    it.each([
      // Developed markets
      ["US", "USD"],
      ["CA", "CAD"],
      ["GB", "GBP"],
      ["AU", "AUD"],
      ["NZ", "NZD"],
      ["SG", "SGD"],
      ["HK", "HKD"],
      ["JP", "JPY"],
      ["KR", "KRW"],
      // Middle East
      ["AE", "AED"],
      ["SA", "SAR"],
      // South & SE Asia — PPP-adjusted
      ["IN", "INR"],
      ["MY", "MYR"],
      ["ID", "IDR"],
      ["PH", "PHP"],
      ["TH", "THB"],
      ["VN", "VND"],
      // Plan test cases (emerging markets)
      ["BR", "BRL"],
      ["MX", "MXN"],
      ["ZA", "ZAR"],
      ["NG", "NGN"],
      ["KE", "KES"],
    ] as const)("maps %s → %s", (cc, expected) => {
      expect(countryToCurrency(cc)).toBe(expected);
    });
  });

  describe("EU member states → EUR", () => {
    it.each([
      "DE",
      "FR",
      "IT",
      "ES",
      "NL",
      "BE",
      "IE",
      "PT",
      "AT",
      "FI",
      "GR",
      "LU",
      "HR",
      "CY",
      "EE",
      "LT",
      "LV",
      "MT",
      "SI",
      "SK",
    ])("%s maps to EUR", (cc) => {
      expect(countryToCurrency(cc)).toBe("EUR");
    });
  });

  describe("edge cases", () => {
    it("returns USD for null input", () => {
      expect(countryToCurrency(null)).toBe("USD");
    });

    it("returns USD for undefined input", () => {
      expect(countryToCurrency(undefined)).toBe("USD");
    });

    it("returns USD for empty string", () => {
      expect(countryToCurrency("")).toBe("USD");
    });

    it("handles lowercase input (case-insensitive)", () => {
      expect(countryToCurrency("gb")).toBe("GBP");
      expect(countryToCurrency("us")).toBe("USD");
      expect(countryToCurrency("de")).toBe("EUR");
      expect(countryToCurrency("in")).toBe("INR");
    });

    it("handles mixed-case input", () => {
      expect(countryToCurrency("Gb")).toBe("GBP");
      expect(countryToCurrency("Ca")).toBe("CAD");
    });

    it("returns USD for unknown country codes", () => {
      expect(countryToCurrency("RU")).toBe("USD");
      expect(countryToCurrency("CN")).toBe("USD");
      expect(countryToCurrency("XX")).toBe("USD");
      expect(countryToCurrency("NK")).toBe("USD");
    });

    it("returns USD for invalid country codes", () => {
      expect(countryToCurrency("ZZ")).toBe("USD");
      expect(countryToCurrency("123")).toBe("USD");
    });
  });

  describe("SUPPORTED_CURRENCIES tuple", () => {
    it("is a readonly tuple of currency codes", () => {
      expect(Array.isArray(SUPPORTED_CURRENCIES)).toBe(true);
      expect(SUPPORTED_CURRENCIES.length).toBeGreaterThanOrEqual(18);
    });

    it("includes all core 18 currencies", () => {
      const core = [
        "USD", "CAD", "GBP", "AUD", "NZD", "EUR",
        "INR", "SGD", "HKD", "MYR", "IDR", "PHP",
        "THB", "VND", "JPY", "KRW", "AED", "SAR",
      ];
      for (const c of core) {
        expect(SUPPORTED_CURRENCIES).toContain(c);
      }
    });
  });

  describe("COUNTRY_TO_CURRENCY map", () => {
    it("only maps to currencies in SUPPORTED_CURRENCIES", () => {
      for (const currency of Object.values(COUNTRY_TO_CURRENCY)) {
        expect(SUPPORTED_CURRENCIES).toContain(currency);
      }
    });
  });
});
