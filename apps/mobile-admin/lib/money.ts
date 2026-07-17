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
