import { afterEach, describe, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { value, configurable: true })
}

describe('copyText', () => {
  afterEach(() => {
    setClipboard(undefined)
    vi.restoreAllMocks()
  })

  it('uses the Clipboard API when the page has it', async () => {
    const writeText = vi.fn(async () => {})
    setClipboard({ writeText })

    expect(await copyText('a.mkv')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('a.mkv')
  })

  // Over plain http on the LAN (not a secure context) there is no Clipboard API.
  it('falls back to a hidden textarea where there is no Clipboard API', async () => {
    setClipboard(undefined)
    let copied = ''
    document.execCommand = vi.fn(() => {
      copied = (document.activeElement as HTMLTextAreaElement | null)?.value ?? ''
      return true
    })

    expect(await copyText('D/影片 1.mkv')).toBe(true)
    expect(copied).toBe('D/影片 1.mkv')
    expect(document.querySelector('textarea')).toBeNull()
  })

  it('says it failed when nothing could copy', async () => {
    setClipboard({ writeText: vi.fn(async () => { throw new Error('denied') }) })
    document.execCommand = vi.fn(() => false)

    expect(await copyText('a.mkv')).toBe(false)
  })
})
