# SPEC — 播放器網速 / 緩衝 HUD（stats-for-nerds 疊層）

> 場景：**Feature**（Spec → Plan → 實作）
> 狀態：已確認，實作中
> 日期：2026-06-21
> Skill：agent-skills:spec（故置於 `docs/agent-skill/specs/`）

---

## 1. Objective（目標與使用者）

在 `PlayerPage` 既有的 `<video>` 上疊一個 **always-on** 的診斷疊層（stats-for-nerds），
讓使用者在原始檔漸進式串流（無轉碼 / 無 ABR）下，**一眼看出「現在卡不卡、為什麼卡」**。

- **主使用者**：自己（單人個人平台），用於播放體驗的即時診斷與未來 HLS/ABR 改造前的 baseline 觀測。
- **主訊號**：**緩衝餘量秒數（buffer headroom）** — 它是「會不會卡」最直接的領先指標。
- **輔助訊號**：下載 Mbps、RTT、rebuffer 次數。
- **核心價值**：當畫面停住時，**明確區分兩種根因**：
  - 🟠 **缺位元組（starved / 緩衝）** — bytes 還沒到，等一下會自己恢復，或網路不足。
  - 🔴 **codec 解不了碼（decode / unsupported）** — bytes 到了但解碼器吃不下，等再久也沒用。

**非目標（明確排除）**

- ❌ 不做對稱的上傳/下載雙錶（個人串流的上傳量無觀測意義）。
- ❌ 不做轉碼、ABR、HLS（那是 roadmap 下一階段，見 `[[project_streaming_roadmap]]`）。
- ❌ 不改後端、不加任何執行期 npm 套件（測試框架 vitest 為已批准的 dev-only 例外）。
- ❌ 不做歷史圖表 / 錄製 / 匯出（保持輕量疊層）。

### 成功定義（Definition of Done）

1. 播放時疊層即時顯示：buffer headroom（主，大字）、↓Mbps、RTT、rebuffer 次數、狀態徽章。
2. 用 DevTools 把網速調到極慢 → 疊層進入 **🟠 缺位元組 / BUFFERING**，rebuffer 次數遞增。
3. 餵一個壞掉/不支援的 codec → 疊層進入 **🔴 codec 解不了碼**，且**不會**被誤判成緩衝。
4. `vitest`、`tsc -b`、`eslint` 全綠；疊層在正常播放時不造成可感知的卡頓或 re-render 風暴。

---

## 2. Commands（建置 / 驗證；container-first）

> 依 `[[feedback_container_dev]]`：所有操作透過容器，不裝本地 Node。
> 開發期驗證用一次性 `node:20-alpine` 容器 bind-mount `web/`（`vaultflix-web` 服務本身是 build-only，無 source 掛載）。

```bash
# 一次性容器：安裝（含新增的 vitest）→ 跑測試 / 型別 / lint
docker run --rm -v "$PWD/web:/app" -w /app node:20-alpine sh -c "npm install && npm test"
docker run --rm -v "$PWD/web:/app" -w /app node:20-alpine sh -c "npm run build"   # tsc -b + vite build
docker run --rm -v "$PWD/web:/app" -w /app node:20-alpine sh -c "npm run lint"

# 前端發版（named volume 陷阱，見 CLAUDE.md）：
docker compose build vaultflix-web
docker compose down vaultflix-web vaultflix-nginx
docker volume rm vaultflix_web_dist
docker compose up -d vaultflix-nginx
```

> ⚠️ 既有 `web_dist` named volume 不會被新 image 覆寫——改完前端若瀏覽器行為沒變，先查 `web_dist` 是否重建。

---

## 3. Project Structure（檔案異動）

| 動作 | 檔案 | 說明 |
|------|------|------|
| 新增 | `web/src/utils/playbackStats.ts` | 純函式：buffer headroom、Mbps 視窗聚合、`MediaError`→phase/family 映射（吃**純資料**、無 DOM 依賴 → 可在 node env 測） |
| 新增 | `web/src/utils/playbackStats.test.ts` | vitest 表格驅動測試 |
| 新增 | `web/src/hooks/usePlaybackStats.ts` | 量測 hook：counters 用 `useRef`，只在顯示頻率（~2Hz）`setState` 一份快照 |
| 新增 | `web/src/components/NetworkHud.tsx` | 純展示疊層；吃 `PlaybackStats`，依 `family` 上色 |
| 修改 | `web/src/pages/PlayerPage.tsx` | 呼叫 hook、在既有 `relative` wrapper 內把 `<NetworkHud>` 疊在 `<video>` 上 |
| 修改 | `web/package.json` | 加 `vitest` devDependency 與 `test` script |
| 新增 | `web/vitest.config.ts` | `environment:'node'`、`include: src/**/*.test.ts` |

- 不新增路由、不改 API client、不改 `types/index.ts`（型別放在 hook/util 旁）。
- `PlaybackStats` 型別是 hook 與 component 之間唯一的契約。

---

## 4. 功能契約（指標定義與狀態機）

### 4.1 Hook ↔ Component 契約

```ts
type PlaybackPhase =
  | 'idle' | 'playing' | 'paused'
  | 'buffering'      // 缺位元組（暫時 starved，可自行恢復）
  | 'network-error'  // 缺位元組（MEDIA_ERR_NETWORK：bytes 拿不到的硬失敗）
  | 'decode-error'   // codec 解不了碼（MEDIA_ERR_DECODE）
  | 'unsupported'    // codec 解不了碼（MEDIA_ERR_SRC_NOT_SUPPORTED）

type StatsFamily = 'ok' | 'starved' | 'codec'  // 疊層上色與標題依此

interface PlaybackStats {
  phase: PlaybackPhase
  family: StatsFamily
  bufferAheadSec: number | null  // 主訊號：buffered 內含 currentTime 的 range.end - currentTime
  downlinkMbps: number | null    // 近窗聚合；無樣本時 null（顯示「—」）
  ttfbMs: number | null          // 最近一筆 /stream range 的 responseStart - requestStart
  rebufferCount: number          // 本次播放（per video id）累計 stall 次數
}
```

### 4.2 指標來源（全部瀏覽器端、零後端）

| 指標 | 來源 | 計算 |
|------|------|------|
| **buffer headroom（主）** | `video.buffered` (TimeRanges) | 找含 `currentTime` 的 range，`end - currentTime`；落在 gap 取 0 |
| **↓ Mbps（估算）** | `video.buffered` 邊緣成長 × 平均位元率 | `(Δ bufferedEnd / Δ wallclock) × (file_size_bytes×8/duration) / 1e6`，EWMA 平滑。負成長（seek/eviction）夾為 0 |
| **RTT（= TTFB）** | `PerformanceObserver({type:'resource'})` | 最近一筆 `/stream` 的 `responseStart - requestStart`；**超過 10s 無新請求 → 顯示「—」**（含伺服器處理時間，非純網路 RTT） |
| **rebuffer 次數** | `<video>` `waiting` 事件 | `currentTime > 0 && !video.seeking` 時 +1（避免把 seek 造成的 waiting 算進去） |

> **為何 ↓Mbps 不用 Resource Timing 逐塊量測**：實測此後端對每次 seek 回一個 open-ended `Range` 206、用單一連線把整段 body 漸進串流，穩態播放時瀏覽器**不再產生新的 per-chunk resource entry**（實測 6 筆全在初載），故 Resource Timing 無樣本可量。改用緩衝邊緣成長 × 平均位元率：零後端、永遠有值、停滯時≈0。
>
> **此估算的特性**：buffer 滿且瀏覽器停止下載時讀數會掉到 ~0（邊緣凍結），補滿時 spike；**網路有壓力（持續下載）時最準**，實測 Slow 3G 下穩定 ~0.3 Mbps 吻合連線速率。RTT 改用 Resource Timing TTFB（seek/初載時有值），同源無 opaque 問題。

### 4.3 狀態機（讀取優先序，每次顯示週期重新判定）

```
1. video.error 存在：
     code 2 MEDIA_ERR_NETWORK           → network-error  (family: starved 🟠)
     code 3 MEDIA_ERR_DECODE            → decode-error   (family: codec   🔴)
     code 4 MEDIA_ERR_SRC_NOT_SUPPORTED → unsupported    (family: codec   🔴)
     code 1 MEDIA_ERR_ABORTED           → idle           (family: ok)
2. 否則 video.paused                     → paused         (family: ok)
3. 否則 stalled
     （waiting 後尚未 playing/canplay，或 readyState < HAVE_FUTURE_DATA=3）
                                         → buffering      (family: starved 🟠)
4. 否則                                   → playing        (family: ok 🟢)
```

**這就是「缺位元組 vs codec」的判別核心**：
缺位元組家族＝`buffering` + `network-error`；codec 家族＝`decode-error` + `unsupported`。
兩者顏色、標題、文案都不同，使用者不會混淆。

---

## 5. Code Style & 架構規範

遵循 CLAUDE.md 前端規範，重點落實：

- **計數一律 `useRef`，不用 `useState`**：rebuffer 次數、Mbps 樣本 ring buffer、最新 TTFB、stalled flag、PerformanceObserver instance 全放 ref。
- **單一 setState 來源**：以一個 `setInterval`（~500ms）為唯一發布點——讀所有 ref + 即時 DOM（`buffered`/`readyState`/`error`）組成 `PlaybackStats` 快照後 `setState`。事件 handler 只改 ref，不直接 setState → 杜絕事件驅動 re-render 迴圈。
- **`useEffect` cleanup**：interval、PerformanceObserver、所有 `addEventListener` 在 cleanup 全移除；依賴只放 video element 與 stream 路徑。
- **純函式可審查＋可測**：buffer headroom、Mbps 聚合、`MediaError`→phase/family 抽到 `utils/playbackStats.ts`，吃純資料、無 DOM 依賴 → vitest 表格驅動測試。
- 與既有 `handleVideoError`（token 刷新重試）**共存**：HUD 只**讀** `video.error`，不攔截既有 `onError` 重試流程。
- Tailwind className 風格對齊現有元件；不引入新格式庫。

---

## 6. Testing Strategy（vitest 已批准）

1. **單元測試（vitest，node env）**：對 `utils/playbackStats.ts` 三個純函式做表格驅動測試，覆蓋：
   - `bufferAhead`：currentTime 在 range 中段 / 邊界 / gap / 空 buffered / 多段 range。
   - `aggregateMbps`：無樣本→null、單樣本、多樣本視窗、超出視窗的舊樣本被排除、0 bytes 樣本忽略。
   - `classifyPhase`：每個 `MediaError` code（1/2/3/4）對應正確 phase+family、paused、stalled、playing；**確認 decode/unsupported 不會落到 starved family**（核心需求的回歸測試）。
2. **靜態驗證**：`tsc -b`（型別）+ `eslint`（hooks 規則）全綠。
3. **瀏覽器 runtime 驗證**（Chrome DevTools MCP）：
   - 正常播放 → 主數字隨播放增減、Mbps/RTT 有值、family=ok 🟢。
   - DevTools Network 限速到極慢 → family 轉 🟠 BUFFERING、rebuffer 次數遞增、Mbps 掉。
   - 餵壞 codec / 不支援格式 → family 轉 🔴（decode/unsupported），**不被誤判為緩衝**。
   - 連續播放數分鐘 → 無 re-render 風暴、無記憶體成長。

---

## 7. Boundaries

| 一律做（Always） | 先問（Ask first） | 絕不（Never） |
|---|---|---|
| 計數用 `useRef`、單一 setState 來源 | 改動 `handleVideoError` 既有重試行為 | 新增**執行期** npm 套件（vitest 為已批准 dev-only 例外） |
| cleanup 所有 observer/listener/interval | 把 HUD 從 always-on 改成可切換 | 改任何後端 / API |
| 純邏輯抽成可審查＋可測的純函式 | 加歷史圖表 / 匯出等延伸 | 做上下傳雙錶 |
| 同源 `/stream` 路徑過濾 Resource Timing | 引入 `navigator.connection`（已選 TTFB） | 攔截 / 吞掉 `<video>` 既有事件 |

---

## 8. 設計決策與 Trade-offs

1. **↓Mbps 改用緩衝成長估算（實測後修正）**：原訂用 Resource Timing 逐塊量測，但實測此漸進式串流後端穩態播放無 per-chunk entry（見 §4.2）。改為 buffered 邊緣成長 × 平均位元率。缺點：用**平均**位元率（非瞬時）、buffer 滿且閒置時讀數 ~0（非連線上限）、estimate 標籤。優點：零後端、永遠有值、有壓力時最準。
2. **RTT = TTFB（已選）+ 10s 新鮮度閘**：`responseStart - requestStart`。實測此後端只在 seek/初載產生 entry，故 RTT 多數時間為「—」（誠實，無新請求可量）。缺點：含伺服器處理時間。
3. **always-on 疊層（已選）**：診斷最直接。缺點：對單純看片是視覺干擾；改可切換是未來一行 state，本次不做。
4. **單一 setInterval 發布**：簡單、可預期。缺點：徽章最差 ~500ms 延遲，可接受。
5. **rebuffer 計數抑制 seek**：`!video.seeking` 才計。**實測驗證**：seek 進未緩衝區 stall 時 ⟳ 不動；正常播放 stall 時 ⟳ 遞增。缺點：seek 後緊接的真實 stall 可能漏算（罕見）。
6. **加 vitest（dev-only）**：27 個表格驅動測試（含「codec 不被誤判為緩衝」守門 + throughput 估算邊界）。代價：多一個 dev 依賴與一份 config。
7. **hook 對 mount 順序免疫**：publish tick 即時讀 `videoRef.current` 並 lazy-bind listener，interval 只 key 在 streamPath。修掉「video 元素晚於 effect 掛載 → HUD 卡在 idle」的實測 bug。

## 8.1 瀏覽器實測結果（Chrome DevTools MCP，admin 帳號）

- ✅ HUD 渲染於播放器左上、隨播放即時更新；buffer headroom 主訊號精準（補滿增、播放減、節流下 1:1 排空）。
- ✅ ↓Mbps 估算即時有值；Slow 3G + seek 進未緩衝區 → 穩定 ~0.3 Mbps 吻合連線。
- ✅ 緩衝 / 缺位元組：seek 進未緩衝 + Slow 3G → `0.0s buffer` + 🟠 `緩衝中 · 缺位元組`，readyState 回 3 轉 `播放中`。
- ✅ rebuffer：seek stall 不計（⟳ 持平）、正常播放 stall 遞增（⟳ 1→2→3）。
- ✅ RTT 新鮮度閘：10s 無新請求 → 「—」。
- ➖ codec 路徑：靠 27 單元測試（React 控管 `<video src>` + handleVideoError 自動復原，瀏覽器無法穩定注入真實 decode error）。

---

## 9. 假設（Assumptions）

1. HUD 疊在 `<video>` 角落（半透明深色底、等寬字），桌面為主；行動版不特別優化版位。
2. 文案繁中為主；單位：headroom 秒（1 位小數）、Mbps（1 位小數）、RTT 毫秒（整數）。
3. rebuffer「一次播放」= per video id；token 刷新重載（同片）不歸零，換片才歸零。
4. Mbps 聚合視窗 ~5s、顯示頻率 ~2Hz（500ms）。
5. 已確認事項：① 加 vitest 測試框架；② Resource Timing 風險接受（Chrome 主用）；③ 本檔置於 `docs/agent-skill/specs/`。
6. 不動 `handleVideoError` 既有 token 重試；HUD 對 `video.error` 只讀不寫。
