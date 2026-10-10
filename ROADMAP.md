# Vaultflix Roadmap

> 用語以 [CONTEXT.md](CONTEXT.md) 為準。已完成的項目直接刪除（歷史在 git）。

### 🎯 進行中（優先）

- [ ] **Enrichment 第 ②③ 階段** — 第 ① 階段（Scraper：javbus / avid，以 Code 比對，產出 Metadata Suggestion 待審）已完成（PR #17/#18）。尚待：
  - ② LLM（Claude API）依 Metadata 產出 Suggestion
  - ③（選用）抽幀畫面分析產出 Suggestion
  - 前提：依 ADR-0010，所有來源一律只產 Suggestion、不自動套用；LLM 設計前先完成下方「Performer / Maker 遷移」，否則 LLM 不知道演員該進 Tag 還是 Performer。

- [ ] **Transcode Play Mode（即時轉碼）** — 把 remux 的即時 HLS 管線從 `ffmpeg -c copy` 擴充成 `libx264` 即時軟轉，讓目前**不能播的 89 部 / 143GB（18% Library）**能播。
  - **重要性=正確性**：這批片現在前端顯示「此格式將於 Phase 2 支援」、完全打不開。依重要性排序（正確性 > 開發速度 > 體驗）嚴格說高於 Enrichment；順序待定（已知張力）。
  - **現況（2026-07-05 核實）**：codec 回填 498/498 完成、Play Mode Classification live。分佈：direct 285(521GB) / remux 124(200GB，已解) / **transcode 89(143GB，本項)**。不能播主因：`mpeg4`(65)、`wmv1/2/3`(16)、`vc1`(5)、`hevc`(2) + `wmav2/wmapro/ac3` 音訊。
  - **範圍**：`internal/streaming` 的 ffmpeg arg builder（`-c:v libx264`）+ Play Mode Classification 把 `transcode` 導向真轉碼；前端 hls.js 路徑共用。硬體 i7-14700F 28 threads 軟轉即時 1080p 足夠；NVENC 視需要再評估。remux 已改為 VOD-on-the-fly（PR #24，見 `docs/streaming.md`），轉碼要沿用同一套 Keyframe Index / Segment Boundary 對齊機制。
  - **已排除**：預轉存檔（+343GB×N 儲存，已否決，見 ADR-0009）。
  - **同時要補 Casbin policy（2026-08-23 發現，2026-09-28 仍未修）**：`casbin/policy.csv` 給了 `viewer` `/api/videos/:id/stream`，但沒有 `/api/videos/:id/hls/index.m3u8` 與 `/api/videos/:id/hls/:segment`。`middleware.CasbinRBAC` 用實際 URL path 過 enforce，所以 viewer 碰到 remux/transcode 影片會拿到 403。改動極小（policy.csv 兩行），但**必須與本項同批驗證**，並補一個 viewer 打 HLS 路由的測試防回歸。

- [ ] **程式碼對齊術語表** — 2026-09-28 建立 CONTEXT.md 時只統一了文件用語，程式碼仍用舊名。拆成獨立 Refactor PR：
  - **Performer / Maker 遷移（ADR-0010）**：`actresses` → performers（DB / Go / API）；`actor`、`studio` Tag 資料遷移成 Performer / Maker 後淘汰這兩個 category
  - **UI 文案**：「掃描匯入」→「匯入」、BackfillProgress「掃描影片」、admin 選單「精選」→「今日推薦」、「續看 / 繼續播放」→「繼續觀看」、「觀看記錄」→「觀看紀錄」
  - **識別字**：Watch Progress 欄位（`progress_seconds` vs `watch_progress`）、裸用的 `label` / `source` / `scope` 視改動成本決定是否改名
  - 觸發：開始 Enrichment ② 之前（Performer 遷移是其前提）

- [ ] **前端 Enrichment UI** — 後端 `/enrich`、`/suggestions`、`/enrich-jobs` 已存在，前端完全沒有對應 UI（types 也沒有）。需要：Suggestion 審核介面、Performer 頁 / Avatar、卡片顯示 **Cover 優先、Thumbnail 後備**（CONTEXT.md 已定此規則，`PosterThumb` 目前只吃 thumbnail）。

### 小型 bug（順手修）

- [ ] 前端 `ImportJob.status` 型別缺 `cancelled`（後端有此狀態）
- [ ] `casbin/policy.csv` 給 viewer `PUT /api/watch-history`，但 router 無此路由（死 policy，刪除）

### 其他未來功能

- [ ] **全文搜尋引擎** — 引入 Meilisearch，改善中日文標題搜尋品質（目前用 PostgreSQL `gin` 索引，對 CJK 分詞效果有限）
- [ ] **LLM Chat 助手** — `/api/chat` 端點串接 Claude API，結合影片 metadata 做語意搜尋與推薦對話
- [ ] **行動端支援** — React Native 或獨立 APP，搭配現有 API
- [ ] **多使用者** — 開放註冊、使用者偏好設定、個人化推薦
- [ ] **API Gateway** — 引入 Traefik 做 rate limiting、SSL termination、反向代理
  - **現狀評估**：反向代理、串流卸載（`X-Accel-Redirect` + `sendfile`）已由 nginx 覆蓋且更精細；後端為單一 monolith、單人自用 → Gateway 的核心價值（跨服務路由、集中認證卸載、多租戶限流）目前無前提，屬過度設計
  - **缺點/trade-off**：串流卸載綁死 nginx，Traefik 只能疊前面 → 變 Traefik→nginx→api 兩層 edge，多 hop、多一份 config drift 與 health check
  - **觸發條件**（任一滿足即啟動）：
    - 後端拆成 ≥2 個獨立服務且需統一入口路由（如 CV 抽幀做成獨立 worker）
    - 正式對外曝露公網且面對不可信流量（rate limiting / WAF / SSL 才有實質安全價值）
    - 多使用者上線，需 per-user API key 或差異化限流
  - **若動機只是「外網安全存取」**：用 Cloudflare Tunnel / Tailscale + nginx TLS，比引入 Traefik 便宜一個數量級，不碰串流卸載架構
- [ ] **前端播放器 UX 重設計（離開裸 `<video>` 控制列）** — 換更好的控制列 / 皮膚 / 互動，延續「私人放映室」設計系統。
  - **重要性=體驗層**（最低優先）。
  - **前提認知**：技術上離不開 `<video>` 元素——所有播放器 lib（Vidstack / Plyr / video.js / Shaka）與現用的 hls.js 都是 `<video>`+MSE 的外皮。真正訴求是控制列/皮膚，不是換播放核心。
  - **決策順序**：先定播放器技術（headless lib 如 Vidstack vs 自刻控制列，屬**工程決策**）→ 再跑設計 pass 做皮膚。**別讓設計驅動技術**。
  - **硬約束（設計/選型都要保住）**：stream-token 進 URL + 重試上限、play_mode 分流（direct 原生 vs remux hls.js）、Safari HLS unmount cleanup（commit 53aea35）、HUD/遙測整合。播放器 lib 若自管 source 載入，易與 token 重試/Safari 清理打架。

- [ ] **ABR（自適應位元率）** — 依頻寬在多位元率階梯間自動切換，改善外網緩衝體驗。
  - **狀態=延後，有前置依賴**：ABR 只解緩衝/頻寬，**不解「不能播」**（那是 codec，走 Transcode Play Mode）；且需多份 rendition 才能切換 → **依賴 Transcode Play Mode 管線先存在**。先做 ABR 是蓋在空地上。
  - **觸發條件**：Transcode Play Mode 完成 **且** Playback Telemetry 數據證明外網 rebuffer ratio 確實高到值得 ABR 的複雜度（單人自用未必成立）。

- [ ] ~~**WebRTC 串流**~~ — **已否決（2026-07-05）**。WebRTC 是即時雙向低延遲（通話/直播）的工具；本專案是 VOD 點播，產業標準即 HTTP-based HLS/DASH，且已有完整基建（X-Accel + nginx sendfile + HLS）。硬上會丟掉串流卸載架構、引入 STUN/TURN/ICE/signaling，過 ngrok 還因 UDP→TURN-over-TCP 幫倒忙，VOD 場景無實質收益。屬選錯工具，不做。

- [ ] **MinIO → SeaweedFS 遷移** — 2026-09 MinIO 官方把 `minio/minio`、`minio/mc` 從 Docker Hub 刪除、Quay 也關閉匿名拉取，社群版不再修 CVE（如 CVE-2026-40344 認證繞過）。目前改用社群 fork `pgsty/minio`（SILO，已 backport CVE 修補），並固定版本。
  - **觸發條件**（任一滿足即啟動）：pgsty fork 超過 6 個月無新 release／出現未修補的 CVE／fork 映像檔無法拉取
  - **範圍**：Go 端用標準 S3 API（minio-go），主要成本在 compose、bucket 初始化（`minio-init` 的 `mc mb`）、healthcheck、nginx `/minio/` 反代與 presigned URL 的重新驗證

- [ ] **`videos.mime_type` 依 codec 重算全庫** — `mime_type` 在匯入時由 `MediaProcessor` 依容器 + codec 推得，但 codec Backfill 只補 codec、不改 MIME；在 codec 支援前匯入的舊 Video，MIME 仍是只看副檔名的舊值。**目前無影響**：唯一讀它的是直連 Stream 的 `http.ServeFile`（`video_handler.go`），而唯一的執行 stack 開著 X-Accel，Content-Type 由 nginx 依副檔名決定；前端只宣告此欄位、未使用。**觸發條件**：有任何正式環境路徑或前端開始讀 `mime_type`。屆時以 migration 或 Backfill 依 codec 重算**全庫**（不能只靠 codec Backfill，它只處理 codec 為空的 Video）。

- [ ] **孤兒檔案清理排程 / MinIO 刪除失敗追蹤** — 影片刪除時 MinIO 刪除為 best-effort（`internal/service/video_service.go` 三個刪除失敗只 log、仍回 nil），孤兒物件會靜默累積。需定期比對 MinIO 與 DB 清理不一致物件。**觸發條件**：實際觀察到孤兒物件累積，或 MinIO 刪除失敗重複發生。

---

## 架構演進

- [ ] **前端 Client / Admin 拆分為獨立專案** — **觸發條件已滿足（2026-09-28）**：admin 已有 6 頁（Analytics、MediaSource、RecommendationManage、TagManage、UserManage、VideoManage），超過 5 頁門檻。
  - **動機**：敏感度劃分（admin 操作不應與 client 共享攻擊面）、獨立演進（技術選型與部署節奏脫鉤）
  - **現狀**：目錄層已分離（`pages/admin/`、`components/admin/`、`api/admin.ts`），共用 AuthContext、types、utils、API client interceptor
  - **重要性**：開發速度
  - **原觸發條件**（任一滿足即啟動）：Admin 頁面 ≥ 5 頁 ✅／發生第 2 次因共用元件改動導致另一端非預期 side effect／Admin 需要獨立的認證流程或部署節奏
  - **下一步**：先做前置步驟 lazy loading code splitting（`web/src` 目前沒有任何 `lazy(`，成本低，立即減少 client bundle），再評估是否真的拆 repo

---

## 優先級框架

以**觸發條件**取代傳統的緊急/不緊急判斷：

| | 重要 | 不重要 |
|---|---|---|
| **已觸發** | 立刻做 | 順手做或不做 |
| **未觸發** | 記錄 + 定義觸發條件 | 從 ROADMAP 移除 |

**重要性**依影響維度排序：安全性 > 穩定性 > 正確性 > 開發速度 > 體驗優化

觸發條件須**可觀察且能明確判斷是/否**，例如「admin 頁面超過 5 頁」而非「覺得該做的時候」。
