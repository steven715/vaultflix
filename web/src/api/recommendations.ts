import client from './client'
import type { RecommendationItem } from '../types'
import { localDate } from '../lib/localDate'

export async function getTodayRecommendations(fallbackCount?: number): Promise<RecommendationItem[]> {
  // The server picks the day's Recommendations by the browser's local date.
  const today = localDate(new Date())
  const params: Record<string, string | number> = { date: today }
  if (fallbackCount) params.fallback_count = fallbackCount
  const res = await client.get<RecommendationItem[]>('/recommendations/today', { params })
  return res.data
}
