CREATE TABLE daily_recommendations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id        UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    recommend_date  DATE NOT NULL,
    sort_order      INT DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (video_id, recommend_date)
);

CREATE INDEX idx_recommendations_date ON daily_recommendations(recommend_date DESC);
