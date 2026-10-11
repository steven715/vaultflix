import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { render, type RenderResult } from '@testing-library/react'

// 包 MemoryRouter，讓需要 router context 的元件可被測試。
export function renderWithRouter(
  ui: ReactElement,
  opts: { route?: string } = {},
): RenderResult {
  const { route = '/admin/library' } = opts
  // As a wrapper, so rerender keeps the router.
  return render(ui, {
    wrapper: ({ children }) => <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>,
  })
}
