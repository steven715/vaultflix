import type { JobPhase } from './jobProgress'
import type { BackfillKind } from '../types'

// Wording of each job kind in JobProgressPanel.
export interface JobLabels {
  running: string
  done: Record<Exclude<JobPhase, 'running'>, string>
  total: string
  succeeded: string
  skipped: string
  skippedShort: string
}

export const importLabels: JobLabels = {
  running: '匯入中',
  done: { completed: '匯入完成', failed: '匯入失敗', cancelled: '已取消' },
  total: '掃描檔案',
  succeeded: '成功匯入',
  skipped: '已跳過（重複）',
  skippedShort: '跳過',
}

const backfillKindLabel: Record<BackfillKind, string> = {
  preview: '預覽',
  codec: '編碼資訊',
  keyframe: 'Keyframe Index',
  code: '番號',
}

// spaced pads a Latin name so「補齊{name}中」keeps a gap between scripts.
function spaced(name: string): string {
  return /^[\x20-\x7e]+$/.test(name) ? ` ${name} ` : name
}

export function backfillLabels(kind: BackfillKind): JobLabels {
  return {
    running: `補齊${spaced(backfillKindLabel[kind])}中`,
    done: { completed: '補齊完成', failed: '補齊失敗', cancelled: '已取消' },
    total: '掃描影片',
    succeeded: '成功補齊',
    skipped: '略過',
    skippedShort: '略過',
  }
}

/** The Backfill kinds the library menu offers; Code waits for Enrichment. */
export const backfillMenu: BackfillKind[] = ['preview', 'keyframe', 'codec']

/** A Backfill kind's name on its own, e.g. in a menu. */
export function backfillKindName(kind: BackfillKind): string {
  return backfillKindLabel[kind]
}
