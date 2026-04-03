package models

import "time"

// Market represents a trading pair (AMM pool) in PlasticX.
type Market struct {
	ID        int       `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	XReserve  float64   `json:"x_reserve" db:"x_reserve"`   // Base asset reserve (e.g., USDC)
	YReserve  float64   `json:"y_reserve" db:"y_reserve"`   // Meme token reserve
	Fee       float64   `json:"fee" db:"fee"`               // Fee in basis points (0.003 = 0.3%)
	Treasury  float64   `json:"treasury" db:"treasury"`     // Accumulated fees in base asset
	CreatorID *string   `json:"creator_id,omitempty" db:"creator_id"`
	InitialX  *float64  `json:"initial_x,omitempty" db:"initial_x"`
	InitialY  *float64  `json:"initial_y,omitempty" db:"initial_y"`
	InitialK  *float64  `json:"initial_k,omitempty" db:"initial_k"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Price returns the marginal price of Y in terms of X (y/x).
func (m *Market) Price() float64 {
	if m.XReserve == 0 {
		return 0
	}
	return m.YReserve / m.XReserve
}
