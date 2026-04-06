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
  is_favorited: boolean
  watch_progress: number
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

export interface WatchHistoryItem {
  id: string
  video_id: string
  title: string
  thumbnail_url?: string
  duration_seconds: number
  progress_seconds: number
  completed: boolean
  watched_at: string
}

export interface VideoSummaryWithURL {
  id: string
  title: string
  thumbnail_url?: string
  duration_seconds: number
  resolution: string
  file_size_bytes: number
  created_at: string
}

export interface ImportError {
  file_name: string
  error: string
}

export interface ImportJob {
  id: string
  source_id: string
  source_label: string
  status: 'running' | 'completed' | 'failed'
  total: number
  processed: number
  imported: number
  skipped: number
  failed: number
  errors: ImportError[]
  started_at: string
  finished_at?: string
}

export interface ImportProgress {
  job_id: string
  file_name: string
  current: number
  total: number
  status: 'processing' | 'success' | 'skipped' | 'error'
  error?: string
}

export interface MediaSource {
  id: string
  label: string
  mount_path: string
  enabled: boolean
  video_count: number
  created_at: string
  updated_at: string
}

export interface RecommendationItem {
  id: string
  video_id: string
  title: string
  thumbnail_url?: string
  duration_seconds: number
  resolution: string
  file_size_bytes: number
  sort_order: number
  is_fallback: boolean
}

export interface DailyRecommendation {
  id: string
  video_id: string
  recommend_date: string
  sort_order: number
  created_at: string
}

export interface RecommendationWithVideo {
  id: string
  video_id: string
  recommend_date: string
  sort_order: number
  title: string
  thumbnail_url?: string
  duration_seconds: number
  resolution: string
  file_size_bytes: number
}

export interface User {
  id: string
  username: string
  role: string
  disabled_at: string | null
  created_at: string
  updated_at: string
}
