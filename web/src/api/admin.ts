import client from './client'
import type {
  SuccessResponse,
  ImportResult,
  Video,
  Tag,
  DailyRecommendation,
  RecommendationItem,
} from '../types'

export async function importVideos(sourceDir: string): Promise<ImportResult> {
  const res = await client.post<SuccessResponse<ImportResult>>('/videos/import', { source_dir: sourceDir })
  return res.data.data
}

export async function updateVideo(id: string, data: { title: string; description: string }): Promise<Video> {
  const res = await client.put<SuccessResponse<Video>>(`/videos/${id}`, data)
  return res.data.data
}

export async function deleteVideo(id: string): Promise<void> {
  await client.delete(`/videos/${id}`)
}

export async function createTag(name: string, category?: string): Promise<Tag> {
  const res = await client.post<SuccessResponse<Tag>>('/tags', { name, category })
  return res.data.data
}

export async function addVideoTag(videoId: string, tagId: number): Promise<void> {
  await client.post(`/videos/${videoId}/tags`, { tag_id: tagId })
}

export async function removeVideoTag(videoId: string, tagId: number): Promise<void> {
  await client.delete(`/videos/${videoId}/tags/${tagId}`)
}

export async function listRecommendationsByDate(date: string): Promise<RecommendationItem[]> {
  const res = await client.get<SuccessResponse<RecommendationItem[]>>('/recommendations', { params: { date } })
  return res.data.data
}

export async function createRecommendation(
  videoId: string,
  date: string,
  sortOrder: number,
): Promise<DailyRecommendation> {
  const res = await client.post<SuccessResponse<DailyRecommendation>>('/recommendations', {
    video_id: videoId,
    recommend_date: date,
    sort_order: sortOrder,
  })
  return res.data.data
}

export async function updateRecommendationSortOrder(id: string, sortOrder: number): Promise<void> {
  await client.put(`/recommendations/${id}`, { sort_order: sortOrder })
}

export async function deleteRecommendation(id: string): Promise<void> {
  await client.delete(`/recommendations/${id}`)
}
