CREATE TABLE playback_telemetry (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    video_id           UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    session_id         UUID NOT NULL,
    play_mode          TEXT NOT NULL,
    network_scope      TEXT NOT NULL,
    ttff_ms            INT,
    watched_ms         INT  NOT NULL DEFAULT 0,
    rebuffer_count     INT  NOT NULL DEFAULT 0,
    rebuffer_ms        INT  NOT NULL DEFAULT 0,
    avg_downlink_mbps  NUMERIC(8,2),
    fatal_error_family TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_playback_telemetry_session   ON playback_telemetry(session_id);
CREATE INDEX        idx_playback_telemetry_created   ON playback_telemetry(created_at DESC);
CREATE INDEX        idx_playback_telemetry_play_mode ON playback_telemetry(play_mode, created_at DESC);
