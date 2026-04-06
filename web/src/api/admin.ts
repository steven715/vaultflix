import client from './client'
import type {
  ImportJob,
  MediaSource,
  Video,
  Tag,
  DailyRecommendation,
  RecommendationItem,
  User,
} from '../types'

export async function importVideos(sourceID: string): Promise<ImportJob> {
  const res = await client.post<ImportJob>('/videos/import', { source_id: sourceID })
  return res.data
}

export async function getActiveImportJob(): Promise<ImportJob | null> {
  const res = await client.get<ImportJob | null>('/import-jobs/active')
  return res.data
}

export async function getImportJob(id: string): Promise<ImportJob> {
  const res = await client.get<ImportJob>(`/import-jobs/${id}`)
  return res.data
}

export async function listMediaSources(): Promise<MediaSource[]> {
  const res = await client.get<MediaSource[]>('/media-sources')
  return res.data
}

export async function updateVideo(id: string, data: { title: string; description: string }): Promise<Video> {
  const res = await client.put<Video>(`/videos/${id}`, data)
  return res.data
}

export async function deleteVideo(id: string): Promise<void> {
  await client.delete(`/videos/${id}`)
}

export async function createTag(name: string, category?: string): Promise<Tag> {
  const res = await client.post<Tag>('/tags', { name, category })
  return res.data
}

export async function addVideoTag(videoId: string, tagId: number): Promise<void> {
  await client.post(`/videos/${videoId}/tags`, { tag_id: tagId })
}

export async function removeVideoTag(videoId: string, tagId: number): Promise<void> {
  await client.delete(`/videos/${videoId}/tags/${tagId}`)
}

export async function listRecommendationsByDate(date: string): Promise<RecommendationItem[]> {
  const res = await client.get<RecommendationItem[]>('/recommendations', { params: { date } })
  return res.data
}

export async function createRecommendation(
  videoId: string,
  date: string,
  sortOrder: number,
): Promise<DailyRecommendation> {
  const res = await client.post<DailyRecommendation>('/recommendations', {
    video_id: videoId,
    recommend_date: date,
    sort_order: sortOrder,
  })
  return res.data
}

export async function updateRecommendationSortOrder(id: string, sortOrder: number): Promise<void> {
  await client.put(`/recommendations/${id}`, { sort_order: sortOrder })
}

export async function deleteRecommendation(id: string): Promise<void> {
  await client.delete(`/recommendations/${id}`)
}

export async function listUsers(): Promise<User[]> {
  const res = await client.get<User[]>('/users')
  return res.data
}

export async function createUser(username: string, password: string, role: string): Promise<User> {
  const res = await client.post<User>('/users', { username, password, role })
  return res.data
}

export async function deleteUser(id: string): Promise<void> {
  await client.delete(`/users/${id}`)
}

export async function enableUser(id: string): Promise<void> {
  await client.put(`/users/${id}/enable`)
}

export async function resetUserPassword(id: string, password: string): Promise<void> {
  await client.put(`/users/${id}/password`, { password })
}
