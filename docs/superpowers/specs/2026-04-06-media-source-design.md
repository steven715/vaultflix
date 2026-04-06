# Phase 8：Media Source 管理（資料層 + API）

## 背景

Vaultflix 儲存架構重構的第一步：從「影片上傳至 MinIO」改為「影片保留在本機磁碟」。本 phase 建立 media source 管理機制，讓 Admin 可透過 API 指定本機影片目錄。

Docker Compose 採磁碟層級掛載（`D:/:/mnt/host/D:ro`），應用層透過 `media_sources` table 管理細粒度路徑，所有路徑必須在 `/mnt/host/` prefix 內。

---

## 資料層

### Migration 009

**up.sql：**

```sql
CREATE TABLE media_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(255) NOT NULL,
    mount_path VARCHAR(1024) NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE videos
    ADD COLUMN source_id UUID REFERENCES media_sources(id) ON DELETE SET NULL,
    ADD COLUMN file_path VARCHAR(2048);

COMMENT ON COLUMN videos.source_id IS '影片所屬的 media source，source 刪除時設為 NULL';
COMMENT ON COLUMN videos.file_path IS '相對於 media source mount_path 的檔案路徑';
COMMENT ON COLUMN videos.minio_object_key IS '舊欄位，重構完成後不再使用，僅用於遷移期間';
```

**down.sql：**

```sql
ALTER TABLE videos DROP COLUMN IF EXISTS file_path;
ALTER TABLE videos DROP COLUMN IF EXISTS source_id;
DROP TABLE IF EXISTS media_sources;
```

---

## Model

### MediaSource struct（`internal/model/media_source.go`）

| 欄位 | Go 型別 | JSON tag |
|------|---------|----------|
| ID | string | `id` |
| Label | string | `label` |
| MountPath | string | `mount_path` |
| Enabled | bool | `enabled` |
| CreatedAt | time.Time | `created_at` |
| UpdatedAt | time.Time | `updated_at` |

### 新增 Sentinel Errors（`internal/model/errors.go`）

- `ErrPathNotAllowed` — 路徑不在允許的 mount prefix 內，或包含 `..` 等不合法元素
- `ErrPathNotExist` — 路徑在檔案系統中不存在

---

## Repository

### Interface（`internal/repository/media_source_repo.go`）

```go
type MediaSourceRepository interface {
    List(ctx context.Context) ([]model.MediaSource, error)
    FindByID(ctx context.Context, id string) (*model.MediaSource, error)
    Create(ctx context.Context, source *model.MediaSource) error
    Update(ctx context.Context, source *model.MediaSource) error
    Delete(ctx context.Context, id string) error
}
```

行為約定：
- `FindByID` 找不到 → `model.ErrNotFound`
- `Create` mount_path 重複 → `model.ErrAlreadyExists`（PG error code `23505`）
- `Update` 同時更新 `updated_at`；找不到 → `model.ErrNotFound`
- `Delete` 找不到 → `model.ErrNotFound`

---

## Service

### MediaSourceService（`internal/service/media_source_service.go`）

```go
type MediaSourceService struct {
    repo        repository.MediaSourceRepository
    mountPrefix string
}
```

- Constructor：`NewMediaSourceService(repo, mountPrefix)`
- Production 初始化時 `mountPrefix` 傳入 `AllowedMountPrefix`（package-level const = `"/mnt/host/"`）

### 設計決策

1. **`ValidateMountPath` 作為 service method**，不獨立為 `pathutil` package。理由：目前專案無 utility package，唯一未來消費者（Phase 9 import service）可直接依賴 media source service。
2. **`mountPrefix` 作為 service field**，測試時注入 `os.MkdirTemp` 臨時路徑，不額外抽象 `os.Stat`。

### 方法行為

| 方法 | 行為 |
|------|------|
| `List` | 回傳所有 media source |
| `GetByID` | 找不到 → wrap `ErrNotFound` |
| `Create` | `ValidateMountPath` → `repo.Create` |
| `Update` | 只允許 `label` + `enabled`，不可改 `mount_path` |
| `Delete` | `repo.Delete`，DB `ON DELETE SET NULL` 處理關聯 |

### ValidateMountPath 邏輯

1. `filepath.Clean(path)` 正規化
2. 檢查 cleaned path 是否以 `s.mountPrefix` 開頭 → 否則 `ErrPathNotAllowed`
3. 檢查 `cleaned != path` → 若不同代表含 `..` 或多餘 `/`，回傳 `ErrPathNotAllowed`
4. `os.Stat(cleaned)` → 不存在回傳 `ErrPathNotExist`
5. 檢查是否為目錄 → 否則 `ErrPathNotAllowed`

---

## Handler

### Endpoints（`internal/handler/media_source_handler.go`）

| Method | Path | 行為 | 成功 Status |
|--------|------|------|-------------|
| GET | `/api/media-sources` | 列出所有 | 200 |
| POST | `/api/media-sources` | 建立 | 201 |
| PUT | `/api/media-sources/:id` | 更新 label/enabled | 200 |
| DELETE | `/api/media-sources/:id` | 刪除 | 204 |

### Request Body

**POST：**
```json
{ "label": "D槽影片", "mount_path": "/mnt/host/D/Videos" }
```

**PUT：**
```json
{ "label": "新名稱", "enabled": false }
```

### Error Mapping

| 情境 | HTTP Status | Error code |
|------|-------------|------------|
| 路徑不合法（prefix 外、含 `..`、非目錄） | 400 | `bad_request` |
| 路徑不存在 | 400 | `bad_request` |
| mount_path 重複 | 409 | `already_exists` |
| media source 不存在 | 404 | `not_found` |
| viewer 角色存取 | 403 | （由 Casbin middleware 處理） |

---

## RBAC

Casbin `policy.csv` 新增（admin only，viewer 無權限）：

```
p, admin, /api/media-sources, GET
p, admin, /api/media-sources, POST
p, admin, /api/media-sources/:id, PUT
p, admin, /api/media-sources/:id, DELETE
```

現有 `p, admin, /api/*, GET/POST/PUT/DELETE` 通配規則已涵蓋所有 `/api/media-sources*` 路徑，**不需要額外新增 admin 規則**。只需確認 viewer 沒有相關權限（目前 policy.csv 中 viewer 無 media-sources 項目，符合預期）。

---

## DI 注入（`cmd/server/main.go`）

按既有順序：
1. `mediaSourceRepo := repository.NewMediaSourceRepository(pool)`
2. `mediaSourceService := service.NewMediaSourceService(mediaSourceRepo, service.AllowedMountPrefix)`
3. `mediaSourceHandler := handler.NewMediaSourceHandler(mediaSourceService)`
4. 在 `api` group 中註冊四條路由

---

## 測試

### Mock（`internal/mock/media_source_repo_mock.go`）

Function pointer struct，與既有 mock pattern 一致。

### Service 測試

- 用 `os.MkdirTemp` 建立臨時目錄結構
- `mountPrefix` 注入臨時路徑
- Table-driven tests 覆蓋：
  - Create 成功 / path outside prefix / path 含 `..` / path 不存在 / path 非目錄 / 重複 path
  - Update 成功 / not found
  - Delete 成功 / not found

### Handler 測試

- 各 endpoint 正常路徑
- Viewer 角色 → 403
- 缺少必要欄位 → 400

---

## 不在此 Phase 範圍

- 不修改 import 流程（Phase 9）
- 不修改 video 串流邏輯（Phase 10）
- 不建立前端 Admin UI（Phase 13）
- 不修改 docker-compose.yml
