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

#### 媒體來源管理 ✅

- 新增/編輯/啟用停用「媒體來源」（label + 掛載磁碟目錄 mount_path）
- mount_path 須位於 `/mnt/host/` 前綴下，經路徑安全驗證防穿越
- 影片以唯讀方式掛載磁碟讀取，原始檔保留在本機磁碟、不複製進物件儲存

#### 影片管理 ✅

- 從媒體來源（掛載磁碟目錄）**非同步**批次匯入影片，進度經 WebSocket 即時回報（ffprobe 提取 metadata、ffmpeg 產生縮圖）
- 匯入具冪等性：重複匯入同一來源不會重複建立記錄
- 影片列表：分頁、搜尋、排序（建立時間/標題/長度/檔案大小）
- 編輯影片 metadata（標題、描述）
- 刪除影片（清除資料庫記錄與 MinIO 縮圖/預覽物件；磁碟原始檔唯讀掛載、不刪除）

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

- 串流播放：影片保留在本機磁碟。prod 經 nginx 以 `X-Accel-Redirect` 直接從磁碟讀檔送出（原生 HTTP Range，支援拖曳），API 只驗 token 與路徑安全、不經手影片 bytes；dev（無 nginx，vite 直連 API）退回 API `http.ServeFile` 直送。legacy 階段存於 MinIO 的影片仍走 presigned URL
- 自動記錄觀看進度
- 從上次進度繼續播放
- **已知限制**：瀏覽器僅能原生解碼 mp4(H.264)；avi/wmv/mkv 雖能正確串流（byte 層 200/206 正常），但 `<video>` 無法在瀏覽器播放，需另行轉碼或下載觀看

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
- 影片串流：影片 bytes 由 nginx 經 `X-Accel-Redirect` 直接從磁碟讀檔送出，API server 不經手影片資料、不隨檔案大小佔住連線（dev 無 nginx 時退回 API `http.ServeFile`）
- 縮圖/預覽：透過 MinIO presigned URL 提供，列表請求應考慮快取機制，避免每次重新產生

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

完整 roadmap —— 未來功能、技術債 backlog、架構演進觸發條件 —— 見專案根目錄的 **[ROADMAP.md](../ROADMAP.md)**（唯一真相）。本 SPEC 只描述**已實作的現況**（✅）；「接下來做什麼」一律以 ROADMAP.md 為準，避免兩處重複而 drift。
