import { render, screen, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import PlayerPage from './PlayerPage'
import * as videosApi from '../api/videos'
import * as recommendationsApi from '../api/recommendations'

vi.mock('../api/videos')
vi.mock('../api/recommendations')

// Mock contexts used by AppShell/PlayerPage
vi.mock('../contexts/ToastContext', () => ({
  useToast: () => ({ info: vi.fn(), success: vi.fn(), error: vi.fn() }),
}))

vi.mock('../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, logout: vi.fn() }),
}))

const base = {
  id: 'v1',
  title: 'T',
  description: '',
  tags: [],
  resolution: '1920x1080',
  duration_seconds: 10,
  file_size_bytes: 1,
  created_at: '2026-06-30T00:00:00Z',
  updated_at: '2026-06-30T00:00:00Z',
  stream_url: '/api/videos/v1/stream',
  is_favorited: false,
  watch_progress: 0,
  minio_object_key: '',
  thumbnail_key: '',
  preview_key: '',
  mime_type: 'video/mp4',
  original_filename: 'T.mp4',
  thumbnail_url: '',
}

function renderPlayer() {
  return render(
    <MemoryRouter initialEntries={['/watch/v1']}>
      <Routes>
        <Route path="/watch/:id" element={<PlayerPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('PlayerPage play_mode', () => {
  beforeEach(() => {
    vi.mocked(videosApi.getStreamToken).mockResolvedValue({ token: 'tok', expires_in: 60 })
    vi.mocked(videosApi.listVideos).mockResolvedValue({
      data: [],
      total: 0,
      page: 1,
      page_size: 12,
    } as never)
    vi.mocked(recommendationsApi.getTodayRecommendations).mockResolvedValue([
      { id: 'r1', video_id: 'rv1', title: 'Rec One', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 1, is_fallback: false },
      { id: 'r2', video_id: 'rv2', title: 'Rec Two', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 2, is_fallback: false },
      { id: 'r3', video_id: 'rv3', title: 'Rec Three', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 3, is_fallback: false },
      { id: 'r4', video_id: 'rv4', title: 'Rec Four', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 4, is_fallback: false },
      { id: 'r5', video_id: 'rv5', title: 'Rec Five', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 5, is_fallback: false },
    ] as never)
  })

  it('shows a notice for transcode videos instead of a player', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({
      ...base,
      play_mode: 'transcode',
    } as never)
    renderPlayer()
    expect(await screen.findByText(/尚未支援|Phase 2|無法播放/)).toBeInTheDocument()
  })

  it('does not count a forward seek as watch time (heartbeat delta excludes the jump)', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({
      ...base,
      play_mode: 'direct',
      duration_seconds: 6000,
    } as never)
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 200 }))

    const { container, unmount } = renderPlayer()
    await screen.findByText('T')
    const el = container.querySelector('video') as HTMLVideoElement
    expect(el).toBeTruthy()

    // Stub currentTime so we can drive playback/seek positions.
    let ct = 0
    Object.defineProperty(el, 'currentTime', {
      get: () => ct,
      set: (v: number) => {
        ct = v
      },
      configurable: true,
    })

    // Play 0 → 10s (10 real seconds of watch time), then scrub forward to 5000s.
    fireEvent.loadedMetadata(el)
    ct = 10
    fireEvent.timeUpdate(el)
    ct = 5000
    fireEvent.seeking(el)
    fireEvent.timeUpdate(el)

    // Unmount flushes the accumulated heartbeat delta via the beacon path.
    unmount()

    const hb = fetchSpy.mock.calls.find(([url]) =>
      String(url).includes('/watch-sessions/heartbeat'),
    )
    expect(hb).toBeTruthy()
    const body = JSON.parse((hb![1] as RequestInit).body as string)
    // Only the 10 real seconds count; the 4990s forward jump contributes 0.
    // Without the onSeeking resync this would be 10 + min(4990, 22) = 32.
    expect(body.played_delta).toBe(10)

    fetchSpy.mockRestore()
  })

  it('returns to the library (not the previous player page) when clicking 返回片庫', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({
      ...base,
      play_mode: 'direct',
    } as never)

    // Simulate history: library → video v0 → video v1 (current).
    render(
      <MemoryRouter initialEntries={['/', '/watch/v0', '/watch/v1']} initialIndex={2}>
        <Routes>
          <Route path="/" element={<div>片庫首頁</div>} />
          <Route path="/watch/:id" element={<PlayerPage />} />
        </Routes>
      </MemoryRouter>,
    )

    const backButton = await screen.findByRole('button', { name: /返回片庫/ })
    await userEvent.click(backButton)

    // Must land on the library, not back on the previous player page (v0).
    expect(await screen.findByText('片庫首頁')).toBeInTheDocument()
  })

  it('shows at most 5 up-next videos, excluding the current one', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)
    // 7 items: item 0 is v1 (current, will be excluded), items 1–6 are u1..u6 (candidates).
    // After filtering v1, 6 candidates remain; slice(0, 5) must drop the 6th.
    const many = Array.from({ length: 7 }, (_, i) => ({
      id: i === 0 ? 'v1' : `u${i}`, // v1 is the current video → must be excluded
      title: `Up ${i}`,
      tags: [],
      resolution: '1920x1080',
      duration_seconds: 10,
      thumbnail_url: '',
    }))
    vi.mocked(videosApi.listVideos).mockResolvedValue({
      data: many,
      total: 7,
      page: 1,
      page_size: 6,
    } as never)

    renderPlayer()

    // Current video v1 excluded; the remaining 5 (u1..u5) all render.
    // The 6th candidate (u6) must NOT render, proving the 5-cap.
    expect(await screen.findByText('Up 5')).toBeInTheDocument()
    expect(screen.getByText('Up 1')).toBeInTheDocument()
    expect(screen.queryByText('Up 0')).not.toBeInTheDocument() // v1 excluded
    expect(screen.queryByText('Up 6')).not.toBeInTheDocument() // 6th candidate trimmed by slice(0, 5)

    // Up-next must request a RANDOM sample (not "newest") so the list differs on
    // each video, and page_size 6 so 5 remain after excluding the current video.
    expect(videosApi.listVideos).toHaveBeenCalledWith(
      expect.objectContaining({ page_size: 6, sort_by: 'random' }),
    )
  })

  it('renders the 今日推薦 block above 接著看 with recommendation items', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)
    renderPlayer()
    const recHeading = await screen.findByText('今日推薦')
    expect(recHeading).toBeInTheDocument()
    expect(await screen.findByText('Rec One')).toBeInTheDocument()
    expect(screen.getByText('Rec Five')).toBeInTheDocument()

    // 今日推薦 must come before 接著看 in document order.
    const upNextHeading = screen.getByText('接著看')
    expect(recHeading.compareDocumentPosition(upNextHeading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('refetches recommendations on navigation to another video (new location.key)', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)

    // A tiny harness that lets the test navigate to a new :id at runtime.
    function Nav() {
      const navigate = useNavigate()
      return <button onClick={() => navigate('/watch/v2')}>go v2</button>
    }
    render(
      <MemoryRouter initialEntries={['/watch/v1']}>
        <Nav />
        <Routes>
          <Route path="/watch/:id" element={<PlayerPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await screen.findByText('今日推薦')
    const before = vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length
    await userEvent.click(screen.getByText('go v2'))
    await screen.findByText('今日推薦')
    expect(vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length).toBeGreaterThan(before)
  })

  it('refetches recommendations when re-navigating to the SAME video id (location.key changes, id does not)', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)

    // A tiny harness that navigates to the SAME :id at runtime, which still pushes
    // a new history entry (new location.key) even though useParams().id stays 'v1'.
    function Nav() {
      const navigate = useNavigate()
      return <button onClick={() => navigate('/watch/v1')}>go v1 again</button>
    }
    render(
      <MemoryRouter initialEntries={['/watch/v1']}>
        <Nav />
        <Routes>
          <Route path="/watch/:id" element={<PlayerPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await screen.findByText('今日推薦')
    const before = vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length
    await userEvent.click(screen.getByText('go v1 again'))
    await screen.findByText('今日推薦')
    expect(vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length).toBeGreaterThan(before)
  })
})
