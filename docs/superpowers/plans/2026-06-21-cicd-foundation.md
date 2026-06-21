# CI/CD 地基 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 為 Vaultflix 建立 CI 為主的 CI/CD 地基：單一 Taskfile 入口、Go production image（SHA-tagged，推 GHCR）、GitHub Actions CI、Stop hook 強制 `task verify`。

**Architecture:** 服務型 deliverable（多容器 daemon）。`Taskfile.yml` 是三情境（agent / 開發者 / CI）共用的單一入口。Go API 改為多階段 build 的不可變 image；prod/test 用 Docker Compose `!override`/`!reset` 標籤對 base `docker-compose.yml` 做最小覆寫，避免 volumes merge-append 陷阱。整合測試在本機與 CI 用同一個 `docker-compose.test.yml` + 同一個 mp4 fixture，達成 parity。

**Tech Stack:** Go 1.25、go-task (Taskfile)、Docker Compose V2（需支援 `!reset`/`!override`，即 v2.24+）、GitHub Actions、GHCR、Vitest。

**前置工具（host 上需有）：** Go 1.25、Node 20、Docker Desktop、go-task（`task`）。已與使用者確認 host 具備 Go/Node；go-task 由 Task 6 的 hook 依賴，若未安裝需 `winget install Task.Task` 或 `scoop install task`。

**全域常數：**
- GHCR image 名稱：`ghcr.io/steven715/vaultflix-api`
- 整合測試 fixture：`.ci/fixtures/sample.mp4`，CI/本機都掛到 `/mnt/host/videos`

---

## File Structure

**新增：**
- `Dockerfile` — Go API 多階段 build（builder + alpine+ffmpeg runtime）
- `.dockerignore` — 縮小 build context
- `docker-compose.prod.yml` — prod 部署 override（用 image 跑 API）
- `docker-compose.test.yml` — 整合測試 override（fixture 掛載、ephemeral DB）
- `.ci/fixtures/sample.mp4` — 整合測試用的小型影片
- `Taskfile.yml` — 單一入口
- `.github/workflows/ci.yml` — CI（verify / integration / build-push）
- `.claude/settings.json` — Stop hook

**改動：**
- `cmd/server/main.go` — 加 `version` 變數 + 啟動 log（給 ldflags 注入 build 來源）
- `web/vitest.config.ts` — include 擴成 `.test.{ts,tsx}`
- `.gitignore` — 追蹤 `.claude/settings.json`
- `CLAUDE.md` — 增補「CI/CD 與單一入口」章節

> **TDD 註記**：本計畫多為基礎設施／設定檔，無法寫傳統 failing unit test。每個 task 的驗證循環是「建立產物 → 跑驗證指令 → 確認預期輸出 → commit」。唯一的程式碼改動（`main.go` version）用 `go build` 驗證。

---

## Task 1: Go production image（build-once 產物）

**Files:**
- Modify: `cmd/server/main.go`（加 `version` 變數與啟動 log）
- Create: `.dockerignore`
- Create: `Dockerfile`

- [ ] **Step 1: 在 main.go 加入 version 變數與啟動 log**

在 `cmd/server/main.go` 的 `import` 區塊之後、`func main()` 之前，加入 package-level 變數：

```go
// version is injected at build time via -ldflags "-X main.version=<sha>".
// Defaults to "dev" for `go run` / local builds.
var version = "dev"
```

在 `func main()` 內 `slog.SetDefault(logger)` 那行之後，加入：

```go
	slog.Info("starting vaultflix", "version", version)
```

- [ ] **Step 2: 確認編譯通過**

Run: `go build ./...`
Expected: 無輸出、exit 0。

- [ ] **Step 3: 建立 `.dockerignore`**

```
.git
.github
.claude
.worktrees
.superpowers
docs
scripts
web/node_modules
web/dist
.ci
*.md
.env
screenshot-*.png
```

> 註：保留 `casbin/`、`migrations/`、`go.mod`、`go.sum`、`cmd/`、`internal/` 進 context（build 需要）。`migrations/` 雖由 migrate 容器使用，但留著無害。

- [ ] **Step 4: 建立 `Dockerfile`（repo 根目錄，Go API）**

```dockerfile
# ---- builder ----
FROM golang:1.25-alpine AS builder
WORKDIR /src

# 先複製 go.mod/go.sum 以利 layer cache
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG GIT_SHA=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${GIT_SHA}" \
    -o /out/server ./cmd/server

# ---- runtime ----
FROM alpine:3.20
WORKDIR /app

# ffmpeg/ffprobe 供 preview/metadata；curl 供 healthcheck；ca-certificates 供 TLS
RUN apk add --no-cache ffmpeg ca-certificates curl

COPY --from=builder /out/server /app/server
# casbin 在 runtime 從相對路徑 casbin/ 載入（main.go: casbin.NewEnforcer("casbin/model.conf", ...)）
COPY casbin /app/casbin

EXPOSE 8080
ENTRYPOINT ["/app/server"]
```

- [ ] **Step 5: build image 確認成功**

Run: `docker build --build-arg GIT_SHA=test -t vaultflix-api:test .`
Expected: build 成功，最後 `naming to docker.io/library/vaultflix-api:test`。

- [ ] **Step 6: 確認 runtime image 含 ffmpeg 與 casbin**

Run: `docker run --rm --entrypoint sh vaultflix-api:test -c "ffmpeg -version | head -1 && ls casbin && /app/server --help 2>&1 | head -1 || true"`
Expected: 印出 ffmpeg 版本、`model.conf`、`policy.csv`（server 因缺 config 會報錯結束，正常）。

- [ ] **Step 7: Commit**

```bash
git add cmd/server/main.go .dockerignore Dockerfile
git commit -m "feat: add Go production Dockerfile with SHA-tagged build"
```

---

## Task 2: prod 部署 override（`docker-compose.prod.yml`）

**Files:**
- Create: `docker-compose.prod.yml`

> 用 Compose `!reset`/`!override` 標籤覆寫 base `docker-compose.yml` 的 `vaultflix-api`：移除 dev 的 `go run` command 與 `.:/app` source 掛載，改用 build 出來的 image。base 的影片掛載路徑（`D:\AdultsV` 等）保留。

- [ ] **Step 1: 建立 `docker-compose.prod.yml`**

```yaml
# Prod override. 用法：
#   docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
# 或透過 `task deploy`。需 Docker Compose v2.24+（!reset / !override 標籤）。
services:
  vaultflix-api:
    image: ghcr.io/steven715/vaultflix-api:${VAULTFLIX_TAG:-latest}
    build:
      context: .
      args:
        GIT_SHA: ${VAULTFLIX_TAG:-dev}
    # 移除 base 的 dev-only 設定，改用 image 的 ENTRYPOINT
    command: !reset null
    working_dir: !reset null
    # 取代 base 的 volumes（去掉 .:/app 與 go caches，只留影片唯讀掛載）
    volumes: !override
      - D:\AdultsV:/mnt/host/D/AdultsV:ro
      - G:\下載:/mnt/host/G/下載:ro
```

- [ ] **Step 2: 驗證 compose 設定合法**

Run: `docker compose -f docker-compose.yml -f docker-compose.prod.yml config > /dev/null && echo OK`
Expected: `OK`（無 merge 錯誤）。

- [ ] **Step 3: 確認 api 不再有 source 掛載與 go run**

Run: `docker compose -f docker-compose.yml -f docker-compose.prod.yml config | grep -A30 "vaultflix-api:"`
Expected: `command` 區段消失或為 null；`volumes` 只剩兩個 `/mnt/host/...:ro`；`image` 為 `ghcr.io/steven715/vaultflix-api:latest`。

- [ ] **Step 4: Commit**

```bash
git add docker-compose.prod.yml
git commit -m "feat: add prod compose override using built API image"
```

---

## Task 3: 整合測試 parity（fixture + `docker-compose.test.yml`）

**Files:**
- Create: `.ci/fixtures/sample.mp4`
- Create: `docker-compose.test.yml`

> 讓 `task test-full` 在本機與 CI 用同一份 fixture 與同一個 override，消除「本機有 D:\ 影片、CI 沒有」的飄移。fixture 掛到 `/mnt/host/videos`（test_import.sh 的預設 `IMPORT_DIR`）。

- [ ] **Step 1: 產生小型 mp4 fixture**

Run:
```bash
mkdir -p .ci/fixtures
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd)/.ci/fixtures:/out" jrottenberg/ffmpeg:6-alpine \
  -y -f lavfi -i testsrc=duration=2:size=320x240:rate=24 -pix_fmt yuv420p /out/sample.mp4
ls -la .ci/fixtures/sample.mp4
```
Expected: 產生 `.ci/fixtures/sample.mp4`（數十 KB）。
> Fallback（若 Windows 路徑掛載失敗）：改用 `docker create` + `docker cp` 取出，或在已啟動的 api 容器內用 ffmpeg 產生後 `docker cp`。

- [ ] **Step 2: 建立 `docker-compose.test.yml`**

```yaml
# 整合測試 override。本機與 CI 共用，達成 parity。
# 用法：docker compose -f docker-compose.yml -f docker-compose.test.yml --profile test up ...
# 或透過 `task test-integration`。
services:
  # DB / MinIO 改為 ephemeral（無 named volume），每次 up 都是乾淨環境
  postgres:
    volumes: !override []
  minio:
    volumes: !override []

  vaultflix-api:
    # 取代 base 的影片掛載：用 repo 內的 fixture，與 host OS 無關
    volumes: !override
      - .:/app
      - go_modules:/go/pkg/mod
      - go_build_cache:/root/.cache/go-build
      - ./.ci/fixtures:/mnt/host/videos:ro
```

- [ ] **Step 3: 驗證 compose 設定合法**

Run: `docker compose -f docker-compose.yml -f docker-compose.test.yml --profile test config > /dev/null && echo OK`
Expected: `OK`。

- [ ] **Step 4: Commit**

```bash
git add .ci/fixtures/sample.mp4 docker-compose.test.yml
git commit -m "test: add integration fixture and test compose override for CI parity"
```

---

## Task 4: 單一入口 `Taskfile.yml` + vitest include 修正

**Files:**
- Modify: `web/vitest.config.ts`
- Create: `Taskfile.yml`

- [ ] **Step 1: 擴大 vitest include 到 `.test.{ts,tsx}`**

把 `web/vitest.config.ts` 的 include 改為：

```ts
    include: ['src/**/*.test.{ts,tsx}'],
```

- [ ] **Step 2: 確認 vitest 仍能跑現有測試**

Run: `npm --prefix web run test`
Expected: `playbackStats.test.ts` 通過（PASS）。

- [ ] **Step 3: 建立 `Taskfile.yml`**

```yaml
version: '3'

vars:
  GIT_SHA:
    sh: git rev-parse --short HEAD
  IMAGE_API: ghcr.io/steven715/vaultflix-api
  COMPOSE_TEST: docker compose -f docker-compose.yml -f docker-compose.test.yml
  COMPOSE_PROD: docker compose -f docker-compose.yml -f docker-compose.prod.yml

tasks:
  default:
    cmds:
      - task --list

  # ---- inner loop（原生，快層）----
  verify:
    desc: "Stop hook 的 gate：= test-fast"
    cmds:
      - task: test-fast

  test-fast:
    desc: "快層檢查（lint/type/unit）— 原生"
    cmds:
      - go vet ./...
      - cmd: test -z "$(gofmt -l .)"
        msg: "gofmt 發現未格式化檔案，跑 `task fmt`"
      - go test ./...
      - dir: web
        cmd: npx tsc -b
      - dir: web
        cmd: npm run lint
      - dir: web
        cmd: npm run test

  fmt:
    desc: "格式化 Go 程式碼"
    cmds:
      - gofmt -w .

  lint:
    desc: "靜態檢查（go vet + eslint）"
    cmds:
      - go vet ./...
      - dir: web
        cmd: npm run lint

  # ---- outer loop / 重層（Docker）----
  test-integration:
    desc: "整合測試（乾淨全棧 + fixture）— 本機/CI 共用"
    cmds:
      - "{{.COMPOSE_TEST}} --profile test up --build --abort-on-container-exit --exit-code-from test-runner"
      - defer: "{{.COMPOSE_TEST}} --profile test down"

  test-full:
    desc: "完整測試 = test-fast + test-integration"
    cmds:
      - task: test-fast
      - task: test-integration

  # ---- build（不可變產物）----
  build:api:
    desc: "build Go API image（SHA-tagged）"
    cmds:
      - docker build --build-arg GIT_SHA={{.GIT_SHA}} -t {{.IMAGE_API}}:{{.GIT_SHA}} -t {{.IMAGE_API}}:latest .

  build:web:
    desc: "build web 靜態產物 image"
    cmds:
      - docker build -t vaultflix-web:{{.GIT_SHA}} ./web

  build:
    desc: "build API + web image"
    cmds:
      - task: build:api
      - task: build:web

  push:api:
    desc: "推 API image 到 GHCR（需先 docker login ghcr.io）"
    deps: [build:api]
    cmds:
      - docker push {{.IMAGE_API}}:{{.GIT_SHA}}
      - docker push {{.IMAGE_API}}:latest

  # ---- deploy（CD：手動，本機）----
  deploy:
    desc: "部署到本機（prod compose）。處理 web_dist named-volume 陷阱"
    cmds:
      - "{{.COMPOSE_PROD}} build vaultflix-api vaultflix-web vaultflix-nginx"
      - "{{.COMPOSE_PROD}} down vaultflix-web vaultflix-nginx"
      - docker volume rm vaultflix_web_dist || true
      - "{{.COMPOSE_PROD}} up -d"

  up:
    desc: "起 dev 全棧"
    cmds:
      - docker compose up -d

  down:
    desc: "停 dev 全棧"
    cmds:
      - docker compose down

  logs:
    desc: "看 api log"
    cmds:
      - docker compose logs -f vaultflix-api
```

- [ ] **Step 4: 驗證 Taskfile 解析與列表**

Run: `task --list`
Expected: 列出 verify / test-fast / test-full / test-integration / build / deploy 等 target，無解析錯誤。

- [ ] **Step 5: 跑 verify 確認快層通過**

Run: `task verify`
Expected: `go vet`、gofmt 檢查、`go test ./...`、web `tsc`/`lint`/`vitest` 全綠，exit 0。
> 若 web 指令因缺 node_modules 失敗，先 `npm --prefix web ci` 再重跑。

- [ ] **Step 6: 跑整合測試確認 parity（含 import → videos 鏈）**

Run: `task test-integration`
Expected: 全棧起來、import 用 fixture 成功、`test_all.sh` 四個 suite（auth/import/videos/tags）PASS、`exit 0`。
> 這是計畫風險最高的一步。若 import 子測試因 fixture metadata 失敗，檢查 ffprobe 能否讀 `sample.mp4`，必要時用更標準的編碼參數重產 fixture。

- [ ] **Step 7: Commit**

```bash
git add Taskfile.yml web/vitest.config.ts
git commit -m "feat: add Taskfile single entrypoint and widen vitest include"
```

---

## Task 5: GitHub Actions CI

**Files:**
- Create: `.github/workflows/ci.yml`

> 三 job 呼叫同一個 `task` 入口（parity）。`verify` 跑快層；`integration` 用 test compose 跑整合層；`build-push` 只在 merge 到 main 後推 GHCR。

- [ ] **Step 1: 建立 `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read
  packages: write

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - uses: arduino/setup-task@v2
        with:
          repo-token: ${{ secrets.GITHUB_TOKEN }}
      - name: Install web deps
        run: npm --prefix web ci
      - name: Run fast checks
        run: task verify

  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: arduino/setup-task@v2
        with:
          repo-token: ${{ secrets.GITHUB_TOKEN }}
      - name: Prepare .env
        run: cp .env.example .env
      - name: Run integration tests
        run: task test-integration

  build-push:
    runs-on: ubuntu-latest
    needs: [verify, integration]
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - uses: arduino/setup-task@v2
        with:
          repo-token: ${{ secrets.GITHUB_TOKEN }}
      - name: Log in to GHCR
        run: echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
      - name: Build and push API image
        run: task push:api
```

- [ ] **Step 2: 本機驗證 workflow YAML 合法**

Run: `docker run --rm -v "$(pwd):/w" -w /w rhysd/actionlint:latest -color` （若無網路可略過）
Expected: 無 error。或人工檢視縮排正確。

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions workflow (verify, integration, build-push to GHCR)"
```

- [ ] **Step 4: push 後確認 CI 綠（merge 後再驗 build-push）**

Run: `git push` 後到 GitHub Actions 觀察。
Expected: `verify` 與 `integration` job 綠；merge 到 main 後 `build-push` 推出 `ghcr.io/steven715/vaultflix-api:<sha>` 與 `:latest`。
> 風險：GHCR 首次推送需 repo 的 package 權限；若 403，到 repo Settings → Actions → Workflow permissions 開 read/write。

---

## Task 6: Stop hook 強制 verify

**Files:**
- Create: `.claude/settings.json`
- Modify: `.gitignore`

- [ ] **Step 1: 讓 `.gitignore` 追蹤 `settings.json`**

把 `.gitignore` 的 Claude Code 區塊：

```
# Claude Code
.claude/
!.claude/commands/
```

改為：

```
# Claude Code
.claude/
!.claude/commands/
!.claude/settings.json
```

- [ ] **Step 2: 建立 `.claude/settings.json`**

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "task verify"
          }
        ]
      }
    ]
  }
}
```

> Stop hook 在 agent 收工前跑 `task verify`；非零 exit 會擋住收工並把輸出回饋給 agent。`task` 須在 host PATH（go-task 已安裝）。

- [ ] **Step 3: 確認檔案被 git 追蹤、且 JSON 合法**

Run: `git check-ignore .claude/settings.json; python -c "import json;json.load(open('.claude/settings.json'))" && echo "valid json"`
Expected: `git check-ignore` 無輸出（代表不再被忽略）；印出 `valid json`。

- [ ] **Step 4: Commit**

```bash
git add .gitignore .claude/settings.json
git commit -m "chore: enforce task verify via Claude Code Stop hook"
```

---

## Task 7: CLAUDE.md 增補「CI/CD 與單一入口」

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: 在 CLAUDE.md 的「Git Commit 規範」章節之前，插入新章節**

```markdown
## CI/CD 與單一入口

所有 build / test / deploy 透過 `Taskfile.yml` 的單一入口執行。agent 本機、開發者本機、CI 呼叫**同一個 target**，不存在「CI 那邊做法不一樣」。

### 入口指令清單

| 指令 | 用途 | 跑在哪 |
|---|---|---|
| `task verify` | 快層 gate（= `test-fast`），Stop hook 自動跑 | 原生 |
| `task test-fast` | `go vet` + `gofmt` 檢查 + `go test ./...` + web `tsc`/`eslint`/`vitest` | 原生 |
| `task test-integration` | 乾淨全棧 + fixture 跑 `scripts/test_all.sh` | Docker |
| `task test-full` | `test-fast` + `test-integration` | Docker |
| `task build` / `task build:api` | build SHA-tagged image | Docker |
| `task push:api` | 推 API image 到 GHCR | Docker |
| `task deploy` | 本機部署（prod compose，處理 web_dist 陷阱） | Docker |

### 各場景 done-condition

對接「對話場景紀律」表，三種場景的 done 條件一致：

- **Bug Fix / Feature / Refactor done** = `task verify` 綠 + 相關範圍的 `task test-integration`（或 `task test-full`）綠 + PR 的 CI 綠。
- 純前端改動：至少 `task test-fast`（含 vitest）綠。
- 改到 import / 影片掃描 / 串流：要跑 `task test-integration`。
- Stop hook 會在收工前強制 `task verify`；別繞過它，紅燈就修到綠。

### 不可變產物與部署

- Go API build 成 SHA-tagged image，CI 推到 `ghcr.io/steven715/vaultflix-api`。同一個產物 promote，不為不同環境 rebuild。
- 部署是手動 gate：本機 `task deploy`。`local`/`test`/整合測試一律自動，無需介入。
- prod / test 用 `docker-compose.prod.yml` / `docker-compose.test.yml` 以 `!override`/`!reset` 覆寫 base，不複製 infra 定義（避免 drift）。
```

- [ ] **Step 2: 確認 Markdown 無破壞、章節位置正確**

Run: `grep -n "## CI/CD 與單一入口" CLAUDE.md`
Expected: 印出新章節的行號，且在「## Git Commit 規範」之前。

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document CI/CD single entrypoint and done-conditions in CLAUDE.md"
```

---

## 完成後驗收（對照 playbook §8 checklist）

- [ ] 單一入口含 `build` / `test-fast` / `test-full` / `verify` / `deploy` → Task 4
- [ ] 容器化 build 產出 SHA-tagged 不可變產物，推 artifact registry（GHCR）→ Task 1 + Task 5
- [ ] 設定三分（build-time ldflags / deploy-time .env / runtime API 參數）統一注入 → Task 1（version）+ 既有 .env
- [ ] CI 乾淨環境呼叫同一入口 `test-integration`，紅燈回饋 → Task 5
- [ ] Stop hook 在收工前跑 `verify` → Task 6
- [ ] CLAUDE.md 增補入口指令清單 + done-condition → Task 7
- [ ] 服務：prod 走手動 gate（`task deploy`）；本次不做 feature flag → Task 2 + 設計決定
- [ ] deliverable 型別 = 服務，套用對應 CD 形狀 → 全計畫

## Self-Review 註記（已檢查）

- **Spec coverage**：spec 第 3–9 節每項都有對應 task（入口→T4、Dockerfile→T1、prod compose→T2、設定三分→T1+既有、CI→T5、hook→T6、CLAUDE.md→T7、整合測試假設→T3）。
- **Scope 精煉（與 spec 的差異，需執行時知悉）**：spec §6 寫「build Go + web image 推 GHCR」，本計畫只推 **API image**（web 維持本機 build，因靜態檔走 `web_dist` named volume，推 registry 效益低）。CI 仍可選擇性 build web 作為檢查，但預設不做以求精簡。
- **Type/名稱一致**：image 名 `ghcr.io/steven715/vaultflix-api`、fixture 路徑 `.ci/fixtures/sample.mp4`、掛載點 `/mnt/host/videos` 全計畫一致。

## 執行期決議與 deferred follow-ups

執行此計畫時，發現兩個既有債務會擋住 gate 變綠，已與使用者確認處理方式並 deferred 成獨立場景：

1. **前端 ESLint 債（→ 另開 Refactor）**：`web/` 有 10 個既有 ESLint error（`react-hooks@7` 的 react-compiler 規則，如 `set-state-in-effect`、`refs-during-render`，散在 `FavoritesPage`、`HistoryPage`、`useWebSocket` 等既有功能頁，從未在 `main` 上 lint 過）。決議：把 `npm run lint` 移出 `task verify`（Stop hook gate 不含 lint），CI 改用 non-blocking 的 `lint` job 持續顯示。**待辦**：另開 Refactor 對話逐一修這 10 個 error（CLAUDE.md 有 useEffect 無窮迴圈前科，這些規則值得認真修而非消音）；修完把 `npm run lint` 加回 `test-fast`、CI lint job 改 blocking。

2. **`url_expiry_minutes` 驗證未實作（→ 另開 Feature）**：`GET /api/videos/:id` 的 `GetByID` 完全忽略 `url_expiry_minutes` query param（expiry 寫死），所以 `url_expiry_minutes=9999` 回 200 而非預期的 400。決議：把 `scripts/test_videos.sh` 的斷言對齊現況（期望 200）並加 `TODO(url_expiry_minutes validation)` 註解。**待辦**：另開 Feature 把 `url_expiry_minutes` 邊界驗證從 handler 串到 service→minio，實作後把該斷言改回期望 400。

其他執行期事實：
- **單一入口工具**：`task` 已透過 `winget install Task.Task` 裝在 host，路徑在使用者 persistent PATH（`%LOCALAPPDATA%\Microsoft\WinGet\Packages\Task.Task_...`）。Stop hook 的 `task verify` 在新 session 可解析。
- **整合測試呼叫法**：`test-integration` 用 `up -d vaultflix-api` + `run --rm test-runner`（非 `--abort-on-container-exit --exit-code-from`），以取得 test-runner 的真實 exit code、避免 false-green，且不啟動 web/nginx。
- **Task 4 連帶修的既有 bug**（非 CI/CD 範圍但為讓整合測試能跑而必要）：`vaultflix-api depends_on minio-init`（test 相依鏈原本跳過 bucket 建立）、test-runner `ADMIN_PASS` 對齊 seeded 密碼、測試腳本改 poll 非同步 import、`gofmt -w` 既有未格式化檔、`.gitattributes` 統一 LF、`media_source_handler_test.go` 對 Windows 路徑做 JSON 轉義。
