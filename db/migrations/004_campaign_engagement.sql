CREATE TABLE IF NOT EXISTS campaign_questions (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id),
  question TEXT NOT NULL CHECK (char_length(question) BETWEEN 10 AND 2000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  answer TEXT,
  answered_by BIGINT REFERENCES users(id),
  answered_at TIMESTAMPTZ,
  CHECK ((answer IS NULL AND answered_by IS NULL AND answered_at IS NULL) OR
    (answer IS NOT NULL AND answered_by IS NOT NULL AND answered_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_questions_campaign
  ON campaign_questions(campaign_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS campaign_fund_updates (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
  created_by BIGINT NOT NULL REFERENCES users(id),
  amount BIGINT NOT NULL CHECK (amount > 0),
  purpose VARCHAR(120) NOT NULL,
  description TEXT NOT NULL,
  evidence_content_type VARCHAR(50),
  evidence_data BYTEA,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK ((evidence_content_type IS NULL AND evidence_data IS NULL) OR
    (evidence_content_type IN ('image/jpeg','image/png','image/webp') AND evidence_data IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_fund_updates_campaign
  ON campaign_fund_updates(campaign_id, created_at DESC, id DESC);
