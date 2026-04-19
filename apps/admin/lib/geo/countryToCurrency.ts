/**
 * Maps Cloudflare CF-IPCountry (ISO-3166 alpha-2) codes to ISO-4217
 * billing currencies supported by Mark8ly (spec §3.2, 18 currencies).
 *
 * All 27 EU member states map to EUR. Countries not in the map default
 * to USD so the pricing page always has a valid currency to display.
 */

export const SUPPORTED_CURRENCIES = [
  "USD",
  "CAD",
  "GBP",
  "AUD",
  "NZD",
  "EUR",
  "INR",
  "SGD",
  "HKD",
  "MYR",
  "IDR",
  "PHP",
  "THB",
  "VND",
  "JPY",
  "KRW",
  "AED",
  "SAR",
  // Additional PPP emerging-market currencies (plan test cases)
  "BRL",
  "MXN",
  "ZAR",
  "NGN",
  "KES",
] as const;

export type Currency = (typeof SUPPORTED_CURRENCIES)[number];

/**
 * All 27 EU member states that use the Euro as their currency.
 * Source: https://ec.europa.eu/info/business-economy-euro/euro-area/euro-area-countries_en
 */
const EU_MEMBER_STATES: readonly string[] = [
  "AT", // Austria
  "BE", // Belgium
  "CY", // Cyprus
  "DE", // Germany
  "EE", // Estonia
  "ES", // Spain
  "FI", // Finland
  "FR", // France
  "GR", // Greece
  "HR", // Croatia
  "IE", // Ireland
  "IT", // Italy
  "LT", // Lithuania
  "LU", // Luxembourg
  "LV", // Latvia
  "MT", // Malta
  "NL", // Netherlands
  "PT", // Portugal
  "SI", // Slovenia
  "SK", // Slovakia
] as const;

export const COUNTRY_TO_CURRENCY: Record<string, Currency> = {
  // Developed markets — USD parity
  US: "USD",
  CA: "CAD",
  GB: "GBP",
  AU: "AUD",
  NZ: "NZD",
  SG: "SGD",
  HK: "HKD",
  JP: "JPY",
  KR: "KRW",

  // Middle East
  AE: "AED",
  SA: "SAR",

  // South & South-East Asia — PPP-adjusted
  IN: "INR",
  MY: "MYR",
  ID: "IDR",
  PH: "PHP",
  TH: "THB",
  VN: "VND",

  // Emerging markets (additional plan test cases)
  BR: "BRL",
  MX: "MXN",
  ZA: "ZAR",
  NG: "NGN",
  KE: "KES",

  // EU member states all map to EUR
  ...Object.fromEntries(EU_MEMBER_STATES.map((cc) => [cc, "EUR" as Currency])),
};

/**
 * Maps a CF-IPCountry header value to a supported billing currency.
 *
 * @param cc - ISO-3166 alpha-2 country code (case-insensitive). Accepts
 *             null/undefined safely and falls back to USD.
 * @returns   An ISO-4217 currency code that Mark8ly supports for billing.
 */
export function countryToCurrency(cc: string | null | undefined): Currency {
  if (!cc) return "USD";
  const upper = cc.toUpperCase();
  return COUNTRY_TO_CURRENCY[upper] ?? "USD";
}
