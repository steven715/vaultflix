CREATE TABLE videos (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title             VARCHAR(500) NOT NULL,
    description       TEXT DEFAULT '',
    minio_object_key  VARCHAR(1000) NOT NULL,
    thumbnail_key     VARCHAR(1000) DEFAULT '',
    duration_seconds  INT DEFAULT 0,
    resolution        VARCHAR(20) DEFAULT '',
    file_size_bytes   BIGINT DEFAULT 0,
    mime_type         VARCHAR(100) DEFAULT '',
    original_filename VARCHAR(500) DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_videos_created_at ON videos(created_at DESC);
CREATE INDEX idx_videos_title ON videos USING gin(to_tsvector('simple', title));
