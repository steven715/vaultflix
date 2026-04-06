# Phase 13 — Admin UI: Media Source Management Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an admin management page for media sources with CRUD operations and integrated import progress, plus update project docs to reflect Phase 8–13 architecture changes.

**Architecture:** Backend adds `video_count` to the List query via LEFT JOIN. Frontend adds a new `/admin/media-sources` page with card-based layout, CRUD modals, and an extracted `ImportProgress` component reused from VideoManagePage. All styling follows existing Tailwind dark theme patterns.

**Tech Stack:** Go 1.22+ / Gin / pgx, React 18 / TypeScript / Tailwind CSS, WebSocket for import progress

---

### Task 1: Backend — Add `VideoCount` to MediaSource model

**Files:**
- Modify: `internal/model/media_source.go`

- [ ] **Step 1: Add `VideoCount` field to MediaSource struct**

```go
type MediaSource struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	MountPath  string    `json:"mount_path"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	VideoCount int       `json:"video_count"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/model/media_source.go
git commit -m "feat: add VideoCount field to MediaSource model"
```

---

### Task 2: Backend — Update List query with LEFT JOIN + COUNT

**Files:**
- Modify: `internal/repository/media_source_repo.go:30-34` (queryListMediaSources)
- Modify: `internal/repository/media_source_repo.go:66-87` (List method)

- [ ] **Step 1: Update the List SQL query**

Replace `queryListMediaSources` (lines 30–34):

```go
const queryListMediaSources = `
    SELECT ms.id, ms.label, ms.mount_path, ms.enabled, ms.created_at, ms.updated_at,
           COUNT(v.id) AS video_count
    FROM media_sources ms
    LEFT JOIN videos v ON v.source_id = ms.id
    GROUP BY ms.id
    ORDER BY ms.created_at ASC
`
```

- [ ] **Step 2: Update the List method to scan VideoCount**

Replace the `List` method (lines 66–87):

```go
func (r *mediaSourceRepository) List(ctx context.Context) ([]model.MediaSource, error) {
	rows, err := r.pool.Query(ctx, queryListMediaSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list media sources: %w", err)
	}
	defer rows.Close()

	var sources []model.MediaSource
	for rows.Next() {
		var s model.MediaSource
		if err := rows.Scan(&s.ID, &s.Label, &s.MountPath, &s.Enabled, &s.CreatedAt, &s.UpdatedAt, &s.VideoCount); err != nil {
			return nil, fmt.Errorf("failed to scan media source: %w", err)
		}
		sources = append(sources, s)
	}

	if sources == nil {
		sources = []model.MediaSource{}
	}

	return sources, nil
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/repository/media_source_repo.go
git commit -m "feat: add video_count to media source list query via LEFT JOIN"
```

---

### Task 3: Backend — Update tests for VideoCount

**Files:**
- Modify: `internal/handler/media_source_handler_test.go`

- [ ] **Step 1: Update existing List_Empty test to verify VideoCount field is present**

The existing `TestMediaSourceHandler_List_Empty` test (line 42) already tests empty array, which is sufficient. Add a new test for the case with video counts:

```go
func TestMediaSourceHandler_List_WithVideoCount(t *testing.T) {
	now := time.Now()
	repo := &mock.MediaSourceRepository{
		ListFunc: func(ctx context.Context) ([]model.MediaSource, error) {
			return []model.MediaSource{
				{ID: "ms-1", Label: "Videos", MountPath: "/mnt/host/Videos", Enabled: true, CreatedAt: now, UpdatedAt: now, VideoCount: 42},
				{ID: "ms-2", Label: "Anime", MountPath: "/mnt/host/Anime", Enabled: false, CreatedAt: now, UpdatedAt: now, VideoCount: 0},
			}, nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/media-sources", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			VideoCount int    `json:"video_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(resp.Data))
	}
	if resp.Data[0].VideoCount != 42 {
		t.Errorf("expected video_count 42, got %d", resp.Data[0].VideoCount)
	}
	if resp.Data[1].VideoCount != 0 {
		t.Errorf("expected video_count 0, got %d", resp.Data[1].VideoCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd d:/Vaultflix && go test ./internal/handler/ -run TestMediaSourceHandler -v`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add internal/handler/media_source_handler_test.go
git commit -m "test: add handler test for media source list with video_count"
```

---

### Task 4: Frontend — Add `video_count` to TypeScript type and API functions

**Files:**
- Modify: `web/src/types/index.ts:113-121`
- Modify: `web/src/api/admin.ts`

- [ ] **Step 1: Add `video_count` to MediaSource interface**

In `web/src/types/index.ts`, update the `MediaSource` interface (lines 113–121):

```typescript
export interface MediaSource {
  id: string
  label: string
  mount_path: string
  enabled: boolean
  video_count: number
  created_at: string
  updated_at: string
}
```

- [ ] **Step 2: Add CRUD API functions to admin.ts**

Add these functions at the end of `web/src/api/admin.ts`:

```typescript
export async function createMediaSource(data: { label: string; mount_path: string }): Promise<MediaSource> {
  const res = await client.post<MediaSource>('/media-sources', data)
  return res.data
}

export async function updateMediaSource(id: string, data: { label: string; enabled: boolean }): Promise<void> {
  await client.put(`/media-sources/${id}`, data)
}

export async function deleteMediaSource(id: string): Promise<void> {
  await client.delete(`/media-sources/${id}`)
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/types/index.ts web/src/api/admin.ts
git commit -m "feat: add video_count to MediaSource type and CRUD API functions"
```

---

### Task 5: Frontend — Extract ImportProgress component from VideoManagePage

**Files:**
- Create: `web/src/components/admin/ImportProgress.tsx`
- Modify: `web/src/pages/admin/VideoManagePage.tsx`

- [ ] **Step 1: Create ImportProgress component**

Create `web/src/components/admin/ImportProgress.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { useWS } from '../../contexts/WebSocketContext'
import { getActiveImportJob } from '../../api/admin'
import type { ImportJob, ImportProgress as ImportProgressType, ImportError } from '../../types'

type ImportState = 'importing' | 'completed' | 'failed'

interface ImportProgressProps {
  jobId: string
  onComplete?: () => void
}

export default function ImportProgress({ jobId, onComplete }: ImportProgressProps) {
  const [importState, setImportState] = useState<ImportState>('importing')
  const [currentFile, setCurrentFile] = useState('')
  const [processed, setProcessed] = useState(0)
  const [importTotal, setImportTotal] = useState(0)
  const [imported, setImported] = useState(0)
  const [skipped, setSkipped] = useState(0)
  const [failed, setFailed] = useState(0)
  const [importErrors, setImportErrors] = useState<ImportError[]>([])
  const [finalResult, setFinalResult] = useState<ImportJob | null>(null)
  const [showErrors, setShowErrors] = useState(false)

  const { lastMessage } = useWS()

  // Restore progress from active job on mount
  useEffect(() => {
    let cancelled = false
    getActiveImportJob().then((job) => {
      if (cancelled || !job || job.id !== jobId) return
      setProcessed(job.processed)
      setImportTotal(job.total)
      setImported(job.imported)
      setSkipped(job.skipped)
      setFailed(job.failed)
      setImportErrors(job.errors || [])
    }).catch(() => {})
    return () => { cancelled = true }
  }, [jobId])

  // WebSocket progress listener
  useEffect(() => {
    if (!lastMessage) return

    switch (lastMessage.type) {
      case 'import_progress': {
        const p = lastMessage.payload as ImportProgressType
        if (p.job_id !== jobId) break
        if (p.status === 'processing') {
          setCurrentFile(p.file_name)
        } else {
          setProcessed(p.current)
          setImportTotal(p.total)
          if (p.status === 'success') setImported((prev) => prev + 1)
          if (p.status === 'skipped') setSkipped((prev) => prev + 1)
          if (p.status === 'error') {
            setFailed((prev) => prev + 1)
            setImportErrors((prev) => [...prev, { file_name: p.file_name, error: p.error || '' }])
          }
        }
        break
      }
      case 'import_complete': {
        const result = lastMessage.payload as ImportJob
        if (result.id !== jobId) break
        setFinalResult(result)
        setImportState(result.failed > 0 && result.imported === 0 ? 'failed' : 'completed')
        onComplete?.()
        break
      }
      case 'import_error': {
        setImportState('failed')
        break
      }
    }
  }, [lastMessage, jobId, onComplete])

  return (
    <div className="bg-gray-800/50 rounded-lg p-4 mt-3">
      {importState === 'importing' && (
        <>
          <div className="mb-3">
            <div className="flex justify-between text-sm text-gray-400 mb-1">
              <span>匯入進度</span>
              <span>{processed} / {importTotal || '...'}</span>
            </div>
            <div className="w-full bg-gray-700 rounded-full h-2">
              <div
                className="bg-indigo-500 h-2 rounded-full transition-all duration-300"
                style={{ width: importTotal > 0 ? `${(processed / importTotal) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-gray-500 mb-2 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div className="text-center">
              <div className="text-green-400 font-medium">{imported}</div>
              <div className="text-gray-500 text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-gray-400 font-medium">{skipped}</div>
              <div className="text-gray-500 text-xs">跳過</div>
            </div>
            <div className="text-center">
              <div className="text-red-400 font-medium">{failed}</div>
              <div className="text-gray-500 text-xs">失敗</div>
            </div>
          </div>
        </>
      )}

      {(importState === 'completed' || importState === 'failed') && (
        <>
          <div className={`text-sm mb-3 font-medium ${importState === 'failed' ? 'text-red-400' : 'text-green-400'}`}>
            {importState === 'completed' ? '匯入完成' : '匯入失敗'}
          </div>
          <div className="space-y-1.5 text-sm mb-3">
            <div className="flex justify-between text-gray-300"><span>掃描檔案</span><span>{finalResult?.total ?? importTotal}</span></div>
            <div className="flex justify-between text-green-400"><span>成功匯入</span><span>{finalResult?.imported ?? imported}</span></div>
            <div className="flex justify-between text-gray-400"><span>已跳過（重複）</span><span>{finalResult?.skipped ?? skipped}</span></div>
            <div className="flex justify-between text-red-400"><span>失敗</span><span>{finalResult?.failed ?? failed}</span></div>
          </div>
          {(finalResult?.errors?.length ?? importErrors.length) > 0 && (
            <div>
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-red-400 hover:text-red-300 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalResult?.errors?.length ?? importErrors.length})
              </button>
              {showErrors && (
                <div className="bg-gray-900 rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {(finalResult?.errors ?? importErrors).map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-gray-300">{e.file_name}</span>
                      <span className="text-gray-600 ml-1">— {e.error}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Refactor VideoManagePage to use ImportProgress component**

In `web/src/pages/admin/VideoManagePage.tsx`:

1. Add import at top:
```typescript
import ImportProgress from '../../components/admin/ImportProgress'
```

2. Remove these state variables (lines 27–35): `currentFile`, `processed`, `importTotal`, `imported`, `skipped`, `failed`, `importErrors`, `finalResult`, `showErrors`.

3. Remove the WebSocket progress listener `useEffect` (lines 132–166).

4. Remove `resetImportState` function's fields that are now in the component — simplify to only reset `importState`, `currentJobId`.

5. Replace the import modal's `importing` and `completed/failed` sections (lines 363–438) with:

```tsx
{importState === 'importing' && currentJobId && (
  <ImportProgress
    jobId={currentJobId}
    onComplete={() => {
      setImportState('completed')
      updateParams({ page: '1' })
    }}
  />
)}
```

Note: Keep the modal wrapper, idle state (source selection), and close/re-import buttons. Only the progress display and completion summary move to the component. The `completed`/`failed` state display also moves to the component. After import completes, the modal shows the ImportProgress component in its completed state with a close + re-import button row below.

Detailed replacement for the import modal content after the idle state:

```tsx
{(importState === 'importing' || importState === 'completed' || importState === 'failed') && currentJobId && (
  <>
    <ImportProgress jobId={currentJobId} onComplete={() => {
      setImportState('completed')
      updateParams({ page: '1' })
    }} />
    {importState !== 'importing' && (
      <div className="flex justify-end gap-2 mt-3">
        <button onClick={() => { resetImportState(); setShowImport(false) }} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">關閉</button>
        <button onClick={resetImportState} className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-1.5 rounded">重新匯入</button>
      </div>
    )}
    {importState === 'importing' && (
      <p className="text-xs text-gray-600 text-center mt-3">匯入進行中，請勿關閉此視窗...</p>
    )}
  </>
)}
```

Simplified `resetImportState`:
```typescript
function resetImportState() {
  setImportState('idle')
  setCurrentJobId(null)
}
```

Remove the active job check useEffect's state restoration (lines 114–128) — simplify to just detect active job and set `currentJobId` + `importState`:

```typescript
useEffect(() => {
  let cancelled = false
  getActiveImportJob().then((job) => {
    if (cancelled || !job) return
    setShowImport(true)
    setCurrentJobId(job.id)
    setImportState('importing')
  }).catch(() => {})
  return () => { cancelled = true }
}, [])
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/admin/ImportProgress.tsx web/src/pages/admin/VideoManagePage.tsx
git commit -m "refactor: extract ImportProgress into reusable component"
```

---

### Task 6: Frontend — Add route and Header navigation

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/Header.tsx`

- [ ] **Step 1: Add route in App.tsx**

Add import at top of `web/src/App.tsx`:
```typescript
import MediaSourcePage from './pages/admin/MediaSourcePage'
```

Add the route inside the AdminRoute children array (after line 62):
```typescript
{ path: '/admin/media-sources', element: <MediaSourcePage /> },
```

- [ ] **Step 2: Add Header navigation link**

In `web/src/components/Header.tsx`, add a "媒體來源" link after the existing admin "管理" link (after line 53, before the users link):

```tsx
{isAdmin && (
  <Link
    to="/admin/media-sources"
    className={`flex items-center gap-1 text-sm transition-colors ${
      location.pathname === '/admin/media-sources' ? 'text-white' : 'text-gray-400 hover:text-white'
    }`}
    title="媒體來源"
  >
    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" />
    </svg>
    <span className="hidden sm:inline">媒體來源</span>
  </Link>
)}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/App.tsx web/src/components/Header.tsx
git commit -m "feat: add /admin/media-sources route and header navigation"
```

---

### Task 7: Frontend — Create MediaSourcePage (main page structure + list)

**Files:**
- Create: `web/src/pages/admin/MediaSourcePage.tsx`

- [ ] **Step 1: Create the page component with list rendering**

Create `web/src/pages/admin/MediaSourcePage.tsx`:

```tsx
import { useState, useEffect, useCallback } from 'react'
import { listMediaSources, createMediaSource, updateMediaSource, deleteMediaSource, importVideos, getActiveImportJob } from '../../api/admin'
import type { MediaSource, ImportJob } from '../../types'
import Header from '../../components/Header'
import ImportProgress from '../../components/admin/ImportProgress'

export default function MediaSourcePage() {
  const [sources, setSources] = useState<MediaSource[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Import state
  const [importingSourceId, setImportingSourceId] = useState<string | null>(null)
  const [currentJobId, setCurrentJobId] = useState<string | null>(null)

  // Modal state
  const [showCreate, setShowCreate] = useState(false)
  const [editingSource, setEditingSource] = useState<MediaSource | null>(null)
  const [confirmAction, setConfirmAction] = useState<{ type: 'disable' | 'enable' | 'delete'; source: MediaSource } | null>(null)

  // Search (Header requires it)
  const [searchQuery] = useState('')

  const fetchSources = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await listMediaSources()
      setSources(data)
    } catch {
      setError('無法載入媒體來源，請確認服務是否正常運作')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSources()
  }, [fetchSources])

  // Check for active import on mount
  useEffect(() => {
    let cancelled = false
    getActiveImportJob().then((job) => {
      if (cancelled || !job) return
      setImportingSourceId(job.source_id)
      setCurrentJobId(job.id)
    }).catch(() => {})
    return () => { cancelled = true }
  }, [])

  async function handleImport(source: MediaSource) {
    try {
      const job = await importVideos(source.id)
      setImportingSourceId(source.id)
      setCurrentJobId(job.id)
    } catch (err: unknown) {
      const axiosErr = err as { response?: { status?: number; data?: { message?: string } } }
      if (axiosErr?.response?.status === 409) {
        // Already running — try to recover active job
        const activeJob = await getActiveImportJob().catch(() => null)
        if (activeJob) {
          setImportingSourceId(activeJob.source_id)
          setCurrentJobId(activeJob.id)
        }
      } else {
        setError(axiosErr?.response?.data?.message || '匯入啟動失敗')
      }
    }
  }

  function handleImportComplete() {
    setImportingSourceId(null)
    setCurrentJobId(null)
    fetchSources()
  }

  async function handleToggleEnabled(source: MediaSource) {
    try {
      await updateMediaSource(source.id, { label: source.label, enabled: !source.enabled })
      setConfirmAction(null)
      fetchSources()
    } catch {
      setError('操作失敗，請重試')
    }
  }

  async function handleDelete(source: MediaSource) {
    try {
      await deleteMediaSource(source.id)
      setConfirmAction(null)
      fetchSources()
    } catch {
      setError('刪除失敗，請重試')
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 flex flex-col">
      <Header searchQuery={searchQuery} onSearch={() => {}} />

      <div className="flex-1 p-6">
        {/* Top bar */}
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl font-bold text-white">媒體來源管理</h1>
          <button
            onClick={() => setShowCreate(true)}
            className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-2 rounded transition-colors"
          >
            + 新增來源
          </button>
        </div>

        {/* Error banner */}
        {error && (
          <div className="bg-red-900/30 border border-red-800 rounded-lg p-3 mb-4 flex items-center justify-between">
            <span className="text-sm text-red-400">{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-300 text-sm ml-4">✕</button>
          </div>
        )}

        {/* Content */}
        {loading ? (
          <div className="text-gray-500 text-center py-20">載入中...</div>
        ) : error && sources.length === 0 ? (
          <div className="text-center py-20">
            <p className="text-gray-400 mb-4">無法載入媒體來源</p>
            <button onClick={fetchSources} className="text-indigo-400 hover:text-indigo-300 text-sm">重試</button>
          </div>
        ) : sources.length === 0 ? (
          <div className="text-gray-500 text-center py-20">
            尚未設定媒體來源，請點擊右上角新增
          </div>
        ) : (
          <div className="space-y-4">
            {sources.map((source) => (
              <div
                key={source.id}
                className={`rounded-lg p-4 transition-colors ${
                  source.enabled
                    ? 'bg-gray-900 border border-gray-800'
                    : 'bg-gray-900/50 border border-gray-800/50 opacity-60'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <h3 className="text-white font-medium truncate">{source.label}</h3>
                      <span className={`text-xs px-2 py-0.5 rounded ${
                        source.enabled
                          ? 'bg-green-900/50 text-green-400'
                          : 'bg-gray-800 text-gray-500'
                      }`}>
                        {source.enabled ? '啟用' : '已停用'}
                      </span>
                    </div>
                    <p className="text-sm text-gray-500 font-mono truncate">{source.mount_path}</p>
                    <p className="text-sm text-gray-400 mt-1">{source.video_count} 部影片</p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0 ml-4">
                    <button
                      onClick={() => setEditingSource(source)}
                      className="text-xs text-gray-400 hover:text-white px-2 py-1 rounded transition-colors"
                    >
                      編輯
                    </button>
                    <button
                      onClick={() => handleImport(source)}
                      disabled={importingSourceId !== null}
                      className="text-xs text-indigo-400 hover:text-indigo-300 disabled:opacity-50 disabled:cursor-not-allowed px-2 py-1 rounded transition-colors"
                    >
                      掃描匯入
                    </button>
                    <button
                      onClick={() => setConfirmAction({
                        type: source.enabled ? 'disable' : 'enable',
                        source,
                      })}
                      className={`text-xs px-2 py-1 rounded transition-colors ${
                        source.enabled
                          ? 'text-yellow-400 hover:text-yellow-300'
                          : 'text-green-400 hover:text-green-300'
                      }`}
                    >
                      {source.enabled ? '停用' : '啟用'}
                    </button>
                    <button
                      onClick={() => setConfirmAction({ type: 'delete', source })}
                      className="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded transition-colors"
                    >
                      刪除
                    </button>
                  </div>
                </div>

                {/* Import progress inline */}
                {importingSourceId === source.id && currentJobId && (
                  <ImportProgress jobId={currentJobId} onComplete={handleImportComplete} />
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create Modal */}
      {showCreate && (
        <CreateSourceModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); fetchSources() }}
        />
      )}

      {/* Edit Modal */}
      {editingSource && (
        <EditSourceModal
          source={editingSource}
          onClose={() => setEditingSource(null)}
          onSaved={() => { setEditingSource(null); fetchSources() }}
        />
      )}

      {/* Confirm Dialog */}
      {confirmAction && (
        <ConfirmDialog
          action={confirmAction}
          onConfirm={() => {
            if (confirmAction.type === 'delete') {
              handleDelete(confirmAction.source)
            } else {
              handleToggleEnabled(confirmAction.source)
            }
          }}
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}

/* ---------- Sub-components (same file, not exported) ---------- */

function CreateSourceModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [label, setLabel] = useState('')
  const [mountPath, setMountPath] = useState('/mnt/host/')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await createMediaSource({ label: label.trim(), mount_path: mountPath.trim() })
      onCreated()
    } catch (err: unknown) {
      const axiosErr = err as { response?: { status?: number; data?: { message?: string } } }
      const status = axiosErr?.response?.status
      const message = axiosErr?.response?.data?.message
      if (status === 400) {
        setError(message || '路徑驗證失敗，請確認路徑正確')
      } else if (status === 409) {
        setError('此路徑已被其他來源使用')
      } else {
        setError('建立失敗，請重試')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-4">新增媒體來源</h2>
        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-400 mb-1">名稱</label>
          <input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="例如：D槽影片"
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-3"
            disabled={submitting}
            required
          />
          <label className="block text-sm text-gray-400 mb-1">路徑</label>
          <input
            value={mountPath}
            onChange={(e) => setMountPath(e.target.value)}
            placeholder="/mnt/host/"
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-1 font-mono"
            disabled={submitting}
            required
          />
          <p className="text-xs text-gray-600 mb-3">路徑必須在 /mnt/host/ 下，對應 Docker 掛載的磁碟目錄</p>

          {error && (
            <div className="bg-red-900/30 border border-red-800 rounded p-2 mb-3">
              <p className="text-sm text-red-400">{error}</p>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button type="button" onClick={onClose} disabled={submitting} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
            <button
              type="submit"
              disabled={submitting || !label.trim() || !mountPath.trim()}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded"
            >
              {submitting ? '建立中...' : '建立'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function EditSourceModal({ source, onClose, onSaved }: { source: MediaSource; onClose: () => void; onSaved: () => void }) {
  const [label, setLabel] = useState(source.label)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await updateMediaSource(source.id, { label: label.trim(), enabled: source.enabled })
      onSaved()
    } catch {
      setError('更新失敗，請重試')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-4">編輯媒體來源</h2>
        <form onSubmit={handleSubmit}>
          <label className="block text-sm text-gray-400 mb-1">名稱</label>
          <input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-3"
            disabled={submitting}
            required
          />
          <label className="block text-sm text-gray-400 mb-1">路徑（不可修改）</label>
          <input
            value={source.mount_path}
            disabled
            className="w-full bg-gray-800/50 text-gray-500 text-sm rounded px-3 py-2 mb-1 font-mono cursor-not-allowed"
          />
          <p className="text-xs text-gray-600 mb-3">路徑不可修改。如需變更路徑，請刪除此來源後重新建立。</p>

          {error && (
            <div className="bg-red-900/30 border border-red-800 rounded p-2 mb-3">
              <p className="text-sm text-red-400">{error}</p>
            </div>
          )}

          <div className="flex justify-end gap-2">
            <button type="button" onClick={onClose} disabled={submitting} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
            <button
              type="submit"
              disabled={submitting || !label.trim()}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded"
            >
              {submitting ? '儲存中...' : '儲存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ConfirmDialog({ action, onConfirm, onCancel }: {
  action: { type: 'disable' | 'enable' | 'delete'; source: MediaSource }
  onConfirm: () => void
  onCancel: () => void
}) {
  const messages: Record<string, string> = {
    disable: `停用後，此來源下的 ${action.source.video_count} 部影片將無法播放。確定要停用嗎？`,
    enable: `確定要重新啟用「${action.source.label}」嗎？`,
    delete: action.source.video_count > 0
      ? `此來源下有 ${action.source.video_count} 部影片記錄，刪除後這些影片將保留在資料庫中但無法播放。確定要刪除嗎？`
      : `確定要刪除「${action.source.label}」嗎？`,
  }

  const isDanger = action.type === 'delete'

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={onCancel}>
      <div className="bg-gray-900 rounded-lg p-6 w-full max-w-sm" onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-white mb-2">
          {action.type === 'delete' ? '確認刪除' : action.type === 'disable' ? '確認停用' : '確認啟用'}
        </h2>
        <p className="text-sm text-gray-400 mb-4">{messages[action.type]}</p>
        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
          <button
            onClick={onConfirm}
            className={`text-white text-sm px-4 py-1.5 rounded ${
              isDanger ? 'bg-red-600 hover:bg-red-500' : 'bg-indigo-600 hover:bg-indigo-500'
            }`}
          >
            確認
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd d:/Vaultflix/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/admin/MediaSourcePage.tsx
git commit -m "feat: add MediaSourcePage with card layout, CRUD modals, import integration"
```

---

### Task 8: Backend + Frontend — Run all tests and verify

**Files:** None (verification only)

- [ ] **Step 1: Run Go tests**

Run: `cd d:/Vaultflix && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Run frontend type check**

Run: `cd d:/Vaultflix/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit if any fixes were needed**

---

### Task 9: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update project overview**

Find and replace the project overview line that says "影片存放在 MinIO" or similar. Update to:

> Vaultflix 是一個 Go + React 的個人影片管理與串流平台。後端為 Go API Server，前端為 React SPA，影片保留在本機磁碟，系統直接讀取串流；MinIO 僅存縮圖與預覽，metadata 存於 PostgreSQL。

- [ ] **Step 2: Add WebSocket specification section**

Add after the "Docker 規範" section:

```markdown
---

## WebSocket 規範

### Hub Pattern

- WebSocket 連線管理集中在 `internal/websocket/` package
- `Hub` struct 透過 channel 序列化所有 client 註冊/移除/訊息推送，避免 map 並發存取
- 支援 per-user targeted message（`SendToUser`）和全域 broadcast
- 同一使用者可有多個連線（多分頁）

### 訊息協議

```go
type Message struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}
```

已定義的 type：
- `import_progress` — 逐檔匯入進度
- `import_complete` — 匯入完成（含最終 ImportJob）
- `import_error` — 匯入致命錯誤
- `notification` — 通用通知
- `ping` — 心跳

### Notifier Interface

跨層依賴透過 `Notifier` interface 解耦，定義在 `internal/websocket/hub.go`：

```go
type Notifier interface {
    SendToUser(userID string, msg *Message)
    Broadcast(msg *Message)
}
```

Service 層（如 `ImportService`）依賴此 interface，不直接依賴 `Hub` struct。

### 前端重連策略

- 使用 exponential backoff：初始 1s，每次 ×2，上限 30s
- 重連次數上限 20 次，超過停止重連
- 用 `useRef` 追蹤重連次數，避免 re-render 導致計數重置
- 心跳間隔 50s，保持連線活躍

---

## 路徑安全規範

### 基本原則

所有使用者可控的檔案路徑必須經過驗證，防止路徑穿越攻擊。

### 標準 Pattern

```go
// 1. 定義允許的前綴
const AllowedMountPrefix = "/mnt/host/"

// 2. Clean + prefix 檢查
cleaned := filepath.Clean(path)
if !strings.HasPrefix(cleaned, strings.TrimSuffix(prefix, string(filepath.Separator))) {
    return model.ErrPathNotAllowed
}

// 3. 拒絕 Clean 後與原始路徑不一致的輸入（含 .., //, 結尾斜線等）
if cleaned != path {
    return model.ErrPathNotAllowed
}

// 4. 驗證路徑存在且為目錄
info, err := os.Stat(cleaned)
```

### Sentinel Errors

- `model.ErrPathNotAllowed` — 路徑不在允許前綴內，或包含非法組件
- `model.ErrPathNotExist` — 路徑不存在於檔案系統
```

- [ ] **Step 3: Update Docker specification**

Add under the Docker 規範 section:

```markdown
### 磁碟層級掛載策略

影片檔案保留在本機磁碟，透過 Docker volume mount 以唯讀模式掛載整個磁碟：

```yaml
volumes:
  - D:/:/mnt/host/D:ro
  - E:/:/mnt/host/E:ro
```

- 掛載點統一在 `/mnt/host/<磁碟代號>/` 下
- 使用 `:ro`（read-only）防止容器內程式修改原始檔案
- Media source 的 `mount_path` 必須在 `/mnt/host/` 前綴下
- 新增磁碟時只需在 `docker-compose.yml` 加一行 volume mount + 在 Admin UI 新增 media source
```

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with WebSocket, path security, and Docker mount specs"
```

---

### Task 10: Update ROADMAP.md

**Files:**
- Modify: `ROADMAP.md`

- [ ] **Step 1: Mark completed items**

Replace these lines:

```
- [ ] **匯入改為非同步處理**
```
→
```
- [x] **匯入改為非同步處理** — Phase 12 已完成：背景 worker + WebSocket 進度推送
```

```
- [ ] **匯入目錄路徑驗證**
```
→
```
- [x] **匯入目錄路徑驗證** — Phase 8 已完成：AllowedMountPrefix + filepath.Clean 防護
```

```
- [ ] **Presigned URL 續期**
```
→
```
- [x] **Presigned URL 續期** — Phase 10 已完成：影片改為 API stream endpoint，不再使用 presigned URL
```

In the 未來功能 section:
```
- [ ] **匯入進度即時回報**
```
→
```
- [x] **匯入進度即時回報** — Phase 12 已完成：WebSocket 即時推送 + 前端進度元件
```

- [ ] **Step 2: Commit**

```bash
git add ROADMAP.md
git commit -m "docs: mark Phase 8-12 completed items in ROADMAP.md"
```

---

### Task 11: Update README.md

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update architecture overview**

Replace the architecture diagram (lines 7-15):

```markdown
## Architecture Overview

```
React SPA (localhost:3000)
    |
    |-- API requests --> Nginx reverse proxy --> Go API Server (:8080) --> PostgreSQL
    |                                               |
    |                                               +--> MinIO (thumbnails only)
    |                                               +--> Local disk (video streaming via API)
    |
    +-- WebSocket --> Go API Server (real-time import progress)
```

**Key design decision**: Video files stay on local disk. The Go API server streams video bytes directly via `http.ServeFile`, which natively handles HTTP Range Requests for seeking. MinIO is used only for thumbnails. Import progress is pushed in real-time via WebSocket.
```

- [ ] **Step 2: Update Quick Start step 2**

Replace step 2 (lines 71-79):

```markdown
### 2. Configure disk mounts

Edit `docker-compose.yml` and mount your video disk(s) as read-only volumes in the `vaultflix-api` service:

```yaml
  vaultflix-api:
    volumes:
      # ...
      - D:/:/mnt/host/D:ro    # Mount D: drive
      - E:/:/mnt/host/E:ro    # Mount E: drive (optional)
```

Each mounted disk will be accessible under `/mnt/host/<drive>/` inside the container.
```

- [ ] **Step 3: Update Quick Start step 5**

Replace step 5 (lines 99-111):

```markdown
### 5. Add media sources and import videos

Open **http://localhost:3000**, log in as admin, then navigate to **媒體來源** (Media Sources) in the top navigation bar.

1. Click **+ 新增來源** to add a media source (e.g., label: "D槽影片", path: `/mnt/host/D/Videos`)
2. Click **掃描匯入** on the source card to start importing
3. Watch the real-time progress bar as videos are scanned, metadata extracted, and thumbnails generated
```

- [ ] **Step 4: Update API Overview — add new endpoint tables**

Add after the Tags table (after line 251):

```markdown
### Watch History

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/watch-history` | Save playback progress | viewer+ |
| GET | `/api/watch-history` | List watch history | viewer+ |

### Favorites

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/favorites` | List favorites | viewer+ |
| POST | `/api/favorites` | Add favorite | viewer+ |
| DELETE | `/api/favorites/:videoId` | Remove favorite | viewer+ |

### Media Sources

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/media-sources` | List all media sources (with video counts) | admin |
| POST | `/api/media-sources` | Create a media source | admin |
| PUT | `/api/media-sources/:id` | Update media source label/enabled | admin |
| DELETE | `/api/media-sources/:id` | Delete a media source | admin |

### Import Jobs

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/videos/import` | Start async video import | admin |
| GET | `/api/import-jobs/active` | Get currently running import job | admin |
| GET | `/api/import-jobs/:id` | Get import job by ID | admin |

### WebSocket

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/ws` | WebSocket connection (import progress, notifications) | Any |
```

- [ ] **Step 5: Update Features section**

Update the Implemented list to include new features and clean up Planned:

Under "### Implemented", add:
```markdown
- **Media Source Management**: Admin UI for managing media sources (CRUD) with real-time import progress
- **Watch History**: Track and resume video playback progress
- **Favorites**: Bookmark videos for quick access
- **Daily Recommendations**: Admin-curated daily video picks
- **User Management**: Admin UI for user CRUD, enable/disable, password reset
- **WebSocket**: Real-time import progress notifications
```

Under "### Planned", remove items now implemented and keep:
```markdown
### Planned

- Meilisearch full-text search
- LLM-powered semantic search
- Automatic tagging
- Mobile client
```

- [ ] **Step 6: Commit**

```bash
git add README.md
git commit -m "docs: update README.md for Phase 8-13 architecture and features"
```

---

### Task 12: Final verification

- [ ] **Step 1: Run Go tests**

Run: `cd d:/Vaultflix && go test ./... -v`
Expected: All tests PASS

- [ ] **Step 2: Run frontend type check**

Run: `cd d:/Vaultflix/web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Review acceptance checklist**

Verify each item from the spec acceptance criteria:

**管理頁面**
- `/admin/media-sources` 頁面可正常載入
- 顯示所有 media source + 影片數量
- 空狀態顯示引導文字
- viewer 角色無法存取（AdminRoute guard）
- admin 角色在 Header 可看到管理入口連結

**Source CRUD**
- 新增 source modal with label + mount_path
- 路徑錯誤時 modal 內顯示錯誤
- 編輯 source 只可改 label
- 停用/啟用 with 確認對話框
- 停用的 source 有灰底 + 降透明度
- 刪除 with 確認（有影片時警告）

**匯入整合**
- 掃描匯入 → 進度即時顯示（inline on card）
- 匯入中所有按鈕停用
- 完成後顯示摘要
- 頁面重整恢復進度

**錯誤處理**
- API 失敗顯示錯誤訊息
- 列表載入失敗顯示重試按鈕

**收尾**
- CLAUDE.md 已更新
- ROADMAP.md 已更新
- README.md 已更新
- `go test ./...` 全部通過
