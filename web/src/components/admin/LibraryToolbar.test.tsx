import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import LibraryToolbar from './LibraryToolbar'

function toolbar(over: Partial<Parameters<typeof LibraryToolbar>[0]> = {}) {
  const props = {
    total: 1, sortBy: 'created_at' as const, sortOrder: 'desc' as const, tagIdsStr: '', allTags: [], selected: [],
    backfillDisabledReason: null, onSort: vi.fn(), onTagFilter: vi.fn(), onStartBackfill: vi.fn(),
    onOpenImport: vi.fn(), onBatchTag: vi.fn(), onBatchDelete: vi.fn(), onClearSelection: vi.fn(),
    ...over,
  }
  render(<LibraryToolbar {...props} />)
  return props
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
  })
})
