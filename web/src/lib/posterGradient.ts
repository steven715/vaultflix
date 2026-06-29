// 缺縮圖時的 poster placeholder：由 id 穩定雜湊取 8 色盤之一（與設計稿一致）。
const POSTER_GRADIENTS: readonly string[] = [
  'linear-gradient(135deg, #FF8A3D 0%, #7A1F4B 100%)',
  'linear-gradient(135deg, #3E7BFF 0%, #0B1A3A 100%)',
  'linear-gradient(135deg, #1FB588 0%, #06241C 100%)',
  'linear-gradient(135deg, #FFC83D 0%, #7A3B00 100%)',
  'linear-gradient(135deg, #B15CFF 0%, #1A0B33 100%)',
  'linear-gradient(135deg, #FF5470 0%, #2A0810 100%)',
  'linear-gradient(135deg, #43C6FF 0%, #062033 100%)',
  'linear-gradient(135deg, #9BD64B 0%, #16280A 100%)',
]

export function posterGradient(id: string): string {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) | 0
  }
  const idx = Math.abs(hash) % POSTER_GRADIENTS.length
  return POSTER_GRADIENTS[idx]
}
