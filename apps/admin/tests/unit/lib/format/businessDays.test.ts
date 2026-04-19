/**
 * businessDaysElapsed unit tests — Vitest.
 *
 * Tests:
 *   1. Same day → 0
 *   2. One weekday later → 1
 *   3. Friday to Monday → 1 (weekend skipped)
 *   4. Monday to Wednesday (across nothing) → 2
 *   5. Thursday to Monday (across weekend) → 2
 *   6. `to` before `from` → 0 (clamp)
 *   7. One full week (Mon → Mon) → 5
 */

import { describe, it, expect } from 'vitest'
import { businessDaysElapsed } from '@/lib/format/businessDays'

// Helper: build a UTC date from parts
function utc(year: number, month: number, day: number): Date {
  return new Date(Date.UTC(year, month - 1, day))
}

describe('businessDaysElapsed', () => {
  it('returns 0 when from and to are the same day', () => {
    const day = utc(2026, 4, 14) // Tuesday
    expect(businessDaysElapsed(day, day)).toBe(0)
  })

  it('returns 1 for one weekday forward (Monday to Tuesday)', () => {
    const from = utc(2026, 4, 13) // Monday
    const to = utc(2026, 4, 14) // Tuesday
    expect(businessDaysElapsed(from, to)).toBe(1)
  })

  it('returns 1 for Friday to Monday (weekend skipped)', () => {
    const from = utc(2026, 4, 10) // Friday
    const to = utc(2026, 4, 13) // Monday
    expect(businessDaysElapsed(from, to)).toBe(1)
  })

  it('returns 2 for Monday to Wednesday', () => {
    const from = utc(2026, 4, 13) // Monday
    const to = utc(2026, 4, 15) // Wednesday
    expect(businessDaysElapsed(from, to)).toBe(2)
  })

  it('returns 2 for Thursday to Monday (across a full weekend)', () => {
    const from = utc(2026, 4, 9)  // Thursday
    const to = utc(2026, 4, 13)   // Monday  (Fri=1, Mon=2; Sat/Sun skipped)
    expect(businessDaysElapsed(from, to)).toBe(2)
  })

  it('returns 0 when to is before from', () => {
    const from = utc(2026, 4, 15) // Wednesday
    const to = utc(2026, 4, 13)   // Monday (earlier)
    expect(businessDaysElapsed(from, to)).toBe(0)
  })

  it('returns 5 for a full Monday-to-Monday week', () => {
    const from = utc(2026, 4, 13) // Monday
    const to = utc(2026, 4, 20)   // next Monday
    expect(businessDaysElapsed(from, to)).toBe(5)
  })
})
