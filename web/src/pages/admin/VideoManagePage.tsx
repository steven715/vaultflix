import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
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
import type { LibrarySortBy, SortOrder } from '../../lib/libraryParams'
import { useJob } from '../../hooks/useJob'
import { backfillJobs, importJobs } from '../../lib/jobKinds'

export default function VideoManagePage() {
  const [searchParams, setSearchParams] = useSearchParams()
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

  const page = Number(searchParams.get('page')) || 1
  const pageSize = Number(searchParams.get('page_size')) || 20
  const query = searchParams.get('q') || ''
  const tagIdsStr = searchParams.get('tag_ids') || ''
  const sortBy = (searchParams.get('sort_by') as LibrarySortBy) || 'created_at'
  const sortOrder = (searchParams.get('sort_order') as SortOrder) || 'desc'
  const totalPages = Math.ceil(total / pageSize)

  const updateParams = useCallback((updates: Record<string, string>) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      for (const [key, value] of Object.entries(updates)) {
        if (value) next.set(key, value)
        else next.delete(key)
      }
      return next
    })
  }, [setSearchParams])

  useEffect(() => {
    let cancelled = false
    setLoading(true); setLoadError(false)
    listVideos({ page, page_size: pageSize, sort_by: sortBy, sort_order: sortOrder, q: query || undefined, tag_ids: tagIdsStr || undefined })
      .then((res) => { if (!cancelled) { setVideos(res.data); setTotal(res.total) } })
      .catch(() => { if (!cancelled) { setVideos([]); setTotal(0); setLoadError(true) } })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [page, pageSize, query, tagIdsStr, sortBy, sortOrder, reloadKey])

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

  // Import Job: a running one keeps the import dialog open (also when it was
  // already running as the page opened); finishing goes back to page 1.
  const importJob = useJob(importJobs, { onFinished: () => updateParams({ page: '1' }) })
  // An adopted job keeps the dialog open until its summary is closed.
  const importModalOpen = showImport || importJob.view !== null
  const backfillJob = useJob(backfillJobs)

  async function handleStartBackfill() {
    const { outcome } = await backfillJob.start('preview')
    if (outcome === 'adopted') toast.error('已有任務進行中')
    else if (outcome === 'failed') toast.error('啟動 backfill 失敗')
  }

  async function handleCancelBackfill() {
    try {
      await backfillJob.cancel?.()
    } catch (err) {
      console.warn('failed to cancel backfill', err)
      toast.error('取消失敗')
    }
  }

  async function handleStartImport() {
    if (!selectedSourceID) return
    const { outcome } = await importJob.start(selectedSourceID)
    if (outcome === 'failed') toast.error('匯入啟動失敗')
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

  async function handleBatchDelete() {
    for (const id of [...selected]) { try { await deleteVideo(id) } catch { /* continue */ } }
    setSelected([]); setReloadKey((k) => k + 1)
  }

  async function handleBatchTag() {
    if (batchTagPickerId === null) return
    for (const id of selected) { try { await addVideoTag(id, batchTagPickerId) } catch { /* continue */ } }
    setBatchTagPickerId(null); setReloadKey((k) => k + 1); toast.success('批次標籤完成')
  }

  function handleCopyPath(filename: string, id: string) {
    navigator.clipboard.writeText(filename)
    setCopyFeedback((prev) => ({ ...prev, [id]: true }))
    clearTimeout(copyTimerRef.current)
    copyTimerRef.current = setTimeout(() => setCopyFeedback((prev) => ({ ...prev, [id]: false })), 1300)
  }

  const copyTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(copyTimerRef.current), [])

  const sortRef = useRef({ sortBy, sortOrder })
  sortRef.current = { sortBy, sortOrder }
  function handleColSort(col: LibrarySortBy) {
    const { sortBy: by, sortOrder: order } = sortRef.current
    const next = by === col ? (order === 'asc' ? 'desc' : 'asc') : 'desc'
    updateParams({ sort_by: col, sort_order: next })
  }

  function refreshTags() { listTags().then(setAllTags).catch((e) => console.warn('failed to refresh tags', e)) }

  return (
    <div className="p-7">
      <h1 className="font-display font-bold tracking-tight text-cream text-xl mb-4">影片庫</h1>
      <LibraryToolbar
        total={total} sortBy={sortBy} sortOrder={sortOrder} tagIdsStr={tagIdsStr}
        allTags={allTags} selected={selected} backfillStarting={backfillJob.starting} backfillJobId={backfillJob.view?.id ?? null}
        onSort={(p) => updateParams({ sort_by: p.sort_by, sort_order: p.sort_order, page: '1' })}
        onTagFilter={(tag_ids) => updateParams({ tag_ids, page: '1' })}
        onStartBackfill={handleStartBackfill}
        onOpenImport={() => { importJob.dismiss(); setShowImport(true) }}
        onBatchTag={() => setBatchTagPickerId(allTags[0]?.id ?? null)}
        onBatchDelete={handleBatchDelete}
        onClearSelection={() => setSelected([])}
      />

      {backfillJob.view && (
        <div className="mb-5">
          <JobProgressPanel
            view={backfillJob.view}
            labels={backfillLabels(backfillJob.view.subject as BackfillKind)}
            onCancel={handleCancelBackfill}
          />
          <div className="text-right mt-1">
            <button onClick={backfillJob.dismiss} className="text-xs text-faint hover:text-muted">關閉</button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="text-faint text-center py-20">載入中...</div>
      ) : loadError ? (
        <ErrorBanner message="無法載入影片，請確認服務是否正常運作" onRetry={() => setReloadKey((k) => k + 1)} />
      ) : videos.length === 0 ? (
        <div className="text-faint text-center py-20">{query || tagIdsStr ? '沒有符合條件的影片' : '尚無影片'}</div>
      ) : (
        <LibraryTable
          videos={videos} allTags={allTags} selected={selected} sortBy={sortBy} sortOrder={sortOrder}
          copyFeedback={copyFeedback} onSelect={setSelected} onColSort={handleColSort}
          onCopyPath={handleCopyPath} onEdit={openEdit} onDelete={setDeletingVideo} onTagsChange={refreshTags}
        />
      )}

      <Pagination page={page} totalPages={totalPages} onPageChange={(p) => updateParams({ page: String(p) })} />

      {selected.length > 0 && batchTagPickerId !== null && (
        <BatchTagPicker selectedCount={selected.length} allTags={allTags} tagId={batchTagPickerId}
          onTagChange={setBatchTagPickerId} onApply={handleBatchTag} onClose={() => setBatchTagPickerId(null)} />
      )}

      {importModalOpen && (
        <ImportModal job={importJob.view} mediaSources={mediaSources} selectedSourceID={selectedSourceID}
          onSourceChange={setSelectedSourceID} onClose={() => { setShowImport(false); importJob.dismiss() }}
          onStartImport={handleStartImport} onResetImport={importJob.dismiss} />
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
