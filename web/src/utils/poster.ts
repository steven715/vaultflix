// Poster gradient fallback: videos without a real thumbnail get a deterministic
// 135deg linear gradient derived from their id, so the same video always shows
// the same color (per the design handoff's 8-color palette).
const POSTER_PALETTE: readonly [string, string][] = [
  ['#FF8A3D', '#7A1F4B'],
  ['#3E7BFF', '#0B1A3A'],
  ['#1FB588', '#06241C'],
  ['#FFC83D', '#7A3B00'],
  ['#B15CFF', '#1A0B33'],
  ['#FF5470', '#2A0810'],
  ['#43C6FF', '#062033'],
  ['#9BD64B', '#16280A'],
]

// Simple deterministic string hash (djb2) → palette index.
function hashString(input: string): number {
  let hash = 5381
  for (let i = 0; i < input.length; i++) {
    hash = (hash * 33) ^ input.charCodeAt(i)
  }
  return Math.abs(hash)
}

// posterGradient returns a CSS background value for a video id's fallback poster.
export function posterGradient(id: string): string {
  const [from, to] = POSTER_PALETTE[hashString(id) % POSTER_PALETTE.length]
  return `linear-gradient(135deg, ${from}, ${to})`
}
