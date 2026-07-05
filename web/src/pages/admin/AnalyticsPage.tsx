import { useEffect, useState } from 'react'
import { getAnalytics, type AnalyticsSummary } from '../../api/analytics'
import StatTile from '../../components/admin/charts/StatTile'
import AreaChart from '../../components/admin/charts/AreaChart'
import BarChart from '../../components/admin/charts/BarChart'

const RANGES = [7, 30, 90] as const

type TrendMetric = 'watch_hours' | 'views'
const TREND_METRICS: { key: TrendMetric; label: string }[] = [
  { key: 'watch_hours', label: '觀看時長' },
  { key: 'views', label: '觀看次數' },
]

export default function AnalyticsPage() {
  const [days, setDays] = useState<number>(30)
  const [trendMetric, setTrendMetric] = useState<TrendMetric>('watch_hours')
  const [data, setData] = useState<AnalyticsSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setLoading(true)
      try {
        const res = await getAnalytics(days)
        if (cancelled) return
        setData(res)
        setError('')
      } catch {
        if (!cancelled) setError('無法載入分析資料')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => {
      cancelled = true
    }
  }, [days])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-cream)', margin: 0 }}>分析</h1>
        <div style={{ display: 'flex', gap: 6 }}>
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setDays(r)}
              style={{
                padding: '6px 12px',
                borderRadius: 8,
                border: '1px solid var(--color-border)',
                background: days === r ? 'var(--color-accent)' : 'transparent',
                color: days === r ? 'var(--color-accent-ink)' : 'var(--color-muted)',
                cursor: 'pointer',
                fontSize: 13,
              }}
            >
              近 {r} 天
            </button>
          ))}
        </div>
      </div>

      {error && <div style={{ color: 'var(--color-fav)' }}>{error}</div>}
      {loading && !data && <div style={{ color: 'var(--color-muted)' }}>載入中…</div>}

      {data && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
            <StatTile label="總觀看次數" value={String(data.total_views)} />
            <StatTile label="觀看時長" value={`${data.total_watch_hours} 小時`} />
            <StatTile label="平均完播率" value={`${Math.round(data.avg_completion_rate * 100)}%`} />
            <StatTile label="活躍使用者" value={String(data.active_users)} />
          </div>

          <section style={panelStyle}>
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, marginBottom: 8 }}>
              {TREND_METRICS.map((m) => (
                <button
                  key={m.key}
                  onClick={() => setTrendMetric(m.key)}
                  style={{
                    padding: '4px 10px',
                    borderRadius: 8,
                    border: '1px solid var(--color-border)',
                    background: trendMetric === m.key ? 'var(--color-accent)' : 'transparent',
                    color: trendMetric === m.key ? 'var(--color-accent-ink)' : 'var(--color-muted)',
                    cursor: 'pointer',
                    fontSize: 12,
                  }}
                >
                  {m.label}
                </button>
              ))}
            </div>
            <AreaChart
              points={data.daily_trend}
              valueKey={trendMetric}
              label={`近 ${data.range_days} 天${trendMetric === 'watch_hours' ? '觀看時長' : '觀看次數'}`}
            />
          </section>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16 }}>
            <section style={panelStyle}>
              <h2 style={panelTitle}>熱門影片</h2>
              <BarChart
                rows={data.top_videos.map((v) => ({
                  label: v.title,
                  value: v.views,
                  sub: `${v.views} 次 · ${v.watch_hours} 小時`,
                }))}
              />
            </section>
            <section style={panelStyle}>
              <h2 style={panelTitle}>熱門標籤</h2>
              <BarChart rows={data.top_tags.map((t) => ({ label: t.name, value: t.views, sub: `${t.views} 次` }))} />
            </section>
          </div>
        </>
      )}
    </div>
  )
}

const panelStyle: React.CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  borderRadius: 14,
  padding: 18,
}
const panelTitle: React.CSSProperties = { fontSize: 15, fontWeight: 600, color: 'var(--color-cream)', margin: '0 0 12px' }
