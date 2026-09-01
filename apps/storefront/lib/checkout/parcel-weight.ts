import type { ShippingOption } from "@/lib/api/checkout-api";

/**
 * Weight assumed for a cart item whose product carries none, when the
 * store has not configured its own.
 *
 * Kept at 500 because that is exactly what checkout hardcoded before
 * this was configurable — so nothing a shopper is quoted changes until a
 * merchant deliberately sets a different value.
 */
export const DEFAULT_PARCEL_WEIGHT_GRAMS = 500;

/**
 * fallbackParcelWeight returns the weight to assume for items with no
 * weight of their own.
 *
 * Checkout used to inline `500`, so an invisible constant in frontend
 * code set real carrier prices: a store selling 200g tanks was quoting as
 * though every one weighed half a kilo, and had no way to correct it.
 *
 * Per-product weights remain the accurate answer. This is only the
 * fallback when one is missing.
 */
export function fallbackParcelWeight(options: ShippingOption[]): number {
  for (const o of options) {
    const w = o.default_parcel_weight_grams;
    // Guard the wire value: a 0, a negative, or a non-finite number from
    // an older or misbehaving API must not become a weight we quote on.
    if (typeof w === "number" && Number.isFinite(w) && w > 0) {
      return w;
    }
  }
  return DEFAULT_PARCEL_WEIGHT_GRAMS;
}
