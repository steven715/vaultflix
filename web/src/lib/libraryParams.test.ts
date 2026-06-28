import { describe, it, expect } from 'vitest'
import {
  toggleSort, parseTagIds, serializeTagIds, toggleTagId,
  toggleSelected, isAllSelected, toggleSelectAll,
} from './libraryParams'

describe('toggleSort', () => {
  it('toggles order when same column clicked', () => {
    expect(toggleSort({ by: 'title', order: 'asc' }, 'title')).toEqual({ by: 'title', order: 'desc' })
    expect(toggleSort({ by: 'title', order: 'desc' }, 'title')).toEqual({ by: 'title', order: 'asc' })
  })
  it('defaults to desc when a new column is clicked', () => {
    expect(toggleSort({ by: 'title', order: 'asc' }, 'file_size_bytes')).toEqual({ by: 'file_size_bytes', order: 'desc' })
  })
})

describe('tag ids', () => {
  it('parses and serializes, filtering NaN', () => {
    expect(parseTagIds('1,2,3')).toEqual([1, 2, 3])
    expect(parseTagIds('')).toEqual([])
    expect(parseTagIds('1,x,3')).toEqual([1, 3])
    expect(serializeTagIds([1, 2])).toBe('1,2')
  })
  it('toggles membership', () => {
    expect(toggleTagId([1, 2], 2)).toEqual([1])
    expect(toggleTagId([1], 3)).toEqual([1, 3])
  })
})

describe('selection', () => {
  it('toggles a single id', () => {
    expect(toggleSelected(['a'], 'b')).toEqual(['a', 'b'])
    expect(toggleSelected(['a', 'b'], 'a')).toEqual(['b'])
  })
  it('isAllSelected only when every page id is selected', () => {
    expect(isAllSelected(['a', 'b'], ['a', 'b'])).toBe(true)
    expect(isAllSelected(['a'], ['a', 'b'])).toBe(false)
    expect(isAllSelected([], [])).toBe(false)
  })
  it('select-all toggles the current page set', () => {
    expect(toggleSelectAll([], ['a', 'b'])).toEqual(['a', 'b'])
    expect(toggleSelectAll(['a', 'b', 'c'], ['a', 'b'])).toEqual(['c'])
    expect(toggleSelectAll(['c'], ['a', 'b'])).toEqual(['c', 'a', 'b'])
  })
})
