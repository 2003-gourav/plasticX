-- ============================================================
-- MIGRATION: Week 5 — meme_signals / market_signals (materialized pipeline)
-- Run after Week 2 attention tables exist.
-- ============================================================

-- Meme-level signals (refreshed periodically by a worker, not on every request)
CREATE TABLE IF NOT EXISTS meme_signals (
    meme_id INTEGER PRIMARY KEY REFERENCES memes(id) ON DELETE CASCADE,
    views_1h INTEGER DEFAULT 0,
    views_24h INTEGER DEFAULT 0,
    reposts_1h INTEGER DEFAULT 0,
    derivatives_1h INTEGER DEFAULT 0,
    velocity_1h FLOAT DEFAULT 0,
    momentum FLOAT DEFAULT 0,
    attention_score FLOAT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Market-level signals (aggregated from memes)
CREATE TABLE IF NOT EXISTS market_signals (
    market_id INTEGER PRIMARY KEY REFERENCES markets(id) ON DELETE CASCADE,
    total_attention_score FLOAT DEFAULT 0,
    total_views_1h INTEGER DEFAULT 0,
    market_velocity FLOAT DEFAULT 0,
    market_momentum FLOAT DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_meme_signals_score ON meme_signals(attention_score DESC);
CREATE INDEX IF NOT EXISTS idx_market_signals_score ON market_signals(total_attention_score DESC);
