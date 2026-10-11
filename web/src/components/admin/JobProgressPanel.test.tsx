import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import JobProgressPanel from './JobProgressPanel'
import { backfillLabels, importLabels } from '../../lib/jobLabels'
import type { JobView } from '../../lib/jobProgress'

const running: JobView = {
  id: 'j1', phase: 'running', currentItem: 'b.mkv', errors: [],
  counts: { total: 10, processed: 4, succeeded: 2, skipped: 1, failed: 1 },
}

describe('JobProgressPanel', () => {
  it('shows a running job with the server counts and uses design tokens', () => {
    const { container } = render(<JobProgressPanel view={running} labels={backfillLabels('keyframe')} />)
    expect(container.textContent).toContain('補齊 Keyframe Index 中')
    expect(container.textContent).toContain('4 / 10')
    expect(container.textContent).toContain('處理中: b.mkv')
    expect(container.textContent).toMatch(/2\s*成功/)
    expect(container.textContent).toMatch(/1\s*略過/)
    expect(container.innerHTML).not.toMatch(/(?:bg|text)-(?:gray|indigo)-/)
  })

  it('offers cancel only when given, and disables it while cancelling', async () => {
    let finish: () => void = () => {}
    const onCancel = vi.fn(() => new Promise<void>((r) => { finish = r }))
    const { rerender } = render(<JobProgressPanel view={running} labels={backfillLabels('preview')} onCancel={onCancel} />)
    fireEvent.click(screen.getByText('取消'))
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(screen.getByText('取消中...')).toBeDisabled()
    finish()
    await waitFor(() => expect(screen.getByText('取消')).toBeEnabled())

    rerender(<JobProgressPanel view={running} labels={importLabels} />)
    expect(screen.queryByText('取消')).toBeNull()
  })

  it('summarises a finished job with its failures on demand', () => {
    const done: JobView = {
      ...running, phase: 'completed', currentItem: '',
      counts: { total: 3, processed: 3, succeeded: 1, skipped: 1, failed: 1 },
      errors: [{ name: 'c.mkv', error: 'boom' }],
    }
    render(<JobProgressPanel view={done} labels={importLabels} />)
    expect(screen.getByText('匯入完成')).toBeInTheDocument()
    expect(screen.getByText('成功匯入').nextSibling?.textContent).toBe('1')
    expect(screen.queryByText('c.mkv')).toBeNull()
    fireEvent.click(screen.getByText('展開失敗詳情 (1)'))
    expect(screen.getByText('c.mkv')).toBeInTheDocument()
  })

  it('names failed and cancelled jobs', () => {
    const { rerender } = render(<JobProgressPanel view={{ ...running, phase: 'failed' }} labels={importLabels} />)
    expect(screen.getByText('匯入失敗')).toBeInTheDocument()
    rerender(<JobProgressPanel view={{ ...running, phase: 'cancelled' }} labels={backfillLabels('code')} />)
    expect(screen.getByText('已取消')).toBeInTheDocument()
  })
})
