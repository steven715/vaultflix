# 前端慣例（React + TypeScript）

> 動 `web/src/` 前先讀。常駐規則（Response 契約、重試上限）在 CLAUDE.md，這裡是踩過的坑沉澱出來的規則。

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

`pagehide` 也會在頁面進入 back/forward cache 時觸發，頁面之後可能原樣回來：`pagehide` 只送出「離開時的回報」，不要結束 session；一次性的回報（遙測）自己防重送。
