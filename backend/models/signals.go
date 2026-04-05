package models

import "time"

// Attention weights for lifetime engagement helpers (/insights) and for the signals refresh worker.
// attention_score in meme_signals should use the same weights × momentum when the worker runs.
const (
	AttentionWeightView       = 1.0
	AttentionWeightRepost     = 3.0
	AttentionWeightDerivative = 5.0
)

// MemeSignal is materialized meme-level attention intelligence (refreshed periodically).
type MemeSignal struct {
	MemeID         int       `json:"meme_id" db:"meme_id"`
	Views1h        int       `json:"views_1h" db:"views_1h"`
	Views24h       int       `json:"views_24h" db:"views_24h"`
	Reposts1h      int       `json:"reposts_1h" db:"reposts_1h"`
	Derivatives1h  int       `json:"derivatives_1h" db:"derivatives_1h"`
	Velocity1h     float64   `json:"velocity_1h" db:"velocity_1h"`
	Momentum       float64   `json:"momentum" db:"momentum"`
	AttentionScore float64   `json:"attention_score" db:"attention_score"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// MarketSignal is aggregated market-level signal row (refreshed periodically).
type MarketSignal struct {
	MarketID            int       `json:"market_id" db:"market_id"`
	TotalAttentionScore float64   `json:"total_attention_score" db:"total_attention_score"`
	TotalViews1h        int       `json:"total_views_1h" db:"total_views_1h"`
	MarketVelocity      float64   `json:"market_velocity" db:"market_velocity"`
	MarketMomentum      float64   `json:"market_momentum" db:"market_momentum"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}
