CREATE TABLE video_keyframe_index (
    video_id  UUID PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
    segments  JSONB NOT NULL,
    probed_at TIMESTAMPTZ NOT NULL
);
