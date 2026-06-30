# Vaultflix Roadmap

## 待優化項目

### 後端

- [ ] **匯入 handler 加入 context timeout** — 目前直接使用 `c.Request.Context()`，沒有明確 timeout 保護
- [ ] **推薦服務批次產生 URL** — Recommendation 仍逐一呼叫；雖然 1.6 加了 cache 緩解 N+1 成本，但 batch query 還沒做
- [ ] **優化 GetRandomUnwatched 查詢** — 目前用 `LEFT JOIN + OR` 條件，大量觀看記錄時效能差，應改用 `NOT EXISTS` 子查詢
- [ ] **MinIO 刪除失敗追蹤** — 影片刪除時 MinIO 失敗只 log 不中斷，長期會累積孤兒檔案，需記錄失敗項供後續清理
- [ ] **新增 `video_tags.tag_id` 索引** — 按標籤篩選影片時缺少索引，影響 GROUP BY 效能
- [ ] **新增 `favorites.user_id` 索引** — 查詢使用者收藏清單缺少獨立索引

### 前端

- [ ] **載入骨架屏** — 影片網格載入時只顯示文字「載入中...」，缺少 skeleton placeholder，造成版面跳動
- [ ] **Header 搜尋列 RWD** — 搜尋框在手機寬度下擠壓變形，需改為可收合的搜尋抽屜
- [ ] **TagSidebar 手機適配** — 固定寬度 `w-56` 在小平板造成水平溢出，需可收合或隱藏
- [ ] **影片資訊區塊 RWD** — metadata（時長、解析度等）在手機上換行不可控，需改為垂直堆疊

---

### 🎯 進行中（優先）

- [ ] **自動標籤（AI 影片分析）** — 透過 AI 分析影片檔名（JAV 番號、女優名）與內容，自動建議標籤分類。**目前最高優先**。
  - 規劃分階段：① 檔名/番號 scraper 比對既有 metadata 來源 → ② LLM（Claude API）依檔名+metadata 建議標籤 → ③（選用）抽幀電腦視覺內容分析
  - 詳細 Spec / 選型見後續 feature 對話與研究筆記

### 其他未來功能

- [ ] **全文搜尋引擎** — 引入 Meilisearch，改善中日文標題搜尋品質（目前用 PostgreSQL `gin` 索引，對 CJK 分詞效果有限）
- [ ] **LLM Chat 助手** — `/api/chat` 端點串接 Claude API，結合影片 metadata 做語意搜尋與推薦對話
- [ ] **行動端支援** — React Native 或獨立 APP，搭配現有 API
- [ ] **多使用者** — 開放註冊、使用者偏好設定、個人化推薦
- [ ] **API Gateway** — 引入 Traefik 做 rate limiting、SSL termination、反向代理
- [ ] **孤兒檔案清理排程** — 定期比對 MinIO 與 DB，清理不一致的孤兒物件

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
