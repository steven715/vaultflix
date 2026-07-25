# remux VOD-on-the-fly — 設計(video-compat Phase 2 之一)

- **日期**:2026-07-21
- **場景**:Feature
- **狀態**:設計已核可,待寫實作計畫
- **前情**:Phase 1(`2026-06-30-video-compat-phase1-design.md`)以單一線性 ffmpeg event session 即時 remux;根因分析確認 event playlist(無 `#EXT-X-ENDLIST`)使 hls.js 視為直播 → `duration=Infinity`、`seekable.length=0`,remux 影片長時間無進度條、不可 seek。

## 目標

remux 影片「秒開 + 任意 seek 準確 + 進度條正常」:回應 manifest 時即為完整 VOD 清單(含 `ENDLIST`),分段按需產生。

## 實測依據(2026-07-20,api container 內)

- Keyframe 探測用 packet-level 掃描(`ffprobe -show_entries packet=pts_time,flags`,不解碼):1.4GB AVI 冷讀 **21s(~67MB/s,瓶頸為 Docker Desktop 磁碟掛載 I/O)**;page cache 命中時 <1s。→ 換算 **~15s/GB**,lazy 首播探測會讓每片首播等 20s+,不可接受為主路徑。
- Keyframe 密度差異大:MIDE-277 每 ~8s 一個(1081 個)、mdyd844 每 ~1.2s 一個(6903 個)→ 邊界表每片 1k–7k 筆,JSONB 序列化約 10–60KB。
- 124 部 remux 影片全量 backfill 約 **85 分鐘**一次性背景 I/O。

## 已定案的設計決定

| 議題 | 決定 | 理由 |
|---|---|---|
| 探測時機 | **Backfill + import 後自動探測 + lazy fallback** | 絕大多數首播秒開;import 不變慢;漏網之魚仍可播(付一次性等待) |
| 邊界表儲存 | **新表 `video_keyframe_index`**(migration) | videos 表保持精簡;跨重啟/session 永久有效;down migration 一行可逆 |
| 快取策略 | **Per-video 共享快取 + singleflight + idle sweep + LRU 容量上限** | 分段內容與使用者無關、可確定重產;LRU 上限 bound 磁碟用量(使用者選擇) |
| 舊 event 路徑 | **完全取代(刪除)** | 單一路徑、測試面最小;分支 + git revert 即退路 |
| 前端 | **僅新增 503 準備中處理** | hls.js 對 VOD manifest 原生給有限 duration + 全片 seekable |

## 架構

```
KeyframeProber(探測)          → ffprobe packet 掃描 → 邊界表 → 存 DB
ManifestBuilder(純函式)       → 邊界表 → 完整 VOD m3u8(含 ENDLIST)
SegmentCache(on-demand 產生) → ffmpeg -ss/-to -c copy 單段 → 快取 → serve
```

播放流程:`GET index.m3u8` → 查 `video_keyframe_index` → 有 → 記憶體組 VOD manifest 秒回(不落地、不等 ffmpeg);`GET segNNNNN.ts` → 快取命中直接回,未命中跑單段 ffmpeg(input seek 只讀該段 byte range,亞秒級)。

## 1. 探測與邊界表

- `ffprobe -v error -select_streams v:0 -show_entries packet=pts_time,flags -of csv=p=0`,取 `K` flag 的 pts 列表。
- 依目標段長 **6s** 分組:每段起點為 keyframe pts,貪婪累積至 ≥6s 的下一個 keyframe 為界;段長依 keyframe 分布浮動(與 Phase 1 實測 8.34s 段成因相同)。末段到影片結尾。
- Migration `018_create_video_keyframe_index`:
  - `video_id uuid PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE`
  - `segments jsonb NOT NULL`(`[{"start": float, "duration": float}, ...]`)
  - `probed_at timestamptz NOT NULL`
  - down:`DROP TABLE`,完整可逆。
- 新 repository interface(定義於使用端 package,遵守 DbC 規範):`Get`(無資料回 `model.ErrNotFound`)、`Upsert`。
- 探測觸發三路,共用同一個 service method(per-video singleflight 防重複探測):
  1. **Backfill**:沿用 `codec_backfill_service.go` pattern,admin 端點掃 `play_mode=remux` 且無邊界表者,循序探測。
  2. **Import 後自動探測**:import 完成後對新增 remux 片非同步觸發,不阻塞 import。
  3. **Lazy fallback**:manifest 請求無邊界表 → 回 503 `stream_not_ready` + 背景觸發探測。

## 2. Segment on-demand 與快取

- 產生指令(概念):`ffmpeg -ss <kf_start> -i <input> -t <dur> -c copy -muxdelay 0 -output_ts_offset <kf_start> -f mpegts <out>`
  - `-ss` 在 `-i` 前(input seek,靠容器索引直接跳);start 即 keyframe pts,落點精準。
  - **`-output_ts_offset` 為關鍵**:預設每段 PTS 重置為 0 會讓 hls.js 對齊錯亂;offset 使段內 PTS 與 manifest 位置一致。列為整合測試必驗項;異常時備案 `-copyts`。
- `internal/streaming` 的 `Manager` 重寫為 **`SegmentCache`**:
  - 快取目錄以 videoID 為 key(拿掉 userID),多分頁/多使用者共享已產分段。
  - 同段並發請求 **singleflight**:只跑一支 ffmpeg,其餘等待同一結果。
  - 清理雙軌:現有 **idle sweep** 保留(整片目錄閒置逾 idleTimeout 清除)+ **LRU 容量上限**(env `TRANSCODE_CACHE_MAX_BYTES`,預設 20GiB;超限踢最久未存取的整片目錄)。容量上限屬基礎設施資源參數 → 環境變數,符合執行期可調性原則例外。
  - 無長駐 ffmpeg;`TranscodeProc`/`Transcoder` 線性 session、event args **刪除**。

## 3. Handler 與授權

- `Playlist`:查邊界表 → `ManifestBuilder` 組 m3u8(`#EXT-X-PLAYLIST-TYPE:VOD`、`#EXT-X-TARGETDURATION`、逐段 `#EXTINF`、`#EXT-X-ENDLIST`)→ 沿用 `rewritePlaylistTokens` 內嵌 token。無邊界表 → 503 `stream_not_ready` + 觸發探測。
- `Segment`:regex 驗名(`seg%05d.ts` 不變)→ 索引超出邊界表回 404 → ensure-segment(快取/singleflight/產生)→ serve。路徑二重防護照舊。
- Token 驗證與 401/403/404/400 行為完全不變。

## 4. 前端

- hls.js 收到 VOD manifest 即有有限 `duration` + 全片 `seekable`,**進度條/seek 零前端改動**。
- 唯一新增:manifest 載入遇 503 → 顯示「首次播放準備中」+ 輪詢重試(上限 + `useRef` 計數,遵守 CLAUDE.md 重試規範)。backfill 覆蓋後為罕見路徑,但不可呈現裸錯誤畫面。

## 5. 測試與驗收

- 單元:邊界分組(table-driven,涵蓋 keyframe 密 1.2s / 疏 8s 兩型 + 末段)、manifest builder(ENDLIST、EXTINF 總和 ≈ duration)、LRU 淘汰、singleflight、handler 正常/400/401/404/503。
- 整合:mkv fixture 走 manifest → 首段 → **跳抓中段**(驗 on-demand 產生 + ts offset 連續性);`task test-integration`。
- 手動驗收:Chrome DevTools 開 remux 片,確認 `video.duration` 有限、`seekable` 涵蓋全片、進度條可見、前後 seek 皆可播。
- Done = `task verify` 綠 + `task test-integration` 綠 + PR CI 綠。

## Trade-offs

- **每段一次 ffmpeg spawn**:啟動開銷 × seek 頻率;單人使用可忽略,換來零長駐程序與精準 seek。
- **`-output_ts_offset` 相容性**為最大技術風險 → 整合測試專驗,備案 `-copyts`。
- **LRU 上限**增加 SegmentCache 複雜度(追蹤目錄大小與存取序);單人 + idle sweep 其實已足,但 bounded disk 為使用者明確偏好。
- **刪除 event 路徑**:Phase 2 真轉碼本就要走 segment-on-demand(per-segment 換 `libx264`),此架構是鋪路非繞路。

## 明確排除

- 真轉碼(`transcode` 影片,89 部)→ 後續 Phase 2 transcode,共用本次 SegmentCache 架構。
- nginx offload、NVENC → 不變,依 roadmap 後續評估。
- Manifest 預熱/首段 prefetch → 單段產生已亞秒級,YAGNI。
