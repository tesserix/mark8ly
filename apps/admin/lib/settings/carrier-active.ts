/**
 * `is_active` gates the rate query outright — marketplace-api filters
 * `WHERE store_id = ? AND provider = ? AND is_active = true`
 * (shipping/repository.go:303). An inactive carrier quotes nothing, and
 * the storefront can only report that no delivery options came back.
 *
 * The column defaults to FALSE, and the form mirrored that for brand-new
 * configs. So a merchant could enter an API key and a full warehouse
 * address, save, and end up with a carrier that silently never quotes —
 * the same symptom as a phoneless warehouse, with no hint of the cause.
 *
 * Nobody enters carrier credentials intending to leave the carrier off,
 * so a NEW config defaults on. An existing config always keeps whatever
 * the merchant chose, including a deliberate off.
 */
export function defaultCarrierActive(saved: boolean | undefined): boolean {
  return saved ?? true;
}
