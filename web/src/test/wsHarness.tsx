import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { act } from '@testing-library/react'
import { WebSocketContext } from '../contexts/WebSocketContext'
import type { WSMessage } from '../hooks/useWebSocket'

export interface WSHarness {
  /** Wrap the tree under test (renderHook `wrapper`, or renderWithRouter's `ws`). */
  Provider: (props: { children: ReactNode }) => ReactNode
  /**
   * Deliver one server message as the context's lastMessage. Assert what it
   * starts asynchronously with findBy / waitFor.
   */
  send(message: WSMessage): void
}

// wsHarness is a WebSocket context a test drives by hand. Messages go through
// an event bus the provider subscribes to, so nothing is reassigned in render.
// Connection state and outgoing messages are fixed; extend it when a test needs them.
export function wsHarness(): WSHarness {
  const bus = new EventTarget()
  let listeners = 0
  function Provider({ children }: { children: ReactNode }) {
    const [lastMessage, setLastMessage] = useState<WSMessage | null>(null)
    useEffect(() => {
      const onMessage = (e: Event) => setLastMessage((e as CustomEvent<WSMessage>).detail)
      bus.addEventListener('message', onMessage)
      listeners += 1
      return () => {
        bus.removeEventListener('message', onMessage)
        listeners -= 1
      }
    }, [])
    const value = useMemo(() => ({ lastMessage, isConnected: true, sendMessage: () => {} }), [lastMessage])
    return <WebSocketContext.Provider value={value}>{children}</WebSocketContext.Provider>
  }
  return {
    Provider,
    send(message) {
      if (listeners === 0) throw new Error('wsHarness.send: no Provider is mounted')
      // A fresh object per frame, as useWebSocket's JSON.parse gives.
      const frame = { ...message }
      // Synchronous act on purpose: an async act here left tests that send
      // several messages through useJob hanging until the 5s timeout.
      act(() => { bus.dispatchEvent(new CustomEvent('message', { detail: frame })) })
    },
  }
}
