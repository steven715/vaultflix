# Vaultflix — 產品需求規格書 (SPEC)

> **文件格式說明**：本文件為單一檔案格式。當章節數量或篇幅增長至難以維護時，
> 應拆分為 `docs/spec/` 目錄結構，每個章節獨立成檔並以 `index.md` 串接。

## 狀態標記說明

- ✅ 已實作
- 🔲 計畫中（近期）
- 💭 遠期願景（不展開細節）

---

## Functional Requirements

### Admin 功能

#### 影片管理 ✅

- 從掛載目錄批次匯入影片（ffprobe 提取 metadata、ffmpeg 產生縮圖）
- 影片列表：分頁、搜尋、排序（建立時間/標題/長度/檔案大小）
- 編輯影片 metadata（標題、描述）
- 刪除影片（同步清除 MinIO 物件與資料庫記錄）

#### 標籤管理 ✅

- 建立標籤（名稱 + 可選分類）
- 為影片新增/移除標籤
- 標籤列表依影片數量排序

#### 每日推薦管理 ✅

- 依日期建立推薦清單，手動選擇影片
- 調整推薦排序
- 刪除推薦項目
- 無手動推薦時自動 fallback 至隨機未觀看影片

#### 用戶管理 ✅

- 列出所有用戶
- 新建用戶（指定帳號、密碼、角色）
- 刪除用戶（soft delete：標記為停用，保留收藏與觀看記錄）
- 重設用戶密碼（由 admin 操作，指定新密碼）

---

### Viewer（一般用戶）功能

#### 瀏覽影片 ✅

- 影片列表：分頁瀏覽
- 依標籤篩選
- 標籤側欄：依影片數量排序的扁平列表

#### 影片播放 ✅

- 串流播放（MinIO presigned URL）
- 自動記錄觀看進度
- 從上次進度繼續播放

#### 收藏 ✅

- 新增/移除收藏
- 收藏列表頁面

#### 觀看記錄 ✅

- 觀看記錄列表，顯示進度百分比
- 點擊可繼續播放

#### 每日推薦 ✅

- 首頁顯示當日推薦影片
- 無手動推薦時顯示隨機未觀看影片

---

### 認證與權限控制

#### 認證 ✅

- JWT Bearer Token 認證（HS256）
- bcrypt 密碼雜湊
- 首次啟動自動建立預設 admin 帳號

#### 角色權限（Casbin RBAC）✅

- admin：完整 API 存取權限
- viewer：僅限瀏覽、播放、收藏、觀看記錄、每日推薦
- 停用帳號：拒絕登入，回傳明確「帳號已停用」訊息 ✅

---

## Non-Functional Requirements

### 效能

- 影片匯入：支援大量檔案批次處理（實測 18GB/4m40s）
- Presigned URL：應考慮快取機制，避免每次列表請求都重新產生
- 影片串流：透過 MinIO presigned URL 直接串流，API server 不經手影片資料

### 安全

- 密碼儲存：bcrypt 雜湊，不可逆
- API 認證：JWT Bearer Token，所有 `/api/*` 端點強制驗證
- 授權：Casbin RBAC 逐路徑逐方法檢查
- 路徑安全：影片匯入的 source_dir 須防範 path traversal 攻擊
- 機敏資訊：JWT secret、DB 密碼、MinIO 憑證等透過 `.env` 注入，不寫死在程式碼中

### 部署

- Docker Compose V2 一鍵啟動（PostgreSQL + MinIO + API Server + Frontend）
- 所有服務使用 alpine-based image
- 環境變數透過 `.env` 統一管理
- 每個服務配置 health check
- 單機部署為主要目標

### 可維護性

- 分層架構：Handler → Service → Repository，各層職責明確
- 跨層依賴透過 interface 定義契約
- 測試：手寫 mock、table-driven tests，不依賴第三方 mock 框架
- Go 檔案不超過 300 行、function 不超過 50 行
- 完整的開發規範見 [CLAUDE.md](../CLAUDE.md)

### 可擴展性

- 所有外部依賴（DB、MinIO）透過 interface 抽象，方便替換實作
- RBAC policy 為外部檔案，新增角色或路徑不需改程式碼
- 前後端分離，API 可獨立被其他 client 消費

---

## 遠期願景

以下為長期考慮方向，不展開細節，優先級與時程未定：

- **全文搜尋** — 引入 Meilisearch，支援影片標題與描述的模糊搜尋
- **LLM 整合** — 語意搜尋、自動標籤、聊天式影片推薦
- **行動端** — Mobile client 或 responsive web
- **非同步匯入** — SSE/WebSocket 進度回報，匯入不阻塞 HTTP 請求
- **API Gateway** — Traefik 反向代理，支援 HTTPS 與 rate limiting
- **孤立檔案清理** — 排程掃描 MinIO 中無對應 DB 記錄的物件並清除
