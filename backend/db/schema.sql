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
