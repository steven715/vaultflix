# Vaultflix

A self-hosted video management and streaming platform. Organize local video files into a browsable, searchable, streamable web application. Single-user focused with multi-user architecture.

## Architecture Overview

```
React SPA (localhost:3000)
    |
    |-- API requests --> Nginx reverse proxy --> Go API Server (:8080) --> PostgreSQL
    |                         |                     |
    |                         |                     +--> MinIO (thumbnails / previews only)
    |                         +--> Local disk (video bytes, via X-Accel-Redirect)
    |
    +-- WebSocket --> Go API Server (real-time import progress)
```

**Key design decision**: Video files stay on local disk. In production the Go API only validates auth + path safety and hands the byte path to nginx via `X-Accel-Redirect`, so nginx reads the file straight off disk with native HTTP Range (seeking) and video bytes never traverse the Go process. In dev (no nginx — vite proxies directly to the API) it falls back to `http.ServeFile`. MinIO holds only thumbnails/previews. Import progress is pushed in real-time via WebSocket.

### Tech Stack

| Layer | Technology | Purpose |
|-------|------------|---------|
| Frontend | React 19 + TypeScript | SPA with Vite, baked into the Nginx image |
| Backend | Go 1.25 + Gin | REST API, JWT auth, path safety, on-the-fly HLS transcode |
| Database | PostgreSQL 16 | Video metadata, users, tags, watch history |
| Object Storage | MinIO | Thumbnails & previews only (videos stay on local disk), S3-compatible |
| Auth | JWT + bcrypt | Stateless authentication |
| Authorization | Casbin | RBAC with admin/viewer roles |
| Infrastructure | Docker Compose V2 | All services containerized |

## Features

### Implemented

- **Authentication**: JWT-based login with bcrypt password hashing
- **Authorization**: Casbin RBAC with admin and viewer roles
- **Video Import**: Bulk import from local directory with automatic ffprobe metadata extraction and ffmpeg thumbnail generation
- **Video Browsing**: Paginated grid view with search, tag filtering, and multi-field sorting
- **Video Streaming**: Direct-from-disk streaming with native HTTP Range (seeking); production offloads byte serving to nginx via `X-Accel-Redirect`, dev falls back to the API's `http.ServeFile`
- **Adaptive Play Mode**: Every video is classified `direct` / `remux` / `transcode` from its container and codecs; files a browser cannot play natively are served as on-the-fly HLS (ffmpeg segments, disk-cached in the `vaultflix-transcode-cache` volume)
- **Scoped Stream Tokens**: Short-lived `scope=stream` JWTs bound to one video ID and to the streaming routes only, so a token leaked through a `<video src>` URL cannot reach any other endpoint
- **Tag System**: Categorized tags (genre, actor, studio, custom) with video-tag associations
- **Media Source Management**: Admin UI for managing media sources (CRUD) with real-time import progress
- **Watch History**: Track and resume video playback progress
- **Favorites**: Bookmark videos for quick access
- **Daily Recommendations**: Admin-curated daily video picks
- **User Management**: Admin UI for user CRUD, enable/disable, password reset
- **Metadata Enrichment**: Scrapes external sources by video code and stores results as suggestions for admin accept/reject, including batch jobs with cancel support
- **Analytics**: Admin dashboard over watch sessions and playback telemetry (time-to-first-frame, stalls, watch time)
- **Backfill Jobs**: Regenerate previews and probe codecs / keyframes across the existing library
- **PWA**: Installable frontend with service-worker caching and an in-app update prompt
- **WebSocket**: Real-time import progress notifications
- **React Frontend**: Login, browse, player, and admin pages with responsive dark theme

### Planned

- Meilisearch full-text search
- LLM-powered semantic search
- Automatic tagging
- Mobile client

## Prerequisites

### Running the app (Docker only)

- **Docker** and **Docker Compose V2**
- No local Go or Node.js installation required -- everything runs in containers.

### Local development / contributing

Build, test, and deploy all go through a single entry point: [`go-task`](https://taskfile.dev)
(`task`). The same targets run on your machine and in CI -- see [`Taskfile.yml`](Taskfile.yml)
and [CLAUDE.md](CLAUDE.md). For the native dev workflow you also need:

| Tool | Why it's needed | Install (Windows) | Install (Linux / WSL) |
|------|-----------------|-------------------|------------------------|
| `task` (go-task) | Single entry point (`task --list`); `task verify` is the Stop-hook gate | `winget install Task.Task` | `sh -c "$(curl -ssL https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin` |
| Go 1.25+ | `task verify` runs `go vet` / `gofmt` / `go test` | `winget install GoLang.Go` | official tarball into `~/.local/go` ([go.dev/dl](https://go.dev/dl)) |
| Node.js 20+ | `task verify` runs `tsc` + `vitest` (web) | `winget install OpenJS.NodeJS.LTS` | nvm or [nodesource](https://github.com/nodesource/distributions) |
| `gh` (GitHub CLI) | Push branches and open PRs (`gh auth login` first) | `winget install GitHub.cli` | [cli.github.com](https://cli.github.com) (apt repo or release tarball) |

> On Linux/WSL, make sure the install targets (`~/.local/bin`, `~/.local/go/bin`) are on your
> `PATH`. Run `task verify` before pushing -- it must be green.

## Quick Start

### 1. Clone and configure

```bash
git clone <repo-url> && cd vaultflix
cp .env.example .env
```

Edit `.env` and set your passwords and secrets. The defaults work for local development, but you should at minimum change `JWT_SECRET`, `DB_PASSWORD`, `MINIO_SECRET_KEY`, and `ADMIN_DEFAULT_PASSWORD`.

### 2. Configure disk mounts

Video disks differ per machine, so they live in `docker-compose.media.yml` — a
gitignored per-machine file, alongside `.env`. Never edit the tracked compose
files for this:

```bash
cp docker-compose.media.yml.example docker-compose.media.yml
```

Then edit the anchor to point at your own disks:

```yaml
x-media-mounts: &media-mounts
  - D:/:/mnt/host/D:ro    # Mount D: drive
  - E:/:/mnt/host/E:ro    # Mount E: drive (optional)

services:
  vaultflix-api:
    volumes: *media-mounts
  vaultflix-nginx:
    volumes: *media-mounts
```

The YAML anchor feeds the identical list to **both** `vaultflix-api` (import +
auth) and `vaultflix-nginx` (X-Accel byte serving) — nginx cannot serve files it
has no mount for, and the anchor makes it impossible for the two to drift apart.

Each mounted disk is reachable at `/mnt/host/<drive>/` inside the container; the
target path must stay under `/mnt/host/` to pass the `AllowedMountPrefix` path
validation. To enable the nginx offload, set `VIDEO_XACCEL_PREFIX=/internal-video/`
(production / behind nginx); leave it empty for `npm run dev`, which proxies
straight to the API without nginx.

Integration tests deliberately ignore this file and mount `.ci/fixtures` instead,
so they run identically on any host.

### 3. Start all services

```bash
task up
```

Use `task up` rather than a bare `docker compose up -d`: it layers in your
`docker-compose.media.yml` (a bare `docker compose` command would start the stack
with no video mounts at all) and fails with a clear message if you skipped step 2.

This automatically:
- Starts PostgreSQL and MinIO
- Creates MinIO buckets (`vaultflix-videos`, `vaultflix-thumbnails`)
- Runs database migrations
- Builds the dev API image (golang toolchain + ffmpeg baked into a layer) and starts the Go API server
- Builds and starts the React frontend via Nginx

### 4. Log in

Open **http://localhost:3000** in your browser. Log in with the admin credentials from your `.env` file (defaults: `admin` / `change-me-admin-password`).

### 5. Add media sources and import videos

Open **http://localhost:3000**, log in as admin, then navigate to **媒體來源** (Media Sources) in the top navigation bar.

1. Click **+ 新增來源** to add a media source (e.g., label: "D槽影片", path: `/mnt/host/D/Videos`)
2. Click **掃描匯入** on the source card to start importing
3. Watch the real-time progress bar as videos are scanned, metadata extracted, and thumbnails generated

### 6. Browse and play

Refresh **http://localhost:3000**. Your videos should appear with thumbnails, duration, and resolution metadata. Click any video to start streaming.

## Environment Variables

See [`.env.example`](.env.example) for a complete template.

| Variable | Required | Description |
|----------|----------|-------------|
| `DB_HOST` | | PostgreSQL hostname (default: `postgres`) |
| `DB_PORT` | | PostgreSQL port (default: `5432`) |
| `DB_USER` | | Database username (default: `vaultflix`) |
| `DB_PASSWORD` | **Yes** | Database password |
| `DB_NAME` | | Database name (default: `vaultflix`) |
| `MINIO_ENDPOINT` | | Internal MinIO endpoint for API server (default: `minio:9000`) |
| `MINIO_PUBLIC_ENDPOINT` | **Yes** | Public MinIO endpoint reachable by the browser (e.g. `localhost:9000`) |
| `MINIO_ACCESS_KEY` | **Yes** | MinIO access key |
| `MINIO_SECRET_KEY` | **Yes** | MinIO secret key |
| `MINIO_USE_SSL` | | Enable HTTPS for MinIO (default: `false`) |
| `MINIO_VIDEO_BUCKET` | | Video storage bucket name (default: `vaultflix-videos`) |
| `MINIO_THUMBNAIL_BUCKET` | | Thumbnail storage bucket name (default: `vaultflix-thumbnails`) |
| `MINIO_PREVIEW_BUCKET` | | Preview-clip bucket name (default: `vaultflix-previews`) |
| `JWT_SECRET` | **Yes** | Secret key for signing JWT tokens |
| `JWT_EXPIRY_HOURS` | | JWT token expiry in hours (default: `24`) |
| `SERVER_PORT` | | API server port (default: `8080`) |
| `VIDEO_XACCEL_PREFIX` | | nginx internal location for X-Accel byte serving (e.g. `/internal-video/`); empty (default) makes the API serve bytes itself via `http.ServeFile` |
| `STREAM_TOKEN_EXPIRY_MINUTES` | | Scoped stream-token lifetime in minutes (default: `60`) |
| `TRANSCODE_CACHE_DIR` | | HLS segment cache directory inside the container (default: `/var/cache/vaultflix/transcode`) |
| `TRANSCODE_CACHE_MAX_BYTES` | | HLS segment cache size cap (default: 20 GiB) |
| `ENRICH_HTTP_TIMEOUT` | | Metadata scraper HTTP timeout (default: `15s`) |
| `ENRICH_USER_AGENT` | | User-Agent sent by the metadata scraper (default: `Vaultflix/1.0`) |
| `ENRICH_JAVBUS_COOKIE` | | Cookie header sent by the JavBus scraper (age gate) |
| `ADMIN_DEFAULT_USERNAME` | | Auto-created admin username (default: `admin`) |
| `ADMIN_DEFAULT_PASSWORD` | **Yes** | Auto-created admin password |

## Project Structure

```
vaultflix/
├── cmd/server/             # Application entrypoint (main.go, admin_reset.go)
├── internal/
│   ├── config/             # Environment-based configuration
│   ├── handler/            # HTTP handlers (Gin)
│   ├── middleware/         # JWT auth, stream-token scope, active-user, Casbin RBAC
│   ├── model/              # Domain models and shared errors
│   ├── repository/         # PostgreSQL data access layer
│   ├── service/            # Business logic layer
│   ├── streaming/          # HLS manifest, ffmpeg segment generation, segment cache
│   ├── scraper/            # External metadata scrapers (enrichment suggestions)
│   ├── websocket/          # Hub + Notifier interface (real-time progress)
│   └── mock/               # Hand-written mock structs for testing
├── migrations/             # SQL migration files (up/down pairs)
├── casbin/                 # RBAC model and policy definitions
├── scripts/                # Integration test scripts (test_all.sh)
├── nginx/
│   ├── Dockerfile          # Multi-stage build: Node (builds web/) -> Nginx
│   └── nginx.conf          # Reverse proxy, SPA routing, X-Accel internal location
├── web/                    # React frontend (Vite + TypeScript)
│   └── src/
│       ├── api/            # Axios API client and service functions
│       ├── components/     # Reusable UI components (incl. admin/)
│       ├── contexts/       # React contexts (auth state)
│       ├── hooks/          # Custom hooks (WebSocket, playback stats)
│       ├── pages/          # Page components (Login, Browse, Player, admin/)
│       └── types/          # TypeScript type definitions
├── docs/                   # SPEC.md (what's built), ADRs, workflow docs
├── .ci/fixtures/           # Host-independent media fixtures for integration tests
├── Dockerfile              # Prod API image (multi-stage, compiled binary)
├── Dockerfile.dev          # Dev API image (go toolchain + ffmpeg baked in)
├── docker-compose.yml      # Base stack
├── docker-compose.prod.yml # Prod overrides (immutable images)
├── docker-compose.test.yml # Integration-test overrides
├── docker-compose.media.yml.example  # Per-machine video mounts (copy, don't edit the base)
├── Taskfile.yml            # Single entry point for build / test / deploy
├── CLAUDE.md               # Development conventions and coding standards
└── ROADMAP.md              # Planned features and tech-debt backlog
```

## Development

### Running locally (outside Docker)

**Backend:**

```bash
# Ensure PostgreSQL and MinIO are running (e.g. via Docker)
export $(cat .env | xargs)
go run ./cmd/server
```

**Frontend:**

```bash
cd web
npm install
npm run dev
```

Vite dev server proxies `/api` requests to `localhost:8080` via the config in `vite.config.ts`.

### Running tests

Everything goes through `task` — the same targets run on your machine and in CI, so there is
no "CI does it differently". See [`Taskfile.yml`](Taskfile.yml) for the full list.

```bash
# Fast gate (native): go vet + gofmt + go test + tsc + eslint + vitest
# This is what the Stop hook runs; it must be green before pushing.
task verify

# Integration tests (Docker): pristine stack + .ci/fixtures, runs scripts/test_all.sh
task test-integration

# Both
task test-full
```

`task test-integration` brings up an isolated compose project with
[`docker-compose.test.yml`](docker-compose.test.yml) and deliberately does **not** layer in
your `docker-compose.media.yml` — it mounts `.ci/fixtures` instead, so results do not depend
on which disks your host happens to have.

### Database migrations

Migrations run automatically on `docker compose up`. To run manually:

```bash
# Apply all pending migrations
docker compose run --rm migrate \
  -path /migrations \
  -database "postgres://vaultflix:YOUR_PASSWORD@postgres:5432/vaultflix?sslmode=disable" \
  up

# Rollback last migration
docker compose run --rm migrate \
  -path /migrations \
  -database "postgres://vaultflix:YOUR_PASSWORD@postgres:5432/vaultflix?sslmode=disable" \
  down 1
```

## API Overview

All endpoints live under `/api` and require a valid JWT, except `POST /api/auth/register`,
`POST /api/auth/login`, and `GET /health` (unauthenticated, used by the Docker healthcheck).
The token is read from the `Authorization: Bearer <token>` header, falling back to a `?token=`
query parameter for contexts that cannot set headers (`<video src>`, WebSocket upgrade).
Role enforcement is Casbin, driven by [`casbin/policy.csv`](casbin/policy.csv).

### Authentication

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/auth/register` | Register a new account | Public |
| POST | `/api/auth/login` | Login, returns JWT token | Public |
| GET | `/api/me` | Get current user info | Any |
| GET | `/api/videos/:id/stream-token` | Issue a short-lived token scoped to one video and to the streaming routes | viewer+ |

### Videos

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/videos` | List videos (paginated, searchable, filterable) | viewer+ |
| GET | `/api/videos/:id` | Video detail: stream URL, play mode, presigned thumbnail/preview URLs | viewer+ |
| GET | `/api/videos/:id/stream` | Stream bytes (HTTP Range; X-Accel-Redirect in production) | viewer+ |
| GET | `/api/videos/:id/hls/index.m3u8` | HLS playlist for `remux` / `transcode` play modes | admin |
| GET | `/api/videos/:id/hls/:segment` | HLS segment (ffmpeg-generated, disk-cached) | admin |
| POST | `/api/videos/import` | Import videos from a mounted directory | admin |
| PUT | `/api/videos/:id` | Update video metadata | admin |
| DELETE | `/api/videos/:id` | Delete video (DB + MinIO) | admin |

> The HLS routes are currently reachable by admin only — `casbin/policy.csv` grants `viewer`
> the progressive `/api/videos/:id/stream` route but has no entry for the HLS pair, so a
> viewer-role account can play `direct`-mode videos only.

### Tags

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/tags` | List all tags (filterable by category) | viewer+ |
| POST | `/api/tags` | Create a tag | admin |
| POST | `/api/videos/:id/tags` | Add tag to video | admin |
| DELETE | `/api/videos/:id/tags/:tagId` | Remove tag from video | admin |

### Watch History & Playback

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/watch-history` | Save playback progress | viewer+ |
| GET | `/api/watch-history` | List watch history | viewer+ |
| DELETE | `/api/watch-history` | Clear watch history | viewer+ |
| POST | `/api/watch-sessions/heartbeat` | Accumulate real watch time | viewer+ |
| POST | `/api/playback/telemetry` | Report playback telemetry (time-to-first-frame, stalls) | viewer+ |
| GET | `/api/admin/playback/telemetry` | Aggregated telemetry summary | admin |

### Favorites

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/favorites` | List favorites | viewer+ |
| POST | `/api/favorites` | Add favorite | viewer+ |
| DELETE | `/api/favorites/:videoId` | Remove favorite | viewer+ |

### Recommendations

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/recommendations/today` | Today's curated picks | viewer+ |
| GET | `/api/recommendations` | List picks by date | admin |
| POST | `/api/recommendations` | Add a pick | admin |
| PUT | `/api/recommendations/:id` | Update sort order | admin |
| DELETE | `/api/recommendations/:id` | Remove a pick | admin |

### Users

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/users` | List users | admin |
| POST | `/api/users` | Create a user | admin |
| PUT | `/api/users/:id/enable` | Enable / disable a user | admin |
| PUT | `/api/users/:id/password` | Reset a user's password | admin |
| DELETE | `/api/users/:id` | Delete a user | admin |

### Media Sources

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/media-sources` | List all media sources (with video counts) | admin |
| POST | `/api/media-sources` | Create a media source | admin |
| PUT | `/api/media-sources/:id` | Update media source label/enabled | admin |
| DELETE | `/api/media-sources/:id` | Delete a media source | admin |

### Import Jobs

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/videos/import` | Start async video import | admin |
| GET | `/api/import-jobs/active` | Get currently running import job | admin |
| GET | `/api/import-jobs/:id` | Get import job by ID | admin |

### Metadata Enrichment

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/videos/:id/enrich` | Scrape metadata for one video | admin |
| GET | `/api/videos/:id/suggestions` | List pending metadata suggestions | admin |
| POST | `/api/videos/:id/suggestions/:sid/accept` | Accept a suggestion | admin |
| DELETE | `/api/videos/:id/suggestions/:sid` | Reject a suggestion | admin |
| POST | `/api/enrich-jobs` | Start a batch enrichment job | admin |
| GET | `/api/enrich-jobs/active` | Get the running batch job | admin |
| DELETE | `/api/enrich-jobs/:jid` | Cancel a batch job | admin |
| POST | `/api/enrich-jobs/backfill-codes` | Backfill video codes from filenames | admin |

### Admin: Analytics & Backfill

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/admin/analytics` | Library and viewing analytics | admin |
| POST | `/api/admin/videos/backfill-previews` | Start preview-clip backfill | admin |
| GET | `/api/admin/backfill-jobs/active` | Get the running backfill job | admin |
| POST | `/api/admin/backfill-jobs/:id/cancel` | Cancel a backfill job | admin |
| POST | `/api/admin/videos/backfill-codecs` | Probe and store missing codec metadata | admin |
| POST | `/api/admin/videos/backfill-keyframes` | Index keyframes for seeking | admin |

### WebSocket

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/ws` | WebSocket connection (import progress, notifications) | Any |

For full request/response details, see the handler source code in [`internal/handler/`](internal/handler/)
and the route table in [`cmd/server/main.go`](cmd/server/main.go).

## Troubleshooting

### `migrate` exits 1 with `password authentication failed for user "vaultflix"`

The `vaultflix-postgres-data` volume already exists from an earlier run.
`POSTGRES_PASSWORD` is only applied when the volume is **first initialised** — on
an existing volume, changing `DB_PASSWORD` in `.env` has no effect on the role
that is actually stored in the database, so the two drift apart.

Fix without losing data — reset the role to match `.env`:

```bash
docker compose exec postgres psql -U vaultflix -d vaultflix \
  -c "ALTER ROLE vaultflix WITH PASSWORD '<the DB_PASSWORD from .env>';"
```

Or start clean (**destroys the database**): `docker compose down -v`.

### The browser shows the default "Welcome to nginx!" page

The `vaultflix-nginx` image is stale or was built before the SPA existed. The
frontend is baked into the image (there is no shared volume), so rebuild it:

```bash
docker compose build vaultflix-nginx && docker compose up -d vaultflix-nginx
```

If the page is merely *outdated* rather than missing, it is the PWA service
worker cache — hard-reload the browser.

### Cannot log in as admin / forgot the admin password

The admin account is **not** created by a migration. It is seeded at API startup
by `initDefaultAdmin` (`cmd/server/main.go`), and only while the `users` table is
still empty:

```
{"level":"INFO","msg":"users table not empty, skipping admin init","user_count":1}
```

That log line means an account already exists, so changing `ADMIN_DEFAULT_PASSWORD`
in `.env` will not reach it — by design, so an env var can never silently take
over an existing account.

To recover, set the password you want in `.env`, then run the reset flag:

```bash
task reset-admin-password
```

Under the hood (prod stack, or if you prefer the raw command):

```bash
# dev  — source is bind-mounted, so run it straight from source
docker compose run --rm --no-deps vaultflix-api go run ./cmd/server -reset-admin-password
# prod — the image ENTRYPOINT is the compiled binary, so pass the flag as an arg
docker compose -f docker-compose.yml -f docker-compose.prod.yml \
  run --rm --no-deps vaultflix-api -reset-admin-password
```

It resets only the account named by `ADMIN_DEFAULT_USERNAME` and exits; it never
runs as part of a normal boot.

### Startup feels slow

A cold start compiles the Go source inside the container and populates the
`go_modules` / `go_build_cache` volumes — expect a few minutes the very first
time, then ~1s on subsequent starts. If **every** start is slow, check that the
API image is actually built from `Dockerfile.dev` (`task up` passes `--build`);
ffmpeg is baked into that layer, and installing it at container start instead
costs ~13s per start.

## Roadmap

See **[ROADMAP.md](ROADMAP.md)** — the single source of truth for planned features, the tech-debt backlog, and architecture-evolution triggers. (`docs/SPEC.md` covers what's already built; ROADMAP.md covers what's next.)

## License

MIT — intended license; a `LICENSE` file has not been added to the repo yet.
