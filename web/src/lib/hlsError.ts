// hls.js 錯誤分類:manifest / HLS Segment 請求拿到 503(stream_not_ready,
// Keyframe Index 探測進行中)屬「準備中」可輪詢重試;401 是 Stream Token 過期
// (segment 網址帶的是載入 manifest 當下的 token),由呼叫端換新 token 後重載;
// 其他 fatal 錯誤即失敗。
// 後端保證 503 只用於「準備中」—— Media Source 停用回 409 source_unavailable,
// 立即失敗不重試。hls.js 只暴露狀態碼(拿不到 body),所以契約必須落在狀態碼上。
// 重試上限 20 次 ×(3s 自訂延遲 + hls.js 內部 manifestLoadPolicy 預設重試約 1s)
// ≈ 80s 上限,涵蓋約 4GB 檔的冷讀探測(15s/GB)。
export const MAX_PREPARING_RETRIES = 20
export const PREPARING_RETRY_DELAY_MS = 3000

export type HlsErrorAction = 'retry-preparing' | 'refresh-token' | 'fatal' | 'ignore'

export interface HlsErrorDataLike {
  fatal: boolean
  response?: { code?: number }
}

export function classifyHlsError(data: HlsErrorDataLike, retryCount: number): HlsErrorAction {
  if (!data.fatal) return 'ignore'
  if (data.response?.code === 503 && retryCount < MAX_PREPARING_RETRIES) return 'retry-preparing'
  if (data.response?.code === 401) return 'refresh-token'
  return 'fatal'
}
