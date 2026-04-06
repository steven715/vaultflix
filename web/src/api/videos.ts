import client from './client'
import type { PaginatedResponse, VideoWithTags, VideoDetail, VideoListParams } from '../types'

export async function listVideos(params: VideoListParams): Promise<PaginatedResponse<VideoWithTags>> {
  const res = await client.get<PaginatedResponse<VideoWithTags>>('/videos', { params })
  return res.data
}

export async function getVideo(id: string, urlExpiryMinutes?: number): Promise<VideoDetail> {
  const params = urlExpiryMinutes ? { url_expiry_minutes: urlExpiryMinutes } : undefined
  const res = await client.get<VideoDetail>(`/videos/${id}`, { params })
  return res.data
}
