import type { ReactNode } from 'react'
import { useAuth } from './AuthContext'
import { useWebSocket } from '../hooks/useWebSocket'
import { WebSocketContext } from './WebSocketContext'

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth()
  const ws = useWebSocket(token)

  return (
    <WebSocketContext.Provider value={ws}>
      {children}
    </WebSocketContext.Provider>
  )
}
