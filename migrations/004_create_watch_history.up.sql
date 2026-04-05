CREATE TABLE watch_history (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id          UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    progress_seconds  INT DEFAULT 0,
    completed         BOOLEAN DEFAULT FALSE,
    watched_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_history_user ON watch_history(user_id, watched_at DESC);
CREATE UNIQUE INDEX idx_watch_history_user_video ON watch_history(user_id, video_id);
