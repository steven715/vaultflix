export type LibrarySortBy = 'created_at' | 'title' | 'duration_seconds' | 'file_size_bytes'
export type SortOrder = 'asc' | 'desc'

export function toggleSort(
  current: { by: LibrarySortBy; order: SortOrder },
  clicked: LibrarySortBy,
): { by: LibrarySortBy; order: SortOrder } {
  if (current.by === clicked) {
    return { by: clicked, order: current.order === 'asc' ? 'desc' : 'asc' }
  }
  return { by: clicked, order: 'desc' }
}

export function parseTagIds(raw: string): number[] {
  if (!raw) return []
  return raw.split(',').map((s) => Number(s)).filter((n) => Number.isFinite(n))
}

export function serializeTagIds(ids: number[]): string {
  return ids.join(',')
}

export function toggleTagId(ids: number[], id: number): number[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]
}

export function toggleSelected(selected: string[], id: string): string[] {
  return selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
}

export function isAllSelected(selected: string[], pageIds: string[]): boolean {
  return pageIds.length > 0 && pageIds.every((id) => selected.includes(id))
}

export function toggleSelectAll(selected: string[], pageIds: string[]): string[] {
  if (isAllSelected(selected, pageIds)) {
    return selected.filter((id) => !pageIds.includes(id))
  }
  const set = new Set(selected)
  for (const id of pageIds) set.add(id)
  return [...set]
}
