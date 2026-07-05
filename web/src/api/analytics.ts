import client from './client'

export interface DailyPoint {
  date: string
  views: number
  watch_hours: number
}
export interface TopVideo {
  video_id: string
  title: string
  thumbnail_key: string
  views: number
  watch_hours: number
}
export interface TopTag {
  tag_id: number
  name: string
  views: number
}
export interface AnalyticsSummary {
  range_days: number
  total_views: number
  total_watch_hours: number
  avg_completion_rate: number
  active_users: number
  daily_trend: DailyPoint[]
  top_videos: TopVideo[]
  top_tags: TopTag[]
}

export async function getAnalytics(days: number, limit = 10): Promise<AnalyticsSummary> {
  const res = await client.get<AnalyticsSummary>('/admin/analytics', { params: { days, limit } })
  return res.data
}
