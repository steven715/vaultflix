import axios from 'axios'
import { toApiError } from './errors'

const client = axios.create({
  baseURL: '/api',
})

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

client.interceptors.response.use(
  (response) => {
    // Unwrap SuccessResponse { data: ... } wrapper
    // PaginatedResponse has 'total' at top level, so skip those
    if (response.data && 'data' in response.data && !('total' in response.data)) {
      response.data = response.data.data
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      const url: string = error.config?.url ?? ''
      const isAuthEndpoint = url.includes('/auth/login')
      if (!isAuthEndpoint) {
        localStorage.removeItem('token')
        window.location.href = '/login'
      }
    }
    // Callers see one error shape; errorMessage turns it into the UI's words.
    return Promise.reject(toApiError(error))
  },
)

export default client
