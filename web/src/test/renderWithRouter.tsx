import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { render, type RenderResult } from '@testing-library/react'
import type { WSHarness } from './wsHarness'

// 包 MemoryRouter，讓需要 router context 的元件可被測試；給 ws 時再包一層
// 可由測試推送訊息的 WebSocket context（見 wsHarness）。
export function renderWithRouter(
  ui: ReactElement,
  opts: { route?: string; ws?: WSHarness } = {},
): RenderResult {
  const { route = '/admin/library', ws } = opts
  // As a wrapper, so rerender keeps the router.
  return render(ui, {
    wrapper: ({ children }) => (
      <MemoryRouter initialEntries={[route]}>
        {ws ? <ws.Provider>{children}</ws.Provider> : children}
      </MemoryRouter>
    ),
  })
}
