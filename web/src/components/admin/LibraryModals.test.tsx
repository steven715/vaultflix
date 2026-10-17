import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { addVideoTag } from '../../api/admin'
import type { TagWithCount } from '../../types'
import { BatchTagDialog } from './LibraryModals'

vi.mock('../../api/admin', () => ({ addVideoTag: vi.fn() }))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => toast }))

const tags = [
  { id: 7, name: '動作', category: 'genre', video_count: 1 },
  { id: 9, name: '喜劇', category: 'genre', video_count: 0 },
] as TagWithCount[]

describe('BatchTagDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('tags every selected Video with the chosen Tag, then is done', async () => {
    vi.mocked(addVideoTag).mockResolvedValue(undefined)
    const onDone = vi.fn()
    render(<BatchTagDialog videoIds={['v1', 'v2']} tags={tags} onDone={onDone} onClose={vi.fn()} />)

    fireEvent.change(screen.getByRole('combobox'), { target: { value: '9' } })
    fireEvent.click(screen.getByText('套用'))

    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(vi.mocked(addVideoTag).mock.calls).toEqual([['v1', 9], ['v2', 9]])
    expect(toast.success).toHaveBeenCalledWith('批次標籤完成')
  })

  it('says how many Videos it could not tag', async () => {
    vi.mocked(addVideoTag).mockImplementation(async (id: string) => { if (id === 'v1') throw new Error('500') })
    const onDone = vi.fn()
    render(<BatchTagDialog videoIds={['v1', 'v2']} tags={tags} onDone={onDone} onClose={vi.fn()} />)

    fireEvent.click(screen.getByText('套用'))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('2 部中有 1 部加標籤失敗'))
    expect(vi.mocked(addVideoTag).mock.calls[0]).toEqual(['v1', 7])
    expect(onDone).toHaveBeenCalled()
  })
  // While the batch runs, a second 套用 or a close would start a second batch
  // or hide the one still running.
  it('runs the batch once and stays open while it runs', async () => {
    let finish!: () => void
    vi.mocked(addVideoTag).mockImplementationOnce(() => new Promise<void>((r) => { finish = r })).mockResolvedValue(undefined)
    const onClose = vi.fn()
    render(<BatchTagDialog videoIds={['v1', 'v2']} tags={tags} onDone={vi.fn()} onClose={onClose} />)

    fireEvent.click(screen.getByText('套用'))
    fireEvent.click(screen.getByText('套用中...'))
    fireEvent.click(screen.getByText('取消'))
    expect(onClose).not.toHaveBeenCalled()

    finish()
    await waitFor(() => expect(onClose).toHaveBeenCalled())
    expect(addVideoTag).toHaveBeenCalledTimes(2)
  })
})
