-- Create markets table
CREATE TABLE IF NOT EXISTS markets (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    x_reserve DECIMAL NOT NULL,
    y_reserve DECIMAL NOT NULL,
    fee DECIMAL NOT NULL DEFAULT 0.003,
    treasury DECIMAL NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create trades table
CREATE TABLE IF NOT EXISTS trades (
    id SERIAL PRIMARY KEY,
    market_id INTEGER REFERENCES markets(id) ON DELETE CASCADE,
    trader TEXT,
    direction TEXT NOT NULL,
    amount_in DECIMAL NOT NULL,
    amount_out DECIMAL NOT NULL,
    fee_paid DECIMAL NOT NULL,
    price DECIMAL NOT NULL,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_trades_market_id ON trades(market_id);

-- Memes table: content linked to a market
CREATE TABLE IF NOT EXISTS memes (
    id SERIAL PRIMARY KEY,
    creator_id TEXT NOT NULL,
    market_id INTEGER NOT NULL REFERENCES markets(id) ON DELETE CASCADE,
    image_url TEXT NOT NULL,
    caption TEXT NOT NULL,
    content_hash TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Raw attention events per meme (time-series)
CREATE TABLE IF NOT EXISTS meme_attention (
    meme_id INTEGER REFERENCES memes(id) ON DELETE CASCADE,
    views INTEGER DEFAULT 0,
    unique_views INTEGER DEFAULT 0,
    reposts INTEGER DEFAULT 0,
    derivatives INTEGER DEFAULT 0,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (meme_id, timestamp)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_memes_market_id ON memes(market_id);
CREATE INDEX IF NOT EXISTS idx_attention_meme_id ON meme_attention(meme_id);
