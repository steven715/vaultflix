DROP TABLE IF EXISTS metadata_suggestions;
DROP TABLE IF EXISTS video_actresses;
DROP TABLE IF EXISTS actresses;

DROP INDEX IF EXISTS idx_videos_enrichment_status;
DROP INDEX IF EXISTS idx_videos_code;

ALTER TABLE videos DROP COLUMN IF EXISTS enriched_at;
ALTER TABLE videos DROP COLUMN IF EXISTS enrichment_status;
ALTER TABLE videos DROP COLUMN IF EXISTS cover_key;
ALTER TABLE videos DROP COLUMN IF EXISTS series;
ALTER TABLE videos DROP COLUMN IF EXISTS label;
ALTER TABLE videos DROP COLUMN IF EXISTS maker;
ALTER TABLE videos DROP COLUMN IF EXISTS runtime_minutes;
ALTER TABLE videos DROP COLUMN IF EXISTS release_date;
ALTER TABLE videos DROP COLUMN IF EXISTS code;
