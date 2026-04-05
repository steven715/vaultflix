CREATE TABLE tags (
    id        SERIAL PRIMARY KEY,
    name      VARCHAR(100) UNIQUE NOT NULL,
    category  VARCHAR(50) NOT NULL DEFAULT 'custom'
);

CREATE INDEX idx_tags_category ON tags(category);

CREATE TABLE video_tags (
    video_id  UUID REFERENCES videos(id) ON DELETE CASCADE,
    tag_id    INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (video_id, tag_id)
);
