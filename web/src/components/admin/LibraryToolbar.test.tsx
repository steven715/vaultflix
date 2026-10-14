import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import LibraryToolbar from './LibraryToolbar'

function toolbar(over: Partial<Parameters<typeof LibraryToolbar>[0]> = {}) {
  const props = {
    total: 1, sortBy: 'created_at' as const, sortOrder: 'desc' as const, tagIds: [], allTags: [], selected: [],
    backfillDisabledReason: null, onSort: vi.fn(), onTagFilter: vi.fn(), onStartBackfill: vi.fn(),
    onOpenImport: vi.fn(), onBatchTag: vi.fn(), onBatchDelete: vi.fn(), onClearSelection: vi.fn(),
    ...over,
  }
  const view = render(<LibraryToolbar {...props} />)
  return { ...props, rerender: (o: Partial<typeof props>) => view.rerender(<LibraryToolbar {...props} {...o} />) }
}

describe('LibraryToolbar backfill menu', () => {
  it('starts the Backfill kind picked from the menu', () => {
    const props = toolbar()
    fireEvent.click(screen.getByRole('button', { name: '補齊 ▾' }))

    const items = screen.getAllByRole('menuitem').map((b) => b.textContent?.trim())
    expect(items).toEqual(['預覽', 'Keyframe Index', '編碼資訊'])

    fireEvent.click(screen.getByRole('menuitem', { name: 'Keyframe Index' }))
    expect(props.onStartBackfill).toHaveBeenCalledWith('keyframe')
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('stays shut and says why while the Media Lane is busy', () => {
    toolbar({ backfillDisabledReason: '匯入執行中' })
    const button = screen.getByRole('button', { name: '補齊 ▾' })
    expect(button).toBeDisabled()
    expect(button).toHaveAttribute('title', '匯入執行中')
    // Visible too: some browsers show no tooltip on a disabled button.
    expect(screen.getByText('匯入執行中')).toBeInTheDocument()
  })

  it('closes on a click outside it', () => {
    toolbar()
    fireEvent.click(screen.getByRole('button', { name: '補齊 ▾' }))
    fireEvent.mouseDown(document.body)
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('closes when the lane becomes busy while it is open', () => {
    const t = toolbar()
    fireEvent.click(screen.getByRole('button', { name: '補齊 ▾' }))
    t.rerender({ backfillDisabledReason: '匯入執行中' })
    expect(screen.queryByRole('menu')).toBeNull()
  })
})
