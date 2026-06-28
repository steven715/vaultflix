import { describe, it, expect } from 'vitest'
import { posterGradient } from './posterGradient'

describe('posterGradient', () => {
  it('is deterministic for the same id', () => {
    expect(posterGradient('abc')).toBe(posterGradient('abc'))
  })
  it('always returns a linear-gradient string', () => {
    for (const id of ['', 'x', 'a-very-long-uuid-1234', '中文']) {
      expect(posterGradient(id)).toMatch(/^linear-gradient\(135deg,/)
    }
  })
  it('spreads different ids across the palette (not all identical)', () => {
    const ids = Array.from({ length: 40 }, (_, i) => `id-${i}`)
    const distinct = new Set(ids.map(posterGradient))
    expect(distinct.size).toBeGreaterThan(3)
  })
})
