import client from './client'

interface LoginResponse {
  token: string
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await client.post<LoginResponse>('/auth/login', { username, password })
  return res.data
}
