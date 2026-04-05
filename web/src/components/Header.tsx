import { useState, useCallback } from 'react'
import { useAuth } from '../contexts/AuthContext'

interface HeaderProps {
  searchQuery: string
  onSearch: (query: string) => void
}

export default function Header({ searchQuery, onSearch }: HeaderProps) {
  const { user, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)

  const handleSearch = useCallback(
    (value: string) => {
      onSearch(value)
    },
    [onSearch],
  )

  return (
    <header className="bg-gray-900 border-b border-gray-800 px-4 py-3 flex items-center gap-4">
      <h1 className="text-xl font-bold text-white shrink-0">Vaultflix</h1>

      <div className="flex-1 max-w-lg">
        <input
          type="text"
          placeholder="搜尋影片..."
          value={searchQuery}
          onChange={(e) => handleSearch(e.target.value)}
          className="w-full bg-gray-800 text-white text-sm rounded px-3 py-1.5 outline-none focus:ring-2 focus:ring-indigo-500 placeholder-gray-500"
        />
      </div>

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
