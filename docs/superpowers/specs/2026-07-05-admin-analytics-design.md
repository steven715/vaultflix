# Admin 分析頁 + 觀看真值管線 — 設計文件

- 日期:2026-07-05
- 場景:Feature
- 狀態:設計已核可,待寫實作計畫

## 1. 目標與範圍

在管理後台新增「分析 Analytics」頁,呈現真實觀看數據,並為此建立能算出**真實累計觀看時長**與**精確每日趨勢**的資料管線。

範圍內:

1. 新增 `watch_sessions`(觀看 session / 心跳)表,播放時前端每 15s 回報,後端累計。
2. 心跳寫入端點 `POST /api/watch-sessions/heartbeat`(viewer 可用)。
3. 分析聚合端點 `GET /api/admin/analytics`(admin only,Casbin 已涵蓋)。
4. 前端 `/admin/analytics` 頁,inline SVG 圖表(零第三方依賴),把側邊欄「分析」由 disabled 改 enabled。
5. `PlayerPage` 整合心跳回報。

範圍外(明確不做):

- 「總覽 Dashboard」頁(已決定不做、已移除)。
- 從既有 `watch_history` 回填歷史(見 §3 決策)。
- session row 的保留 / rollup / cron 清理(永久保留,見 §3 決策)。
- 修改既有 `watch_history` 的任何行為(心跳是**平行**寫入)。

## 2. 背景與現況

- `watch_history` 每個 `(user, video)` 只存**最後進度** `progress_seconds` + `completed` + `watched_at`(UPSERT,見 `migrations/004`)。它是累加不出真實觀看時長、也算不出精確每日趨勢的——這正是本 Feature 要補的缺口。`watch_history` 繼續負責續看位置與「接著看」清單,本 Feature 不動它。
- 播放頁 `web/src/pages/PlayerPage.tsx` 已有:`timeupdate` 節流回報 `saveProgress`、pause 回報、離開時 `sendBeacon`。心跳掛在同一套時脈上。
- 路由:所有 `/api/*` 走 `RequireActiveUser` + `CasbinRBAC`。`casbin/policy.csv` 中 `admin` 已擁有 `/api/*` 全動詞;`viewer` 需逐路由授權。
- Admin 頁:`App.tsx` 於 `AdminLayout` 下掛子路由;`web/src/lib/adminNav.ts` 的 `analytics` 目前 `enabled:false`。
- 設計 token:深色主題,`--color-accent:#FFB23F`(琥珀),另有 `--color-data-blue`、`--color-data-purple`、`--color-live`、`--color-fav` 可作 categorical。

## 3. 已拍板決策

| 決策 | 選擇 | 理由 |
|---|---|---|
| 心跳資料模型 | **Session 累計**(每段連續播放一 row,心跳累加 `watched_seconds`) | 天然定義「一次觀看 = 一次播放」,可直接聚合真實時長 / 活躍使用者 / 每日趨勢;row 量中等 |
| 心跳間隔 | **15 秒** | 精度、寫入量、時長誤差的平衡點 |
| 保留策略 | **永久保留** | 個人專案 row 量小;不做 rollup/cron,避免過度設計。查詢只抓窗內 |
| 歷史回填 | **不回填,從今開始** | `watch_history` 只有最後進度,回填出的是假時長與假日期分布,反而污染真值 |
| 一次觀看門檻 | `watched_seconds >= 10` | 濾掉誤開 2 秒的雜訊 |
| session 身分 | **前端產生 `session_id`(uuid)** | 「一次觀看 = 一次播放」無歧義;fresh open → 新 uuid;UPSERT by id |
| delta 防灌水 | 前端累加「實際播放增量」並夾 `[0, 22]`(15×1.5) | seek/背景分頁不計入時長 |
| 時間窗 | **單一 `days` 窗驅動全頁**(KPI 也是窗內,非 all-time) | 全頁一致、和趨勢對得起來;口徑單一 |

## 4. 資料模型

Migration `015_create_watch_sessions.up.sql` / `.down.sql`(down 完整可逆,DROP TABLE)。

```sql
CREATE TABLE watch_sessions (
    id                     UUID PRIMARY KEY,             -- 前端產生的 session_id
    user_id                UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id               UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    watched_seconds        INT  NOT NULL DEFAULT 0,      -- 累計實際播放秒數
    max_progress_seconds   INT  NOT NULL DEFAULT 0,      -- 到達的最遠位置
    video_duration_seconds INT  NOT NULL DEFAULT 0,      -- 快照,用來算完播率
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_watch_sessions_started ON watch_sessions(started_at DESC);
CREATE INDEX idx_watch_sessions_video   ON watch_sessions(video_id);
CREATE INDEX idx_watch_sessions_user    ON watch_sessions(user_id);
```

定義:

- **一次觀看(view)** = 一筆 `watched_seconds >= 10` 的 session。
- **完播率(completion)/session** = `min(max_progress_seconds / video_duration_seconds, 1)`,只計 `video_duration_seconds > 0` 者。

## 5. API 契約

### 5.1 心跳寫入(viewer)

```
POST /api/watch-sessions/heartbeat
Body: { "session_id": "uuid", "video_id": "uuid",
        "played_delta": 15, "position_seconds": 312 }
→ 204 No Content
```

- policy.csv 新增:`p, viewer, /api/watch-sessions/heartbeat, POST`。
- 驗證:`session_id`/`video_id` 必填合法 uuid;`played_delta` 夾 `[0, 22]`(負數 → 400,超過 → clamp);`position_seconds >= 0`。video 不存在 → 404。
- 語意(UPSERT by `id`):
  - insert:從 `videos` 快照 `duration_seconds` 存入 `video_duration_seconds`;`watched_seconds = played_delta`;`max_progress_seconds = position_seconds`。
  - conflict:`watched_seconds += played_delta`;`max_progress_seconds = GREATEST(max_progress_seconds, position_seconds)`;`last_heartbeat_at = NOW()`。
- 冪等:同 `session_id` 重送只累加,不報錯。

### 5.2 分析聚合(admin)

```
GET /api/admin/analytics?days=30&limit=10
→ 200 { "data": { ...AnalyticsSummary } }
```

依「執行期可調性原則」,業務參數走 query param:

- `days`:預設 30,夾 `[1, 365]`。驅動全頁時間窗(`started_at >= now() - days`)。
- `limit`:預設 10,夾 `[1, 50]`。Top 影片 / Top 標籤取幾筆。

`AnalyticsSummary`(經 `SuccessResponse` 的 `data` wrapper):

```jsonc
{
  "range_days": 30,
  "total_views": 1234,          // COUNT(session WHERE watched_seconds>=10)
  "total_watch_hours": 87.5,    // SUM(watched_seconds)/3600,四捨五入 1 位
  "avg_completion_rate": 0.62,  // AVG(min(max_progress/duration,1)),只計 duration>0
  "active_users": 3,            // COUNT(DISTINCT user_id)(watched_seconds>=10)
  "daily_trend": [              // 後端補齊每一天(空日補 0),長度 = range_days
    { "date": "2026-06-06", "views": 12, "watch_hours": 3.2 }
  ],
  "top_videos": [               // 依窗內 SUM(watched_seconds) 排序
    { "video_id": "uuid", "title": "...", "views": 40,
      "watch_hours": 12.1, "thumbnail_key": "..." }
  ],
  "top_tags": [                 // 依窗內 session 數(經 video_tags 關聯)排序
    { "tag_id": "uuid", "name": "...", "views": 88 }
  ]
}
```

- `daily_trend` 由後端補 0(等長陣列,前端直接畫)。
- 所有 view 計數統一套 `watched_seconds >= 10` 門檻。

## 6. 後端分層

嚴格 Handler → Service → Repository;interface 定義在使用端 package;mock 手寫放 `internal/mock/`。

心跳側:

- `internal/model/watch_session.go` — `WatchSession` struct + heartbeat 輸入型別。
- `internal/handler/watch_session_handler.go` — 解析/驗證、呼叫 service、組回應。定義 `WatchSessionService` interface。
- `internal/service/watch_session_service.go` — 夾 delta、決定 insert/累加語意。定義 `WatchSessionRepository` interface。
- `internal/repository/watch_session_repository.go` — 單一 UPSERT SQL(const、parameterized)。

分析側:

- `internal/model/analytics.go` — `AnalyticsSummary`、`DailyPoint`、`TopVideo`、`TopTag`。
- `internal/handler/analytics_handler.go` — 讀 `days`/`limit`(clamp)、呼叫 service。定義 `AnalyticsService`。
- `internal/service/analytics_service.go` — 組 summary、`daily_trend` 補 0、換算小時 / 完播率。定義 `AnalyticsRepository`。
- `internal/repository/analytics_repository.go` — 聚合 query 組(KPI、daily GROUP BY date、top_videos JOIN videos、top_tags JOIN video_tags→tags),每條 SQL const 置檔頂。
- `internal/mock/` — 補 `WatchSessionRepository`、`AnalyticsRepository`(及 handler 測試所需 service mock)。

兩 handler 在 `cmd/server/main.go` 既有 `api` group 註冊(心跳於 watch 區塊、analytics 於 admin 區塊)。錯誤處理照規範:service/repo 只 `%w` wrap + return;log + HTTP 只在 handler。`slog` 結構化欄位。

## 7. 前端

心跳整合(`PlayerPage.tsx`):

- 新 `web/src/api/watchSession.ts` — `postHeartbeat(payload)`。
- `session_id`:每次開播 `crypto.randomUUID()`,存 `useRef`。
- `playedDelta` 累加器:`timeupdate` 時累加實際播放增量(以 `currentTime` 差計、只在 playing、夾 `[0,22]`),`useRef` 存。
- 15s `setInterval` 送心跳並歸零累加器;既有 beacon 路徑一併送最後一次。
- `useEffect` cleanup 清 interval,遵守 hooks 規範(ref 追蹤、不放 useCallback 引用進依賴)。

分析頁:

- `web/src/pages/admin/AnalyticsPage.tsx` + `App.tsx` 加 `/admin/analytics` route。
- `adminNav.ts` 把 `analytics` 改 `enabled:true`,拿掉「即將推出」註解。
- 頁頂 7/30/90 天窗切換,改 `days` 觸發 refetch(cleanup flag 防 unmount setState)。
- `web/src/api/analytics.ts` — `getAnalytics(days, limit)`。

圖表(依 dataviz skill,全 inline SVG,吃現有 token,深色 surface 驗色;共用 `components/admin/charts/`):

- **KPI 列**(stat tile,非 chart):總觀看次數 / 觀看時長(hrs)/ 平均完播率 / 活躍使用者。
- **近 N 天每日趨勢**:單一系列 **area chart**(accent 一色 sequential)+ crosshair/tooltip;預設 watch_hours,可切 views。**不做雙軸**(完播率不入此圖)。
- **熱門影片 Top N**:**水平長條**(sequential 單色),直接標籤標題。
- **熱門標籤 Top N**:**水平長條**(sequential 單色),尾端折「其他」。**不用圓餅**。
- 空資料狀態:「尚無觀看資料,開始播放後這裡就會有數據」。每圖附 table view / aria,identity 不靠顏色單獨承載。
- 完成後用 Chrome DevTools MCP 截圖檢查 label 碰撞 / 溢出。

## 8. 測試與完成條件

後端(table-driven):每個 service/handler 覆蓋正常 + 404 + 400 +(analytics)空資料;repo 以 mock 隔離。重點案例:心跳 UPSERT 累加、delta 夾值、`days`/`limit` clamp、daily 補 0、完播率夾 1、Top 排序、view 門檻。

前端(vitest):heartbeat delta 夾值 / session 重生、chart 純函式(scale/補點)、AnalyticsPage 載入與空狀態。

完成條件:

- `task verify` 綠(含 go vet/gofmt/go test + web tsc/eslint/vitest)。
- 有動 DB/串流 → `task test-integration` 綠(補 heartbeat → analytics 的 e2e 斷言)。
- PR CI 綠。

## 9. 假設

- 分析口徑為**全體使用者聚合**(個人平台,單/少使用者),不做 per-user 分析頁。
- 前端 `crypto.randomUUID()` 於目標瀏覽器可用(現代 Chrome,已為專案除錯目標)。
- Top 標籤經既有 `video_tags` 關聯;無標籤影片不計入標籤分佈但仍計入其他指標。
