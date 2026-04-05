export interface Video {
  id: string
  title: string
  description: string
  minio_object_key: string
  thumbnail_key: string
  duration_seconds: number
  resolution: string
  file_size_bytes: number
  mime_type: string
  original_filename: string
  created_at: string
  updated_at: string
}

export interface Tag {
  id: number
  name: string
  category: string
}

export interface TagWithCount extends Tag {
  video_count: number
}

export interface VideoWithTags extends Video {
  tags: Tag[]
  thumbnail_url?: string
}

export interface VideoDetail extends VideoWithTags {
  stream_url: string
  thumbnail_url: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  page_size: number
}

export interface SuccessResponse<T> {
  data: T
}

export interface ErrorResponse {
  error: string
  message: string
}

export interface VideoListParams {
  page?: number
  page_size?: number
  sort_by?: string
  sort_order?: string
  q?: string
  tag_ids?: string
}
