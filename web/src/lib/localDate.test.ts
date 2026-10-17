import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { localDate, shiftLocalDate } from './localDate'

// The bug only shows east of UTC: before 08:00 in Taipei, the UTC date is
// still yesterday.
beforeAll(() => { vi.stubEnv('TZ', 'Asia/Taipei') })
afterAll(() => { vi.unstubAllEnvs() })

describe('localDate', () => {
  it('is the local calendar date, not the UTC one', () => {
    expect(localDate(new Date(2026, 9, 11, 7, 30))).toBe('2026-10-11')
  })

  it('pads month and day', () => {
    expect(localDate(new Date(2026, 0, 5, 12))).toBe('2026-01-05')
  })
})

describe('shiftLocalDate', () => {
  it.each([
    ['2026-10-11', 1, '2026-10-12'],
    ['2026-10-11', -1, '2026-10-10'],
    ['2026-12-31', 1, '2027-01-01'],
    ['2026-03-01', -1, '2026-02-28'],
  ])('moves %s by %i day(s) to %s', (from, days, want) => {
    expect(shiftLocalDate(from, days)).toBe(want)
  })
})
