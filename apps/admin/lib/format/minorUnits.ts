/**
 * Minor-unit money formatting for billing surfaces.
 *
 * Extracted from the invoices list so the promo confirmation quotes a price
 * the same way the invoice for it will (#770). Distinct from
 * `formatMoney` in lib/format.ts, which takes a MAJOR-unit amount — passing
 * minor units to that prints 1520 as "$1,520.00".
 */

/**
 * Currencies Stripe treats as zero-decimal: the amount is already in base
 * units, so 1520 JPY is ¥1,520 and not ¥15.20.
 */
const ZERO_DECIMAL = new Set([
  'bif', 'clp', 'djf', 'gnf', 'jpy', 'kmf', 'krw', 'mga', 'pyg',
  'rwf', 'ugx', 'vnd', 'vuv', 'xaf', 'xof', 'xpf',
])

/**
 * Formats a minor-unit amount in the given ISO 4217 currency.
 *
 * Falls back to "CODE 12.34" when Intl rejects the currency code, so an
 * unexpected code degrades to a readable number rather than throwing.
 */
export function formatMinorUnits(minor: number, currency: string): string {
  const code = currency.toUpperCase()
  const lower = currency.toLowerCase()
  const zeroDecimal = ZERO_DECIMAL.has(lower)
  const major = zeroDecimal ? minor : minor / 100
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: code,
      minimumFractionDigits: zeroDecimal ? 0 : 2,
    }).format(major)
  } catch {
    return `${code} ${major.toFixed(zeroDecimal ? 0 : 2)}`
  }
}
