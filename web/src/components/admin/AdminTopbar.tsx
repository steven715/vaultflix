import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../../contexts/AuthContext'
import { adminPageTitle } from '../../lib/adminNav'

export default function AdminTopbar() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)
  const [search, setSearch] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // debounce 後導向影片庫並帶 q；空字串回乾淨影片庫
  useEffect(() => () => clearTimeout(debounceRef.current), [])
  function handleSearch(value: string) {
    setSearch(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      navigate(value ? `/admin/library?q=${encodeURIComponent(value)}` : '/admin/library')
    }, 300)
  }

  const initial = (user?.username?.[0] ?? '?').toUpperCase()

  return (
    <header className="sticky top-0 h-16 flex items-center gap-4 px-7 border-b border-border z-20"
      style={{ background: 'rgba(13,11,10,0.86)', backdropFilter: 'blur(16px)' }}>
      <div className="flex items-baseline gap-2 shrink-0">
        <span className="text-muted text-sm">管理後台 /</span>
        <span className="font-display font-bold text-lg tracking-tight text-cream">{adminPageTitle(pathname)}</span>
      </div>

      <div className="flex-1 flex justify-center">
        <input
          value={search}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="搜尋影片、檔名、標籤..."
          className="w-full max-w-[420px] bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none border border-transparent focus:border-accent placeholder-faint"
        />
      </div>

      <div className="flex items-center gap-3 shrink-0">
        <Link
          to="/"
          title="回前台"
          className="flex items-center gap-1.5 text-xs text-muted hover:text-cream bg-surface-2 rounded-pill px-2.5 py-1 transition-colors"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 15L3 9m0 0l6-6M3 9h12a6 6 0 010 12h-3" />
          </svg>
          <span>回前台</span>
        </Link>
        <span className="hidden md:flex items-center gap-1.5 text-xs font-mono text-muted bg-surface-2 rounded-pill px-2.5 py-1">
          <span className="w-1.5 h-1.5 rounded-full bg-live" /> API 正常 · 8080
        </span>
        <div className="relative">
          <button onClick={() => setMenuOpen((o) => !o)} className="flex items-center gap-2">
            <span className="w-7 h-7 rounded-full flex items-center justify-center text-accent-ink text-sm font-bold"
              style={{ background: 'linear-gradient(150deg, #FFB23F, #FF8A3D)' }}>{initial}</span>
            <span className="text-sm text-cream">{user?.username}</span>
            <span className="text-xs text-accent">admin</span>
          </button>
          {menuOpen && (
            <>
              <div className="fixed inset-0" onClick={() => setMenuOpen(false)} />
              <div className="absolute right-0 mt-2 w-36 bg-surface-2 rounded-card shadow-float py-1 z-10 border border-border">
                <Link to="/" onClick={() => setMenuOpen(false)}
                  className="block px-3 py-1.5 text-sm text-muted hover:bg-surface hover:text-cream">回前台</Link>
                <button onClick={() => { logout(); setMenuOpen(false) }}
                  className="w-full text-left px-3 py-1.5 text-sm text-muted hover:bg-surface hover:text-cream">登出</button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
