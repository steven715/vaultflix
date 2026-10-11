import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { render, type RenderResult } from '@testing-library/react'
import { wsHarness, type WSHarness } from './wsHarness'

// 包 MemoryRouter 與 WebSocket context，讓頁面元件可被測試。要推送伺服器訊息時
// 自己建一個 wsHarness 傳進 ws；沒傳就用一個不會收到訊息的。
export function renderWithRouter(
  ui: ReactElement,
  opts: { route?: string; ws?: WSHarness } = {},
): RenderResult {
  const { route = '/admin/library', ws = wsHarness() } = opts
  // As a wrapper, so rerender keeps the router.
  return render(ui, {
    wrapper: ({ children }) => (
      <MemoryRouter initialEntries={[route]}>
        <ws.Provider>{children}</ws.Provider>
      </MemoryRouter>
    ),
  })
}
