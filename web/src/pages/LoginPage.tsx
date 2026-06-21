import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, Navigate } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import type { AxiosError } from 'axios'
import type { ErrorResponse } from '../types'
import { LogoMark, UserIcon, LockIcon } from '../components/icons'

export default function LoginPage() {
  const { login, isAuthenticated } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (isAuthenticated) {
    return <Navigate to="/" replace />
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!username.trim() || !password.trim()) {
      setError('請輸入帳號和密碼')
      return
    }

    setLoading(true)
    try {
      await login(username, password)
      navigate('/', { replace: true })
    } catch (err) {
      const axiosErr = err as AxiosError<ErrorResponse>
      if (axiosErr.response) {
        setError(axiosErr.response.data?.message || '帳號或密碼錯誤')
      } else if (axiosErr.request) {
        setError('伺服器連線失敗，請稍後再試')
      } else {
        setError('登入失敗，請稍後再試')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen bg-bg">
      {/* Left cinematic panel (desktop only) */}
      <div
        className="relative hidden flex-[1.15] overflow-hidden md:block"
        style={{ background: 'linear-gradient(150deg, #FF8A3D, #7A1F4B)' }}
      >
        {/* Soft glow + darkening */}
        <div className="absolute -left-24 top-1/4 h-96 w-96 rounded-full bg-white/20 blur-3xl" />
        <div className="absolute inset-0 bg-black/35" />
        <div className="relative flex h-full flex-col justify-between p-12">
          <div className="flex items-center gap-2 text-cream">
            <LogoMark className="h-8 w-8" />
            <span className="font-display text-2xl font-extrabold tracking-tight">Vaultflix</span>
          </div>
          <div>
            <h2 className="max-w-md font-display text-[58px] font-extrabold leading-[1.02] tracking-tight text-cream">
              你的私人放映室
            </h2>
            <p className="mt-4 max-w-sm text-lg text-cream/80">
              收藏、續看、隨選即播。專屬於你的影音收藏，隨時隨地。
            </p>
          </div>
          <p className="font-mono text-xs text-cream/60">© Vaultflix · Private Screening Room</p>
        </div>
      </div>

      {/* Right form panel */}
      <div className="flex flex-1 items-center justify-center px-6 py-12">
        <div className="w-full max-w-[380px]">
          {/* Mobile brand */}
          <div className="mb-8 flex items-center gap-2 text-cream md:hidden">
            <LogoMark className="h-7 w-7 text-accent" />
            <span className="font-display text-xl font-extrabold tracking-tight">Vaultflix</span>
          </div>

          <h1 className="font-display text-3xl font-extrabold tracking-tight text-cream">歡迎回來</h1>
          <p className="mt-2 text-sm text-muted">登入以繼續你的觀影體驗</p>

          <form onSubmit={handleSubmit} className="mt-8 space-y-4">
            {error && (
              <div className="rounded-btn border border-fav/30 bg-fav/10 px-3.5 py-2.5 text-sm text-fav">
                {error}
              </div>
            )}

            <div>
              <label htmlFor="username" className="mb-1.5 block text-sm font-medium text-muted">
                使用者名稱
              </label>
              <div className="relative">
                <UserIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4.5 w-4.5 -translate-y-1/2 text-faint" />
                <input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="h-12 w-full rounded-btn border border-border bg-surface pl-11 pr-3.5 text-cream outline-none transition-colors placeholder:text-faint focus:border-accent"
                  placeholder="輸入帳號"
                  autoFocus
                />
              </div>
            </div>

            <div>
              <label htmlFor="password" className="mb-1.5 block text-sm font-medium text-muted">
                密碼
              </label>
              <div className="relative">
                <LockIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4.5 w-4.5 -translate-y-1/2 text-faint" />
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="h-12 w-full rounded-btn border border-border bg-surface pl-11 pr-3.5 text-cream outline-none transition-colors placeholder:text-faint focus:border-accent"
                  placeholder="輸入密碼"
                />
              </div>
            </div>

            <div className="flex items-center justify-between text-sm">
              <label className="flex items-center gap-2 text-muted">
                <input type="checkbox" defaultChecked className="accent-[var(--color-accent)]" />
                保持登入
              </label>
              <button type="button" className="text-faint transition-colors hover:text-muted">
                忘記密碼？
              </button>
            </div>

            <button
              type="submit"
              disabled={loading}
              className="h-12 w-full rounded-btn bg-accent font-bold text-accent-ink transition-transform hover:-translate-y-0.5 disabled:translate-y-0 disabled:opacity-50"
            >
              {loading ? '登入中…' : '登入'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
