import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import PosterThumb from './PosterThumb'

describe('PosterThumb', () => {
  it('renders an img when src is provided', () => {
    render(<PosterThumb id="v1" src="http://x/thumb.jpg" alt="poster" />)
    const img = screen.getByRole('img', { name: 'poster' })
    expect(img).toHaveAttribute('src', 'http://x/thumb.jpg')
  })
  it('renders a gradient fallback when src is missing', () => {
    const { container } = render(<PosterThumb id="v1" />)
    expect(screen.queryByRole('img')).toBeNull()
    const fallback = container.querySelector('[data-poster-fallback]') as HTMLElement
    expect(fallback).toBeTruthy()
    expect(fallback.style.background).toContain('linear-gradient')
  })
})
