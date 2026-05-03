import { useSyncExternalStore } from 'react'

// useMatchMedia returns whether the given media query currently matches and
// re-renders when the match state changes (e.g. when DevTools toggles a
// device emulator at runtime).
//
// Returns false during SSR or when window.matchMedia is unavailable so that
// downstream code can safely short-circuit hover-only behaviour on touch
// devices and headless contexts.
export function useMatchMedia(query: string): boolean {
  return useSyncExternalStore(
    (notify) => {
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return () => {}
      }
      const mql = window.matchMedia(query)
      mql.addEventListener('change', notify)
      return () => mql.removeEventListener('change', notify)
    },
    () => {
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return false
      }
      return window.matchMedia(query).matches
    },
    () => false,
  )
}
