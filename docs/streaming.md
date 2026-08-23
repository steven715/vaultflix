# Vaultflix 串流架構參考

> 2026-08 從「remux 播放斷斷續續」的除錯討論中沉澱。涵蓋：播放路徑分流、容器與編碼的區別、
> 緩衝策略的歸屬、串流策略光譜（對照 YouTube / Jellyfin）、規模前提與瓶頸分析。
> 場景定位的正式決策見 [ADR-0009](adr/0009-jellyfin-scale-on-the-fly-streaming.md)。

---

## 1. 容器 vs 編碼

- **編碼（codec）**：畫面/聲音的壓縮演算法（h264 / hevc / mpeg4 / wmv；aac / mp3 / ac3）。
  解碼需要對應的解碼器 —— 瀏覽器沒內建的編碼就是天書，只能**轉碼**（解開重壓，分鐘級、吃滿 CPU）。
- **容器（container）**：把壓縮好的視訊流、音訊流、字幕、時間戳索引封裝在一起的檔案格式
  （MP4 / MKV / AVI / WMV）。不碰壓縮，只管封裝與尋址。容器不相容只需要
  **remux**（拆盒換盒，`ffmpeg -c copy`，秒級、幾乎零 CPU）。

同一份 h264 視訊流裝在 MP4 或 MKV，內容物一模一樣 —— 這就是 remux 便宜、transcode 昂貴的原因。

## 2. 三條播放路徑

分流邏輯在 `internal/service/play_mode.go`，判準是「瀏覽器能不能直接解這個容器+編碼」，
不是單看副檔名（裝著 mpeg4 的 MP4 一樣進 transcode）：

| play_mode | 條件 | 播放方式 | 緩衝誰管 |
|---|---|---|---|
| **direct** | MP4/MOV 容器 + h264 + aac/mp3 | 原生 `<video src>` + HTTP Range（`http.ServeFile`，可 X-Accel 卸載給 nginx，見 ADR-0008） | 瀏覽器（不可控） |
| **remux** | 編碼相容、容器不相容（MKV/AVI） | keyframe 索引 → VOD manifest → 逐段 on-demand `ffmpeg -c copy` 切 mpegts（`internal/streaming/`）→ hls.js (MSE) 播放 | hls.js config（可控） |
| **transcode** | 編碼不相容（mpeg4/wmv/hevc…） | 尚未實作（Phase 2），前端顯示占位訊息 | —— |

片庫分布（2026-08-24 快照）：direct 285 部 / 521GB、remux 124 部 / 200GB、transcode 89 部 / 143GB。

## 3. 緩衝策略：執行在客戶端，策略是業務參數

**兩條可播路徑都有前向緩衝**，差別只在「誰決定策略」：

- **direct**：Chrome 自己持續發 Range request 預讀、維持它自定的窗口（幾十秒~分鐘級），
  囤夠暫停、消耗再續。程式端無法控制（`preload` 只是提示）。telemetry 實測 direct 下載速率
  50–118 Mbps、rebuffer 0，區網下綽綽有餘。Jellyfin 的 DirectPlay 網頁端同構，一樣交給瀏覽器。
- **remux**：hls.js 依 config 維持前向緩衝，目前 `new Hls()` 全預設 =
  `maxBufferLength` 30 秒、`maxBufferSize` 60MB、`maxMaxBufferLength` 600 秒上限。
  「播放時看起來總是實時加載」就是這個 30 秒目標的表現，是預設值而非缺陷。

分層原則（對照 YouTube/Netflix 的做法）：**緩衝的執行必然在各平台播放器裡；
緩衝的策略參數是業務邏輯，應集中管理、由伺服器下發**。對本專案即：緩衝參數不寫死在前端，
走「執行期可調性原則」（CLAUDE.md）由後端 playback config 提供 —— 未來有第二種客戶端時共用同一份。

平台注意事項：
- hls.js 不是 PC 專屬 —— 前提是 MSE，Android Chrome/PWA 完全支援，M/PC 走同一條路徑同一套參數。
- 唯一例外是 **iOS Safari**（不開放 MSE），走 PlayerPage 的原生 HLS fallback，緩衝由 Safari 決定。
  這是所有網頁播放器的共同限制，Jellyfin/Plex 亦同。
- jellyfin-web 的調參史是現成教訓：曾把 buffer 上限壓到 6 秒躲 Chrome `BufferFullError`，
  結果任何網路抖動都變成可見卡頓，最後回歸 hls.js 預設。**結論：buffer 寧大勿小**；
  區網頻寬成本 ≈ 0，唯一硬限制是瀏覽器的 MSE 配額（數十~百餘 MB 級，塞爆拋 `QuotaExceededError`，
  也因此「全片預載」在 MSE 架構下物理上做不到）。

## 4. 串流策略光譜

| 策略 | 代表 | 優點 | 缺點 |
|---|---|---|---|
| 整檔下載 | 舊式網頁 | 零實作成本 | 不能 seek 未下載處、頻寬全浪費；已淘汰 |
| HTTP Range | 本專案 direct | 伺服器零運算、原檔直出、CDN 天然支援 | 容器/編碼受瀏覽器限制、緩衝不可控、無多畫質 |
| 分段 HLS/DASH + MSE | 本專案 remux；Jellyfin | 來源彈性（即時 remux/transcode）、緩衝可控、可 ABR、per-segment 權限 | 複雜度高一個量級（manifest/切段/時間戳對齊）、首播延遲 |
| 預處理 + 靜態分發 | YouTube | 播放路徑零運算、無限水平擴展 | 存儲換運算：入庫時全轉標準格式階梯，衍生檔管理 |

**YouTube 模式**：上傳收一切格式，入庫一次性轉成固定 ladder（H.264 全解析度 + 熱門加轉 VP9/AV1，
音訊分離），之後永遠靜態分發。播放器只選畫質和拉段。不全片預載的三個理由：頻寬經濟學
（多數人不看完）、MSE 配額、保留 ABR 換畫質的彈性 —— 前兩者在本專案區網場景不成立或影響小。

**Jellyfin 模式**（本專案同路線）：保留原檔、不預轉碼、即時處理。
播放決策由客戶端 DeviceProfile 協商：DirectPlay > DirectStream(remux) > Transcode，永遠選最便宜的可行路徑。
值得借鑑的兩個設計：
1. **Keyframe 對齊不信任 `ffmpeg -ss`** —— 用 `MatroskaKeyframeExtractor` 直接解析 MKV EBML/Cues
   拿 keyframe 位置（ffprobe 只是 fallback）。本專案 2026-08 發現的 remux 斷續 bug
   （`-ss <keyframe_pts>` 在部分 MKV 上落點提早一個 GOP，切出的段與 manifest 錯位、相鄰段大量重疊）
   正是 Jellyfin 用這個設計繞開的坑。
2. **一條連續 ffmpeg + Transcoding Throttler** —— 不是 per-segment spawn；ffmpeg 領先播放進度超過
   `ThrottleDelaySeconds` 就暫停程序省 CPU，seek 未轉碼區 = 殺掉重啟。

## 5. 規模前提與瓶頸

本專案場景 = Jellyfin 場景：**個人/少數使用者、區網為主、偶爾 ngrok 對外分享**。
不做 YouTube 式規模（見 ADR-0009）。多人並發時的瓶頸順序：

1. **上行頻寬（最先撞到，direct 也逃不掉）**：所有流量過家用上行 + ngrok 隧道，
   1080p 一條 4–8 Mbps，40 Mbps 上行約 5–8 個觀眾就滿。ngrok 免費方案另有流量/連線限制，
   且多繞邊緣節點、延遲抖動上升 —— 此時 remux/HLS 反而是體驗更可控的路徑（buffer 可調大吸收抖動）。
2. **transcode CPU（Phase 2 後的真殺手）**：軟轉 1080p 約 2–4 核/路，無 GPU 時 2–3 路並發即打死普通機器。
   對策比照 Jellyfin：硬體加速、限制並發轉碼數、或干脆對特定使用者禁轉碼。
3. **remux 很便宜**：`-c copy` 單段 0.1–0.5s、CPU 極低；SegmentCache 有同段 singleflight 去重 + LRU 快取，
   「多人看同一部」成本趨近 direct，會炸的是「多人同時看不同 remux 片」。

若未來「對外多人」變成常態需求，正確方向不是加硬體，而是切換到預處理路線：
全庫約 +350GB 磁碟（remux 124 部預先換盒 ~200GB、transcode 89 部一次性轉碼 ~150GB）
即可讓全庫變 direct、即時管線整類消失 —— 該選項已記錄於 ADR-0009 的重新評估條件。
