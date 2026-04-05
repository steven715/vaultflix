CREATE TABLE favorites (
    user_id    UUID REFERENCES users(id) ON DELETE CASCADE,
    video_id   UUID REFERENCES videos(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, video_id)
);
