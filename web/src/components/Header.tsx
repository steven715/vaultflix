import { useState, useCallback } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

interface HeaderProps {
  searchQuery: string
  onSearch: (query: string) => void
}

export default function Header({ searchQuery, onSearch }: HeaderProps) {
  const { user, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()

  const handleSearch = useCallback(
    (value: string) => {
      onSearch(value)
    },
    [onSearch],
  )

  return (
    <header className="bg-gray-900 border-b border-gray-800 px-4 py-3 flex items-center gap-4">
      <Link to="/" className="text-xl font-bold text-white shrink-0 hover:text-indigo-400 transition-colors">
        Vaultflix
      </Link>

      <div className="flex-1 max-w-lg">
        <input
          type="text"
          placeholder="搜尋影片..."
          value={searchQuery}
          onChange={(e) => handleSearch(e.target.value)}
          className="w-full bg-gray-800 text-white text-sm rounded px-3 py-1.5 outline-none focus:ring-2 focus:ring-indigo-500 placeholder-gray-500"
        />
      </div>

      {/* Navigation links */}
      <nav className="flex items-center gap-3 shrink-0">
        <Link
          to="/favorites"
          className={`flex items-center gap-1 text-sm transition-colors ${
            location.pathname === '/favorites' ? 'text-white' : 'text-gray-400 hover:text-white'
          }`}
          title="收藏"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 8.25c0-2.485-2.099-4.5-4.688-4.5-1.935 0-3.597 1.126-4.312 2.733-.715-1.607-2.377-2.733-4.313-2.733C5.1 3.75 3 5.765 3 8.25c0 7.22 9 12 9 12s9-4.78 9-12z" />
          </svg>
          <span className="hidden sm:inline">收藏</span>
        </Link>
        <Link
          to="/history"
          className={`flex items-center gap-1 text-sm transition-colors ${
            location.pathname === '/history' ? 'text-white' : 'text-gray-400 hover:text-white'
          }`}
          title="觀看記錄"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span className="hidden sm:inline">記錄</span>
        </Link>
      </nav>

      <div className="relative shrink-0">
        <button
          onClick={() => setMenuOpen(!menuOpen)}
          className="flex items-center gap-2 text-sm text-gray-400 hover:text-white transition-colors"
        >
          <span>{user?.username}</span>
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {menuOpen && (
          <>
            <div className="fixed inset-0" onClick={() => setMenuOpen(false)} />
            <div className="absolute right-0 mt-2 w-36 bg-gray-800 rounded-lg shadow-lg py-1 z-10">
              <div className="px-3 py-1.5 text-xs text-gray-500 border-b border-gray-700">
                {user?.role}
              </div>
              <button
                onClick={() => { logout(); setMenuOpen(false) }}
                className="w-full text-left px-3 py-1.5 text-sm text-gray-400 hover:bg-gray-700 hover:text-white"
              >
                登出
              </button>
            </div>
          </>
        )}
      </div>
    </header>
  )
}
