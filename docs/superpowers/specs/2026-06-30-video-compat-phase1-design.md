# 影片相容性串流 — Phase 1 設計（即時 remux）

- **日期**：2026-06-30
- **場景**：Feature
- **狀態**：設計已核可，待寫實作計畫

## 問題

部分影片的 codec / container 瀏覽器原生 `<video>` 無法播放，PWA 也不一定能播。系統目前把磁碟上的原始檔直接以 `http.ServeFile`（或 nginx `X-Accel-Redirect`）送出，沒有任何轉換層，遇到不相容格式就播不動。

## 片庫實測數據（2026-06-30 ffprobe 掃描 499 部）

| 層級 | 數量 | 佔比 | 說明 |
|---|---|---|---|
| `direct` 直接可播 | 285 | 57% | MP4 + H.264 + AAC/MP3 |
| `remux` 只需換殼 | 124 | 25% | codec 相容，僅 AVI/MKV 容器不被瀏覽器接受 |
| `transcode` 需轉碼 | 89 | 18% | 視訊/音訊 codec 不被瀏覽器解 |
| 檔案遺失 | 1 | — | 路徑已不存在 |

`remux` 群組明細：89 `avi/h264/aac`、28 `mkv/h264/aac`、7 `avi/h264/mp3`。
`transcode` 群組明細：63 `mpeg4`(Part2/Xvid)、21 WMV 家族（wmv1/2/3 + vc1 + wma）、2 `hevc`、3 其他（mpeg2 / ac3 / mp2）。

需處理的 213 部合計 **343 GB**（整體片庫 865 GB）。

## 硬體現況

- CPU：i7-14700F，28 threads —— 軟體轉碼即時 1080p 綽綽有餘。
- GPU：主機有 RTX 5070，但未接進容器（api image 的 ffmpeg 未編 NVENC，compose 無 `--gpus`）。
- QSV/VAAPI 實際不可用（14700**F** 內顯關閉、主機無 `/dev/dri`）。
- 結論：本期靠 CPU 軟體路徑；NVENC 留待 Phase 2 transcode 真的需要時再評估。

## 方案選擇

採「分階段、最終收斂到即時 HLS（方案 A）」：

- **Phase 1（本文件）**：建立即時 HLS 串流基礎建設，但只跑 `ffmpeg -c copy`（remux），解決 124 部 `remux` 影片 + 修既有 MIME bug + 匯入存真實 codec。
- **Phase 2（後續）**：在同一套 session manager 把 `-c copy` 換成 `libx264`，解決 89 部 `transcode` 影片；前端 hls.js 兩階段共用。

不採方案 B（背景預轉存檔），因為會多吃 ~343GB 儲存且 213 檔各存兩份。

## Phase 1 範圍

### 1. 播放策略分類器

新增純函式分類器，輸入 `(container, videoCodec, audioCodec)`，輸出 `PlayMode`：

- `direct`：container ∈ {mp4, mov}、videoCodec ∈ {h264}、audioCodec ∈ {aac, mp3}
- `remux`：videoCodec/audioCodec 皆瀏覽器相容（h264 + aac/mp3），但 container 不相容（avi, mkv, …）
- `transcode`：其餘（videoCodec 或 audioCodec 不相容）

策略**不存 DB**，由已儲存的 codec 欄位即時推導，維持單一真相來源。分類器放在 service 層，table-driven 測試涵蓋片庫實測出現的所有 combo。

### 2. 資料模型

- Migration（`NNN_add_video_codecs.up.sql` / `.down.sql`）新增：
  - `videos.video_codec varchar(64)` nullable
  - `videos.audio_codec varchar(64)` nullable
- `down.sql` 需完整可逆（`DROP COLUMN`）。
- `probeMetadata` 多解析 video/audio stream 的 `codec_name`（ffprobe 已在跑，零額外成本）。
- 修正既有 bug：`MimeType` 不再純靠副檔名推導；direct 的 MP4 才回 `video/mp4`，其餘交由播放策略決定路徑。
- Backfill：沿用 `backfill_service.go` 模式新增一個 backfill，補掃既有 499 部的 codec，不需重新匯入。

### 3. 即時 HLS 串流機制

- 新 package `internal/streaming/`，內含 session manager。
- 新端點（remux 影片專用）：
  - `GET /api/videos/:id/hls/index.m3u8` — 啟動或取得既有 session，回 HLS playlist
  - `GET /api/videos/:id/hls/:segment` — 回對應分段檔
- ffmpeg 以 `-c copy -f hls` 產生分段，寫入可寫暫存目錄。
- compose 新增可寫 volume `vaultflix-transcode-cache`（非 tmpfs，避免大分段吃爆記憶體），掛載到 api 容器的暫存路徑。
- Session 以 `(videoID, userID)` 為 key。背景 goroutine 做 **idle timeout 清理**（預設 60s 無請求即殺 ffmpeg、刪暫存目錄）。
- 並發安全比照 WebSocket Hub：用 channel 序列化 session 註冊/查詢/移除，不裸用 map。
- ffmpeg 透過可注入的 command runner interface 呼叫，方便單元測試 mock。

### 4. 串流授權

- 沿用現有 stream token 機制（`?token=` query param）。
- `index.m3u8` 與每個分段請求都需帶 token；hls.js 設定 `xhrSetup` / query param 附帶。
- 路徑安全：segment 檔名以白名單 / sessionID 綁定驗證，拒絕任意路徑拼接（比照現有 path traversal 防護）。

### 5. 前端

- 新增 `hls.js` 依賴。
- `VideoDetail` 回應新增 `play_mode` 欄位（`direct` / `remux` / `transcode`）。
- `PlayerPage` 依 `play_mode` 分流：
  - `direct` → 原生 `<video src>`（現狀不變）
  - `remux` → hls.js 掛 `index.m3u8`（Safari 可原生播 HLS，仍以 hls.js 統一處理）
  - `transcode` → 顯示「此格式將於 Phase 2 支援」明確提示，不嘗試播放（誠實 UX，不轉圈）
- 維持現有 stream-token 過期重試邏輯（重試上限、`useRef` 計數）。
- 遵守 CLAUDE.md 前端規範：cleanup flag、依賴陣列、重試上限。

### 6. 測試

- 分類器：table-driven，涵蓋片庫實測所有 combo。
- Session manager：單元測試（mock command runner，驗證 idle 清理、並發註冊）。
- Handler：正常路徑、404（影片不存在）、403/401（token 無效/過期）、400（非法 segment 名）。
- 整合測試：放一個小的 `mkv/h264/aac` fixture，跑完整 remux → HLS 播放鏈。改到串流，依規範跑 `task test-integration`。
- done condition：`task verify` 綠 + 串流範圍 `task test-integration` 綠 + PR CI 綠。

## 明確排除（非 Phase 1）

- libx264 即時轉碼（89 部 `transcode` 影片）→ Phase 2。
- NVENC / 硬體加速 → Phase 2 視需要。
- segment-on-demand 的 keyframe 對齊優化（本期用整檔 session remux，`-c copy` 為 I/O bound、足夠快）→ 後續優化。
- nginx 直接 serve HLS 分段的 offload（本期由 Go serve 分段）→ 後續優化。

## 已做的設計決定（可推翻）

1. Phase 1 的 `transcode` 影片不假裝能播，給明確提示而非壞掉的播放器。
2. 暫存用 Docker volume 而非 tmpfs，避免大分段吃爆記憶體。
3. NVENC 本期不接；remux 是 `-c copy` 不解碼，CPU 完全夠。

## 風險與 trade-off

- **整檔 session remux 的首播延遲**：VOD playlist 需要 ffmpeg 處理整檔才能定出所有分段時長；`-c copy` 雖快，大檔仍可能有數秒～數十秒延遲。若驗收體感不佳，後續可改 event/growing playlist 或 segment-on-demand。
- **兩條前端播放路徑**（原生 + hls.js）增加維護面，但 `direct` 路徑零改動，風險可控。
- **暫存空間**：idle 清理 + volume 大小上限需留意；單人使用同時 session 數有限，風險低。
