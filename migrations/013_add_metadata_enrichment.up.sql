ALTER TABLE videos ADD COLUMN code VARCHAR(50);
ALTER TABLE videos ADD COLUMN release_date DATE;
ALTER TABLE videos ADD COLUMN runtime_minutes INT;
ALTER TABLE videos ADD COLUMN maker VARCHAR(255);
ALTER TABLE videos ADD COLUMN label VARCHAR(255);
ALTER TABLE videos ADD COLUMN series VARCHAR(255);
ALTER TABLE videos ADD COLUMN cover_key VARCHAR(1000) DEFAULT '';
ALTER TABLE videos ADD COLUMN enrichment_status VARCHAR(20) NOT NULL DEFAULT 'none';
ALTER TABLE videos ADD COLUMN enriched_at TIMESTAMPTZ;

CREATE INDEX idx_videos_code ON videos(code);
CREATE INDEX idx_videos_enrichment_status ON videos(enrichment_status);

CREATE TABLE actresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_ja     VARCHAR(255) NOT NULL,
    name_romaji VARCHAR(255) NOT NULL DEFAULT '',
    avatar_key  VARCHAR(1000) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_ja)
);

CREATE TABLE video_actresses (
    video_id   UUID REFERENCES videos(id) ON DELETE CASCADE,
    actress_id UUID REFERENCES actresses(id) ON DELETE CASCADE,
    PRIMARY KEY (video_id, actress_id)
);

CREATE TABLE metadata_suggestions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id   UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    source     VARCHAR(50) NOT NULL,
    code       VARCHAR(50) NOT NULL,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status     VARCHAR(20) NOT NULL DEFAULT 'pending'
);
CREATE INDEX idx_metadata_suggestions_video ON metadata_suggestions(video_id);
