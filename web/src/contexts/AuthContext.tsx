import { createContext, useContext, useState, useCallback, useEffect } from 'react'
import type { ReactNode } from 'react'
import { login as apiLogin } from '../api/auth'

interface JWTPayload {
  user_id: string
  username: string
  role: string
  exp: number
}

interface AuthContextValue {
  user: JWTPayload | null
  token: string | null
  isAuthenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

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

  useEffect(() => {
    if (token && !user) {
      localStorage.removeItem('token')
      setToken(null)
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
    <AuthContext.Provider value={{ user, token, isAuthenticated: !!user, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
