import client from './client'
import type { PaginatedResponse, WatchHistoryItem } from '../types'

export async function saveProgress(videoID: string, progressSeconds: number): Promise<void> {
  await client.post('/watch-history', {
    video_id: videoID,
    progress_seconds: progressSeconds,
  })
}

export async function listWatchHistory(
  page = 1,
  pageSize = 20,
): Promise<PaginatedResponse<WatchHistoryItem>> {
  const res = await client.get<PaginatedResponse<WatchHistoryItem>>('/watch-history', {
    params: { page, page_size: pageSize },
  })
  return res.data
}
