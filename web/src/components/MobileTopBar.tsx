import { Link } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { SearchIcon } from './icons'

// MobileTopBar is the mobile home header: greeting + avatar, then a search pill
// that routes to the dedicated mobile search view. Hidden on desktop.
export default function MobileTopBar() {
  const { user } = useAuth()
  return (
    <div className="pt-2 md:hidden">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-muted">歡迎回來</p>
          <h1 className="font-display text-xl font-extrabold tracking-tight text-cream">
            {user?.username}
          </h1>
        </div>
        <span className="h-10 w-10 rounded-full bg-gradient-to-br from-accent to-fav" />
      </div>
      <Link
        to="/search"
        className="mt-4 flex h-11 items-center gap-2.5 rounded-pill border border-border bg-surface px-4 text-sm text-faint"
      >
        <SearchIcon className="h-4.5 w-4.5" />
        搜尋影片、標籤…
      </Link>
    </div>
  )
}
