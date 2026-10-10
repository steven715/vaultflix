# Vaultflix

個人影片管理與串流平台：影片留在本機磁碟，系統負責登錄、補全資料、推薦與播放。主詞條為英文（與程式碼識別字對齊），括號內為對應的 UI 顯示名稱。

## Language

### 片庫與匯入

**Library**（片庫）:
所有 Media Source 下全部 Video 的集合。
_Avoid_: 影片庫, collection

**Video**（影片）:
Library 中的一部作品，對應磁碟上的一個影片檔。
_Avoid_: 檔案, media

**Media Source**（媒體來源）:
一個以唯讀方式掛載、供 Import 讀取的磁碟目錄。
_Avoid_: source（裸用）, 掛載磁碟目錄, 來源（裸用）

**Source Name**:
Media Source 的顯示名稱。
_Avoid_: label

**Import**（匯入）:
對一個 Media Source 執行一次 **Import Job**，把尚未登錄的影片檔登錄成 Video。
_Avoid_: 掃描匯入

**Scan**（掃描）:
Import 內部列舉 Media Source 目錄中影片檔的步驟；不是獨立動作。
_Avoid_: 用「掃描」稱呼 Import 或 Backfill

**Backfill**（補齊）:
對既有 Video 一次性補算衍生資料的 admin 工作；目前有 Preview、codec、Keyframe Index、番號四種。

**Backfill Job**:
一次 Backfill 的執行；在背景逐部處理 Video，每部結果為成功、略過（如 Media Source 停用）或失敗，可於兩部之間取消。同一時間全域只有一個 Backfill Job。

### 作品資訊

**Metadata**:
作品的描述性資訊 —— 標題、描述、番號、Maker、Label、Performer、類型；不論手動填寫或經 Enrichment 取得。
_Avoid_: 用 metadata 稱呼 Media Info

**Media Info**（技術資訊）:
從影片檔讀出的技術屬性 —— 時長、解析度、檔案大小、video/audio codec。
_Avoid_: metadata, ffprobe metadata

**Code**（番號）:
作品的發行編號，是 Enrichment 比對外部來源的主鍵。

**Performer**（演員）:
出演作品的人，是有頭像與別名的實體，不是 Tag。
_Avoid_: Actress, actor tag, 女優, 人物

**Maker**（片商）:
製作作品的公司，是 Metadata 欄位，不是 Tag。
_Avoid_: studio tag, 工作室

**Label**（發行品牌）:
Maker 旗下的發行品牌。
_Avoid_: 用 label 稱呼 Source Name

**Tag**（標籤）:
使用者對 Video 的分類，分為 genre（類型）與 custom（自訂）兩種 category。
_Avoid_: 用 Tag 表示 Performer 或 Maker

**Enrichment**（資料補全）:
從外部來源取得 Metadata、產出 Metadata Suggestion 待審的流程；來源可以是 Scraper、LLM 或畫面分析。
_Avoid_: 自動標籤, auto-tagging, AI 影片分析

**Scraper**:
以 Code 查詢外部網站取得 Metadata 的 Enrichment 來源。
_Avoid_: source（裸用）

**Metadata Suggestion**（建議）:
Enrichment 產出、尚待使用者接受或拒絕的一筆 Metadata 變更；未被接受前不影響 Video。

### 圖像資產

**Thumbnail**（縮圖）:
系統從影片中抽出的一幀靜態圖。

**Cover**（封面）:
經 Enrichment 取得的官方封面圖；卡片顯示時優先於 Thumbnail。

**Preview**（預覽）:
系統從影片剪出的短片段，用於瀏覽時預覽。

**Avatar**（頭像）:
Performer 的照片。
_Avoid_: poster（poster 只是 UI 元件名，不是領域詞）

### 觀看

**Playback Session**（播放場次）:
一次開啟播放頁觀看一部 Video、直到離開為止；Watch Session 與 Playback Telemetry 都附屬於它。
_Avoid_: 裸用 session

**Watch Session**:
Playback Session 期間以心跳累積的觀看時長紀錄。

**Watch History**（觀看紀錄）:
使用者對每部 Video 的最新觀看狀態，含 Watch Progress 與是否看完。
_Avoid_: 觀看記錄

**Watch Progress**:
使用者在某部 Video 看到的時間點。
_Avoid_: progress_seconds / watch_progress 兩種叫法混用

**Continue Watching**（繼續觀看）:
Watch History 中尚未看完且已有進度的 Video。
_Avoid_: 續看, 繼續播放

**Favorite**（收藏）:
使用者標記保留的 Video。

### 推薦

**Daily Recommendation**（今日推薦）:
某一天呈現給使用者的推薦 Video；由 admin 挑選，當天無人挑選時由 Fallback Recommendation 補上。
_Avoid_: 精選, 每日推薦, picks, curated picks

**Fallback Recommendation**（為你推薦）:
當天沒有 admin 挑選時，系統從該使用者未看過的 Video 隨機選出的 Daily Recommendation。

**Up Next**（接著看）:
播放頁隨機列出的其他 Video；**不是**推薦，不含任何個人化。
_Avoid_: 推薦

### 串流

**Play Mode**:
一部 Video 的播放方式，依其 codec 與容器分類為 direct、remux、transcode 三者之一。
_Avoid_: DirectPlay / DirectStream（只在與 Jellyfin 對照時使用）

**Play Mode Classification**:
依 Media Info 決定 Play Mode 的判斷。
_Avoid_: 裸用 classification, 相容性分類

**Stream Source**（播放來源）:
播放端把一部 Video 接到播放器的那一段：依 Play Mode 選擇直連或 HLS、Keyframe Index 準備中時輪詢、Stream Token 過期時換新並回到原位置、從 Watch Progress 繼續。
_Avoid_: player source, 串流來源（指後端時用 HLS／Stream 端點）

**Transcode Play Mode**:
Play Mode = transcode，需即時轉碼才能播放；尚未實作。
_Avoid_: 影片相容性 Phase 2

**HLS Segment**:
remux/transcode 播放時實際送給播放器的一個分片檔。
_Avoid_: part, 裸用 segment

**Segment Boundary**:
Keyframe Index 中定義一個 HLS Segment 起訖的時間切點。

**Keyframe Index**:
一部 Video 的 keyframe 時間表，用來預先切出 Segment Boundary。

**Stream Token**（串流憑證）:
只能用於串流請求、放在 URL 上的短效憑證。

**Token Scope**:
憑證可用範圍（如 stream）。
_Avoid_: 裸用 scope

**Concurrent Streams**:
同一時間進行中的串流數。
_Avoid_: session 數

### 播放品質

**Playback Telemetry**（播放遙測）:
一個 Playback Session 結束時回報的播放品質紀錄 —— 首播延遲、stall、吞吐、Play Mode、Network Scope。

**Stall Family**:
卡頓的成因分類：starved（缺位元組）或 codec（解碼失敗）。
_Avoid_: 裸用 classification

**Network Scope**:
播放時的網路情境：lan 或 external。
_Avoid_: 裸用 scope

### 使用者

**Role**:
使用者的權限等級：admin 或 viewer。

**Disable**（停用）:
讓使用者無法登入但保留其資料；系統不做硬刪除。
_Avoid_: 刪除使用者

**Login Token**:
登入後取得、用於 API 請求的 JWT。
_Avoid_: session
