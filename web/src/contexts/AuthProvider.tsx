import { useState, useCallback, useEffect } from 'react'
import type { ReactNode } from 'react'
import { login as apiLogin } from '../api/auth'
import { AuthContext } from './AuthContext'
import type { JWTPayload } from './AuthContext'

function decodeToken(token: string): JWTPayload | null {
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    if (payload.exp * 1000 < Date.now()) {
      return null
    }
    return payload as JWTPayload
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => {
    const stored = localStorage.getItem('token')
    if (!stored) return null
    const payload = decodeToken(stored)
    if (!payload) {
      localStorage.removeItem('token')
      return null
    }
    return stored
  })

  const user = token ? decodeToken(token) : null
  // Once a token no longer decodes to a user (expired/invalid), treat it as
  // logged-out everywhere downstream — including the WebSocket, which reads
  // this value. Deriving it here means we don't need an effect to setState.
  const activeToken = user ? token : null

  // Drop the now-stale token from storage. No setState here, so it can't
  // trigger cascading renders — the derived state above already reflects logout.
  useEffect(() => {
    if (token && !user) {
      localStorage.removeItem('token')
    }
  }, [token, user])

  const login = useCallback(async (username: string, password: string) => {
    const res = await apiLogin(username, password)
    localStorage.setItem('token', res.token)
    setToken(res.token)
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem('token')
    setToken(null)
  }, [])

  return (
    <AuthContext.Provider value={{ user, token: activeToken, isAuthenticated: !!user, isAdmin: user?.role === 'admin', login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
