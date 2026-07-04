CREATE TABLE watch_sessions (
    id                     UUID PRIMARY KEY,
    user_id                UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id               UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    watched_seconds        INT NOT NULL DEFAULT 0,
    max_progress_seconds   INT NOT NULL DEFAULT 0,
    video_duration_seconds INT NOT NULL DEFAULT 0,
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_sessions_started ON watch_sessions(started_at DESC);
CREATE INDEX idx_watch_sessions_video   ON watch_sessions(video_id);
CREATE INDEX idx_watch_sessions_user    ON watch_sessions(user_id);
