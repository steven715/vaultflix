# Tag 交互重新設計

## 目標

將 admin 影片管理頁面的標籤編輯從多步驟操作（下拉選單 + category 選擇 + 建立按鈕）改為 Jira 風格的 autocomplete tag input。同時簡化首頁 sidebar 的標籤篩選，從 category 分組改為按影片數量排序的扁平列表。

## 設計決策

- **移除 category 概念（前端層面）**：DB 的 `tags.category` 欄位保留（default `'custom'`），但前端不再顯示或要求使用者選擇 category。新建 tag 時固定傳 `custom`。
- **變更範圍限定在 admin 影片管理頁面和首頁 sidebar**：播放頁等其他地方的 tag 顯示不受影響。

## 變更範圍

### 1. Admin 影片管理頁面 — Tag 編輯 UI

**現在**：點「+」進入編輯模式 → 下拉選單選現有 tag 或輸入名稱 + 選 category + 按建立。

**改為**：Jira 風格 autocomplete tag input。

交互行為：
- 已加的 tag 顯示為 pill（小膠囊），每個 pill 右側有 × 按鈕可移除
- pill 後方緊接一個文字輸入框，placeholder 為「輸入標籤...」
- 打字時即時過濾現有 tag（client-side filter，資料來自已載入的 `GET /api/tags`），以 dropdown 顯示匹配項
- 已加在這部影片上的 tag 從 dropdown 中排除
- 點選 dropdown 中的現有 tag → 呼叫 `POST /api/videos/:id/tags` 加到影片
- 沒有匹配項時，dropdown 最底顯示「+ 建立 "xxx" 為新標籤」選項
- 選擇建立 → 先 `POST /api/tags`（category 固定 `custom`）→ 再 `POST /api/videos/:id/tags`
- 按 Enter 時：有 autocomplete 高亮項則選中，沒有則建立新 tag
- 按 × 移除 tag → 呼叫 `DELETE /api/videos/:id/tags/:tagId`
- 輸入框為空時不顯示 dropdown
- 加完 tag 後輸入框清空、focus 保持，方便連續操作

UI 佈局：
- 整個元件是一個看起來像 input 的容器（深色背景、圓角、border）
- 內部是 flex wrap 佈局：pill tags + input 自然換行
- dropdown 出現在容器下方，絕對定位

### 2. 首頁 Sidebar — 標籤篩選

**現在**：按 `tags.category` 分組顯示（類型/演員/工作室/自訂），每組有標題。

**改為**：扁平列表，按 `video_count` 降序排列。

- 移除 category 分組標題
- 所有 tag 列在同一層，每個 tag 顯示名稱和影片數量
- 按影片數量多到少排序
- `video_count` 為 0 的 tag 不顯示
- 點擊 tag 的篩選行為不變（toggle tag_id filter）

### 3. 不改的部分

- **後端 API**：全部不動。`tags.category` 欄位保留，`GET /api/tags` 回傳格式不變
- **DB schema**：不做 migration
- **PlayerPage tag 顯示**：維持現狀（只顯示 tag pill，不提供編輯）
- **`createTag` API function**：保留 `category` 參數，前端固定傳 `'custom'`

## 影響的檔案

| 檔案 | 變更 |
|------|------|
| `web/src/pages/admin/VideoManagePage.tsx` | 用新的 TagInput 元件取代現有的 tag 編輯區塊 |
| `web/src/components/TagInput.tsx` | 新增：Jira 風格 autocomplete tag input 元件 |
| `web/src/components/TagSidebar.tsx` | 移除 category 分組，改為按 video_count 降序的扁平列表 |
| `web/src/api/admin.ts` | `createTag` 呼叫時 category 固定 `'custom'`（或在 TagInput 內處理） |

## 技術細節

### TagInput 元件 Props

```typescript
interface TagInputProps {
  videoId: string
  initialTags: Tag[]
  allTags: TagWithCount[]
  onTagsChange?: () => void  // 通知父元件重新載入
}
```

### Autocomplete 過濾邏輯

- 資料來源：父元件傳入的 `allTags`（已從 `GET /api/tags` 載入）
- 過濾：`tag.name.toLowerCase().includes(input.toLowerCase())`
- 排除已加的 tag
- 最多顯示 10 個匹配項
- 匹配項為 0 時只顯示「+ 建立」選項

### 鍵盤操作

- `Enter`：選中高亮項 / 建立新 tag
- `Backspace`（輸入框為空時）：移除最後一個 tag
- `Escape`：關閉 dropdown
- `↑` / `↓`：在 dropdown 中移動高亮

### Sidebar 排序

後端 `GET /api/tags` 已回傳 `video_count`，前端排序即可：
```typescript
tags.filter(t => t.video_count > 0).sort((a, b) => b.video_count - a.video_count)
```
