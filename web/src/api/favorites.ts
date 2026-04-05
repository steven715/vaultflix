import client from './client'
import type { PaginatedResponse, VideoSummaryWithURL } from '../types'

export async function addFavorite(videoID: string): Promise<void> {
  await client.post('/favorites', { video_id: videoID })
}

export async function removeFavorite(videoID: string): Promise<void> {
  await client.delete(`/favorites/${videoID}`)
}

export async function listFavorites(
  page = 1,
  pageSize = 20,
): Promise<PaginatedResponse<VideoSummaryWithURL>> {
  const res = await client.get<PaginatedResponse<VideoSummaryWithURL>>('/favorites', {
    params: { page, page_size: pageSize },
  })
  return res.data
}
