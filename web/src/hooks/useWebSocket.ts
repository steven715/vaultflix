import { useState, useEffect, useRef, useCallback } from 'react'

export interface WSMessage {
  type: string
  payload: unknown
}

export interface UseWebSocketReturn {
  lastMessage: WSMessage | null
  isConnected: boolean
  sendMessage: (msg: WSMessage) => void
}

const MAX_RECONNECT_ATTEMPTS = 20
const BASE_RECONNECT_DELAY = 1000
const MAX_RECONNECT_DELAY = 30000
const HEARTBEAT_INTERVAL = 50000 // slightly under server pongWait (54s)

export function useWebSocket(token: string | null): UseWebSocketReturn {
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null)
  const [isConnected, setIsConnected] = useState(false)

  const wsRef = useRef<WebSocket | null>(null)
  const reconnectAttemptRef = useRef(0)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const heartbeatTimerRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined)
  const intentionalCloseRef = useRef(false)

  const cleanup = useCallback(() => {
    if (heartbeatTimerRef.current) {
      clearInterval(heartbeatTimerRef.current)
      heartbeatTimerRef.current = undefined
    }
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current)
      reconnectTimerRef.current = undefined
    }
  }, [])

  const connect = useCallback(() => {
    if (!token) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws?token=${token}`)

    ws.onopen = () => {
      reconnectAttemptRef.current = 0
      setIsConnected(true)

      // Start heartbeat
      heartbeatTimerRef.current = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping', payload: {} }))
        }
      }, HEARTBEAT_INTERVAL)
    }

    ws.onclose = () => {
      setIsConnected(false)
      cleanup()

      if (!intentionalCloseRef.current) {
        scheduleReconnect()
      }
    }

    ws.onerror = () => {
      // onclose will fire after onerror; reconnect is handled there.
    }

    ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)
        setLastMessage(msg)
      } catch {
        // Ignore non-JSON messages
      }
    }

    wsRef.current = ws
  }, [token, cleanup])

  const scheduleReconnect = useCallback(() => {
    if (reconnectAttemptRef.current >= MAX_RECONNECT_ATTEMPTS) {
      return
    }
    const delay = Math.min(
      BASE_RECONNECT_DELAY * Math.pow(2, reconnectAttemptRef.current),
      MAX_RECONNECT_DELAY
    )
    reconnectAttemptRef.current += 1
    reconnectTimerRef.current = setTimeout(() => {
      connect()
    }, delay)
  }, [connect])

  const sendMessage = useCallback((msg: WSMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
    }
  }, [])

  // Connect when token becomes available; disconnect on logout
  useEffect(() => {
    if (!token) {
      // No token — close any existing connection intentionally
      intentionalCloseRef.current = true
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      cleanup()
      setIsConnected(false)
      return
    }

    // Token available — connect
    intentionalCloseRef.current = false
    reconnectAttemptRef.current = 0
    connect()

    return () => {
      intentionalCloseRef.current = true
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      cleanup()
    }
  }, [token, connect, cleanup])

  return { lastMessage, isConnected, sendMessage }
}
