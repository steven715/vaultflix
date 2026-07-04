export default function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div
      style={{
        background: 'var(--color-surface-up)',
        border: '1px solid var(--color-border)',
        borderRadius: 12,
        padding: '16px 18px',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
      }}
    >
      <span style={{ fontSize: 13, color: 'var(--color-muted)' }}>{label}</span>
      <span style={{ fontSize: 30, fontWeight: 700, color: 'var(--color-cream)', lineHeight: 1.1 }}>{value}</span>
      {hint && <span style={{ fontSize: 12, color: 'var(--color-faint)' }}>{hint}</span>}
    </div>
  )
}
