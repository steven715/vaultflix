import { barWidths } from './scale'

export interface BarRow {
  label: string
  value: number
  sub?: string
}

export default function BarChart({ rows }: { rows: BarRow[] }) {
  if (rows.length === 0) {
    return <div style={{ color: 'var(--color-faint)', fontSize: 13, padding: '12px 0' }}>尚無資料</div>
  }
  const max = Math.max(0, ...rows.map((r) => r.value))
  const widths = barWidths(rows.map((r) => r.value), max, 100)

  return (
    <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
      {rows.map((r, i) => (
        <li key={`${r.label}-${i}`} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, color: 'var(--color-cream)' }}>
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '75%' }}>{r.label}</span>
            <span style={{ color: 'var(--color-muted)' }}>{r.sub ?? r.value}</span>
          </div>
          <div style={{ height: 8, background: 'var(--color-surface-up)', borderRadius: 4 }}>
            <div style={{ width: `${widths[i]}%`, height: 8, background: 'var(--color-accent)', borderRadius: 4 }} />
          </div>
        </li>
      ))}
    </ul>
  )
}
