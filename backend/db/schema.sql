-- ============================================================
-- PLASTICX CORE SCHEMA (Week 2)
-- Markets hold money, memes move minds, attention is raw.
-- ============================================================

-- -------------------- MARKETS --------------------
CREATE TABLE IF NOT EXISTS markets (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    x_reserve DECIMAL NOT NULL,
    y_reserve DECIMAL NOT NULL,
    fee DECIMAL NOT NULL DEFAULT 0.003,          -- e.g., 0.003 = 0.3%
    treasury DECIMAL NOT NULL DEFAULT 0,         -- accumulated fees in base asset
    creator_id TEXT,                             -- who created the market
    initial_x DECIMAL,                           -- initial x_reserve at creation
    initial_y DECIMAL,                           -- initial y_reserve at creation
    initial_k DECIMAL,                           -- initial product x*y
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- -------------------- TRADES --------------------
CREATE TABLE IF NOT EXISTS trades (
    id SERIAL PRIMARY KEY,
    market_id INTEGER NOT NULL REFERENCES markets(id) ON DELETE CASCADE,
    trader TEXT,                                  -- user ID or address
    direction TEXT NOT NULL CHECK (direction IN ('buy', 'sell', 'buyback')),
    amount_in DECIMAL NOT NULL,
    amount_out DECIMAL NOT NULL,
    fee_paid DECIMAL NOT NULL,
    price DECIMAL NOT NULL,                       -- average price = amount_out/amount_in
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_trades_market_id ON trades(market_id);
CREATE INDEX IF NOT EXISTS idx_trades_timestamp ON trades(timestamp);

-- -------------------- MEMES --------------------
CREATE TABLE IF NOT EXISTS memes (
    id SERIAL PRIMARY KEY,
    creator_id TEXT NOT NULL,
    market_id INTEGER NOT NULL REFERENCES markets(id) ON DELETE CASCADE,
    image_url TEXT NOT NULL,
    caption TEXT NOT NULL,
    content_hash TEXT UNIQUE NOT NULL,            -- prevents exact duplicates
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memes_market_id ON memes(market_id);

-- -------------------- ATTENTION (Event Sourcing) --------------------
-- Raw, immutable event log – source of truth
CREATE TABLE IF NOT EXISTS attention_events (
    event_id BIGSERIAL PRIMARY KEY,
    meme_id INTEGER NOT NULL REFERENCES memes(id) ON DELETE CASCADE,
    user_id TEXT,                                 -- NULL for anonymous
    event_type TEXT NOT NULL CHECK (event_type IN ('view', 'unique_view', 'repost', 'derivative')),
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_attention_events_meme_time ON attention_events(meme_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_attention_events_user_time ON attention_events(user_id, timestamp);

-- Fast aggregated counters – derived cache, can be rebuilt from events
CREATE TABLE IF NOT EXISTS meme_attention_stats (
    meme_id INTEGER PRIMARY KEY REFERENCES memes(id) ON DELETE CASCADE,
    views INTEGER DEFAULT 0,
    unique_views INTEGER DEFAULT 0,
    reposts INTEGER DEFAULT 0,
    derivatives INTEGER DEFAULT 0
);

-- Optional: materialized view for hourly market attention (can be added later)
