CREATE TABLE media_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(255) NOT NULL,
    mount_path VARCHAR(1024) NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE videos
    ADD COLUMN source_id UUID REFERENCES media_sources(id) ON DELETE SET NULL,
    ADD COLUMN file_path VARCHAR(2048);

COMMENT ON COLUMN videos.source_id IS '影片所屬的 media source，source 刪除時設為 NULL';
COMMENT ON COLUMN videos.file_path IS '相對於 media source mount_path 的檔案路徑';
COMMENT ON COLUMN videos.minio_object_key IS '舊欄位，重構完成後不再使用，僅用於遷移期間';
