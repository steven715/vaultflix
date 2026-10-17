import { describe, expect, it } from 'vitest'
import { runOnEach } from './batch'

describe('runOnEach', () => {
  it('runs every id past a failure and returns the ones that failed', async () => {
    const ran: string[] = []
    const failed = await runOnEach(['a', 'b', 'c'], async (id) => {
      ran.push(id)
      if (id === 'b') throw new Error('500')
    })
    expect(ran).toEqual(['a', 'b', 'c'])
    expect(failed).toEqual(['b'])
  })

  it('returns no failures when every op succeeds', async () => {
    expect(await runOnEach(['a'], async () => {})).toEqual([])
  })
})
