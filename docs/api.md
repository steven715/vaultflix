# Vaultflix API Reference

All endpoints live under `/api` and require a valid JWT, except `POST /api/auth/register`,
`POST /api/auth/login`, and `GET /health` (unauthenticated, used by the Docker healthcheck).
The token is read from the `Authorization: Bearer <token>` header, falling back to a `?token=`
query parameter for contexts that cannot set headers (`<video src>`, WebSocket upgrade).
Role enforcement is Casbin, driven by [`casbin/policy.csv`](../casbin/policy.csv).

## Authentication

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/auth/register` | Register a new account | Public |
| POST | `/api/auth/login` | Login, returns JWT token | Public |
| GET | `/api/me` | Get current user info | Any |
| GET | `/api/videos/:id/stream-token` | Issue a short-lived token scoped to one video and to the streaming routes | viewer+ |

## Videos

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/videos` | List videos (paginated, searchable, filterable) | viewer+ |
| GET | `/api/videos/:id` | Video detail: stream URL, play mode, presigned thumbnail/preview URLs | viewer+ |
| GET | `/api/videos/:id/stream` | Stream bytes (HTTP Range; X-Accel-Redirect in production; 409 `source_unavailable` when the Media Source is disabled) | viewer+ |
| GET | `/api/videos/:id/hls/index.m3u8` | HLS playlist for `remux` play mode (other modes → 404; 409 `source_unavailable` when the Media Source is disabled; 503 `stream_not_ready` — and only that — while the Keyframe Index is probed) | admin |
| GET | `/api/videos/:id/hls/:segment` | HLS Segment (ffmpeg-generated, disk-cached; same 404/503 rules as the playlist) | admin |
| POST | `/api/videos/import` | Import videos from a mounted directory | admin |
| PUT | `/api/videos/:id` | Update video metadata | admin |
| DELETE | `/api/videos/:id` | Delete video (DB + MinIO) | admin |

> The HLS routes are currently reachable by admin only — `casbin/policy.csv` grants `viewer`
> the progressive `/api/videos/:id/stream` route but has no entry for the HLS pair, so a
> viewer-role account can play `direct`-mode videos only.

## Tags

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/tags` | List all tags (filterable by category) | viewer+ |
| POST | `/api/tags` | Create a tag | admin |
| POST | `/api/videos/:id/tags` | Add tag to video | admin |
| DELETE | `/api/videos/:id/tags/:tagId` | Remove tag from video | admin |

## Watch History & Playback

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/watch-history` | Save playback progress | viewer+ |
| GET | `/api/watch-history` | List watch history | viewer+ |
| DELETE | `/api/watch-history` | Clear watch history | viewer+ |
| POST | `/api/watch-sessions/heartbeat` | Accumulate real watch time | viewer+ |
| POST | `/api/playback/telemetry` | Report playback telemetry (time-to-first-frame, stalls) | viewer+ |
| GET | `/api/admin/playback/telemetry` | Aggregated telemetry summary | admin |

## Favorites

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/favorites` | List favorites | viewer+ |
| POST | `/api/favorites` | Add favorite | viewer+ |
| DELETE | `/api/favorites/:videoId` | Remove favorite | viewer+ |

## Recommendations

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/recommendations/today` | Today's curated picks | viewer+ |
| GET | `/api/recommendations` | List picks by date | admin |
| POST | `/api/recommendations` | Add a pick | admin |
| PUT | `/api/recommendations/:id` | Update sort order | admin |
| DELETE | `/api/recommendations/:id` | Remove a pick | admin |

## Users

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/users` | List users | admin |
| POST | `/api/users` | Create a user | admin |
| PUT | `/api/users/:id/enable` | Enable / disable a user | admin |
| PUT | `/api/users/:id/password` | Reset a user's password | admin |
| DELETE | `/api/users/:id` | Delete a user | admin |

## Media Sources

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/media-sources` | List all media sources (with video counts) | admin |
| POST | `/api/media-sources` | Create a media source | admin |
| PUT | `/api/media-sources/:id` | Update media source label/enabled | admin |
| DELETE | `/api/media-sources/:id` | Delete a media source | admin |

## Import Jobs

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | `/api/videos/import` | Start async video import | admin |
| GET | `/api/import-jobs/active` | Get currently running import job | admin |
| GET | `/api/import-jobs/:id` | Get import job by ID | admin |

## Metadata Enrichment

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

## Admin: Analytics & Backfill

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/admin/analytics` | Library and viewing analytics | admin |
| POST | `/api/admin/videos/backfill-previews` | Start preview-clip backfill | admin |
| GET | `/api/admin/backfill-jobs/active` | Get the running backfill job | admin |
| POST | `/api/admin/backfill-jobs/:id/cancel` | Cancel a backfill job | admin |
| POST | `/api/admin/videos/backfill-codecs` | Probe and store missing codec metadata | admin |
| POST | `/api/admin/videos/backfill-keyframes` | Index keyframes for seeking | admin |

## WebSocket

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/ws` | WebSocket connection (import progress, notifications) | Any |

For full request/response details, see the handler source code in [`internal/handler/`](../internal/handler/)
and the route table in [`cmd/server/main.go`](../cmd/server/main.go).

> Domain vocabulary (Media Source, Import Job, Enrichment, Metadata Suggestion, Play Mode…) is defined in [CONTEXT.md](../CONTEXT.md).
