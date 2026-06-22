import { createContext, useContext } from 'react'

export interface JWTPayload {
  user_id: string
  username: string
  role: string
  exp: number
}

export interface AuthContextValue {
  user: JWTPayload | null
  token: string | null
  isAuthenticated: boolean
  isAdmin: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
