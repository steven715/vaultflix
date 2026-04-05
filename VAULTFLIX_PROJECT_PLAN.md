# Vaultflix — 個人影片管理平台開發計畫

## 專案概述

Vaultflix 是一個個人影片管理與串流平台，將本地硬碟中的影片內容組織為可瀏覽、可搜尋、可管理的 Web 應用。具備使用者認證、權限控制、影片串流、標籤分類、觀看記錄、每日推薦等功能。

**定位**：個人專案，以單人使用為主，但架構上預留多使用者擴展能力。

---

## Tech Stack

| 層級 | 技術選型 | 說明 |
|------|----------|------|
| 前端 | React + TypeScript | SPA，未來可擴展至行動端 (React Native 或獨立 APP) |
| 後端 | Go (Gin 或 Echo) | API Server，處理認證、metadata CRUD、pre-signed URL 產生 |
| 資料庫 | PostgreSQL 16 | 結構化 metadata、使用者資料、觀看記錄 |
| 物件儲存 | MinIO | 影片檔案與縮圖儲存，支援 pre-signed URL 與 HTTP Range Request |
| 認證 | JWT + bcrypt（自建） | 使用者登入、token 驗證 |
| 授權 | Casbin | RBAC 權限引擎，admin / viewer 角色區分 |
| 容器化 | Docker Compose | 開發與部署統一環境 |

---

## 部署架構

### Docker Compose 服務組成

```yaml
services:
  vaultflix-api:      # Go API Server（multi-stage build: golang → alpine）
  vaultflix-web:      # React 前端（build 後用 nginx:alpine 託管靜態檔）
  postgres:           # postgres:16-alpine 官方 image
  minio:              # minio/minio 官方 image
```

### 重要架構決策：影片串流不經過 API Server

```
React 前端
    │
    ├─ API 請求 ──→ Go API Server ──→ PostgreSQL
    │                    │
    │                    └─→ MinIO（產生 pre-signed URL）
    │
    └─ 影片串流 ──→ MinIO（前端直連，不經過 Go）
```

影片的 bytes 不經過 Go 服務。流程為：
1. 前端請求影片詳情 → Go 服務做權限檢查
2. Go 服務向 MinIO 產生帶時效的 pre-signed URL
3. 回傳 URL 給前端
4. 前端 `<video>` 標籤直接從 MinIO 拉影片流
5. MinIO 原生處理 HTTP Range Request（支援進度條拖動 seek）

### Go 服務 Dockerfile（multi-stage build）

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o vaultflix-api ./cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ffmpeg  # ffprobe + ffmpeg 用於影片 metadata 擷取與縮圖產生
COPY --from=builder /app/vaultflix-api /usr/local/bin/
EXPOSE 8080
CMD ["vaultflix-api"]
```

> **為什麼不用 Ubuntu 24 當 base image？**
> Go 編譯出靜態二進位檔，alpine 就夠跑了，image 約 20-30MB。Ubuntu base image 約 70MB+，在個人專案中沒有實質收益。Docker host（跑 Docker 的機器）用 Ubuntu 24 即可。

---

## 資料結構定義

### ER 關係總覽

```
users 1──N watch_history N──1 videos
users N──N videos（透過 favorites）
videos N──N tags（透過 video_tags）
daily_recommendations N──1 videos
```

### 完整 Schema

```sql
-- ========================================
-- 使用者管理
-- ========================================

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(20) NOT NULL DEFAULT 'viewer',  -- 'admin' | 'viewer'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ========================================
-- 影片 Metadata
-- ========================================

CREATE TABLE videos (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title             VARCHAR(500) NOT NULL,
    description       TEXT DEFAULT '',
    minio_object_key  VARCHAR(1000) NOT NULL,      -- MinIO 中的影片檔案路徑
    thumbnail_key     VARCHAR(1000) DEFAULT '',     -- MinIO 中的縮圖路徑
    duration_seconds  INT DEFAULT 0,
    resolution        VARCHAR(20) DEFAULT '',       -- e.g. "1920x1080"
    file_size_bytes   BIGINT DEFAULT 0,
    mime_type         VARCHAR(100) DEFAULT '',
    original_filename VARCHAR(500) DEFAULT '',      -- 原始檔名，方便追溯
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_videos_created_at ON videos(created_at DESC);
CREATE INDEX idx_videos_title ON videos USING gin(to_tsvector('simple', title));

-- ========================================
-- 標籤系統
-- ========================================

CREATE TABLE tags (
    id        SERIAL PRIMARY KEY,
    name      VARCHAR(100) UNIQUE NOT NULL,
    category  VARCHAR(50) NOT NULL DEFAULT 'custom'  -- genre | actor | studio | custom
);

CREATE INDEX idx_tags_category ON tags(category);

CREATE TABLE video_tags (
    video_id  UUID REFERENCES videos(id) ON DELETE CASCADE,
    tag_id    INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (video_id, tag_id)
);

-- ========================================
-- 使用者互動
-- ========================================

CREATE TABLE watch_history (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id          UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    progress_seconds  INT DEFAULT 0,          -- 上次觀看到哪裡（支援續播）
    completed         BOOLEAN DEFAULT FALSE,
    watched_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_history_user ON watch_history(user_id, watched_at DESC);
CREATE UNIQUE INDEX idx_watch_history_user_video ON watch_history(user_id, video_id);

CREATE TABLE favorites (
    user_id    UUID REFERENCES users(id) ON DELETE CASCADE,
    video_id   UUID REFERENCES videos(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, video_id)
);

-- ========================================
-- 每日推薦（Admin 手動排定）
-- ========================================

CREATE TABLE daily_recommendations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id        UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    recommend_date  DATE NOT NULL,
    sort_order      INT DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (video_id, recommend_date)
);

CREATE INDEX idx_recommendations_date ON daily_recommendations(recommend_date DESC);
```

---

## 認證與授權設計

### 認證層（Authentication）— JWT + bcrypt

- 使用者註冊時，密碼以 bcrypt hash 儲存
- 登入成功後簽發 JWT（含 user_id、role、過期時間）
- 所有 `/api/*` 路由掛 JWT middleware 驗證
- Token 過期時間建議 24 小時（個人專案不需要 refresh token 機制）

### 授權層（Authorization）— Casbin RBAC

Casbin model 定義（`model.conf`）：

```ini
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act
```

Casbin policy 定義（`policy.csv`）：

```csv
# Admin 可做一切操作
p, admin, /api/*, GET
p, admin, /api/*, POST
p, admin, /api/*, PUT
p, admin, /api/*, DELETE

# Viewer 只能讀取影片與操作自己的互動資料
p, viewer, /api/videos, GET
p, viewer, /api/videos/:id, GET
p, viewer, /api/tags, GET
p, viewer, /api/watch-history, GET
p, viewer, /api/watch-history, POST
p, viewer, /api/watch-history, PUT
p, viewer, /api/favorites, GET
p, viewer, /api/favorites, POST
p, viewer, /api/favorites, DELETE
p, viewer, /api/recommendations/today, GET
```

**分界線**：JWT middleware 回答「你是誰」，Casbin middleware 回答「你能做什麼」。兩者獨立運作、順序執行。

---

## API 端點設計

### 認證

| Method | Path | 說明 | 權限 |
|--------|------|------|------|
| POST | `/api/auth/register` | 註冊帳號 | Public（可透過 config 關閉） |
| POST | `/api/auth/login` | 登入，回傳 JWT | Public |

### 影片

| Method | Path | 說明 | 權限 |
|--------|------|------|------|
| GET | `/api/videos` | 影片列表（分頁、篩選、搜尋） | viewer+ |
| GET | `/api/videos/:id` | 影片詳情 + pre-signed URL | viewer+ |
| POST | `/api/videos/import` | 觸發影片匯入掃描 | admin |
| PUT | `/api/videos/:id` | 更新影片 metadata | admin |
| DELETE | `/api/videos/:id` | 刪除影片 | admin |

### 標籤

| Method | Path | 說明 | 權限 |
|--------|------|------|------|
| GET | `/api/tags` | 標籤列表（可按 category 篩選） | viewer+ |
| POST | `/api/tags` | 建立標籤 | admin |
| POST | `/api/videos/:id/tags` | 為影片加標籤 | admin |
| DELETE | `/api/videos/:id/tags/:tagId` | 移除影片標籤 | admin |

### 使用者互動

| Method | Path | 說明 | 權限 |
|--------|------|------|------|
| GET | `/api/watch-history` | 我的觀看記錄 | viewer+ |
| POST | `/api/watch-history` | 記錄/更新觀看進度 | viewer+ |
| GET | `/api/favorites` | 我的收藏 | viewer+ |
| POST | `/api/favorites` | 加入收藏 | viewer+ |
| DELETE | `/api/favorites/:videoId` | 取消收藏 | viewer+ |

### 每日推薦

| Method | Path | 說明 | 權限 |
|--------|------|------|------|
| GET | `/api/recommendations/today` | 取得今日推薦 | viewer+ |
| POST | `/api/recommendations` | 設定推薦影片 | admin |
| DELETE | `/api/recommendations/:id` | 移除推薦 | admin |

---

## Go 專案結構

```
vaultflix/
├── cmd/
│   └── server/
│       └── main.go                 # 程式進入點
├── internal/
│   ├── config/
│   │   └── config.go               # 環境變數 / 設定檔載入
│   ├── middleware/
│   │   ├── auth.go                 # JWT 驗證 middleware
│   │   └── rbac.go                 # Casbin 權限檢查 middleware
│   ├── handler/                    # HTTP handler（對應各 API 端點）
│   │   ├── auth_handler.go
│   │   ├── video_handler.go
│   │   ├── tag_handler.go
│   │   ├── history_handler.go
│   │   ├── favorite_handler.go
│   │   └── recommendation_handler.go
│   ├── service/                    # 業務邏輯層
│   │   ├── auth_service.go
│   │   ├── video_service.go
│   │   ├── import_service.go       # 影片掃描匯入邏輯
│   │   └── minio_service.go        # MinIO 操作（上傳、pre-signed URL）
│   ├── repository/                 # 資料存取層（SQL 查詢）
│   │   ├── user_repo.go
│   │   ├── video_repo.go
│   │   ├── tag_repo.go
│   │   ├── history_repo.go
│   │   ├── favorite_repo.go
│   │   └── recommendation_repo.go
│   └── model/                      # 資料結構定義
│       ├── user.go
│       ├── video.go
│       ├── tag.go
│       └── response.go             # 統一 API 回應格式
├── migrations/                     # SQL migration 檔案
│   ├── 001_create_users.up.sql
│   ├── 001_create_users.down.sql
│   ├── 002_create_videos.up.sql
│   └── ...
├── casbin/
│   ├── model.conf
│   └── policy.csv
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

---

## 開發階段與執行清單

### Phase 1：專案骨架與基礎設施

- [ ] 初始化 Go module：`go mod init github.com/<user>/vaultflix`
- [ ] 建立上述專案目錄結構
- [ ] 撰寫 `docker-compose.yml`（PostgreSQL 16 + MinIO + Go API）
- [ ] 設定 MinIO：建立 `vaultflix-videos` 和 `vaultflix-thumbnails` 兩個 bucket
- [ ] 安裝 golang-migrate，撰寫所有 migration 檔案
- [ ] 執行 migration 確認 schema 建立成功
- [ ] 建立 `config.go` 讀取環境變數（DB DSN、MinIO endpoint、JWT secret 等）

### Phase 2：認證與授權系統

- [ ] 實作 `POST /api/auth/register`（bcrypt hash 密碼、寫入 users 表）
- [ ] 實作 `POST /api/auth/login`（驗證密碼、簽發 JWT）
- [ ] 實作 JWT middleware（解析 token、注入 user context）
- [ ] 整合 Casbin（載入 model.conf + policy.csv）
- [ ] 實作 RBAC middleware（從 JWT 取 role，向 Casbin 查詢權限）
- [ ] 寫一支初始化腳本：首次啟動自動建立 admin 帳號

### Phase 3：影片匯入系統

- [ ] 實作 `import_service.go`：掃描指定目錄，遞迴找出影片檔（mp4, mkv, avi, wmv, mov）
- [ ] 對每個影片呼叫 ffprobe 擷取 metadata（時長、解析度、檔案大小）
- [ ] 對每個影片呼叫 ffmpeg 產生縮圖（取影片中段某幀）
- [ ] 上傳影片檔到 MinIO `vaultflix-videos` bucket
- [ ] 上傳縮圖到 MinIO `vaultflix-thumbnails` bucket
- [ ] 寫入 `videos` 表（含 minio_object_key、thumbnail_key、metadata）
- [ ] 實作 `POST /api/videos/import` 觸發匯入（admin only）
- [ ] 匯入過程加入冪等檢查：若 original_filename + file_size 已存在則跳過

### Phase 4：影片瀏覽 API

- [ ] 實作 `GET /api/videos`（分頁、排序、按標籤篩選、標題搜尋）
- [ ] 實作 `GET /api/videos/:id`（回傳影片詳情 + MinIO pre-signed URL，URL 有效期 2 小時）
- [ ] 實作 `GET /api/tags`（列出所有標籤，可按 category 篩選）
- [ ] 實作 `POST /api/tags`、`POST /api/videos/:id/tags`、`DELETE /api/videos/:id/tags/:tagId`
- [ ] 實作 `PUT /api/videos/:id`（更新標題、描述）
- [ ] 實作 `DELETE /api/videos/:id`（同時刪除 MinIO 物件）

### Phase 5：React 前端初版

- [ ] 初始化 React + TypeScript 專案（Vite）
- [ ] 登入頁面（表單 → 呼叫 login API → 儲存 JWT）
- [ ] 影片網格瀏覽頁（縮圖卡片，分頁載入）
- [ ] 標籤篩選側欄 / 搜尋框
- [ ] 影片播放頁（HTML5 `<video>` 標籤 + pre-signed URL）
- [ ] 路由保護（未登入重導至登入頁）
- [ ] 前端 Dockerfile（build → nginx:alpine 託管）

### Phase 6：使用者互動功能

- [ ] 實作觀看進度記錄 API（`POST /api/watch-history`）
- [ ] 前端播放器定期（每 10 秒 或暫停時）回報 progress_seconds
- [ ] 進入播放頁時讀取上次進度，自動 seek 到對應位置（續播）
- [ ] 實作收藏功能 API + 前端收藏按鈕
- [ ] 收藏頁面：列出所有已收藏影片

### Phase 7：後台管理與每日推薦

- [ ] Admin dashboard 頁面（影片列表管理：編輯 metadata、管理標籤、刪除）
- [ ] 每日推薦管理介面（選擇影片 → 設定日期與排序）
- [ ] 首頁加入「今日推薦」區塊，展示 `GET /api/recommendations/today` 結果
- [ ] 若當日無手動推薦，fallback 為隨機挑選未看過的影片

---

## Go 核心依賴

```
github.com/gin-gonic/gin           # 或 github.com/labstack/echo/v4
github.com/jackc/pgx/v5            # PostgreSQL driver
github.com/golang-migrate/migrate  # DB migration
github.com/minio/minio-go/v7       # MinIO SDK
github.com/golang-jwt/jwt/v5       # JWT
github.com/casbin/casbin/v2        # 權限引擎
golang.org/x/crypto                # bcrypt
```

---

## 環境變數定義

```env
# PostgreSQL
DB_HOST=postgres
DB_PORT=5432
DB_USER=vaultflix
DB_PASSWORD=<secure_password>
DB_NAME=vaultflix

# MinIO
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=<access_key>
MINIO_SECRET_KEY=<secret_key>
MINIO_USE_SSL=false
MINIO_VIDEO_BUCKET=vaultflix-videos
MINIO_THUMBNAIL_BUCKET=vaultflix-thumbnails

# JWT
JWT_SECRET=<random_secret_key>
JWT_EXPIRY_HOURS=24

# Server
SERVER_PORT=8080
ADMIN_DEFAULT_USERNAME=admin
ADMIN_DEFAULT_PASSWORD=<initial_password>

# Import
IMPORT_SOURCE_DIR=/mnt/videos  # Docker volume mount 本地影片目錄
```

---

## 未來擴展方向（不在第一階段範圍內）

- **全文搜尋引擎**：引入 Meilisearch，對中日文內容做高品質搜尋
- **LLM Chat 助手**：`/api/chat` 端點串接 Claude API，透過 OCR 提取影片文字資訊做語意搜尋
- **行動端**：React Native 或獨立 APP
- **多使用者**：開放註冊、使用者偏好設定
- **自動標籤**：透過 LLM 分析影片內容自動產生標籤
- **API Gateway**：引入 Traefik 做 rate limiting、SSL termination
