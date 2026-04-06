import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { useWebSocket } from '../hooks/useWebSocket'
import type { UseWebSocketReturn } from '../hooks/useWebSocket'

const WebSocketContext = createContext<UseWebSocketReturn | null>(null)

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const ws = useWebSocket(token)

  return (
    <WebSocketContext.Provider value={ws}>
      {children}
    </WebSocketContext.Provider>
  )
}

export function useWS(): UseWebSocketReturn {
  const ctx = useContext(WebSocketContext)
  if (!ctx) throw new Error('useWS must be used within WebSocketProvider')
  return ctx
}
