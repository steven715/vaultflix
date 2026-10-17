// Dates the user picks (a day's Recommendations) are local calendar dates.
// Date.toISOString() is UTC: east of UTC it is still yesterday in the early
// morning, so never build a YYYY-MM-DD from it.

/** d's local calendar date as YYYY-MM-DD. */
export function localDate(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** The local date days after date (YYYY-MM-DD); negative days go back. */
export function shiftLocalDate(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00`)
  d.setDate(d.getDate() + days)
  return localDate(d)
}
