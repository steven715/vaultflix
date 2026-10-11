import { describe, expect, it, vi } from 'vitest'

// vitest.config sets mockReset: an implementation one test swaps in must not
// leak into the next (vi.clearAllMocks alone keeps it).
const fetchList = vi.fn(() => 'original')

describe('mock reset between tests', () => {
  it('lets a test swap the implementation', () => {
    fetchList.mockImplementation(() => 'swapped')
    expect(fetchList()).toBe('swapped')
  })

  it('starts the next test from the original implementation', () => {
    expect(fetchList()).toBe('original')
  })
})
