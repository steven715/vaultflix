import client from './client'
import type { SuccessResponse, RecommendationItem } from '../types'

export async function getTodayRecommendations(fallbackCount?: number): Promise<RecommendationItem[]> {
  const today = new Date().toISOString().slice(0, 10)
  const params: Record<string, string | number> = { date: today }
  if (fallbackCount) params.fallback_count = fallbackCount
  const res = await client.get<SuccessResponse<RecommendationItem[]>>('/recommendations/today', { params })
  return res.data.data
}
