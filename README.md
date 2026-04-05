# Vaultflix

A self-hosted video management and streaming platform. Organize local video files into a browsable, searchable, streamable web application. Single-user focused with multi-user architecture.

## Architecture Overview

```
React SPA (localhost:3000)
    |
    |-- API requests --> Nginx reverse proxy --> Go API Server (:8080) --> PostgreSQL
    |                                               |
    |                                               +--> MinIO (generate pre-signed URLs)
    |
    +-- Video streaming --> MinIO (:9000) directly (pre-signed URL, no API proxy)
```

**Key design decision**: Video bytes never pass through the Go API server. The API generates time-limited pre-signed URLs, and the browser's `<video>` element fetches directly from MinIO. MinIO natively handles HTTP Range Requests for seeking.

### Tech Stack

| Layer | Technology | Purpose |
|-------|------------|---------|
| Frontend | React 18 + TypeScript | SPA with Vite, served via Nginx |
| Backend | Go 1.24 + Gin | REST API, JWT auth, pre-signed URL generation |
| Database | PostgreSQL 16 | Video metadata, users, tags, watch history |
| Object Storage | MinIO | Video files and thumbnails, S3-compatible |
| Auth | JWT + bcrypt | Stateless authentication |
| Authorization | Casbin | RBAC with admin/viewer roles |
| Infrastructure | Docker Compose V2 | All services containerized |

## Features

### Implemented

- **Authentication**: JWT-based login with bcrypt password hashing
- **Authorization**: Casbin RBAC with admin and viewer roles
- **Video Import**: Bulk import from local directory with automatic ffprobe metadata extraction and ffmpeg thumbnail generation
- **Video Browsing**: Paginated grid view with search, tag filtering, and multi-field sorting
- **Video Streaming**: Pre-signed URL based playback with HTTP Range Request support (seeking)
- **Tag System**: Categorized tags (genre, actor, studio, custom) with video-tag associations
- **React Frontend**: Login, browse, and player pages with responsive dark theme

### Planned

- Watch history with resume playback
- Favorites / bookmarks
- Daily recommendations (admin-curated)
- Admin dashboard
- Meilisearch full-text search
- LLM-powered semantic search
- Mobile client
- Automatic tagging

## Prerequisites

- **Docker** and **Docker Compose V2**
- No local Go or Node.js installation required -- everything runs in containers

## Quick Start

### 1. Clone and configure

```bash
git clone <repo-url> && cd vaultflix
cp .env.example .env
```

Edit `.env` and set your passwords and secrets. The defaults work for local development, but you should at minimum change `JWT_SECRET`, `DB_PASSWORD`, `MINIO_SECRET_KEY`, and `ADMIN_DEFAULT_PASSWORD`.

### 2. Configure video source directory

Edit `docker-compose.yml` and update the video mount path in the `vaultflix-api` service to point to your local video directory:

```yaml
  vaultflix-api:
    volumes:
      # ...
      - /path/to/your/videos:/mnt/videos  # <-- change this
```

### 3. Start all services

```bash
docker compose up -d
```

This automatically:
- Starts PostgreSQL and MinIO
- Creates MinIO buckets (`vaultflix-videos`, `vaultflix-thumbnails`)
- Runs database migrations
- Starts the Go API server (compiles on first run, may take ~30s)
- Builds and starts the React frontend via Nginx

### 4. Log in

Open **http://localhost:3000** in your browser. Log in with the admin credentials from your `.env` file (defaults: `admin` / `change-me-admin-password`).

### 5. Import videos

Trigger a video import via the API. This scans the mounted directory, uploads files to MinIO, and extracts metadata:

```bash
curl -X POST http://localhost:8080/api/videos/import \
  -H "Authorization: Bearer $(curl -s -X POST http://localhost:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"YOUR_ADMIN_PASSWORD"}' | jq -r '.data.token')" \
  -H 'Content-Type: application/json' \
  -d '{"source_dir": "/mnt/videos"}'
```

Import runs synchronously. For large libraries this may take a while (~4m40s for 18 GB in testing).

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
| `JWT_SECRET` | **Yes** | Secret key for signing JWT tokens |
| `JWT_EXPIRY_HOURS` | | JWT token expiry in hours (default: `24`) |
| `SERVER_PORT` | | API server port (default: `8080`) |
| `ADMIN_DEFAULT_USERNAME` | | Auto-created admin username (default: `admin`) |
| `ADMIN_DEFAULT_PASSWORD` | **Yes** | Auto-created admin password |

## Project Structure

```
vaultflix/
├── cmd/server/             # Application entrypoint (main.go)
├── internal/
│   ├── config/             # Environment-based configuration
│   ├── handler/            # HTTP handlers (Gin)
│   ├── middleware/         # JWT auth and Casbin RBAC middleware
│   ├── model/              # Domain models and shared errors
│   ├── repository/         # PostgreSQL data access layer
│   ├── service/            # Business logic layer
│   └── mock/               # Hand-written mock structs for testing
├── migrations/             # SQL migration files (up/down pairs)
├── casbin/                 # RBAC model and policy definitions
├── scripts/                # Integration test scripts
├── web/                    # React frontend (Vite + TypeScript)
│   ├── src/
│   │   ├── api/            # Axios API client and service functions
│   │   ├── components/     # Reusable UI components
│   │   ├── contexts/       # React contexts (auth state)
│   │   ├── pages/          # Page components (Login, Browse, Player)
│   │   └── types/          # TypeScript type definitions
│   ├── Dockerfile          # Multi-stage build: Node -> Nginx
│   └── nginx.conf          # Reverse proxy + SPA routing config
├── docker-compose.yml
├── CLAUDE.md               # Development conventions and coding standards
└── VAULTFLIX_PROJECT_PLAN.md
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

```bash
# Go unit tests
go test ./... -v

# Integration tests (requires running services)
docker compose --profile test up test-runner
```

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

All endpoints except auth require a valid JWT in the `Authorization: Bearer <token>` header.

### Authentication

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/auth/register` | Register a new account | Public |
| POST | `/api/auth/login` | Login, returns JWT token | Public |
| GET | `/api/me` | Get current user info | Any |

### Videos

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/videos` | List videos (paginated, searchable, filterable) | viewer+ |
| GET | `/api/videos/:id` | Video detail with pre-signed stream URL | viewer+ |
| POST | `/api/videos/import` | Import videos from mounted directory | admin |
| PUT | `/api/videos/:id` | Update video metadata | admin |
| DELETE | `/api/videos/:id` | Delete video (DB + MinIO) | admin |

### Tags

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/tags` | List all tags (filterable by category) | viewer+ |
| POST | `/api/tags` | Create a tag | admin |
| POST | `/api/videos/:id/tags` | Add tag to video | admin |
| DELETE | `/api/videos/:id/tags/:tagId` | Remove tag from video | admin |

For full request/response details, see the handler source code in [`internal/handler/`](internal/handler/).

## Roadmap

- **Watch history & resume**: Track playback progress, continue where you left off
- **Favorites**: Bookmark videos for quick access
- **Daily recommendations**: Admin-curated daily picks
- **Full-text search**: Meilisearch integration for fast, typo-tolerant search
- **Semantic search**: LLM-powered natural language video discovery
- **Auto-tagging**: Automated metadata extraction and categorization
- **Mobile client**: Dedicated mobile app or responsive PWA
- **Admin dashboard**: Web UI for import management, user management, and system monitoring

## License

[MIT](LICENSE)
