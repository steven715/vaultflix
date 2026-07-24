import { describe, expect, it } from 'vitest'
import { classifyHlsError, MAX_PREPARING_RETRIES } from './hlsError'

describe('classifyHlsError', () => {
  it('non-fatal errors are ignored', () => {
    expect(classifyHlsError({ fatal: false, response: { code: 503 } }, 0)).toBe('ignore')
  })

  it('fatal 503 under retry cap → retry-preparing', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, 0)).toBe('retry-preparing')
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, MAX_PREPARING_RETRIES - 1)).toBe('retry-preparing')
  })

  it('fatal 503 at retry cap → fatal', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, MAX_PREPARING_RETRIES)).toBe('fatal')
  })

  it('fatal non-503 → fatal', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 404 } }, 0)).toBe('fatal')
    expect(classifyHlsError({ fatal: true }, 0)).toBe('fatal')
  })
})
