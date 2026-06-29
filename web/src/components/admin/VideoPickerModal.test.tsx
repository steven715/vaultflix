import { describe, it, expect, vi } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import VideoPickerModal from './VideoPickerModal'

vi.mock('../../api/videos', () => ({
  listVideos: vi.fn(() =>
    Promise.resolve({
      data: [
        {
          id: 'v1',
          title: '片X',
          duration_seconds: 60,
          thumbnail_url: undefined,
          tags: [],
          description: '',
          minio_object_key: '',
          thumbnail_key: '',
          preview_key: '',
          resolution: '',
          file_size_bytes: 0,
          mime_type: '',
          original_filename: '',
          created_at: '',
          updated_at: '',
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    }),
  ),
}))

describe('VideoPickerModal', () => {
  it('renders video title and uses no gray/indigo classes', async () => {
    const { container } = render(
      <VideoPickerModal onSelect={() => {}} onClose={() => {}} />,
    )

    await waitFor(() => {
      const found = Array.from(container.querySelectorAll('*')).some(
        (el) => el.textContent === '片X',
      )
      expect(found).toBe(true)
    })

    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
  })
})
