import type { ReactNode } from 'react'
import Header from './Header'
import MobileTabBar from './MobileTabBar'

interface AppShellProps {
  children: ReactNode
  // Player view hides the bottom tab bar for an immersive layout.
  showTabBar?: boolean
}

// AppShell is the chrome shared by every authenticated view: the desktop sticky
// header (hidden on mobile) and the mobile floating tab bar (hidden on desktop).
export default function AppShell({ children, showTabBar = true }: AppShellProps) {
  return (
    <div className="min-h-screen bg-bg">
      <Header />
      <main className={showTabBar ? 'pb-28 md:pb-0' : ''}>{children}</main>
      {showTabBar && <MobileTabBar />}
    </div>
  )
}

// Container is the centered content column. width="wide" = 1360px main column,
// "narrow" = 900px (history). Horizontal padding shrinks on mobile.
export function Container({
  children,
  width = 'wide',
  className = '',
}: {
  children: ReactNode
  width?: 'wide' | 'narrow'
  className?: string
}) {
  const max = width === 'narrow' ? 'max-w-[900px]' : 'max-w-[1360px]'
  return <div className={`mx-auto w-full ${max} px-4 sm:px-6 lg:px-10 ${className}`}>{children}</div>
}
