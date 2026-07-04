import { describe, it, expect } from 'vitest'
import { niceMax, barWidths } from './scale'

describe('niceMax', () => {
  it('returns 1 for all-zero data (avoids divide-by-zero)', () => {
    expect(niceMax(0)).toBe(1)
  })
  it('rounds up to a readable ceiling', () => {
    expect(niceMax(7)).toBeGreaterThanOrEqual(7)
    expect(niceMax(42)).toBeGreaterThanOrEqual(42)
  })
})

describe('barWidths', () => {
  it('scales the largest value to full width', () => {
    const w = barWidths([2, 4, 8], 8, 100)
    expect(w[2]).toBe(100)
    expect(w[0]).toBe(25)
  })
  it('handles an all-zero series without NaN', () => {
    const w = barWidths([0, 0], 0, 100)
    expect(w.every((x) => Number.isFinite(x))).toBe(true)
  })
})
