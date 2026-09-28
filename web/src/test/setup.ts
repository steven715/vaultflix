// 擴充 vitest 的 expect 帶 jest-dom matchers（含 TS 型別）
import '@testing-library/jest-dom/vitest'

// Node 25+ 內建 Web Storage global：沒給 --localstorage-file 時，globalThis.localStorage
// 的 getter 回傳 undefined，且因為 key 已存在，vitest 不會用 jsdom 的版本覆蓋 → 元件裡
// localStorage.getItem 直接炸。把 jsdom 的 Storage 接回 global；Node ≤24 是 no-op。
const jsdomWindow = (globalThis as { jsdom?: { window: Window } }).jsdom?.window
for (const key of ['localStorage', 'sessionStorage'] as const) {
  if (globalThis[key] === undefined && jsdomWindow) {
    Object.defineProperty(globalThis, key, {
      value: jsdomWindow[key],
      configurable: true,
      writable: true,
    })
  }
}
