-- Reverse-lookup index: videos-by-tag filtering (video_tags.tag_id = ANY(...))
-- and per-tag video counts join on tag_id. The PK (video_id, tag_id) only
-- serves video_id-leading lookups, so tag_id-only access needs its own index.
CREATE INDEX idx_video_tags_tag_id ON video_tags(tag_id);
