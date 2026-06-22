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
  // Holds the latest `connect` so `scheduleReconnect` can call it without
  // depending on it — breaking the connect ⇄ scheduleReconnect cycle.
  const connectRef = useRef<() => void>(() => {})

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
      connectRef.current()
    }, delay)
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
      // onclose fires asynchronously, so by the time it runs this socket may no
      // longer be the live one. Branch on socket identity:
      //   - wsRef.current === ws    → this live socket dropped → reset + reconnect
      //   - wsRef.current === null  → intentional teardown, no successor
      //                               (logout / unmount) → reset, do not reconnect
      //   - wsRef.current === other → replaced by a newer generation
      //                               (token change) → the successor owns all
      //                               shared state, so do nothing at all
      // The third case is the bug a cross-generation boolean could not express:
      // a stale socket must not reset isConnected, clear the new socket's
      // heartbeat via cleanup(), or schedule a spurious reconnect.
      if (wsRef.current !== null && wsRef.current !== ws) return

      setIsConnected(false)
      cleanup()

      if (wsRef.current === ws) {
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
  }, [token, cleanup, scheduleReconnect])

  // Keep connectRef pointing at the latest connect for scheduleReconnect.
  useEffect(() => {
    connectRef.current = connect
  }, [connect])

  const sendMessage = useCallback((msg: WSMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
    }
  }, [])

  // Close any existing connection intentionally (e.g. on logout). We null
  // wsRef.current first; the socket's async onclose then sees the "intentional
  // teardown, no successor" case (wsRef.current === null) and resets
  // isConnected without reconnecting. We must not setState here directly —
  // disconnect() is called synchronously from the effect, which react-hooks
  // forbids (set-state-in-effect).
  const disconnect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }
    cleanup()
  }, [cleanup])

  // Connect when token becomes available; disconnect on logout
  useEffect(() => {
    if (!token) {
      disconnect()
      return
    }

    // Token available — connect
    reconnectAttemptRef.current = 0
    connect()

    return () => {
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      cleanup()
    }
  }, [token, connect, cleanup, disconnect])

  return { lastMessage, isConnected, sendMessage }
}
