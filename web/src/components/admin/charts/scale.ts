export interface XY { x: number; y: number }

// niceMax rounds an axis maximum up to a readable ceiling; never returns 0.
export function niceMax(max: number): number {
  if (max <= 0) return 1
  const pow = Math.pow(10, Math.floor(Math.log10(max)))
  const steps = [1, 2, 2.5, 5, 10]
  for (const s of steps) {
    const candidate = s * pow
    if (candidate >= max) return candidate
  }
  return 10 * pow
}

// Map values (oldest→newest) to evenly spaced x, inverted y within [0,h].
function toPoints(values: number[], w: number, h: number, max: number): XY[] {
  const n = values.length
  return values.map((v, i) => ({
    x: n <= 1 ? 0 : (i / (n - 1)) * w,
    y: h - (v / max) * h,
  }))
}

export function linePath(values: number[], w: number, h: number, max: number): string {
  const pts = toPoints(values, w, h, max)
  return pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ')
}

export function areaPath(values: number[], w: number, h: number, max: number): string {
  const pts = toPoints(values, w, h, max)
  if (pts.length === 0) return ''
  const top = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ')
  return `${top} L${pts[pts.length - 1].x.toFixed(2)},${h} L${pts[0].x.toFixed(2)},${h} Z`
}

// barWidths scales each value to a pixel width; all-zero → all zero (no NaN).
export function barWidths(values: number[], max: number, fullWidth: number): number[] {
  const m = max > 0 ? max : 0
  return values.map((v) => (m === 0 ? 0 : (v / m) * fullWidth))
}
