export function getErrorMessage(err: unknown, fallback: string): string {
  const ax = err as { response?: { data?: { message?: string } } } | null
  return ax?.response?.data?.message || fallback
}
