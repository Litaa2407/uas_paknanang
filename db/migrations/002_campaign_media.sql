CREATE TABLE IF NOT EXISTS campaign_media (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
  media_type VARCHAR(10) NOT NULL CHECK (media_type IN ('IMAGE','VIDEO')),
  content_type VARCHAR(50) NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp','video/mp4')),
  file_data BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_campaign_media_campaign ON campaign_media(campaign_id, id);
