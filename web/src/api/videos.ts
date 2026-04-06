import client from './client'
import type { PaginatedResponse, VideoWithTags, VideoDetail, VideoListParams } from '../types'

export async function listVideos(params: VideoListParams): Promise<PaginatedResponse<VideoWithTags>> {
  const res = await client.get<PaginatedResponse<VideoWithTags>>('/videos', { params })
  return res.data
}

export async function getVideo(id: string): Promise<VideoDetail> {
  const res = await client.get<VideoDetail>(`/videos/${id}`)
  return res.data
}
