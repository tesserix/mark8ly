/**
 * Formats money in the currency the store actually uses. Every amount in this
 * app used to render as USD via a hardcoded Intl option, while the store is
 * AUD and the wire reports the real `currency_code`. Pass the store's code so
 * a non-USD merchant sees the right symbol, not "$".
 */
export function formatMoney(amount: number, currencyCode?: string): string {
  if (!currencyCode) {
    return new Intl.NumberFormat("en-AU", { minimumFractionDigits: 2 }).format(amount);
  }
  return new Intl.NumberFormat("en-AU", {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 2,
  }).format(amount);
}

/**
 * Whole-dollar money for DISPLAY columns the eye scans rather than reconciles
 * — the Dashboard's hero numeral, its today/this-week line, and the serif
 * amount column down the right of the queue rows. Cents add four glyphs of
 * noise to a figure that exists to be glanced at ("$189", not "$189.00").
 *
 * `formatMoney` above keeps 2dp and stays the default: use it anywhere the
 * exact amount matters (order totals, refunds, line items, anything a
 * merchant might read back to a customer).
 */
export function formatWholeMoney(amount: number, currencyCode?: string): string {
  if (!currencyCode) {
    return new Intl.NumberFormat("en-AU", { maximumFractionDigits: 0 }).format(amount);
  }
  return new Intl.NumberFormat("en-AU", {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(amount);
}
