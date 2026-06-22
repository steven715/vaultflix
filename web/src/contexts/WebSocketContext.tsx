import { createContext, useContext } from 'react'
import type { UseWebSocketReturn } from '../hooks/useWebSocket'

export const WebSocketContext = createContext<UseWebSocketReturn | null>(null)

export function useWS(): UseWebSocketReturn {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error('useWS must be used within WebSocketProvider')
  return ctx
}
