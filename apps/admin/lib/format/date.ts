/**
 * Date formatting utilities.
 *
 * Uses Intl.DateTimeFormat for locale-aware formatting without pulling in
 * a heavy date library dependency. All billing period dates are rendered
 * with the en-GB locale (day-first, spelled-out month) per the plan spec.
 */

const BILLING_DATE_FORMAT: Intl.DateTimeFormatOptions = {
  day: 'numeric',
  month: 'long',
  year: 'numeric',
}

/**
 * Format an ISO date string (or Date) into the billing display format:
 * "18 April 2027"
 *
 * Returns an empty string for null/undefined input so callers can safely
 * inline without conditional guards.
 */
export function formatBillingDate(
  input: string | Date | null | undefined,
): string {
  if (!input) return ''
  const date = typeof input === 'string' ? new Date(input) : input
  if (isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat('en-GB', BILLING_DATE_FORMAT).format(date)
}
