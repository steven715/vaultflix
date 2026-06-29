import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { render, type RenderResult } from '@testing-library/react'

// 包 MemoryRouter，讓需要 router context 的元件可被測試。
export function renderWithRouter(
  ui: ReactElement,
  opts: { route?: string } = {},
): RenderResult {
  const { route = '/admin/library' } = opts
  return render(<MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>)
}
