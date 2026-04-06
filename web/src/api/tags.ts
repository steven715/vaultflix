import client from './client'
import type { TagWithCount } from '../types'

export async function listTags(category?: string): Promise<TagWithCount[]> {
  const params = category ? { category } : undefined
  const res = await client.get<TagWithCount[]>('/tags', { params })
  return res.data
}
