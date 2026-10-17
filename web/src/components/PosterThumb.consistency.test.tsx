import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import PosterThumb from './PosterThumb'
import AdminPosterThumb from './admin/PosterThumb'

// A Video without a Thumbnail is recognised by its colour: the admin's
// library and the viewer's pages must paint it the same.
describe('poster fallback colour', () => {
  it.each(['v1', '3f2a9c1e-0b7d-4e55-9a10-6c2f8d4e7b21', 'DASD-626'])('is the same for the viewer and the admin (%s)', (id) => {
    const viewer = render(<PosterThumb id={id} title="t" />).container.querySelector<HTMLElement>('[style*="gradient"]')
    const admin = render(<AdminPosterThumb id={id} />).container.querySelector<HTMLElement>('[data-poster-fallback]')
    expect(viewer?.style.background).toContain('linear-gradient')
    expect(admin?.style.background).toBe(viewer?.style.background)
  })
})
