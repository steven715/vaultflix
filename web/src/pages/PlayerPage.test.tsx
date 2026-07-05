import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import PlayerPage from './PlayerPage'
import * as videosApi from '../api/videos'

vi.mock('../api/videos')

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
  })

  it('shows a notice for transcode videos instead of a player', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({
      ...base,
      play_mode: 'transcode',
    } as never)
    renderPlayer()
    expect(await screen.findByText(/尚未支援|Phase 2|無法播放/)).toBeInTheDocument()
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
    const many = Array.from({ length: 6 }, (_, i) => ({
      id: i === 0 ? 'v1' : `u${i}`, // v1 is the current video → must be excluded
      title: `Up ${i}`,
      tags: [],
      resolution: '1920x1080',
      duration_seconds: 10,
      thumbnail_url: '',
    }))
    vi.mocked(videosApi.listVideos).mockResolvedValue({
      data: many,
      total: 6,
      page: 1,
      page_size: 6,
    } as never)

    renderPlayer()

    // Current video v1 excluded; the remaining 5 (u1..u5) all render.
    expect(await screen.findByText('Up 5')).toBeInTheDocument()
    expect(screen.getByText('Up 1')).toBeInTheDocument()
    expect(screen.queryByText('Up 0')).not.toBeInTheDocument() // v1 excluded
  })
})
