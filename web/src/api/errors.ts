/** A failed API call, as the api module reports it. */
export class ApiError extends Error {
  /** The HTTP status; null when the server never answered. */
  readonly status: number | null
  /** The ErrorResponse error code, e.g. "username_taken"; null without one. */
  readonly code: string | null
  /** The ErrorResponse message: English, for API callers, not for the UI. */
  readonly serverMessage: string | null

  constructor(status: number | null, code: string | null, serverMessage: string | null, cause: unknown) {
    super(serverMessage ?? (cause instanceof Error ? cause.message : 'request failed'), { cause })
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.serverMessage = serverMessage
  }
}

/** err as an ApiError: what the client rejects with, or an axios-shaped error (e.g. a test's). */
export function toApiError(err: unknown): ApiError {
  if (err instanceof ApiError) return err
  const res = (err as { response?: { status?: number; data?: { error?: string; message?: string } } } | null)?.response
  return new ApiError(res?.status ?? null, res?.data?.error ?? null, res?.data?.message ?? null, err)
}

/** What to tell the user for each error code the page expects; fallback for anything else. */
export type ErrorTexts = { fallback: string } & Record<string, string>

// errorMessage is the text a page shows for a failed call. The UI's words are
// the page's: it names the error codes it expects, and anything else gets the
// fallback, never the server's English or axios's "Request failed with status
// code …". The one exception is a code whose message the server writes for
// the admin (a busy Job Lane names the Job holding it).
export function errorMessage(err: unknown, texts: ErrorTexts): string {
  const e = toApiError(err)
  if (e.code && Object.hasOwn(texts, e.code)) return texts[e.code]
  if (e.code && e.serverMessage && serverWritesUiText(e.code)) return e.serverMessage
  return texts.fallback
}

function serverWritesUiText(code: string): boolean {
  switch (code) {
    case 'job_lane_busy':
      return true
  }
  return false
}
