# Docker 與部署慣例

> 動 `docker-compose*.yml`、`Dockerfile*`、`nginx/`、`Taskfile.yml` 的部署 target 前先讀。

## 通用規則

- 使用 Docker Compose V2 語法（`services:` 頂層，無 `version:` 欄位）
- 所有服務使用 alpine-based image（除非有特殊需求）
- Volume 命名格式：`vaultflix-<service>-data`（如 `vaultflix-postgres-data`）
- 環境變數透過 `.env` 檔案注入，不寫死在 `docker-compose.yml` 中
- Health check 必須配置在每個服務上
- 對外暴露 port 的服務（如 MinIO），`.env` 中必須同時定義 internal endpoint（Docker hostname）和 public endpoint（host-accessible），命名慣例：`<SERVICE>_ENDPOINT` / `<SERVICE>_PUBLIC_ENDPOINT`

## 前端發版流程（不可變 nginx image）

前端 SPA 直接 build 進 `vaultflix-nginx` image：`nginx/Dockerfile` 是多階段 build，第一階段用 `node:20-alpine` 編譯 `web/`，第二階段把產物 `COPY` 進 nginx 的 `/usr/share/nginx/html`。**沒有 `web_dist` 共享 volume、沒有獨立的 `vaultflix-web` 容器** —— 前端是不可變產物，與 Go API image 對稱。

因為 build context 是 repo root（nginx Dockerfile 要讀 `web/` 與 `nginx/nginx.conf`），compose 的 nginx 服務用 `context: .` + `dockerfile: nginx/Dockerfile`，root 的 `.dockerignore` 已排除 `web/node_modules`、`web/dist`。

正確流程（image 換了 `up -d` 會自動 recreate，無需手動刪 volume）：

```bash
docker compose build vaultflix-nginx
docker compose up -d vaultflix-nginx
# 或一次到位：task up
```

改完前端若瀏覽器行為沒變，第一反應是 image 沒重 build（或 PWA/瀏覽器在吃舊快取，hard reload）—— 不再有 named volume 陷阱。

## 磁碟層級掛載策略

影片檔案保留在本機磁碟，透過 Docker volume mount 以唯讀模式掛載整個磁碟。

**磁碟配置是「因機器而異」的設定，不進被 git 追蹤的檔案。** 它住在 gitignore 的 `docker-compose.media.yml`（範本：`docker-compose.media.yml.example`），與 `.env` 同一類。tracked 的 `docker-compose.yml` / `docker-compose.test.yml` 一行影片掛載都不該有。

```yaml
# docker-compose.media.yml — 用 YAML anchor 一次餵給 api 與 nginx
x-media-mounts: &media-mounts
  - /Volumes/Media:/mnt/host/Media:ro         # macOS 外接碟
  - /Users/you/Movies:/mnt/host/Movies:ro     # macOS 本機目錄
  # - /mnt/d/Videos:/mnt/host/D/Videos:ro     # Windows（WSL 路徑）

services:
  vaultflix-api:
    volumes: *media-mounts
  vaultflix-nginx:
    volumes: *media-mounts
```

- 掛載點統一在 `/mnt/host/<名稱>/` 下（Windows 慣例用磁碟代號，如 `/mnt/host/D/`）
- 使用 `:ro`（read-only）防止容器內程式修改原始檔案
- Media Source 的 `mount_path` 必須在 `/mnt/host/` 前綴下
- api 與 nginx 必須拿到**完全相同**的掛載（nginx 少一個就無法做 X-Accel byte serving）。用 anchor 而不是抄兩份，讓兩者不可能 drift
- 新增磁碟只需在 `docker-compose.media.yml` 的 anchor 加一行 + 在 Admin UI 新增 Media Source
- `task up` 會自動把這個檔案疊在最後；整合測試**刻意不疊**，改掛 `.ci/fixtures`，維持 host OS 無關

## 單一執行 stack（ADR-0011）

- `docker-compose.yml` 就是日常跑的 stack：API 是 `Dockerfile` 預設（最後一個）stage 的編譯後 binary，`VIDEO_XACCEL_PREFIX` 在 base 設定，所以 bytes 一律由 nginx 送。不要再加 dev/prod override 檔；改 code 後 `task up` 重 build
- `Dockerfile` 的 stage 順序有意義：runtime 必須是**最後**一個 stage（compose 不指定 `target` 時 build 它）。`dev` stage（Go toolchain + ffmpeg）只給整合測試的 `go-test` 用
- 任何**繞過 nginx 直連 API** 的消費者都要處理 X-Accel：整合測試在 `docker-compose.test.yml` 把 `VIDEO_XACCEL_PREFIX` 清空；`npm run dev` 的 Vite proxy 指向 nginx（`:3000`）而不是 `:8080`
- 整合測試的 Taskfile `defer` 必須寫在它要保護的步驟**之前**：Task 的 defer 執行到該行才登記，寫在最後的話中途失敗就不會清理，`vaultflix-inttest` 會一直留著
