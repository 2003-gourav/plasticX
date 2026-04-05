-- ============================================================
-- MIGRATION: Week 2 schema updates (preserve existing data)
-- ============================================================

-- 1. Add missing columns to markets (if not exist)
ALTER TABLE markets ADD COLUMN IF NOT EXISTS creator_id TEXT;
ALTER TABLE markets ADD COLUMN IF NOT EXISTS initial_x DECIMAL;
ALTER TABLE markets ADD COLUMN IF NOT EXISTS initial_y DECIMAL;
ALTER TABLE markets ADD COLUMN IF NOT EXISTS initial_k DECIMAL;

-- 2. Extend trades.direction to include 'buyback'
ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_direction_check;
ALTER TABLE trades ADD CONSTRAINT trades_direction_check 
    CHECK (direction IN ('buy', 'sell', 'buyback'));

-- 3. Create new event‑sourced attention tables (if not exist)
CREATE TABLE IF NOT EXISTS attention_events (
    event_id BIGSERIAL PRIMARY KEY,
    meme_id INTEGER NOT NULL REFERENCES memes(id) ON DELETE CASCADE,
    user_id TEXT,
    event_type TEXT NOT NULL CHECK (event_type IN ('view', 'unique_view', 'repost', 'derivative')),
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS meme_attention_stats (
    meme_id INTEGER PRIMARY KEY REFERENCES memes(id) ON DELETE CASCADE,
    views INTEGER DEFAULT 0,
    unique_views INTEGER DEFAULT 0,
    reposts INTEGER DEFAULT 0,
    derivatives INTEGER DEFAULT 0
);

-- 4. Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_attention_events_meme_time ON attention_events(meme_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_attention_events_user_time ON attention_events(user_id, timestamp);

-- 5. Optional: migrate existing data from old `meme_attention` table to new tables
--    (if you have important historical data, run this. Otherwise skip.)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'meme_attention') THEN
        -- Insert raw events from old aggregated table (approximate: assume each row is a snapshot at that hour)
        INSERT INTO attention_events (meme_id, user_id, event_type, timestamp)
        SELECT 
            meme_id, 
            NULL, -- user_id unknown
            'view', 
            timestamp 
        FROM meme_attention 
        WHERE views > 0
        ON CONFLICT DO NOTHING;
        
        -- Similarly for reposts, derivatives, unique_views
        INSERT INTO attention_events (meme_id, user_id, event_type, timestamp)
        SELECT meme_id, NULL, 'repost', timestamp FROM meme_attention WHERE reposts > 0;
        
        INSERT INTO attention_events (meme_id, user_id, event_type, timestamp)
        SELECT meme_id, NULL, 'derivative', timestamp FROM meme_attention WHERE derivatives > 0;
        
        INSERT INTO attention_events (meme_id, user_id, event_type, timestamp)
        SELECT meme_id, NULL, 'unique_view', timestamp FROM meme_attention WHERE unique_views > 0;
        
        -- Build stats from events
        INSERT INTO meme_attention_stats (meme_id, views, unique_views, reposts, derivatives)
        SELECT 
            meme_id,
            COUNT(*) FILTER (WHERE event_type = 'view'),
            COUNT(*) FILTER (WHERE event_type = 'unique_view'),
            COUNT(*) FILTER (WHERE event_type = 'repost'),
            COUNT(*) FILTER (WHERE event_type = 'derivative')
        FROM attention_events
        GROUP BY meme_id
        ON CONFLICT (meme_id) DO UPDATE SET
            views = EXCLUDED.views,
            unique_views = EXCLUDED.unique_views,
            reposts = EXCLUDED.reposts,
            derivatives = EXCLUDED.derivatives;
        
        RAISE NOTICE 'Migrated data from old meme_attention table. You can now drop it if desired.';
    ELSE
        RAISE NOTICE 'No old meme_attention table found, skipping data migration.';
    END IF;
END $$;

-- 6. Optional: Drop old attention table (after confirming migration)
-- DROP TABLE IF EXISTS meme_attention;
