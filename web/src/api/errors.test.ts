import { describe, expect, it } from 'vitest'
import { ApiError, errorMessage, toApiError } from './errors'

// The shape axios rejects with when the server answered.
function serverError(status: number, error: string, message: string) {
  return Object.assign(new Error(`Request failed with status code ${status}`), { response: { status, data: { error, message } } })
}

describe('toApiError', () => {
  it('reads the status, the error code and the server message of a failed call', () => {
    const e = toApiError(serverError(409, 'username_taken', 'username already exists'))
    expect(e).toBeInstanceOf(ApiError)
    expect([e.status, e.code, e.serverMessage]).toEqual([409, 'username_taken', 'username already exists'])
  })

  it('has no status or code when the server never answered', () => {
    const e = toApiError(new Error('Network Error'))
    expect([e.status, e.code, e.serverMessage]).toEqual([null, null, null])
  })

  it('keeps an ApiError as it is', () => {
    const e = toApiError(serverError(404, 'not_found', 'user not found'))
    expect(toApiError(e)).toBe(e)
  })
})

describe('errorMessage', () => {
  it('says what the page wrote for the error code', () => {
    const err = serverError(409, 'username_taken', 'username already exists')
    expect(errorMessage(err, { fallback: '建立失敗', username_taken: '使用者名稱已存在' })).toBe('使用者名稱已存在')
  })

  // Server messages are English for API callers; a code the page did not
  // expect must not leak them, nor axios's "Request failed with status code".
  it('falls back for a code the page did not name', () => {
    const err = serverError(500, 'internal_error', 'failed to create user')
    expect(errorMessage(err, { fallback: '建立失敗，請重試' })).toBe('建立失敗，請重試')
    expect(errorMessage(new Error('Network Error'), { fallback: '建立失敗，請重試' })).toBe('建立失敗，請重試')
  })

  // A busy Job Lane's message names the Job holding it, written for the admin.
  it('shows the server message of a busy Job Lane', () => {
    const err = serverError(409, 'job_lane_busy', '補齊正在執行，完成或取消後再試')
    expect(errorMessage(err, { fallback: '匯入啟動失敗' })).toBe('補齊正在執行，完成或取消後再試')
  })
})
