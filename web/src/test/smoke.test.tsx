import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithRouter } from './renderWithRouter'

describe('test infra', () => {
  it('renders into jsdom with router + jest-dom matchers', () => {
    renderWithRouter(<h1>hello admin</h1>)
    expect(screen.getByRole('heading', { name: 'hello admin' })).toBeInTheDocument()
  })
})
