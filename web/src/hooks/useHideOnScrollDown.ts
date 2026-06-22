import { useEffect, useRef, useState } from 'react'

// useHideOnScrollDown reports whether a fixed bottom bar should currently be
// hidden, mirroring Threads/Instagram: hide while the user scrolls down, reveal
// the moment they scroll up, and always stay visible near the top of the page.
//
// Listens on window scroll (the app uses native document scroll, not a custom
// scroll container) with a passive listener throttled via requestAnimationFrame.
// `threshold` is the minimum delta in px before flipping state, to avoid jitter.
export function useHideOnScrollDown(threshold = 8): boolean {
  const [hidden, setHidden] = useState(false)
  const lastY = useRef(0)

  useEffect(() => {
    lastY.current = window.scrollY
    let ticking = false

    const update = () => {
      ticking = false
      const y = window.scrollY
      const delta = y - lastY.current
      if (Math.abs(delta) < threshold) return
      // Always reveal near the very top; otherwise hide on down, show on up.
      setHidden(delta > 0 && y > 24)
      lastY.current = y
    }

    const onScroll = () => {
      if (ticking) return
      ticking = true
      window.requestAnimationFrame(update)
    }

    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [threshold])

  return hidden
}
