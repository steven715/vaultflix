export type AdminIconKey =
  | 'dashboard' | 'video' | 'star' | 'tag' | 'folder' | 'users' | 'chart'

export interface AdminNavItem {
  key: string
  label: string
  path: string
  enabled: boolean
  icon: AdminIconKey
}

// 側邊欄 7 項。dashboard / analytics 尚未實作 → enabled:false（置灰 + 即將推出）。
export const ADMIN_NAV: readonly AdminNavItem[] = [
  { key: 'dashboard', label: '總覽', path: '/admin', enabled: false, icon: 'dashboard' },
  { key: 'library', label: '影片', path: '/admin/library', enabled: true, icon: 'video' },
  { key: 'recommendations', label: '精選', path: '/admin/recommendations', enabled: true, icon: 'star' },
  { key: 'tags', label: '標籤', path: '/admin/tags', enabled: true, icon: 'tag' },
  { key: 'media-sources', label: '來源', path: '/admin/media-sources', enabled: true, icon: 'folder' },
  { key: 'users', label: '帳號', path: '/admin/users', enabled: true, icon: 'users' },
  { key: 'analytics', label: '分析', path: '/admin/analytics', enabled: false, icon: 'chart' },
]

// /admin 是 Dashboard 完成前的暫時 landing，對應到影片庫。
export function isNavItemActive(itemPath: string, currentPath: string): boolean {
  if (itemPath === '/admin/library') {
    return currentPath === '/admin' || currentPath === '/admin/library' ||
      currentPath.startsWith('/admin/library/')
  }
  return currentPath === itemPath || currentPath.startsWith(itemPath + '/')
}

// 麵包屑標題：reverse 讓 library 的特例優先於 dashboard。
export function adminPageTitle(currentPath: string): string {
  const match = [...ADMIN_NAV].reverse().find((n) => isNavItemActive(n.path, currentPath))
  return match ? match.label : '管理後台'
}
