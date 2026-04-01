package amm

type Pool struct {
    X   float64
    Y   float64
    Fee float64
}

func NewPool(x, y, fee float64) *Pool {
    return &Pool{X: x, Y: y, Fee: fee}
}

func (p *Pool) Price() float64 {
    return p.Y / p.X
}

// SwapXToY swaps dx of token X for token Y.
// Returns (dy, fee) where fee is in token Y.
func (p *Pool) SwapXToY(dx float64) (dy, fee float64) {
    newX := p.X + dx
    newY := (p.X * p.Y) / newX
    dyGross := p.Y - newY
    fee = dyGross * p.Fee
    dy = dyGross - fee

    p.X = newX
    p.Y = newY
    return
}

// SwapYToX swaps dy of token Y for token X.
// Returns (dx, fee) where fee is in token Y.
func (p *Pool) SwapYToX(dy float64) (dx, fee float64) {
    fee = dy * p.Fee
    dyNet := dy - fee
    newY := p.Y + dyNet
    newX := (p.X * p.Y) / newY
    dx = p.X - newX

    p.X = newX
    p.Y = newY
    return
}
