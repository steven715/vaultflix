export function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  }
  return `${m}:${String(s).padStart(2, '0')}`
}

export function formatFileSize(bytes: number): string {
  if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(0)} MB`
  return `${(bytes / 1024).toFixed(0)} KB`
}

export function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('zh-TW', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

// formatRelativeTime renders a coarse "N 天前 / N 小時前 / 剛剛" label for
// watch-history rows, relative to now.
export function formatRelativeTime(dateStr: string): string {
  const then = new Date(dateStr).getTime()
  const diffMs = Date.now() - then
  const min = Math.floor(diffMs / 60000)
  if (min < 1) return '剛剛'
  if (min < 60) return `${min} 分鐘前`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} 小時前`
  const day = Math.floor(hr / 24)
  if (day < 30) return `${day} 天前`
  const month = Math.floor(day / 30)
  if (month < 12) return `${month} 個月前`
  return `${Math.floor(month / 12)} 年前`
}

export type HistoryGroup = '今天' | '本週' | '更早'

// historyGroup buckets a timestamp into the three groups the history page uses.
export function historyGroup(dateStr: string): HistoryGroup {
  const then = new Date(dateStr)
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const t = then.getTime()
  if (t >= startOfToday) return '今天'
  if (t >= startOfToday - 6 * 24 * 60 * 60 * 1000) return '本週'
  return '更早'
}
