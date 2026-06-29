import type { TagWithCount } from '../types'

export interface TagCategoryGroup {
  category: string
  label: string
  tags: TagWithCount[]
}

const CATEGORY_ORDER: { category: string; label: string }[] = [
  { category: 'genre', label: '類型' },
  { category: 'studio', label: '工作室' },
  { category: 'actor', label: '人物' },
  { category: 'custom', label: '自訂' },
]

// 依設計固定順序分組；空組與未知 category 不顯示。
export function groupTagsByCategory(tags: TagWithCount[]): TagCategoryGroup[] {
  return CATEGORY_ORDER
    .map(({ category, label }) => ({ category, label, tags: tags.filter((x) => x.category === category) }))
    .filter((g) => g.tags.length > 0)
}
