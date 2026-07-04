import { describe, it, expect } from 'vitest'
import { clampDelta } from './heartbeat'

describe('clampDelta', () => {
  it('returns positive elapsed play time', () => {
    expect(clampDelta(10, 22)).toBe(12)
  })
  it('caps a forward seek to the max (22)', () => {
    expect(clampDelta(10, 500)).toBe(22)
  })
  it('returns 0 for a backward seek (negative delta)', () => {
    expect(clampDelta(100, 40)).toBe(0)
  })
  it('returns 0 when position is unchanged', () => {
    expect(clampDelta(30, 30)).toBe(0)
  })
})
