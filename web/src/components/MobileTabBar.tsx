import { Link, useLocation } from 'react-router-dom'
import { HomeIcon, SearchIcon, HeartIcon, ClockIcon } from './icons'
import type { SVGProps, ReactElement } from 'react'

type IconCmp = (p: SVGProps<SVGSVGElement>) => ReactElement

const TABS: { to: string; label: string; Icon: IconCmp }[] = [
  { to: '/', label: '首頁', Icon: HomeIcon },
  { to: '/search', label: '搜尋', Icon: SearchIcon },
  { to: '/favorites', label: '收藏', Icon: HeartIcon },
  { to: '/history', label: '記錄', Icon: ClockIcon },
]

// Floating glass tab bar shown only on mobile (md:hidden). Hidden on the player
// and login views by simply not rendering it from those pages' shells.
export default function MobileTabBar() {
  const location = useLocation()
  return (
    <nav
      className="fixed inset-x-4 bottom-3.5 z-30 flex h-16 items-stretch rounded-[24px] border border-border px-2 backdrop-blur-[20px] md:hidden"
      style={{ backgroundColor: 'rgba(20,16,13,0.8)' }}
    >
      {TABS.map(({ to, label, Icon }) => {
        const active = location.pathname === to
        return (
          <Link
            key={to}
            to={to}
            className={`flex flex-1 flex-col items-center justify-center gap-1 transition-colors ${
              active ? 'text-accent' : 'text-faint'
            }`}
          >
            <Icon className="h-6 w-6" />
            <span className="text-[11px] font-medium">{label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
