CREATE TABLE IF NOT EXISTS users (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  email VARCHAR(150) UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role VARCHAR(20) NOT NULL CHECK (role IN ('DONATUR','PENGELOLA','ADMIN')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS campaigns (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(100) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  target BIGINT NOT NULL CHECK (target > 0),
  collected BIGINT NOT NULL DEFAULT 0 CHECK (collected >= 0),
  status VARCHAR(20) NOT NULL DEFAULT 'MENUNGGU' CHECK (status IN ('MENUNGGU','AKTIF','SELESAI','DITOLAK')),
  owner_id BIGINT REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS donations (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT NOT NULL REFERENCES campaigns(id),
  donor_name VARCHAR(60) NOT NULL,
  amount BIGINT NOT NULL CHECK (amount > 0),
  payment_reference VARCHAR(120) UNIQUE NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SETTLEMENT','FAILED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Transactional outbox: event tersimpan satu transaksi dengan perubahan bisnis.
CREATE TABLE IF NOT EXISTS outbox_events (
  id BIGSERIAL PRIMARY KEY,
  event_type VARCHAR(100) NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_campaigns_status ON campaigns(status);
CREATE INDEX IF NOT EXISTS idx_donations_created ON donations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events(id) WHERE published_at IS NULL;
