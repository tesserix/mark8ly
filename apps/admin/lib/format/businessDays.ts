/**
 * businessDays — count elapsed business days between two dates.
 *
 * Rules:
 *   - Counts complete business days elapsed (not inclusive of `from` itself).
 *   - Saturday (6) and Sunday (0) are skipped.
 *   - Public holidays are NOT accounted for (MVP).
 *   - If `to` is before `from`, returns 0 (clamp, never negative).
 *
 * @param from - Start date (e.g. appeal_submitted_at).
 * @param to   - End date. Defaults to now.
 * @returns    Number of business days elapsed (integer, >= 0).
 */
export function businessDaysElapsed(from: Date, to: Date = new Date()): number {
  const start = new Date(from)
  const end = new Date(to)

  // Normalise both to midnight UTC so time-of-day doesn't skew counts.
  start.setUTCHours(0, 0, 0, 0)
  end.setUTCHours(0, 0, 0, 0)

  if (end <= start) return 0

  let count = 0
  const cursor = new Date(start)

  while (cursor < end) {
    cursor.setUTCDate(cursor.getUTCDate() + 1)
    const day = cursor.getUTCDay()
    // 0 = Sunday, 6 = Saturday
    if (day !== 0 && day !== 6) {
      count++
    }
  }

  return count
}
