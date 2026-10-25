import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import { cancelImportJob, createMediaSource, deleteMediaSource, getActiveBackfill, importVideos, listMediaSources, updateMediaSource } from '../../api/admin'
import { wsHarness } from '../../test/wsHarness'
import MediaSourcePage from './MediaSourcePage'

vi.mock('../../api/admin', () => ({
  listMediaSources: vi.fn(() => Promise.resolve([
    { id: 's1', label: 'D槽', mount_path: '/mnt/host/D', enabled: true, video_count: 3, created_at: '', updated_at: '' },
  ])),
  getActiveImportJob: vi.fn(() => Promise.resolve(null)),
  getActiveBackfill: vi.fn(() => Promise.resolve(null)),
  createMediaSource: vi.fn(), updateMediaSource: vi.fn(), deleteMediaSource: vi.fn(), importVideos: vi.fn(),
  cancelImportJob: vi.fn(() => Promise.resolve()),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('MediaSourcePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders sources and uses design-token background (no embedded admin header)', async () => {
    const { container } = renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    await waitFor(() => expect(screen.getByText('D槽')).toBeInTheDocument())
    expect(screen.getByText('/mnt/host/D')).toBeInTheDocument()
    // 不再自帶搜尋框（那是 AdminTopbar 的職責）
    expect(screen.queryByPlaceholderText('搜尋影片...')).toBeNull()
    // 頁面根容器用 token，不再是 bg-gray-950
    expect(container.querySelector('.bg-gray-950')).toBeNull()
  })

  const runningJob = {
    id: 'j1', source_id: 's1', status: 'running', total: 2, processed: 0,
    imported: 0, skipped: 0, failed: 0, errors: [], started_at: '',
  }

  // Start an Import on the D槽 card and finish it over WebSocket.
  async function importAndFinish() {
    vi.mocked(importVideos).mockResolvedValue(runningJob as never)
    const ws = wsHarness()
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources', ws })
    fireEvent.click(await screen.findByText('匯入'))
    await screen.findByText('匯入中')
    expect(screen.queryByText('關閉')).toBeNull()
    // The refresh after the job ends hangs: the summary must not wait for it.
    vi.mocked(listMediaSources).mockImplementationOnce(() => new Promise(() => {}))
    ws.send({ type: 'import_complete', payload: { ...runningJob, status: 'completed', processed: 2, imported: 2 } })
  }

  it('keeps the Import summary on the source card after the job ends, until closed', async () => {
    await importAndFinish()
    expect(await screen.findByText('匯入完成')).toBeInTheDocument()
    expect(screen.queryByText('載入中...')).toBeNull()
    // The refresh starts in an effect after the summary renders.
    await waitFor(() => expect(listMediaSources).toHaveBeenCalledTimes(2))
    fireEvent.click(screen.getByText('關閉'))
    expect(screen.queryByText('匯入完成')).toBeNull()
  })

  it('drops the summary when its source is deleted', async () => {
    await importAndFinish()
    await screen.findByText('匯入完成')
    vi.mocked(deleteMediaSource).mockResolvedValue(undefined as never)
    fireEvent.click(screen.getByText('刪除'))
    fireEvent.click(await screen.findByRole('button', { name: '確認' }))
    await waitFor(() => expect(deleteMediaSource).toHaveBeenCalledWith('s1'))
    await waitFor(() => expect(screen.queryByText('匯入完成')).toBeNull())
  })

  it('cancels a running Import from its panel', async () => {
    vi.mocked(importVideos).mockResolvedValue(runningJob as never)
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    fireEvent.click(await screen.findByText('匯入'))
    await screen.findByText('匯入中')

    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    await waitFor(() => expect(cancelImportJob).toHaveBeenCalledWith('j1'))
  })

  it('keeps 匯入 shut, and says why, while a Backfill holds the Media Lane', async () => {
    vi.mocked(getActiveBackfill).mockResolvedValue({
      id: 'b1', kind: 'keyframe', status: 'running', total: 4, processed: 1, succeeded: 1, failed: 0, skipped: 0, errors: [], started_at: '',
    })
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })

    await waitFor(() => expect(screen.getByRole('button', { name: '匯入' })).toBeDisabled())
    expect(screen.getByText('補齊執行中，完成或取消後再匯入')).toBeInTheDocument()
  })

  it('does not repeat its own running Import above the list', async () => {
    vi.mocked(importVideos).mockResolvedValue(runningJob as never)
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    fireEvent.click(await screen.findByText('匯入'))
    await screen.findByText('匯入中')
    expect(screen.queryByText('匯入執行中')).toBeNull()
  })
  // Refreshing after an edit keeps the cards up instead of flashing 載入中.
  it('keeps the list shown while it refreshes after a toggle', async () => {
    vi.mocked(updateMediaSource).mockResolvedValue(undefined as never)
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    fireEvent.click(await screen.findByText('停用'))
    vi.mocked(listMediaSources).mockImplementationOnce(() => new Promise(() => {}))
    fireEvent.click(screen.getByText('確認'))

    await waitFor(() => expect(listMediaSources).toHaveBeenCalledTimes(2))
    expect(screen.getByText('D槽')).toBeInTheDocument()
    expect(screen.queryByText('載入中...')).toBeNull()
  })
  // A refused mount path says why in the form, in Chinese, not the server's English.
  it.each([
    ['path_not_allowed', 'path is not within allowed mount prefix', '路徑必須在允許的掛載目錄之下'],
    ['path_not_exist', 'path does not exist on filesystem', '路徑不存在，請確認磁碟已掛載'],
  ])('says why a new source\'s path is refused (%s)', async (code, serverMessage, shown) => {
    vi.mocked(createMediaSource).mockRejectedValueOnce(Object.assign(new Error('Request failed with status code 400'), {
      response: { status: 400, data: { error: code, message: serverMessage } },
    }))
    renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    fireEvent.click(await screen.findByText('+ 新增來源'))
    const [label] = screen.getAllByRole('textbox')
    fireEvent.change(label, { target: { value: 'E槽' } })
    fireEvent.click(screen.getByText('建立', { selector: 'button' }))

    expect(await screen.findByText(shown)).toBeInTheDocument()
  })
})
