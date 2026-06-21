import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { LogoMark, SearchIcon, ChevronDown } from './icons'

const NAV = [
  { to: '/', label: '首頁' },
  { to: '/favorites', label: '收藏' },
  { to: '/history', label: '記錄' },
]

// Desktop sticky header (hidden on mobile, where the bottom tab bar takes over).
// Owns the global search box: typing navigates to the browse view filtered by q.
export default function Header() {
  const { user, isAdmin, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const urlQuery = searchParams.get('q') ?? ''
  const [value, setValue] = useState(urlQuery)
  const [menuOpen, setMenuOpen] = useState(false)

  // Keep the input in sync when the URL query changes elsewhere (e.g. back nav).
  useEffect(() => {
    setValue(urlQuery)
  }, [urlQuery])

  // `/` focuses the search box from anywhere (unless already typing).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== '/') return
      const el = document.activeElement
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return
      e.preventDefault()
      inputRef.current?.focus()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  function handleSearch(next: string) {
    setValue(next)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const params = new URLSearchParams()
      if (next) params.set('q', next)
      navigate(`/?${params.toString()}`)
    }, 300)
  }

  return (
    <header
      className="sticky top-0 z-30 hidden h-[72px] items-center gap-6 border-b border-border px-6 backdrop-blur-[18px] md:flex lg:px-10"
      style={{ backgroundColor: 'rgba(13,11,10,0.82)' }}
    >
      <Link to="/" className="flex shrink-0 items-center gap-2 text-cream transition-colors hover:text-accent">
        <LogoMark className="h-7 w-7 text-accent" />
        <span className="font-display text-xl font-extrabold tracking-tight">Vaultflix</span>
      </Link>

      <div className="relative mx-auto w-full max-w-[540px]">
        <SearchIcon className="pointer-events-none absolute left-3.5 top-1/2 h-4.5 w-4.5 -translate-y-1/2 text-faint" />
        <input
          ref={inputRef}
          type="text"
          value={value}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="搜尋影片、標籤…"
          className="h-11 w-full rounded-pill border border-border bg-surface pl-11 pr-12 text-sm text-cream outline-none transition-colors placeholder:text-faint focus:border-accent"
        />
        <kbd className="absolute right-3 top-1/2 hidden -translate-y-1/2 rounded-md border border-border bg-surface-up px-1.5 py-0.5 font-mono text-xs text-faint lg:block">
          /
        </kbd>
      </div>

      <nav className="flex shrink-0 items-center gap-5">
        {NAV.map((item) => {
          const active = location.pathname === item.to
          return (
            <Link
              key={item.to}
              to={item.to}
              className={`text-sm font-medium transition-colors ${
                active ? 'text-accent' : 'text-muted hover:text-cream'
              }`}
            >
              {item.label}
            </Link>
          )
        })}

        <div className="h-6 w-px bg-border" />

        <div className="relative">
          <button
            onClick={() => setMenuOpen((o) => !o)}
            className="flex items-center gap-2 rounded-pill border border-border bg-surface py-1 pl-1 pr-2.5 transition-colors hover:bg-surface-up"
          >
            <span className="h-7 w-7 rounded-full bg-gradient-to-br from-accent to-fav" />
            <span className="text-sm font-medium text-cream">{user?.username}</span>
            <ChevronDown className="h-4 w-4 text-faint" />
          </button>
          {menuOpen && (
            <>
              <div className="fixed inset-0 z-10" onClick={() => setMenuOpen(false)} />
              <div className="absolute right-0 z-20 mt-2 w-48 overflow-hidden rounded-card border border-border bg-surface-up py-1 shadow-[var(--shadow-float)]">
                <div className="border-b border-border px-3.5 py-2">
                  <div className="text-sm font-medium text-cream">{user?.username}</div>
                  <div className="font-mono text-xs text-faint">{user?.role}</div>
                </div>
                {isAdmin && (
                  <Link
                    to="/admin"
                    onClick={() => setMenuOpen(false)}
                    className="block px-3.5 py-2 text-sm text-muted transition-colors hover:bg-surface hover:text-cream"
                  >
                    管理後台
                  </Link>
                )}
                <button
                  onClick={() => {
                    logout()
                    setMenuOpen(false)
                  }}
                  className="block w-full px-3.5 py-2 text-left text-sm text-muted transition-colors hover:bg-surface hover:text-cream"
                >
                  登出
                </button>
              </div>
            </>
          )}
        </div>
      </nav>
    </header>
  )
}
