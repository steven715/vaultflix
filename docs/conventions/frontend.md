# 前端慣例（React + TypeScript）

> 動 `web/src/` 前先讀。

## 前後端 Response 契約

後端所有成功回應使用 `{ data: ... }` wrapper（`SuccessResponse`）。前端 API client 層是唯一處理這個契約的地方：

- axios response interceptor 統一解開 `SuccessResponse` wrapper，讓呼叫端直接拿到 `data` 內容
- 個別 API function 不應出現 `res.data.data` 的雙層解包 — 如果需要這樣寫，代表 interceptor 沒有正確處理
- 分頁回應（`PaginatedResponse`）因為頂層就包含 `total`、`page` 等欄位，不適用自動解包，需在 interceptor 中區分處理

```typescript
// ✅ 正確：interceptor 統一解包，呼叫端簡潔
// client.ts
client.interceptors.response.use((response) => {
  // PaginatedResponse has 'total' at top level, so skip those
  if (response.data && 'data' in response.data && !('total' in response.data)) {
    response.data = response.data.data
  }
  return response
})
// auth.ts
const res = await client.post<LoginResponse>('/auth/login', { username, password })
return res.data // 直接是 { token: "..." }

// ❌ 錯誤：每個呼叫端自己解包，容易遺漏
return res.data.data
```

## 給使用者看的錯誤訊息用 `errorMessage`

API client 一律以 `ApiError`（`api/errors.ts`）reject，帶 `status`、錯誤碼 `code` 與後端的 `serverMessage`。後端的 message 是給 API 呼叫端看的英文；畫面上的中文由頁面負責：

- 用 `errorMessage(err, { fallback, <錯誤碼>: '中文' })`，列出這個操作預期的錯誤碼（鍵的型別是 `ErrorCode`，打錯字編譯不過），其餘一律顯示 fallback。不要顯示 `err.message`（會是 axios 的「Request failed with status code 409」），也不要自己轉型讀 `err.response`。
- 前端需要分辨的狀況，後端要給**專屬錯誤碼**（如 `username_taken`、`path_not_exist`），不要讓前端比對 message 字串或只看狀態碼。
- 例外：後端寫給 admin 看的訊息（目前只有 `job_lane_busy`，會寫出是哪個 Job 占著 Lane）由 `errorMessage` 直接顯示。

## 自動重試與錯誤處理

- 任何自動重試邏輯必須設定**重試上限**，且重試次數用 `useRef` 追蹤，不用 `useState`（避免觸發重新渲染導致迴圈）
- 事件驅動的錯誤處理（如 `<video onError>`）不可直接觸發導致同一事件再次發生的狀態更新，必須有中斷條件

```tsx
// ✅ 正確：ref 追蹤重試次數，超過上限停止
const retryRef = useRef(0)
function handleVideoError() {
  if (retryRef.current >= 1) return
  retryRef.current += 1
  refetchAndReload()
}

// ❌ 錯誤：無限重試 — error → refetch → setState → re-render → error → ...
function handleVideoError() {
  refetchAndReload() // 永遠重試，沒有上限
}
```

## useEffect 非同步操作

- `useEffect` 中執行 async 操作時，必須用 cleanup flag 防止 unmount 後的狀態更新
- 依賴陣列只放真正的觸發條件（如 route param `id`），不放 `useCallback` 包裝的函式引用

```tsx
// ✅ 正確：cleanup flag + 直接依賴 id
useEffect(() => {
  let cancelled = false
  const fetchData = async () => {
    const data = await getData(id)
    if (!cancelled) setData(data)
  }
  fetchData()
  return () => { cancelled = true }
}, [id])

// ❌ 錯誤：依賴 useCallback 函式引用，可能因 closure 不穩定導致重複執行
const fetchData = useCallback(async () => { ... }, [id])
useEffect(() => { fetchData() }, [fetchData])
```

## 頁面顯示的 API 資料用 `useRemoteResource`

頁面要載入並顯示後端資料（列表、搜尋結果、某日的推薦）時，用 `hooks/useRemoteResource`，不要再手寫 `useState` + `useEffect` + `cancelled`。手寫版本在各頁漏掉的地方都不一樣：錯誤看起來像空列表、沒有 loading、過期回應蓋掉新資料、重整時列表閃爍。

- 資料由某個值決定（日期、搜尋字串），把它當 `key` 傳入：key 變了就從 `loading` 重抓，舊 key 的資料與回應都會被丟掉。
- 畫面三態都要畫：`loading`、`error`（`ErrorBanner` + `onRetry={resource.retry}`）、`ready`。空列表只在 `ready` 時代表「沒有資料」。
- 編輯後重抓用 `reload()`：舊資料留在畫面上，失敗時 reject，頁面自己 toast；本地就能算出結果的（刪除、停用）用 `mutate()`，不必重抓。

```tsx
// ✅ 正確
const recList = useRemoteResource(() => listRecommendationsByDate(date), date)
recList.reload().catch(() => toast.error('無法更新推薦列表'))

// ❌ 錯誤：重抓沒有過期防護，換日期後舊日期的回應會蓋掉新列表
const items = await listRecommendationsByDate(selectedDate)
setRecs(items)
```

## 分頁清單用 `usePagedList`

有頁碼與總數的清單（影片管理、片庫、收藏、觀看紀錄）用 `hooks/usePagedList`，它建在 `useRemoteResource` 上，key 包含頁碼與其他條件，所以不會拿別頁或上一次查詢的總數來判斷。頁碼超出最後一頁時（刪掉最後一頁的最後一筆、過期的書籤），它自動 `setPage(last, { replace: true })`；本地移除用 `mutate` 調整 `total`，修正就會跟著發生；但 `total` 要等伺服器確認移除後才減，否則換到的那一頁會在移除生效前載入、拿到舊的總數（樂觀地先隱藏 items 沒問題）。Job 或批次操作後的重整用 `reload()`：列保留，失敗時頁面自己 toast，不要清空列表；已經知道結果的改動（刪除成功的項目）先用 `mutate` 反映，重整失敗時畫面才不會留著已刪掉的東西。頁碼存在網址或元件狀態都可以，由頁面傳入 `page` 與 `setPage`。

## 片庫的 URL 參數用 `useLibraryParams`

片庫查詢（頁碼、排序、搜尋字串、Tag 篩選）存在 URL，讀寫一律經 `hooks/useLibraryParams`，不要在頁面裡手寫 `searchParams.get` 與 `setSearchParams`。它套用後端的預設值（第 1 頁、每頁 20、最新在前），而且**任何排序或篩選的改變都回到第 1 頁**：舊頁碼指向的是另一份清單（影片管理頁的欄位排序曾經停在原頁碼）。元件只回報使用者點了哪一欄、選了哪些 Tag，排序方向與換頁由 hook 決定。

## 日曆日期用 `lib/localDate`，不要用 `toISOString()`

使用者眼中的「某一天」（推薦的日期、「今天」）是**本地**日曆日期。`Date.toISOString()` 是 UTC：在台灣（UTC+8）早上 8 點前它還是昨天，拿來 `.slice(0, 10)` 會差一天，加減日期也會跟著錯（推薦管理頁的 `>` 曾經按了沒反應、`<` 一次退兩天）。用 `localDate(date)` 產生 YYYY-MM-DD、用 `shiftLocalDate(date, days)` 加減。測試要用 `vi.stubEnv('TZ', 'Asia/Taipei')` 在 UTC 以東的時區跑，UTC 下重現不出來。

## 複製到剪貼簿用 `lib/clipboard` 的 `copyText`

`navigator.clipboard` 只在安全環境（https、localhost）存在；透過區網 http 存取（ADR-0009 的主要情境）時它是 `undefined`，直接呼叫會丟例外。用 `copyText(text)`：有 Clipboard API 就用，否則退回隱藏 textarea + `execCommand('copy')`，回傳是否成功；失敗時要告訴使用者，不要照樣顯示「已複製」。

## 假時鐘測試：前置條件要用 tick 逼出來，不要靠真實時間燒過去

`vi.useFakeTimers({ shouldAdvanceTime: true })` 讓假時鐘跟著真實時間前進，方便 awaited 的資料載入正常解析。代價是：**任何「靠這段等待剛好燒掉 N 毫秒」才成立的前置條件都是 flaky**，會隨機器快慢、CI 負載翻面 —— 同一份 tree 可以一次綠一次紅。

實際踩過的地雷：`usePlaybackStats` 是在 500ms 的 publish tick **裡面**才綁 `<video>` 的 listener（`streamPath` 比 `<video>` 早一個 render 變 non-null，effect 當下那次 `publish()` 看到的 `videoRef.current` 還是 null，而 effect 不會因為元素掛上而重跑）。測試若在任何 tick 跑之前就 `fireEvent.play()`，`onPlay` 根本還沒註冊 → `playStartRef` 是 null → ttff 算不出來 → unmount 的 telemetry beacon 被 `ttffMs == null && watchedMs <= 0` 這道 guard 擋掉 → 斷言整串垮。

規則：**要讓事件被聽到，先逼出綁定的那個 tick，再 fire 事件，再逼一次 tick 讓 hook 計算。**

```tsx
// ✅ 正確：每個 tick 都是明確逼出來的
await act(async () => { await vi.advanceTimersByTimeAsync(600) })  // tick 1：hook 綁上 listener
fireEvent.play(videoEl)                                            // 這下才聽得到
await act(async () => { await vi.advanceTimersByTimeAsync(600) })  // tick 2：hook 算 ttff

// ❌ 錯誤：假設 awaited 的載入「會順便」讓 interval 跑過一輪
await screen.findByText('T')
fireEvent.play(videoEl)        // 機器夠快 → 一個 tick 都還沒跑 → 事件掉了
await act(async () => { await vi.advanceTimersByTimeAsync(600) })
```

判斷標準：把這個測試放到一台快 10 倍的機器上，結論還成立嗎？只要答案取決於「那段 await 花了多久」，就是 flaky。

## 測試紅燈先分辨 flaky 再分辨壞掉

CI 紅不代表「這次改動弄壞了什麼」—— `task verify` 是整包 gate，任何既有或隨機的失敗都會在下一個推 code 的人頭上炸開。判斷順序：

1. 這次改動有沒有碰到相關檔案？（`git diff --stat <上次綠的 commit> HEAD -- <路徑>`）
2. 沒碰到的話，比對 **tree hash**：`git rev-parse <A>^{tree}` vs `<B>^{tree}`。同 tree 不同結果 = flaky，不是壞掉
3. 本機重跑多次取得失敗率，別用單次結果下結論

## 同路徑重新導航的 refetch

點擊指向「目前所在路徑」的連結（如已在首頁時再點 logo / 首頁）不會改變 URL 參數，靠 `[query]`、`[searchParams]` 之類的依賴**不會觸發 refetch**，畫面看起來「卡住不更新」。需要「每次導航都重抓」的資料（如首頁輪播推薦、續看清單），依賴 `useLocation().key` —— React Router 每次導航（即使目標與現況相同）都會 push 新 entry 並產生新的 `location.key`。

```tsx
// ✅ 正確：同路徑再點 logo / 首頁也會 refetch
const location = useLocation()
useEffect(() => {
  if (query) return
  let cancelled = false
  getTodayRecommendations().then((items) => !cancelled && setRecommendations(items))
  return () => { cancelled = true }
}, [query, location.key])

// ❌ 錯誤：已在首頁時點 logo，query 沒變 → effect 不跑 → 推薦永遠不更新
}, [query])
```

判斷標準：這份資料是否預期「回到此頁就刷新」？是 → 加 `location.key`；否（純由 URL 參數決定、deterministic 的如分頁列表）→ 不加，避免每次導航都多打一次 API。

同一個陷阱也出現在程式碼自己改參數時：資料變了（例如 Import Job 完成）要帶使用者回第 1 頁看新資料，**已經在第 1 頁就重整（`usePagedList` 的 `reload()`），否則才改 page 參數**。只改參數時，在第 1 頁不會更新（影片管理頁曾經如此）；兩個一起做則會抓兩次。

## 具名導航控制要導到目的地，不要用 `navigate(-1)`

當一個按鈕/連結的文案承諾了**具體目的地**（如「返回片庫」、「回首頁」），它就必須導向那個路由（`navigate('/')`），不能用 `navigate(-1)` / `history.back()`。`navigate(-1)` 是「上一頁」而非「片庫」—— 當 back stack 裡上一頁剛好是**同類型頁面**時（如播放頁 A → 從「接著看」點進播放頁 B），按「返回片庫」會回到 A 而不是片庫，與文案不符。

判斷標準：文案是否指名一個固定目的地？是 → `navigate('<目的地>')`；否（純粹語意是「上一步」的通用返回鍵）→ 才用 `navigate(-1)`。

```tsx
// ✅ 正確：文案說「返回片庫」就導到片庫
<button onClick={() => navigate('/')}>返回片庫</button>

// ❌ 錯誤：B → 返回片庫 會回到上一個播放頁 A，不是片庫
<button onClick={() => navigate(-1)}>返回片庫</button>
```

## 在 effect 裡掛的 listener：先等 effect 跑完再 fire 事件

Stream Source 與 Playback Session 是在 `useEffect` 裡對 `<video>` 掛 listener（不是 JSX 的 `onXxx` prop）。`findByText` 等到畫面出現時，passive effect **不一定已經執行**，此時 `fireEvent.timeUpdate` 等事件沒有人在聽——測試會隨機失敗（實測約 1/8）。

規則：fire 媒體事件前，先用一個「effect 已執行」的可觀察證據等待，例如 `await waitFor(() => expect(getStreamToken).toHaveBeenCalled())`（兩個 hook 在同一次 commit 啟動）。PlayerPage 測試的 `sessionListening()` 就是這個用途。

## 「離開時送出」不能只靠 unmount

關閉分頁或瀏覽器**不會**觸發 React unmount，只靠 effect cleanup 送出的最後回報會整段遺失。離開時必須送出的資料（Watch Progress、心跳、遙測）同時掛在 `pagehide` 上，並用 keepalive 請求送出（`api/keepalive.ts`）。

手機（Android Chrome／PWA）上把 App 滑掉或系統殺掉背景分頁時常常**不觸發** `pagehide`，最後可靠的訊號是 `document` 的 `visibilitychange` 轉為 hidden：此時也送出 Watch Progress 與心跳。切到別的 App 很常見、之後可能繼續播放，所以遙測只在 `pagehide` 或真正結束時送。

`pagehide` 也會在頁面進入 back/forward cache 時觸發，頁面之後可能原樣回來：`pagehide` 只送出「離開時的回報」，不要結束 session；一次性的回報（遙測）自己防重送。

## 作業面板跟著「有沒有 job」顯示，不跟著「是否執行中」

Import Job、Backfill Job 這類背景作業的進度面板，條件寫成 `view !== null`（或比對 job 的對象，如 Media Source id），不要寫成 `phase === 'running'`。用 running 當條件，job 一結束面板就消失，使用者永遠看不到完成摘要與失敗清單（MediaSourcePage 曾經如此）。結束後的摘要留著，由使用者按「關閉」（`useJob` 的 `dismiss`）收掉；「執行中」只拿來決定按鈕是否 disabled、能不能關閉。

## 用到 WebSocket 的測試：用 `wsHarness`，不要 mock `useWS`

元件或 hook 讀 `useWS()`（如 `useJob`）時，用 `src/test/wsHarness.tsx`：`renderWithRouter(ui, { ws })`（頁面測試預設就會包一個）或 `renderHook(..., { wrapper: ws.Provider })`，再用 `ws.send(message)` 推送伺服器訊息，之後的非同步結果用 `findBy`／`waitFor` 等。需要斷線狀態或記錄送出的訊息時，擴充 harness 本身。它走真的 `WebSocketContext`，測到的是頁面實際收訊息的路徑；`vi.mock` 掉 `useWS` 則要每個檔案各寫一份，hook 一改到 context 就全部壞掉。
