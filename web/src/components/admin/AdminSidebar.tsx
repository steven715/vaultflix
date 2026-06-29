import { Link, useLocation } from 'react-router-dom'
import { ADMIN_NAV, isNavItemActive } from '../../lib/adminNav'
import NavIcon from './NavIcon'

export default function AdminSidebar() {
  const { pathname } = useLocation()
  return (
    <aside className="fixed inset-y-0 left-0 w-[78px] bg-surface border-r border-border flex flex-col items-center py-4 z-30">
      <Link
        to="/admin/library"
        aria-label="Vaultflix 管理後台"
        className="w-[42px] h-[42px] rounded-card flex items-center justify-center shrink-0"
        style={{ background: 'linear-gradient(150deg, #FF8A3D, #7A1F4B)' }}
      >
        <svg className="w-6 h-6 text-cream" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M12 2l8 3.5v6c0 5-3.4 8.7-8 10.5-4.6-1.8-8-5.5-8-10.5v-6L12 2z" opacity="0.92" />
        </svg>
      </Link>

      <nav className="flex flex-col items-stretch gap-1 mt-6 w-full px-2">
        {ADMIN_NAV.map((item) => {
          const active = item.enabled && isNavItemActive(item.path, pathname)
          const inner = (
            <>
              <NavIcon name={item.icon} />
              <span className="text-[10px] mt-1 leading-none">{item.label}</span>
            </>
          )
          const shape = 'flex flex-col items-center justify-center py-2 rounded-btn transition-colors'
          if (!item.enabled) {
            return (
              <div
                key={item.key}
                aria-disabled="true"
                title="即將推出"
                className={`${shape} text-faint/50 cursor-not-allowed`}
              >
                {inner}
              </div>
            )
          }
          return (
            <Link
              key={item.key}
              to={item.path}
              aria-current={active ? 'page' : undefined}
              className={`${shape} ${
                active ? 'text-accent bg-accent/15' : 'text-muted hover:text-cream hover:bg-surface-2'
              }`}
            >
              {inner}
            </Link>
          )
        })}
      </nav>
    </aside>
  )
}
