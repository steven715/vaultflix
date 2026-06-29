import { posterGradient } from '../../lib/posterGradient'

interface PosterThumbProps {
  id: string
  src?: string | null
  alt?: string
  className?: string
}

// 縮圖：有真圖用 img，缺圖用 id 雜湊出的漸層 placeholder（維持一致性）。
export default function PosterThumb({ id, src, alt = '', className = '' }: PosterThumbProps) {
  const base = 'aspect-video rounded-btn overflow-hidden bg-surface-2'
  if (src) {
    return (
      <div className={`${base} ${className}`}>
        <img src={src} alt={alt} className="w-full h-full object-cover" loading="lazy" />
      </div>
    )
  }
  return (
    <div
      data-poster-fallback
      className={`${base} ${className}`}
      style={{ background: posterGradient(id) }}
      aria-hidden="true"
    />
  )
}
