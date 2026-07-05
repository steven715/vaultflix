# PlayerPage 右側欄調整：接著看 5 部 + 今日推薦區塊

- 場景：Feature
- 分支：`feat/player-up-next-recommendations`（從 `main` 開）
- 日期：2026-07-05

## 目標

播放頁（`web/src/pages/PlayerPage.tsx`）右側欄調整：

1. 「接著看」清單從目前顯示的 8 部（fetch 12）改成 **5 部**。
2. 右側欄新增「今日推薦」區塊，顯示首頁同一份「今日推薦」資料的 **5 部**（同一資料來源，不另做推薦邏輯）。
3. 推薦區塊「每次進到播放頁就重新抓」：即使從「接著看」點進另一部（`:id` 改變），甚至再點到同一部，也要重新觸發 refetch。

## 現況

- 接著看資料：[PlayerPage.tsx:115-127](../../../web/src/pages/PlayerPage.tsx#L115-L127)
  `listVideos({ page:1, page_size:12, sort_by:'created_at', sort_order:'desc' })`，
  `.filter((v) => v.id !== id).slice(0, 8)` 存入 `upNext` state，依賴 `[id]`。
- 接著看元件：[UpNextList.tsx](../../../web/src/components/UpNextList.tsx)，props `{ items: VideoWithTags[] }`，垂直 row（PosterThumb + 標題 + resolution + tags[0]）。
- 今日推薦 API：[recommendations.ts](../../../web/src/api/recommendations.ts) 的 `getTodayRecommendations(fallbackCount?)` → `RecommendationItem[]`，打 `GET /recommendations/today`。
- 首頁 [BrowsePage.tsx:69-84](../../../web/src/pages/BrowsePage.tsx#L69-L84) 已用 `getTodayRecommendations()`，其 rails effect 依賴 `[query, reloadKey, location.key]`（location.key 讓同路徑再導航也 refetch）。
- `RecommendationItem`（[types/index.ts:156](../../../web/src/types/index.ts#L156)）有 `video_id, title, thumbnail_url?, duration_seconds, resolution`，**無 `tags`**，與 `VideoWithTags` 不同。

## 設計

### 需求 1：接著看 5 部

改 [PlayerPage.tsx:118-121](../../../web/src/pages/PlayerPage.tsx#L118-L121)：

```tsx
listVideos({ page: 1, page_size: 6, sort_by: 'created_at', sort_order: 'desc' })
  .then((res) => {
    if (cancelled) return
    setUpNext(res.data.filter((v) => v.id !== id).slice(0, 5))
  })
```

- `page_size` 用 **6**（不是字面上的 5）：若當前影片剛好落在最新 5 部內，排除後只剩 4 部湊不滿。fetch 6、排除當前、取 5，保證滿 5 部。這是需求「湊滿 5 部」的正確解讀。
- 排除當前影片的 `.filter((v) => v.id !== id)` 邏輯保留。
- 依賴維持 `[id]`（純由 `:id` 決定的 deterministic 清單，**不加 location.key**，避免每次導航多打一次 API）。

### 需求 2：今日推薦區塊

- 新元件 `web/src/components/RecommendationList.tsx`，props `{ items: RecommendationItem[] }`，比照 `UpNextList` 的垂直 row 樣式（PosterThumb + 標題 + resolution），因 `RecommendationItem` 無 tags 故省去 tag 行。`items.length === 0` 回傳 `null`。每列 `<Link to={/videos/${item.video_id}}>`。
- PlayerPage 新增 `const [recommendations, setRecommendations] = useState<RecommendationItem[]>([])`。
- 資料來源 `getTodayRecommendations(5)`（帶 `fallback_count=5`，與首頁同一份資料，不另做邏輯），並 `.slice(0, 5)` 保險。
- 右側 `<aside>` 內排版：上「接著看」（`<h2>接著看</h2>` + `<UpNextList>`），下「今日推薦」（`<h2>今日推薦</h2>` + `<RecommendationList>`），兩塊間留間距。

### 需求 3：每次進頁 refetch（location.key）

新 effect，依賴 `[location.key]`：

```tsx
const location = useLocation()
useEffect(() => {
  let cancelled = false
  getTodayRecommendations(5)
    .then((items) => !cancelled && setRecommendations(items.slice(0, 5)))
    .catch((err) => console.warn('failed to load recommendations', err))
  return () => { cancelled = true }
}, [location.key])
```

- React Router 每次導航（含同路徑 `:id` 不變、或換 `:id`）都產生新的 `location.key` → effect 重跑 → refetch。這是 CLAUDE.md「同路徑重新導航的 refetch」章節的做法（首頁輪播推薦同款）。
- cleanup flag `cancelled` 防 unmount 後 setState。
- 無重試邏輯；若日後加，計數用 `useRef`（CLAUDE.md 規範）。

## 測試

`web/src/pages/PlayerPage.test.tsx`（擴充）+ `web/src/components/RecommendationList.test.tsx`（新增）：

1. **接著看限 5 部**：mock `listVideos` 回 6 部（含當前 `v1`），斷言排除 `v1` 且只渲染 5 列。
2. **推薦區塊渲染**：mock `getTodayRecommendations` 回 5 部，斷言「今日推薦」標題與 5 張推薦卡出現。
3. **location.key 觸發 refetch**：在 PlayerPage 測試中導航到另一 `:id`（及同一 `:id`），斷言 `getTodayRecommendations` 再度被呼叫（呼叫次數增加）。
4. **RecommendationList 單元**：空陣列渲染 `null`；非空渲染每列標題與正確 `/videos/:video_id` 連結。

現有 PlayerPage 測試的 `beforeEach` 需補 mock `api/recommendations` 的 `getTodayRecommendations`（預設回 `[]`），避免未 mock 造成的網路呼叫/例外。

## Trade-off / 缺點

- 多一個 `RecommendationList.tsx`，與 `UpNextList` 有樣式重複。刻意不共用：兩者型別（`VideoWithTags` vs `RecommendationItem`）不同，強行泛化會引入 union/shim，增加耦合與複雜度，不划算（YAGNI）。
- 推薦 effect 每次導航多打一次 `/recommendations/today`。這正是需求 3 要的行為，可接受；接著看仍只依賴 `[id]` 不受影響。

## 完成條件

- `task verify` 綠（含前端 tsc / eslint zero-warning / vitest）。純前端改動至少 `task test-fast` 綠。
- 補上上述測試。
- 收工前本機 `task deploy`（重 build 不可變 nginx image），到 http://localhost:3000 播放頁實機確認：接著看 5 部、推薦區塊 5 部、從接著看點進另一部時推薦有重新載入。
