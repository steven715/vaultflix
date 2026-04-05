import client from './client'
import type { TagWithCount, SuccessResponse } from '../types'

export async function listTags(category?: string): Promise<TagWithCount[]> {
  const params = category ? { category } : undefined
  const res = await client.get<SuccessResponse<TagWithCount[]>>('/tags', { params })
  return res.data.data
}
