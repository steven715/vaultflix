# CI/CD 地基設計（Vaultflix）

- 日期：2026-06-21
- 來源 spec：`docs/cicd-foundation-playbook.md`
- 場景：Feature（建立 CI/CD 基礎設施）
- 狀態：設計已與使用者確認，待寫實作計畫

## 1. 目標與範圍

依 playbook 為 Vaultflix 建立 CI/CD 地基，原則優先於工具。本次**範圍 = CI 為主 + build-once 產物**：把 outer loop（CI）、單一入口、不可變產物打穩；CD 維持現有 docker-compose 手動部署，不建 feature flag 系統。

### 已確認的關鍵決定

| 項目 | 決定 |
|---|---|
| Scope | CI 為主 + build-once 產物（CD 手動 compose） |
| 單一入口 | Taskfile（go-task） |
| Verify 強制 | Stop hook，只跑最快層；**原生跑**（host 已有 Go 1.25 + Node 20） |
| 產物 | CI build SHA-tagged image → 推 GHCR（真 build-once） |
| 前端快層測試 | 沿用專案既有 vitest（近期新功能引入） |
| 整合測試 CI | 精簡 compose（無 host 影片掛載）+ fixture；import 子測試在 CI 跳過/調整 |

### 明確不做（YAGNI / playbook §7「從窄開始」）

- feature flag 系統、runtime flag 平台
- registry 之外的 promote 自動化（test→prod 仍手動）
- golangci-lint 等重型 linter（先 `go vet` + `gofmt`，量測後再擴張）

## 2. Deliverable 型別（playbook §3）

Vaultflix 是**服務 / daemon**（多容器 app）。推論：

- CI（build + test）對所有東西相同；只有 CD 分歧。CD = `deploy + run`（compose 起服務）。
- 沒有函式庫，不需 publish/version channel。
- prod gate = 手動（本機 `task deploy ENV=prod`）。

## 3. 單一入口：`Taskfile.yml`（playbook §2 / §8）

三情境（agent 本機、開發者本機、CI）呼叫**同一個 target**，確保 parity。

| Target | 內容 | 跑在哪 |
|---|---|---|
| `verify` | = `test-fast`，給 Stop hook 的 gate | 原生 |
| `test-fast` | `go vet` + `gofmt -l` 檢查 + `go test ./...`（unit）＋ web `tsc -b` + `eslint` + `vitest run` | 原生 |
| `test-full` | `test-fast` + 整合測試（compose `--profile test` 起全棧跑 `scripts/test_all.sh`） | Docker |
| `build` | 多階段 build Go + web production image，SHA-tagged（`build:api` / `build:web` 子 target） | Docker |
| `fmt` / `lint` | `gofmt -w` / 靜態檢查 | 原生 |
| `deploy ENV=local\|prod` | 用 production compose 起服務（pull/run SHA-tagged image） | Docker |
| `up` / `down` / `logs` | 日常開發便利指令 | Docker |

**前提**：`verify` / `test-fast` 原生跑，host 需有 Go 1.25 + Node 20（已確認具備）。`test-full` 與 `build` 走 Docker。CI 與本機呼叫同一 `task` target，達成 target 層級 parity。

## 4. Go production Dockerfile（playbook §1 build-once）

現況：API 用 `golang:1.24-alpine` + bind mount + 啟動時 `go mod tidy && go run`，違反 env-agnostic / build-once。

新增根目錄 `Dockerfile`（Go API）：

- **Stage 1（builder）**：`golang:1.25-alpine`，`go mod download` → `CGO_ENABLED=0 go build -ldflags "-X main.version=$GIT_SHA" -o /server ./cmd/server`
- **Stage 2（runtime）**：`alpine` + **ffmpeg**（API 做 preview/metadata 需 ffmpeg/ffprobe）+ curl（healthcheck），只 COPY `/server`，不烤任何 `.env`
- image tag = git SHA（不可變 ID）

新增 `docker-compose.prod.yml`（override）：`vaultflix-api` 改用此 image（移除 source bind mount 與 `go run`），保留 `.env`、影片來源唯讀掛載、migrate。

新增 `.dockerignore`：排除 `.git`、`web/node_modules`、`web/dist`、`.env`、測試與文件，縮小 build context。

## 5. 設定三分，統一注入路徑（playbook §0 設定層 / §8）

- **build-time 常數** → 只有 version/SHA，用 ldflags 烤進 binary（唯一合法的 bake）。
- **deploy-time 環境值** → `.env` 經 `env_file` 注入（DB DSN、MinIO endpoint、JWT secret）。維持現狀。
- **runtime 值** → 既有「執行期可調性原則」（業務參數從 API request 傳入）承擔，不另建 flag 系統。

## 6. CI — GitHub Actions（playbook §4 outer loop / §5 pipeline）

`.github/workflows/ci.yml`，trigger：push + PR to `main`。CI 呼叫同一 `task` 入口。

| Job | 觸發 | 做什麼 |
|---|---|---|
| `verify` | PR + push | 裝 Go / Node / task → `task verify`（lint / type / unit + vitest，快回饋） |
| `integration` | PR + push | docker compose `--profile test` 起乾淨全棧 → 跑 `task test-full` 的整合層 |
| `build-push` | **只在 push 到 main** | `task build` 產 SHA-tagged image → 推 GHCR（`GITHUB_TOKEN` + `packages:write`），tag = git SHA + `latest` |

- 紅燈回饋回 inner loop：CI fail → 本機用同一 `task` target 重現修復。
- `build-push` 只在 merge 後，PR 階段不污染 registry，符合「PR merge 是主 gate」。

### 整合測試 CI 假設（已確認）

`scripts/test_*.sh` 打活的 API，import 測試可能需要影片來源目錄，但 CI runner 沒有 Windows 的 `D:\AdultsV` 等掛載。處理方式：CI 用精簡 compose（無 host 影片掛載）+ 小型 fixture 目錄；必要時調整 `scripts/test_all.sh` 在 CI 模式跳過需真實媒體的 import 子測試。實作計畫獨立成一步處理。

## 7. Stop hook 強制 verify（playbook §4 / §8）

- `.claude/settings.json` 加 `hooks.Stop` → 跑 `task verify`；非零 exit 擋住收工並把輸出餵回 agent（「系統強制」勝過「叮嚀」）。
- 產生與評估分開：寫 code 的 agent ≠ 打分的 hook；hook 跑客觀的 `task verify`，agent 不能改自己的考卷。
- **gitignore 修正**：目前 `.gitignore` 忽略整個 `.claude/`（只留 `commands/`）。改成也追蹤 `.claude/settings.json`（可分享、無 secret）；`settings.local.json` 維持忽略。

## 8. CLAUDE.md 增補（playbook §4「給脈絡勝過規定流程」/ §8）

新增一節「CI/CD 與單一入口」：

- 入口指令清單（第 3 節的 `task` target 表）。
- 各場景 done-condition，接到既有「對話場景紀律」表：Bug Fix / Feature / Refactor 的 done = `task verify` 綠 + 對應 `task test-full` 綠 + PR CI 綠。
- 「改動 → 該跑哪些檢查」對照，而非 TDD 說教。

## 9. Repo 變更總覽

**新增**：`Taskfile.yml`、`Dockerfile`（Go API）、`docker-compose.prod.yml`、`.github/workflows/ci.yml`、`.claude/settings.json`、`.dockerignore`

**改動**：`.gitignore`（追蹤 settings.json）、`CLAUDE.md`（增補章節）、`web/vitest.config.ts`（include 擴成 `.test.{ts,tsx}`，可選）、必要時 `scripts/test_all.sh`（CI 模式）

**沿用**：`scripts/test_*.sh` + `test-runner` profile、Go 單元測試 + `internal/mock/`、`web/Dockerfile`、既有 vitest、GitHub remote。

## 10. 驗收（一句話自我檢查，playbook 結尾）

一個改動：agent 能在本機透過 `task verify` 自我驗證到綠 → CI 用同一入口在乾淨環境跑 `test-full` 再確認 → image 帶 SHA 推 GHCR 被 promote → 你只在 PR 與 prod deploy 兩個 gate 出手。地基即通。
