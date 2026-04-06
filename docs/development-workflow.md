# Vaultflix Development Workflow

本文件記錄專案的開發流程，供討論和制定正式規範使用。

## 流程總覽

```
┌─────────────────────────────────────────────────────────────────────┐
│                        開發循環                                      │
│                                                                     │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐      │
│  │ 1. 需求   │───▶│ 2. 設計   │───▶│ 3. 實作   │───▶│ 4. 驗證   │      │
│  │   釐清    │    │   規劃    │    │   開發    │    │   測試    │      │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘      │
│       │                                               │            │
│       │         ┌──────────┐    ┌──────────┐         │            │
│       │         │ 6. PR    │◀───│ 5. Code  │◀────────┘            │
│       │         │   流程    │    │  Review  │                      │
│       │         └──────────┘    └──────────┘                      │
│       │              │                                             │
│       │              ▼                                             │
│       │         ┌──────────┐                                      │
│       │         │ 7. Merge │                                      │
│       │         └──────────┘                                      │
│       │              │                                             │
│       └──────────────┘  (下一個需求)                                │
└─────────────────────────────────────────────────────────────────────┘
```

## 詳細流程

### 1. 需求釐清

```
需求輸入（使用者描述 / Bug 回報 / 功能規劃）
    │
    ├─ Bug？──▶ 用 Chrome DevTools MCP 重現
    │           ├─ 觀察 network requests
    │           ├─ 檢查 console errors
    │           └─ 截圖記錄現象
    │
    └─ 新功能？──▶ 進入設計階段
```

### 2. 設計規劃（Brainstorming → Spec → Plan）

```
Brainstorming（superpowers:brainstorming）
    │
    ├─ 探索專案現狀（讀 code、查 API）
    ├─ 逐一問釐清問題（一次一個問題）
    ├─ 視覺化 mockup（可選，用 Visual Companion）
    └─ 提出 2-3 個方案 + 推薦
         │
         ▼
Design Spec
    │
    ├─ 寫入 docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md
    ├─ Self-review（placeholder、一致性、範圍）
    ├─ 使用者確認
    └─ Commit spec
         │
         ▼
Implementation Plan（superpowers:writing-plans）
    │
    ├─ 寫入 docs/superpowers/plans/YYYY-MM-DD-<topic>.md
    ├─ 拆成 bite-sized tasks（每個 2-5 分鐘）
    ├─ 每個 task 包含完整 code + 測試指令
    └─ Commit plan
```

### 3. 實作開發（Subagent-Driven Development）

```
Per Task:
    │
    ├─ 派 Implementer Subagent
    │   ├─ 有問題？先回答再繼續
    │   ├─ 實作 + 測試 + Self-review
    │   └─ Commit
    │
    ├─ Spec Compliance Review（subagent 檢查是否符合 spec）
    │   └─ 有問題？Implementer 修 → 重新 review
    │
    └─ Code Quality Review（subagent 檢查程式碼品質）
        └─ 有問題？Implementer 修 → 重新 review
             │
             ▼
        Mark task complete → 下一個 task
```

### 4. 驗證測試

```
    ├─ Go 測試：docker compose exec vaultflix-api go test ./...
    ├─ 前端 build：docker compose build vaultflix-web
    ├─ 部署：docker compose up -d --force-recreate
    └─ 瀏覽器驗證（Chrome DevTools MCP）
        ├─ 功能正常
        ├─ Network 無異常（無重複 request、正確 status code）
        └─ Console 無 error
```

### 5. Code Review（兩階段）

```
第一階段：Superpowers Review（本地 diff）
    │
    ├─ 派 code-reviewer subagent
    ├─ 檢查：架構合規、error handling、測試覆蓋、安全
    ├─ 分級：Critical → Important → Suggestion
    └─ 修復 Critical + Important → 重跑測試
         │
         ▼
第二階段：Code Review Plugin（PR 上）
    │
    ├─ 5 個平行 reviewer agents：
    │   ├─ #1 CLAUDE.md 合規
    │   ├─ #2 Bug 掃描
    │   ├─ #3 Git history 脈絡
    │   ├─ #4 歷史 PR 回饋
    │   └─ #5 Code comment 合規
    │
    ├─ 信心評分過濾（< 80 分排除）
    └─ Comment 到 PR → 修復 → Push
```

### 6. PR 流程

```
    ├─ 建 feature branch
    ├─ Push
    ├─ gh pr create（標題 + 摘要 + 測試計畫）
    ├─ Code Review Plugin 審查
    ├─ 修復 review 問題
    └─ Push 更新
```

### 7. Merge

```
    ├─ gh pr merge --merge
    ├─ git checkout main && git pull
    └─ 進入下一個開發循環
```

## Bug 修復的額外步驟

```
修完 Bug 後：
    │
    ├─ 從 bug 中提煉設計規範
    ├─ 寫成正向規則加入 CLAUDE.md
    └─ 避免同類問題再發生
```

## 工具清單

| 工具 | 用途 | 觸發時機 |
|------|------|---------|
| Chrome DevTools MCP | 瀏覽器測試、截圖、network/console 檢查 | 驗證、debug |
| superpowers:brainstorming | 需求釐清 + 設計 | 新功能開始前 |
| superpowers:writing-plans | 實作計畫 | 設計確認後 |
| superpowers:subagent-driven-development | 按 plan 逐 task 執行 | 實作階段 |
| superpowers:requesting-code-review | 本地 code review | 實作完成後 |
| code-review plugin | PR 上的多角度 review | PR 建立後 |
| gh CLI | PR 管理 | PR 流程 |

## 開發原則

- **所有操作透過容器**：不在本機安裝 Go/Node，保持環境乾淨
- **TDD**：先寫測試再寫實作（或至少實作完立即補測試）
- **Bug → 規範**：每次修 bug 都提煉規則到 CLAUDE.md
- **Review 不跳過**：即使「很簡單」也要跑 review
- **Commit 頻繁**：每完成一個有意義的步驟就 commit
