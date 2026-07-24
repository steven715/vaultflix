// hls.js 錯誤分類:manifest / segment 請求拿到 503(stream_not_ready,
// 首播 keyframe 探測進行中)屬「準備中」可輪詢重試;其他 fatal 錯誤即失敗。
// 重試上限 20 次 × 3s = 60s,涵蓋約 4GB 檔的冷讀探測(15s/GB)。
export const MAX_PREPARING_RETRIES = 20
export const PREPARING_RETRY_DELAY_MS = 3000

export type HlsErrorAction = 'retry-preparing' | 'fatal' | 'ignore'

export interface HlsErrorDataLike {
  fatal: boolean
  response?: { code?: number }
}

export function classifyHlsError(data: HlsErrorDataLike, retryCount: number): HlsErrorAction {
  if (!data.fatal) return 'ignore'
  if (data.response?.code === 503 && retryCount < MAX_PREPARING_RETRIES) return 'retry-preparing'
  return 'fatal'
}
