import { useEffect, useState, type ReactNode } from 'react'
import { act } from '@testing-library/react'
import { WebSocketContext } from '../contexts/WebSocketContext'
import type { WSMessage } from '../hooks/useWebSocket'

export interface WSHarness {
  /** Wrap the tree under test (renderHook `wrapper`, or renderWithRouter's `ws`). */
  Provider: (props: { children: ReactNode }) => ReactNode
  /** Deliver one server message as the context's lastMessage. */
  send(message: WSMessage): void
}

// wsHarness is a WebSocket context a test drives by hand. Messages go through
// an event bus the provider subscribes to, so nothing is reassigned in render.
export function wsHarness(): WSHarness {
  const bus = new EventTarget()
  function Provider({ children }: { children: ReactNode }) {
    const [lastMessage, setLastMessage] = useState<WSMessage | null>(null)
    useEffect(() => {
      const onMessage = (e: Event) => setLastMessage((e as CustomEvent<WSMessage>).detail)
      bus.addEventListener('message', onMessage)
      return () => bus.removeEventListener('message', onMessage)
    }, [])
    return (
      <WebSocketContext.Provider value={{ lastMessage, isConnected: true, sendMessage: () => {} }}>
        {children}
      </WebSocketContext.Provider>
    )
  }
  return {
    Provider,
    send: (message) => act(() => { bus.dispatchEvent(new CustomEvent('message', { detail: message })) }),
  }
}
