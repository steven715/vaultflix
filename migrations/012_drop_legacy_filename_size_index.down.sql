-- Recreate the legacy import dedup index removed by the up migration.
CREATE UNIQUE INDEX idx_videos_filename_size ON videos(original_filename, file_size_bytes);
