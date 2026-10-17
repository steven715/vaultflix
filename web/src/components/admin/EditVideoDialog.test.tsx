import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { updateVideo } from '../../api/admin'
import type { VideoWithTags } from '../../types'
import EditVideoDialog from './EditVideoDialog'

vi.mock('../../api/admin', () => ({ updateVideo: vi.fn() }))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => toast }))

const video = { id: 'v1', title: '片A', description: '舊說明' } as VideoWithTags

describe('EditVideoDialog', () => {
  beforeEach(() => vi.clearAllMocks())

  it('saves the edited title and description, and hands back the saved Video', async () => {
    vi.mocked(updateVideo).mockImplementation(async (id, input) => ({ ...video, id, ...input }) as never)
    const onSaved = vi.fn()
    render(<EditVideoDialog video={video} onSaved={onSaved} onClose={vi.fn()} />)

    fireEvent.change(screen.getByDisplayValue('片A'), { target: { value: '片A（導演版）' } })
    fireEvent.change(screen.getByDisplayValue('舊說明'), { target: { value: '新說明' } })
    fireEvent.click(screen.getByText('儲存'))

    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(expect.objectContaining({ title: '片A（導演版）', description: '新說明' })))
    expect(updateVideo).toHaveBeenCalledWith('v1', { title: '片A（導演版）', description: '新說明' })
    expect(toast.success).toHaveBeenCalledWith('已儲存')
  })

  it('stays open and says so when saving fails', async () => {
    vi.mocked(updateVideo).mockRejectedValueOnce(new Error('500'))
    const onSaved = vi.fn()
    render(<EditVideoDialog video={video} onSaved={onSaved} onClose={vi.fn()} />)

    fireEvent.click(screen.getByText('儲存'))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('儲存失敗，請重試'))
    expect(onSaved).not.toHaveBeenCalled()
    expect(screen.getByDisplayValue('片A')).toBeInTheDocument()
  })
})
