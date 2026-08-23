# Vaultflix Roadmap

### 🎯 進行中（優先）

- [ ] **自動標籤（AI 影片分析）** — 透過 AI 分析影片檔名（JAV 番號、女優名）與內容，自動建議標籤分類。**目前最高優先**。
  - 規劃分階段：① 檔名/番號 scraper 比對既有 metadata 來源 → ② LLM（Claude API）依檔名+metadata 建議標籤 → ③（選用）抽幀電腦視覺內容分析
  - 詳細 Spec / 選型見後續 feature 對話與研究筆記

- [ ] **影片相容性 Phase 2（即時轉碼）** — 把 Phase 1 已建好的 `internal/streaming` session manager 從 `ffmpeg -c copy` 換成 `libx264` 即時軟轉，讓目前**不能播的 89 部 / 143GB（18% 片庫）**能播。
  - **重要性=正確性**：這批片現在前端顯示「此格式將於 Phase 2 支援」、完全打不開。依重要性排序（正確性 > 開發速度 > 體驗）嚴格說高於自動標籤；順序待定（自動標籤目前掛「最高優先」，此為已知張力）。
  - **現況（2026-07-05 核實）**：codec 回填 498/498 完成、分類器 live。分佈：direct 285(521GB) / remux 124(200GB，Phase 1 已解) / **transcode 89(143GB，本項)**。不能播主因：`mpeg4`(65)、`wmv1/2/3`(16)、`vc1`(5)、`hevc`(2) + `wmav2/wmapro/ac3` 音訊。
  - **範圍**：後端 ffmpeg arg builder（`-c:v libx264`）+ 分類器把 `transcode` 導向真轉碼；前端 hls.js 路徑 Phase 1 已鋪好，共用。硬體 i7-14700F 28 threads 軟轉即時 1080p 足夠；NVENC 視需要再評估。
  - **已排除**：預轉存檔（方案 B，+343GB×N 儲存，已否決）。
  - **同時要補 Casbin policy（2026-08-23 發現）**：`casbin/policy.csv` 給了 `viewer` `/api/videos/:id/stream`，但沒有 `/api/videos/:id/hls/index.m3u8` 與 `/api/videos/:id/hls/:segment` 兩條。`middleware.CasbinRBAC` 用實際 URL path 過 enforce，所以 viewer 角色碰到 `remux`/`transcode` 影片會拿到 403 —— Phase 1 的 remux 其實對 viewer 還沒真的解鎖。目前單人以 admin 使用不會踩到，但 Phase 2 讓那 89 部能播之後，任何 viewer 帳號仍舊播不了，等於白做。改動極小（policy.csv 兩行），但**必須與 Phase 2 同批驗證**，並補一個 viewer 角色打 HLS 路由的測試防回歸。
  - Spec 已寫：`docs/superpowers/specs/2026-06-30-video-compat-phase1-design.md`（Phase 2 段落）。

- [ ] **播放遙測 / 量化（效能優化前置 enabler）** — 把 ADR-0006 HUD 已量測的指標（stall 分類 starved/codec、buffer-edge 吞吐、TTFB RTT）從「即時顯示、關掉就沒」變成**可留存、可聚合**，建立效能 before/after baseline。
  - **動機**：任何串流效能優化（ABR 等）前必須能量化。目前無法回答「外網 remux 片首播 P50 = 幾秒、rebuffer ratio = 幾 %」。
  - **範圍**：`POST /api/playback/telemetry` 端點，收每個播放 session 的（首播延遲、stall 次數/時長、有效吞吐、play_mode、網路情境）→ 存 DB。零新基礎設施。
  - **重要性=次高 enabler**：小、獨立有價值、解鎖後續效能工作。建議作為串流效能線的**第一步**。

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
  - **狀態=延後，有前置依賴**：ABR 只解緩衝/頻寬，**不解「不能播」**（那是 codec，走 Phase 2）；且需多份 rendition 才能切換 → **依賴 Phase 2 轉碼管線先存在**。先做 ABR 是蓋在空地上。
  - **觸發條件**：Phase 2 完成 **且** 播放遙測數據證明外網 rebuffer ratio 確實高到值得 ABR 的複雜度（單人自用未必成立）。

- [ ] ~~**WebRTC 串流**~~ — **已否決（2026-07-05）**。WebRTC 是即時雙向低延遲（通話/直播）的工具；本專案是 VOD 點播，產業標準即 HTTP-based HLS/DASH，且已有完整基建（X-Accel + nginx sendfile + HLS）。硬上會丟掉串流卸載架構、引入 STUN/TURN/ICE/signaling，過 ngrok 還因 UDP→TURN-over-TCP 幫倒忙，VOD 場景無實質收益。屬選錯工具，不做。

- [ ] **孤兒檔案清理排程 / MinIO 刪除失敗追蹤** — 影片刪除時 MinIO 刪除為 best-effort（`internal/service/video_service.go` 三個刪除失敗只 log、仍回 nil），孤兒物件會靜默累積。需定期比對 MinIO 與 DB 清理不一致物件。**觸發條件**：實際觀察到孤兒物件累積，或 MinIO 刪除失敗重複發生。

---

## 架構演進

- [ ] **前端 Client / Admin 拆分為獨立專案**
  - **動機**：敏感度劃分（admin 操作不應與 client 共享攻擊面）、獨立演進（技術選型與部署節奏脫鉤）
  - **現狀**：目錄層已分離（`pages/admin/`、`components/admin/`、`api/admin.ts`），共用 AuthContext、types、utils、API client interceptor
  - **重要性**：開發速度 — 目前 2 頁 admin 不構成負擔，但隨功能增長會拖慢兩邊的迭代
  - **觸發條件**（任一滿足即啟動）：
    - Admin 頁面成長至 5 頁以上
    - 發生第 2 次因共用元件改動導致另一端非預期 side effect
    - Admin 需要獨立的認證流程或部署節奏
  - **前置步驟**：先完成 lazy loading code splitting（成本低，立即減少 client bundle 體積），拆分時可作為邊界參考

---

## 優先級框架

以**觸發條件**取代傳統的緊急/不緊急判斷：

| | 重要 | 不重要 |
|---|---|---|
| **已觸發** | 立刻做 | 順手做或不做 |
| **未觸發** | 記錄 + 定義觸發條件 | 從 ROADMAP 移除 |

**重要性**依影響維度排序：安全性 > 穩定性 > 正確性 > 開發速度 > 體驗優化

觸發條件須**可觀察且能明確判斷是/否**，例如「admin 頁面超過 5 頁」而非「覺得該做的時候」。
