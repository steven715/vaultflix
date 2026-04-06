CREATE UNIQUE INDEX uq_videos_source_file
    ON videos (source_id, file_path)
    WHERE source_id IS NOT NULL AND file_path IS NOT NULL;
