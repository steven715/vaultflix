import { useState, useEffect, useRef } from 'react'
import { listVideos } from '../../api/videos'
import { listTags } from '../../api/tags'
import { deleteVideo } from '../../api/admin'
import type { VideoWithTags, TagWithCount, BackfillKind } from '../../types'
import LibraryToolbar from '../../components/admin/LibraryToolbar'
import LibraryTable from '../../components/admin/LibraryTable'
import { BatchTagDialog, DeleteConfirm } from '../../components/admin/LibraryModals'
import ImportDialog from '../../components/admin/ImportDialog'
import EditVideoDialog from '../../components/admin/EditVideoDialog'
import Pagination from '../../components/Pagination'
import JobProgressPanel from '../../components/admin/JobProgressPanel'
import { backfillLabels } from '../../lib/jobLabels'
import ErrorBanner from '../../components/ErrorBanner'
import { useToast } from '../../contexts/ToastContext'
import { useMediaLane } from '../../hooks/useMediaLane'
import { useLibraryParams } from '../../hooks/useLibraryParams'
import { usePagedList } from '../../hooks/usePagedList'
import { runOnEach } from '../../lib/batch'

export default function VideoManagePage() {
  const params = useLibraryParams()
  const { page, pageSize, query, tagIdsParam, sortBy, sortOrder } = params
  const [allTags, setAllTags] = useState<TagWithCount[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [copyFeedback, setCopyFeedback] = useState<Record<string, boolean>>({})
  const [batchTagging, setBatchTagging] = useState(false)
  const toast = useToast()

  const [showImport, setShowImport] = useState(false)

  const [editingVideo, setEditingVideo] = useState<VideoWithTags | null>(null)
  const [deletingVideo, setDeletingVideo] = useState<VideoWithTags | null>(null)
  const [confirmingBatchDelete, setConfirmingBatchDelete] = useState(false)

  // The library's current page. A refresh (after a Job or a batch) is quiet:
  // the rows stay while it loads, and a failure only says so.
  const { filter } = params
  const list = usePagedList<VideoWithTags>({
    load: (p) => listVideos({ ...filter, page: p }),
    page, pageSize, setPage: params.setPage,
    key: JSON.stringify(filter),
  })
  const { items: videos, total, totalPages } = list
  function refreshList() {
    list.reload().catch(() => toast.error('無法更新影片列表'))
  }

  useEffect(() => {
    listTags().then(setAllTags).catch((err) => { console.warn('failed to load tags', err) })
  }, [])

  // Show the newest Videos on page 1, loaded once: navigating to the page
  // already shown does not refetch (frontend.md「同路徑重新導航的 refetch」).
  function showFromPageOne() {
    if (page === 1) refreshList()
    else params.setPage(1)
  }

  // The Media Lane's two Jobs. An Import that added Videos shows them; one
  // that added none leaves the admin where they were. A Backfill changes
  // Videos in place: reload the page being looked at, quietly.
  const lane = useMediaLane({
    onImportFinished: (view) => { if (view.counts.succeeded > 0) showFromPageOne() },
    onBackfillFinished: refreshList,
  })

  async function handleDelete() {
    if (!deletingVideo) return
    const id = deletingVideo.id; setDeletingVideo(null)
    try { await deleteVideo(id); list.mutate((p) => ({ items: p.items.filter((v) => v.id !== id), total: p.total - 1 })) }
    catch { toast.error('刪除失敗，請重試') }
  }

  // A batch keeps going past a failure, then says how many failed and keeps
  // those selected, so the admin can retry them.
  async function handleBatchDelete() {
    setConfirmingBatchDelete(false)
    const ids = [...selected]
    const failed = await runOnEach(ids, deleteVideo)
    const deleted = ids.filter((id) => !failed.includes(id))
    list.mutate((p) => ({ items: p.items.filter((v) => !deleted.includes(v.id)), total: p.total - deleted.length }))
    setSelected(failed); refreshList()
    if (failed.length > 0) toast.error(`${ids.length} 部中有 ${failed.length} 部刪除失敗`)
    else toast.success(`已刪除 ${ids.length} 部影片`)
  }

  function handleCopyPath(filename: string, id: string) {
    navigator.clipboard.writeText(filename)
    setCopyFeedback((prev) => ({ ...prev, [id]: true }))
    clearTimeout(copyTimerRef.current)
    copyTimerRef.current = setTimeout(() => setCopyFeedback((prev) => ({ ...prev, [id]: false })), 1300)
  }

  const copyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(copyTimerRef.current), [])

  function refreshTags() { listTags().then(setAllTags).catch((e) => console.warn('failed to refresh tags', e)) }

  return (
    <div className="p-7">
      <h1 className="font-display font-bold tracking-tight text-cream text-xl mb-4">影片庫</h1>
      <LibraryToolbar
        total={total} sortBy={sortBy} sortOrder={sortOrder} tagIds={params.tagIds}
        allTags={allTags} selected={selected} backfillDisabledReason={lane.backfillBlockedReason}
        onSort={params.sortByColumn}
        onTagFilter={params.setTagIds}
        onStartBackfill={(kind) => { void lane.backfill.start(kind) }}
        onOpenImport={() => { lane.import.dismiss(); setShowImport(true) }}
        onBatchTag={() => { if (allTags.length > 0) setBatchTagging(true) }}
        onBatchDelete={() => setConfirmingBatchDelete(true)}
        onClearSelection={() => setSelected([])}
      />

      {lane.backfill.view && (
        <div className="mb-5">
          <JobProgressPanel
            view={lane.backfill.view}
            labels={backfillLabels(lane.backfill.view.subject as BackfillKind)}
            onCancel={lane.backfill.cancel}
            onDismiss={lane.backfill.dismiss}
          />
        </div>
      )}

      {list.status === 'loading' ? (
        <div className="text-faint text-center py-20">載入中...</div>
      ) : list.status === 'error' ? (
        <ErrorBanner message="無法載入影片，請確認服務是否正常運作" onRetry={list.retry} />
      ) : videos.length === 0 ? (
        <div className="text-faint text-center py-20">{query || tagIdsParam ? '沒有符合條件的影片' : '尚無影片'}</div>
      ) : (
        <LibraryTable
          videos={videos} allTags={allTags} selected={selected} sortBy={sortBy} sortOrder={sortOrder}
          copyFeedback={copyFeedback} onSelect={setSelected} onColSort={params.sortByColumn}
          onCopyPath={handleCopyPath} onEdit={setEditingVideo} onDelete={setDeletingVideo} onTagsChange={refreshTags}
        />
      )}

      <Pagination page={page} totalPages={totalPages} onPageChange={params.setPage} />

      {selected.length > 0 && batchTagging && (
        <BatchTagDialog videoIds={selected} tags={allTags} onDone={refreshList} onClose={() => setBatchTagging(false)} />
      )}

      <ImportDialog job={lane.import} blockedReason={lane.importBlockedReason} open={showImport} onClose={() => setShowImport(false)} />

      {editingVideo && (
        <EditVideoDialog video={editingVideo} onClose={() => setEditingVideo(null)}
          onSaved={(updated) => {
            setEditingVideo(null)
            list.mutate((p) => ({ ...p, items: p.items.map((v) => v.id === updated.id ? { ...v, title: updated.title, description: updated.description } : v) }))
          }} />
      )}

      {deletingVideo && (
        <DeleteConfirm what={`「${deletingVideo.title}」`} onConfirm={handleDelete} onClose={() => setDeletingVideo(null)} />
      )}
      {confirmingBatchDelete && (
        <DeleteConfirm what={`選取的 ${selected.length} 部影片`} onConfirm={handleBatchDelete} onClose={() => setConfirmingBatchDelete(false)} />
      )}
    </div>
  )
}
