import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect } from 'vitest'
import RecommendationList from './RecommendationList'
import type { RecommendationItem } from '../types'

function makeItem(n: number): RecommendationItem {
  return {
    id: `r${n}`,
    video_id: `v${n}`,
    title: `Rec ${n}`,
    thumbnail_url: '',
    duration_seconds: 60,
    resolution: '1920x1080',
    file_size_bytes: 1,
    sort_order: n,
    is_fallback: false,
  }
}

function renderList(items: RecommendationItem[]) {
  return render(
    <MemoryRouter>
      <RecommendationList items={items} />
    </MemoryRouter>,
  )
}

describe('RecommendationList', () => {
  it('renders nothing when items is empty', () => {
    const { container } = renderList([])
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a row per item linking to /videos/:video_id', () => {
    renderList([makeItem(1), makeItem(2)])
    expect(screen.getByText('Rec 1')).toBeInTheDocument()
    expect(screen.getByText('Rec 2')).toBeInTheDocument()
    const link = screen.getByText('Rec 1').closest('a')
    expect(link).toHaveAttribute('href', '/videos/v1')
  })
})
