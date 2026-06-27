# ADR-0001: Keep videos on local disk, manage paths via a `media_sources` table

- Status: Accepted
- Date: 2026-04-06

## Context

The product pivoted away from "upload videos into MinIO" toward "video files stay where they already are on local disk, and we stream them directly." Whole disks are mounted read-only into the containers at `/mnt/host/<drive>`. The operator needs to register specific directories inside those disks as browseable sources, and must be able to add/remove a source **without restarting the service or editing compose** (runtime-adjustability principle, CLAUDE.md「執行期可調性原則」).

## Decision

- Introduce a `media_sources` DB table (label + `mount_path`), CRUD via API + Admin UI.
- `videos` gain a `source_id` FK (`ON DELETE SET NULL`) and a `file_path` relative to the source's `mount_path`. The legacy `minio_object_key` column is kept only for the migration window.
- Path-traversal validation (`filepath.Clean` + prefix check + reject `cleaned != path` + `os.Stat` is-dir) lives as a method on `MediaSourceService`, with the allowed prefix **injected** as a struct field (prod const `AllowedMountPrefix = "/mnt/host/"`). No standalone `pathutil` package — there is a single consumer, and injecting the prefix is what makes the logic testable (tests pass an `os.MkdirTemp` prefix).

## Alternatives rejected

- **Hardcode source directories in env/compose** — changing a path would need a restart, and per-directory granularity inside a mounted disk needs app-level management.
- **Extract a `pathutil` package / abstract `os.Stat` behind an interface** — premature abstraction for a single consumer; temp-dir injection covers the tests.

## Consequences

- Enables runtime path management, per-source enable/disable, and video counts.
- Costs a migration, a path-traversal validation surface, and a transitional dead column.
- `ON DELETE SET NULL` means deleting a source **orphans** its video rows (retained but unplayable) rather than cascading deletes.
- Validation logic is coupled to the media-source service; a second consumer would force an extraction later.
- The path-safety pattern itself is codified in CLAUDE.md「路徑安全規範」. The byte path on top of this model later evolved — see [ADR-0008](0008-xaccel-stream-offload.md).
