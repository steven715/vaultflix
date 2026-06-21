// Skeleton renders a shimmering placeholder block. Compose several to mirror a
// view's real layout so nothing jumps when content loads.
export default function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`skeleton rounded-card ${className}`} />
}
