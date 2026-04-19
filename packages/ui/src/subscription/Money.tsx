/**
 * Money — formats a minor-unit integer (cents/paise) into a localised currency
 * string using Intl.NumberFormat.
 *
 * @example
 *   <Money amount={149800} currency="CAD" />  // → CA$1,498.00
 *   <Money amount={118800} currency="USD" showCents={false} />  // → $1,188
 */
export interface MoneyProps {
  /** Amount in minor units (cents, paise, etc.). Divided by 100 internally. */
  amount: number
  /** ISO 4217 currency code, e.g. "USD", "CAD", "INR", "EUR". */
  currency: string
  /**
   * BCP 47 locale string. Defaults to `"en-US"` so that CAD renders as `CA$`
   * rather than the browser-locale ambiguous `$`.
   */
  locale?: string
  /** When false, fractional digits are omitted. Defaults to true. */
  showCents?: boolean
  className?: string
}

export function Money({
  amount,
  currency,
  locale = 'en-US',
  showCents = true,
  className,
}: MoneyProps): React.JSX.Element {
  const digits = showCents ? 2 : 0

  const formatter = new Intl.NumberFormat(locale, {
    style: 'currency',
    currency,
    currencyDisplay: 'symbol',
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })

  return <span className={className}>{formatter.format(amount / 100)}</span>
}

export default Money
