import { describe, it, expect } from 'vitest'
import { groupTagsByCategory } from './tagGroups'
import type { TagWithCount } from '../types'

const t = (id: number, name: string, category: string): TagWithCount =>
  ({ id, name, category, video_count: id })

describe('groupTagsByCategory', () => {
  it('groups by category in genre→studio→actor→custom order', () => {
    const groups = groupTagsByCategory([
      t(1, 'a', 'custom'), t(2, 'b', 'genre'), t(3, 'c', 'actor'),
    ])
    expect(groups.map((g) => g.category)).toEqual(['genre', 'actor', 'custom'])
    expect(groups.map((g) => g.label)).toEqual(['類型', '人物', '自訂'])
  })
  it('drops empty groups and unknown categories', () => {
    const groups = groupTagsByCategory([t(1, 'a', 'genre'), t(2, 'b', 'weird')])
    expect(groups).toHaveLength(1)
    expect(groups[0].tags.map((x) => x.name)).toEqual(['a'])
  })
})
