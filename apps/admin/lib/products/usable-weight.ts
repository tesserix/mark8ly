/**
 * A product weight only counts if a carrier could price on it.
 *
 * Blank dimensions fall back to a standard envelope and cost nothing;
 * a blank WEIGHT is different. Checkout substitutes the store's default
 * parcel weight and the shopper is charged carrier rates derived from
 * that guess — so "no weight" is a silently mispriced product, not a
 * neutral default.
 *
 * The field is a free-text decimal input, so this has to cope with
 * whitespace, unparseable text, and non-positive numbers as well as an
 * empty string.
 */
export function isUsableWeight(value: string | null | undefined): boolean {
  if (value == null) return false;
  const trimmed = value.trim();
  if (trimmed === "") return false;
  const n = Number(trimmed);
  return Number.isFinite(n) && n > 0;
}
