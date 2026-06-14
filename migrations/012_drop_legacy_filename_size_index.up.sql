-- Drop the legacy import dedup index from migration 007. Import dedup is now
-- driven by the (source_id, file_path) partial unique index added in migration
-- 010 (uq_videos_source_file). The 007 index over (original_filename,
-- file_size_bytes) is no longer consulted by application logic and wrongly
-- rejects legitimately distinct files that share a name + byte size.
DROP INDEX IF EXISTS idx_videos_filename_size;
