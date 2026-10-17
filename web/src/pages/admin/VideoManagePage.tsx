import { useState, useEffect, useRef } from 'react'
import { listVideos } from '../../api/videos'
import { listTags } from '../../api/tags'
import { updateVideo, deleteVideo, listMediaSources, addVideoTag } from '../../api/admin'
import type { VideoWithTags, TagWithCount, MediaSource, BackfillKind } from '../../types'
import LibraryToolbar from '../../components/admin/LibraryToolbar'
import LibraryTable from '../../components/admin/LibraryTable'
import { BatchTagPicker, ImportModal, EditModal, DeleteConfirm } from '../../components/admin/LibraryModals'
import Pagination from '../../components/Pagination'
import JobProgressPanel from '../../components/admin/JobProgressPanel'
import { backfillLabels } from '../../lib/jobLabels'
import ErrorBanner from '../../components/ErrorBanner'
import { useToast } from '../../contexts/ToastContext'
import { useMediaLane } from '../../hooks/useMediaLane'
import { useLibraryParams } from '../../hooks/useLibraryParams'
import { runOnEach } from '../../lib/batch'

export default function VideoManagePage() {
  const params = useLibraryParams()
  const { page, pageSize, query, tagIdsParam, sortBy, sortOrder } = params
  const [videos, setVideos] = useState<VideoWithTags[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const [allTags, setAllTags] = useState<TagWithCount[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [copyFeedback, setCopyFeedback] = useState<Record<string, boolean>>({})
  const [batchTagPickerId, setBatchTagPickerId] = useState<number | null>(null)
  const toast = useToast()

  const [showImport, setShowImport] = useState(false)
  const [mediaSources, setMediaSources] = useState<MediaSource[]>([])
  const [selectedSourceID, setSelectedSourceID] = useState('')

  const [editingVideo, setEditingVideo] = useState<VideoWithTags | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [saving, setSaving] = useState(false)
  const [deletingVideo, setDeletingVideo] = useState<VideoWithTags | null>(null)

  const totalPages = Math.ceil(total / pageSize)

  // A quiet reload (after a Job) keeps the rows on screen while it loads.
  const quietReloadRef = useRef(false)
  useEffect(() => {
    let cancelled = false
    const quiet = quietReloadRef.current
    quietReloadRef.current = false
    if (!quiet) setLoading(true)
    setLoadError(false)
    listVideos({ page, page_size: pageSize, sort_by: sortBy, sort_order: sortOrder, q: query || undefined, tag_ids: tagIdsParam || undefined })
      .then((res) => { if (!cancelled) { setVideos(res.data); setTotal(res.total) } })
      .catch(() => { if (!cancelled) { setVideos([]); setTotal(0); setLoadError(true) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [page, pageSize, query, tagIdsParam, sortBy, sortOrder, reloadKey])

  useEffect(() => {
    listTags().then(setAllTags).catch((err) => { console.warn('failed to load tags', err) })
  }, [])

  useEffect(() => {
    if (!showImport) return
    listMediaSources()
      .then((sources) => {
        const enabled = sources.filter((s) => s.enabled)
        setMediaSources(enabled)
        if (enabled.length > 0) setSelectedSourceID((prev) => prev || enabled[0].id)
      })
      .catch((err) => { console.warn('failed to load media sources', err); setMediaSources([]) })
  }, [showImport])

  // Show the newest Videos on page 1, loaded once: navigating to the page
  // already shown does not refetch (frontend.md「同路徑重新導航的 refetch」),
  // and a page change commits in a later render than reloadKey would.
  function showFromPageOne() {
    if (page === 1) setReloadKey((k) => k + 1)
    else params.setPage(1)
  }

  // The Media Lane's two Jobs. An Import that added Videos shows them; one
  // that added none leaves the admin where they were. A Backfill changes
  // Videos in place: reload the page being looked at, quietly.
  const lane = useMediaLane({
    onImportFinished: (view) => { if (view.counts.succeeded > 0) showFromPageOne() },
    onBackfillFinished: () => { quietReloadRef.current = true; setReloadKey((k) => k + 1) },
  })
  // An adopted Import keeps the dialog open until its summary is closed.
  const importModalOpen = showImport || lane.import.view !== null

  async function handleStartImport() {
    if (selectedSourceID) await lane.import.start(selectedSourceID)
  }

  function openEdit(video: VideoWithTags) {
    setEditingVideo(video); setEditTitle(video.title); setEditDesc(video.description)
  }

  async function handleSaveEdit() {
    if (!editingVideo) return
    setSaving(true)
    try {
      const updated = await updateVideo(editingVideo.id, { title: editTitle, description: editDesc })
      setVideos((prev) => prev.map((v) => v.id === updated.id ? { ...v, title: updated.title, description: updated.description } : v))
      setEditingVideo(null); toast.success('已儲存')
    } catch { toast.error('儲存失敗，請重試') }
    finally { setSaving(false) }
  }

  async function handleDelete() {
    if (!deletingVideo) return
    const id = deletingVideo.id; setDeletingVideo(null)
    try { await deleteVideo(id); setVideos((prev) => prev.filter((v) => v.id !== id)); setTotal((p) => p - 1) }
    catch { toast.error('刪除失敗，請重試') }
  }

  // A batch keeps going past a failure, then says how many failed and keeps
  // those selected, so the admin can retry them.
  async function handleBatchDelete() {
    const ids = [...selected]
    const failed = await runOnEach(ids, deleteVideo)
    setSelected(failed); setReloadKey((k) => k + 1)
    if (failed.length > 0) toast.error(`${ids.length} 部中有 ${failed.length} 部刪除失敗`)
    else toast.success(`已刪除 ${ids.length} 部影片`)
  }

  async function handleBatchTag() {
    if (batchTagPickerId === null) return
    const ids = [...selected]
    const tagId = batchTagPickerId
    const failed = await runOnEach(ids, (id) => addVideoTag(id, tagId))
    setBatchTagPickerId(null); setReloadKey((k) => k + 1)
    if (failed.length > 0) toast.error(`${ids.length} 部中有 ${failed.length} 部加標籤失敗`)
    else toast.success('批次標籤完成')
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
        onBatchTag={() => setBatchTagPickerId(allTags[0]?.id ?? null)}
        onBatchDelete={handleBatchDelete}
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

      {loading ? (
        <div className="text-faint text-center py-20">載入中...</div>
      ) : loadError ? (
        <ErrorBanner message="無法載入影片，請確認服務是否正常運作" onRetry={() => setReloadKey((k) => k + 1)} />
      ) : videos.length === 0 ? (
        <div className="text-faint text-center py-20">{query || tagIdsParam ? '沒有符合條件的影片' : '尚無影片'}</div>
      ) : (
        <LibraryTable
          videos={videos} allTags={allTags} selected={selected} sortBy={sortBy} sortOrder={sortOrder}
          copyFeedback={copyFeedback} onSelect={setSelected} onColSort={params.sortByColumn}
          onCopyPath={handleCopyPath} onEdit={openEdit} onDelete={setDeletingVideo} onTagsChange={refreshTags}
        />
      )}

      <Pagination page={page} totalPages={totalPages} onPageChange={params.setPage} />

      {selected.length > 0 && batchTagPickerId !== null && (
        <BatchTagPicker selectedCount={selected.length} allTags={allTags} tagId={batchTagPickerId}
          onTagChange={setBatchTagPickerId} onApply={handleBatchTag} onClose={() => setBatchTagPickerId(null)} />
      )}

      {importModalOpen && (
        <ImportModal job={lane.import.view} mediaSources={mediaSources} selectedSourceID={selectedSourceID}
          onSourceChange={setSelectedSourceID} onClose={() => { setShowImport(false); lane.import.dismiss() }}
          onStartImport={handleStartImport} onCancelImport={lane.import.cancel} onResetImport={lane.import.dismiss}
          startDisabledReason={lane.importBlockedReason} />
      )}

      {editingVideo && (
        <EditModal title={editTitle} desc={editDesc} saving={saving}
          onTitleChange={setEditTitle} onDescChange={setEditDesc} onSave={handleSaveEdit} onClose={() => setEditingVideo(null)} />
      )}

      {deletingVideo && (
        <DeleteConfirm video={deletingVideo} onConfirm={handleDelete} onClose={() => setDeletingVideo(null)} />
      )}
    </div>
  )
}
