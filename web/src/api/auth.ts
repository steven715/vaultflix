import client from './client'

interface LoginResponse {
  data: {
    token: string
  }
}

export async function login(username: string, password: string): Promise<{ token: string }> {
  const res = await client.post<LoginResponse>('/auth/login', { username, password })
  return res.data.data
}
