import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { listMediaSources } from '../../api/admin'
import type { LaneJob } from '../../hooks/useMediaLane'
import type { JobView } from '../../lib/jobProgress'
import ImportDialog from './ImportDialog'

vi.mock('../../api/admin', () => ({ listMediaSources: vi.fn() }))

const source = (id: string, label: string, enabled = true) => ({
  id, label, mount_path: `/mnt/host/${label}`, enabled, video_count: 0, created_at: '', updated_at: '',
})

function laneJob(view: JobView | null = null): LaneJob<string> {
  return { view, busy: false, start: vi.fn(async () => 'started' as const), cancel: vi.fn(async () => {}), dismiss: vi.fn() }
}

describe('ImportDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listMediaSources).mockResolvedValue([source('s1', 'D槽'), source('s2', 'E槽', false), source('s3', 'F槽')] as never)
  })

  it('offers only enabled Media Sources and imports the chosen one', async () => {
    const job = laneJob()
    render(<ImportDialog job={job} blockedReason={null} open onClose={vi.fn()} />)

    const select = await screen.findByRole('combobox')
    expect(screen.queryByText(/E槽/)).toBeNull()
    fireEvent.change(select, { target: { value: 's3' } })
    fireEvent.click(screen.getByText('開始匯入'))

    expect(job.start).toHaveBeenCalledWith('s3')
  })

  // A failure to list the Media Sources must not read as "there are none".
  it('says the Media Sources failed to load, and retries', async () => {
    vi.mocked(listMediaSources).mockRejectedValueOnce(new Error('down'))
    render(<ImportDialog job={laneJob()} blockedReason={null} open onClose={vi.fn()} />)

    expect(await screen.findByRole('alert')).toHaveTextContent('無法載入媒體來源')
    expect(screen.queryByText('沒有可用的媒體來源')).toBeNull()
    fireEvent.click(screen.getByText('重試'))
    expect(await screen.findByRole('combobox')).toBeInTheDocument()
  })

  it('keeps 開始匯入 shut, and says why, while the Media Lane is held', async () => {
    render(<ImportDialog job={laneJob()} blockedReason="補齊執行中，完成或取消後再匯入" open onClose={vi.fn()} />)

    await screen.findByRole('combobox')
    expect(screen.getByText('補齊執行中，完成或取消後再匯入')).toBeInTheDocument()
    expect(screen.getByText('開始匯入')).toBeDisabled()
  })

  // An Import started elsewhere shows its progress even though nobody opened the dialog.
  it('shows a running Import Job without being opened, and nothing when there is neither', () => {
    const running = { id: 'j1', subject: 's1', phase: 'running', counts: { processed: 1, total: 3, succeeded: 1, skipped: 0, failed: 0 }, errors: [] } as unknown as JobView
    const { rerender, container } = render(<ImportDialog job={laneJob(running)} blockedReason={null} open={false} onClose={vi.fn()} />)
    expect(screen.getByText('匯入影片')).toBeInTheDocument()

    rerender(<ImportDialog job={laneJob()} blockedReason={null} open={false} onClose={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('forgets the shown Job when closed', async () => {
    const job = laneJob()
    const onClose = vi.fn()
    render(<ImportDialog job={job} blockedReason={null} open onClose={onClose} />)
    await waitFor(() => expect(listMediaSources).toHaveBeenCalled())

    fireEvent.click(screen.getByText('取消'))

    expect(job.dismiss).toHaveBeenCalled()
    expect(onClose).toHaveBeenCalled()
  })
})
