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

  it('fatal 503 at retry cap → preparing-timeout', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, MAX_PREPARING_RETRIES)).toBe('preparing-timeout')
  })

  // The probe failed: polling cannot help, and the player says why.
  it('fatal 422 (Keyframe Index failed) → index-failed, never retried', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 422 } }, 0)).toBe('index-failed')
    expect(classifyHlsError({ fatal: true, response: { code: 422 } }, 5)).toBe('index-failed')
  })

  it('fatal 409 (Media Source disabled) → fatal immediately, never retried as preparing', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 409 } }, 0)).toBe('fatal')
  })

  it('fatal 401 (Stream Token expired) → refresh-token', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 401 } }, 0)).toBe('refresh-token')
  })

  it('fatal non-503 → fatal', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 404 } }, 0)).toBe('fatal')
    expect(classifyHlsError({ fatal: true }, 0)).toBe('fatal')
  })
})
