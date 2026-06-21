import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

interface EmptyStateProps {
  icon: ReactNode
  title: string
  description?: string
  // Optional call-to-action rendered as an accent pill link.
  ctaLabel?: string
  ctaTo?: string
}

// EmptyState is the shared "nothing here yet" panel used by favorites, history
// and empty search results: centered icon disc + title + description + optional CTA.
export default function EmptyState({ icon, title, description, ctaLabel, ctaTo }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <div className="mb-5 flex h-20 w-20 items-center justify-center rounded-full bg-surface text-faint">
        <div className="h-9 w-9">{icon}</div>
      </div>
      <h2 className="font-display text-xl font-bold text-cream">{title}</h2>
      {description && <p className="mt-2 max-w-xs text-sm text-muted">{description}</p>}
      {ctaLabel && ctaTo && (
        <Link
          to={ctaTo}
          className="mt-6 rounded-pill bg-accent px-5 py-2.5 text-sm font-medium text-accent-ink transition-transform hover:-translate-y-0.5"
        >
          {ctaLabel}
        </Link>
      )}
    </div>
  )
}
