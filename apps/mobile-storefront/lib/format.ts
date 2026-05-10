/**
 * Formats a backend money string ("19.99") + currency code ("USD") into
 * a localized price string. Falls back to a plain "{symbol}{amount}"
 * when Intl can't resolve the currency.
 */
export function formatMoney(amount: string | number, currency = "USD"): string {
  const value = typeof amount === "number" ? amount : Number(amount);
  if (!Number.isFinite(value)) return "";
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(value);
  } catch {
    return `${currency} ${value.toFixed(2)}`;
  }
}
